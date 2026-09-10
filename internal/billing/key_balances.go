package billing

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// APIKeyBalance is the current tracked balance for a client API key. APIKeyID
// is a short one-way identifier; the complete credential is never persisted.
type APIKeyBalance struct {
	APIKeyID       string  `json:"api_key_id"`
	APIKey         string  `json:"api_key"`
	CallerScope    string  `json:"caller_scope,omitempty"`
	Note           string  `json:"note,omitempty"`
	Balance        float64 `json:"balance"`
	Configured     bool    `json:"configured"`
	BalanceVersion string  `json:"balance_version"`
	Requests       int64   `json:"requests"`
	Cost           float64 `json:"cost"`
}

func (s *Store) KeyBalances() ([]APIKeyBalance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT api_key_id, api_key, caller_scope, balance, note, requests, cost, balance_version
	 FROM api_key_accounts WHERE api_key_id <> '' ORDER BY api_key, api_key_id`)
	if err != nil {
		return nil, fmt.Errorf("query API key accounts: %w", err)
	}
	defer rows.Close()
	result := make([]APIKeyBalance, 0)
	for rows.Next() {
		var item APIKeyBalance
		var balance sql.NullFloat64
		if err := rows.Scan(&item.APIKeyID, &item.APIKey, &item.CallerScope, &balance, &item.Note, &item.Requests, &item.Cost, &item.BalanceVersion); err != nil {
			return nil, fmt.Errorf("scan API key account: %w", err)
		}
		item.Configured, item.Balance = balance.Valid, balance.Float64
		result = append(result, item)
	}
	return result, rows.Err()
}

// ErrBalanceConflict means usage or another editor changed a balance after the
// caller read it. The entire patch is rolled back so no partial edit is saved.
var ErrBalanceConflict = errors.New("API key balance has changed; refresh and retry")

// APIKeyBalanceUpdate modifies only the supplied fields for one key. Balance
// changes, disabling tracking and deletion require the version from KeyBalances;
// notes can be edited independently while requests continue to consume credit.
type APIKeyBalanceUpdate struct {
	APIKeyID               string   `json:"api_key_id"`
	APIKey                 string   `json:"api_key"`
	CallerScope            string   `json:"caller_scope,omitempty"`
	Note                   *string  `json:"note,omitempty"`
	Balance                *float64 `json:"balance,omitempty"`
	Configured             *bool    `json:"configured,omitempty"`
	Delete                 bool     `json:"delete,omitempty"`
	ExpectedBalanceVersion *string  `json:"expected_balance_version,omitempty"`
}

func (s *Store) PatchKeyBalances(items []APIKeyBalanceUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]bool, len(items))
	for index := range items {
		item := &items[index]
		item.APIKeyID = strings.ToLower(strings.TrimSpace(item.APIKeyID))
		if item.APIKeyID == "" || seen[item.APIKeyID] {
			return fmt.Errorf("API key update identifiers must be nonempty and unique")
		}
		seen[item.APIKeyID] = true
		if item.Delete && (item.Note != nil || item.Balance != nil || item.Configured != nil) {
			return fmt.Errorf("API key deletion cannot include other updates")
		}
		if item.Balance != nil && (math.IsNaN(*item.Balance) || math.IsInf(*item.Balance, 0) || *item.Balance < 0) {
			return fmt.Errorf("API key balance must be a non-negative number")
		}
		if item.Configured != nil && !*item.Configured && item.Balance != nil {
			return fmt.Errorf("disabled balance tracking cannot include a balance")
		}
		if item.Configured != nil && *item.Configured && item.Balance == nil {
			return fmt.Errorf("enabling balance tracking requires a balance")
		}
		if (item.Delete || item.Balance != nil || item.Configured != nil) && item.ExpectedBalanceVersion == nil {
			return fmt.Errorf("API key balance version is required")
		}
		if item.Note != nil {
			note := strings.TrimSpace(*item.Note)
			if len([]rune(note)) > 200 {
				return fmt.Errorf("API key note must not exceed 200 characters")
			}
			item.Note = &note
		}
		item.CallerScope = strings.ToLower(strings.TrimSpace(item.CallerScope))
		if item.CallerScope == "" && strings.TrimSpace(item.APIKey) != "" && !strings.ContainsRune(item.APIKey, '•') {
			item.CallerScope = CallerScope(item.APIKey)
		}
		if item.CallerScope != "" {
			decoded, err := hex.DecodeString(item.CallerScope)
			if err != nil || len(decoded) != 32 {
				return fmt.Errorf("API key caller scope must be a 64-character SHA-256 value")
			}
		}
		item.APIKey = strings.TrimSpace(item.APIKey)
		if item.APIKey == "" {
			item.APIKey = "未命名密钥"
		} else if !strings.ContainsRune(item.APIKey, '•') {
			item.APIKey = MaskAPIKey(item.APIKey)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := s.withTransaction(func(tx *sql.Tx) error {
		for _, item := range items {
			if item.ExpectedBalanceVersion != nil {
				var version string
				err := tx.QueryRow(`SELECT balance_version FROM api_key_accounts WHERE api_key_id = ?`, item.APIKeyID).Scan(&version)
				if err != nil && err != sql.ErrNoRows {
					return fmt.Errorf("read API key balance version: %w", err)
				}
				if version != *item.ExpectedBalanceVersion {
					return ErrBalanceConflict
				}
			}
			if item.Delete || (item.Configured != nil && !*item.Configured) {
				if _, err := tx.Exec(`UPDATE api_key_accounts SET balance=NULL, caller_scope='', balance_version='', updated_at=? WHERE api_key_id = ?`, now, item.APIKeyID); err != nil {
					return fmt.Errorf("delete API key balance: %w", err)
				}
			} else if item.Balance != nil {
				if _, err := tx.Exec(`INSERT INTO api_key_accounts (api_key_id, api_key, caller_scope, balance, balance_version, updated_at) VALUES (?, ?, ?, ?, ?, ?)
					ON CONFLICT(api_key_id) DO UPDATE SET api_key = excluded.api_key,
					caller_scope = CASE WHEN excluded.caller_scope = '' THEN api_key_accounts.caller_scope ELSE excluded.caller_scope END,
					balance = excluded.balance, balance_version = excluded.balance_version, updated_at = excluded.updated_at`, item.APIKeyID, item.APIKey, item.CallerScope, *item.Balance, now, now); err != nil {
					return fmt.Errorf("update API key balance: %w", err)
				}
			}
			if item.Delete || (item.Note != nil && *item.Note == "") {
				if _, err := tx.Exec(`UPDATE api_key_accounts SET note='', updated_at=? WHERE api_key_id = ?`, now, item.APIKeyID); err != nil {
					return fmt.Errorf("delete API key note: %w", err)
				}
			} else if item.Note != nil {
				if _, err := tx.Exec(`INSERT INTO api_key_accounts (api_key_id, api_key, note, updated_at) VALUES (?, ?, ?, ?)
					ON CONFLICT(api_key_id) DO UPDATE SET api_key = excluded.api_key, note = excluded.note, updated_at = excluded.updated_at`, item.APIKeyID, item.APIKey, *item.Note, now); err != nil {
					return fmt.Errorf("update API key note: %w", err)
				}
			}
		}
		return pruneEmptyAccounts(tx)
	})
	if err != nil && !errors.Is(err, ErrBalanceConflict) {
		s.lastErr = err
	}
	return err
}

// SetKeyBalanceNotes replaces the optional descriptions associated with API
// keys. Notes do not enable balance tracking, so a key can have a description
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
		if _, err := tx.Exec(`UPDATE api_key_accounts SET note='', updated_at=?`, now); err != nil {
			return fmt.Errorf("clear API key balance notes: %w", err)
		}
		for _, record := range records {
			if _, err := tx.Exec(`INSERT INTO api_key_accounts (api_key_id, api_key, note, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT(api_key_id) DO UPDATE SET api_key=excluded.api_key, note=excluded.note, updated_at=excluded.updated_at`, record.id, record.apiKey, record.note, now); err != nil {
				return fmt.Errorf("save API key balance note %q: %w", record.id, err)
			}
		}
		return pruneEmptyAccounts(tx)
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
		if _, err := tx.Exec(`UPDATE api_key_accounts SET balance=NULL, caller_scope='', balance_version='', updated_at=?`, now); err != nil {
			return fmt.Errorf("clear API key balances: %w", err)
		}
		for _, item := range items {
			if _, err := tx.Exec(`INSERT INTO api_key_accounts (api_key_id, api_key, caller_scope, balance, balance_version, updated_at) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(api_key_id) DO UPDATE SET api_key=excluded.api_key, caller_scope=excluded.caller_scope, balance=excluded.balance, balance_version=excluded.balance_version, updated_at=excluded.updated_at`, item.APIKeyID, item.APIKey, item.CallerScope, item.Balance, now, now); err != nil {
				return fmt.Errorf("save API key balance %q: %w", item.APIKeyID, err)
			}
		}
		return pruneEmptyAccounts(tx)
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
	err := s.db.QueryRow(`SELECT balance FROM api_key_accounts WHERE caller_scope = ? AND balance IS NOT NULL`, callerScope).Scan(&balance)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("query API key balance by caller scope: %w", err)
	}
	return balance, true, nil
}
