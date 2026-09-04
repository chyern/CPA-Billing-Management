package billing

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// APIKeyBalance is the current tracked balance for a client API key. APIKeyID
// is a short one-way identifier; the complete credential is never persisted.
type APIKeyBalance struct {
	APIKeyID    string  `json:"api_key_id"`
	APIKey      string  `json:"api_key"`
	CallerScope string  `json:"caller_scope,omitempty"`
	Note        string  `json:"note,omitempty"`
	Balance     float64 `json:"balance"`
	Configured  bool    `json:"configured"`
	Requests    int64   `json:"requests"`
	Cost        float64 `json:"cost"`
}

func (s *Store) KeyBalances() ([]APIKeyBalance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	byID := make(map[string]APIKeyBalance, len(s.state.APIKeyAggregates))
	rows, err := s.db.Query(`SELECT api_key_id, api_key, caller_scope, balance FROM api_key_balances ORDER BY api_key`)
	if err != nil {
		return nil, fmt.Errorf("query API key balances: %w", err)
	}
	for rows.Next() {
		var item APIKeyBalance
		if err := rows.Scan(&item.APIKeyID, &item.APIKey, &item.CallerScope, &item.Balance); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan API key balance: %w", err)
		}
		item.Configured = true
		byID[item.APIKeyID] = item
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	noteRows, err := s.db.Query(`SELECT api_key_id, api_key, note FROM api_key_balance_notes ORDER BY api_key`)
	if err != nil {
		return nil, fmt.Errorf("query API key balance notes: %w", err)
	}
	for noteRows.Next() {
		var id, apiKey, note string
		if err := noteRows.Scan(&id, &apiKey, &note); err != nil {
			_ = noteRows.Close()
			return nil, fmt.Errorf("scan API key balance note: %w", err)
		}
		item := byID[id]
		item.APIKeyID = id
		if strings.TrimSpace(item.APIKey) == "" {
			item.APIKey = apiKey
		}
		item.Note = note
		byID[id] = item
	}
	if err := noteRows.Close(); err != nil {
		return nil, err
	}
	for id, aggregate := range s.state.APIKeyAggregates {
		if strings.TrimSpace(id) == "" || aggregate == nil {
			continue
		}
		item := byID[id]
		item.APIKeyID = id
		if strings.TrimSpace(item.APIKey) == "" {
			item.APIKey = aggregate.APIKey
		}
		item.Requests = aggregate.Requests
		item.Cost = aggregate.Cost
		byID[id] = item
	}
	result := make([]APIKeyBalance, 0, len(byID))
	for _, item := range byID {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].APIKey == result[j].APIKey {
			return result[i].APIKeyID < result[j].APIKeyID
		}
		return result[i].APIKey < result[j].APIKey
	})
	return result, nil
}

// SetKeyBalanceNotes replaces the optional descriptions associated with API
// keys. Notes are stored independently so a key can have a description
// without enabling balance tracking.
func (s *Store) SetKeyBalanceNotes(items []APIKeyBalance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	type noteRecord struct {
		id     string
		apiKey string
		note   string
	}
	records := make([]noteRecord, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		id := strings.ToLower(strings.TrimSpace(item.APIKeyID))
		if id == "" {
			return fmt.Errorf("API key note identifier is required")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate API key note identifier %q", id)
		}
		seen[id] = struct{}{}
		note := strings.TrimSpace(item.Note)
		if len([]rune(note)) > 200 {
			return fmt.Errorf("API key note must not exceed 200 characters")
		}
		if note == "" {
			continue
		}
		label := strings.TrimSpace(item.APIKey)
		if label == "" {
			label = "未命名密钥"
		} else if !strings.ContainsRune(label, '•') {
			label = MaskAPIKey(label)
		}
		records = append(records, noteRecord{id: id, apiKey: label, note: note})
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := s.withTransaction(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM api_key_balance_notes`); err != nil {
			return fmt.Errorf("clear API key balance notes: %w", err)
		}
		for _, record := range records {
			if _, err := tx.Exec(`INSERT INTO api_key_balance_notes (api_key_id, api_key, note, updated_at) VALUES (?, ?, ?, ?)`, record.id, record.apiKey, record.note, now); err != nil {
				return fmt.Errorf("save API key balance note %q: %w", record.id, err)
			}
		}
		return nil
	})
	if err != nil {
		s.lastErr = err
	}
	return err
}

func (s *Store) SetKeyBalances(items []APIKeyBalance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]struct{}, len(items))
	seenCallerScopes := make(map[string]struct{}, len(items))
	for index := range items {
		items[index].APIKeyID = strings.ToLower(strings.TrimSpace(items[index].APIKeyID))
		if items[index].APIKeyID == "" {
			return fmt.Errorf("API key balance identifier is required")
		}
		if _, exists := seen[items[index].APIKeyID]; exists {
			return fmt.Errorf("duplicate API key balance identifier %q", items[index].APIKeyID)
		}
		seen[items[index].APIKeyID] = struct{}{}
		items[index].CallerScope = strings.ToLower(strings.TrimSpace(items[index].CallerScope))
		if items[index].CallerScope == "" && !strings.ContainsRune(items[index].APIKey, '•') {
			items[index].CallerScope = CallerScope(items[index].APIKey)
		}
		if items[index].CallerScope != "" {
			decoded, err := hex.DecodeString(items[index].CallerScope)
			if err != nil || len(decoded) != 32 {
				return fmt.Errorf("API key caller scope must be a 64-character SHA-256 value")
			}
			if _, exists := seenCallerScopes[items[index].CallerScope]; exists {
				return fmt.Errorf("duplicate API key caller scope")
			}
			seenCallerScopes[items[index].CallerScope] = struct{}{}
		}
		if math.IsNaN(items[index].Balance) || math.IsInf(items[index].Balance, 0) || items[index].Balance < 0 {
			return fmt.Errorf("API key balance must be a non-negative number")
		}
		label := strings.TrimSpace(items[index].APIKey)
		if label == "" {
			label = "未命名密钥"
		} else if !strings.ContainsRune(label, '•') {
			label = MaskAPIKey(label)
		}
		items[index].APIKey = label
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := s.withTransaction(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM api_key_balances`); err != nil {
			return fmt.Errorf("clear API key balances: %w", err)
		}
		for _, item := range items {
			if _, err := tx.Exec(`INSERT INTO api_key_balances (api_key_id, api_key, caller_scope, balance, updated_at) VALUES (?, ?, ?, ?, ?)`, item.APIKeyID, item.APIKey, item.CallerScope, item.Balance, now); err != nil {
				return fmt.Errorf("save API key balance %q: %w", item.APIKeyID, err)
			}
		}
		return nil
	})
	if err != nil {
		s.lastErr = err
	}
	return err
}

// BalanceForCallerScope returns the configured balance for a downstream
// caller. An unconfigured caller is deliberately allowed by the interceptor.
func (s *Store) BalanceForCallerScope(callerScope string) (float64, bool, error) {
	callerScope = strings.ToLower(strings.TrimSpace(callerScope))
	if callerScope == "" {
		return 0, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var balance float64
	err := s.db.QueryRow(`SELECT balance FROM api_key_balances WHERE caller_scope = ?`, callerScope).Scan(&balance)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("query API key balance by caller scope: %w", err)
	}
	return balance, true, nil
}
