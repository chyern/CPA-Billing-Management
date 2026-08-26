package billing

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	stateVersion       = 3
	maxPersistedEvents = 10000
	defaultCurrency    = "USD"
)

func DefaultRules() []PriceRule {
	return []PriceRule{}
}

func NewStore(dataDir string) (*Store, error) {
	dataDir = ResolveDataDir(dataDir, "")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create billing data directory: %w", err)
	}
	dbPath := filepath.Join(dataDir, "billing.db")
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open billing database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure billing database: %w", err)
	}
	s := &Store{dataDir: dataDir, db: db}
	s.state = State{
		Version: stateVersion, Currency: defaultCurrency, Rules: DefaultRules(),
		Aggregates: map[string]*Aggregate{}, APIKeyAggregates: map[string]*APIKeyAggregate{},
	}
	if err := s.initDatabase(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.load(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func defaultDataDir() string {
	if configured := strings.TrimSpace(os.Getenv("cpa_billing_data_dir")); configured != "" {
		return configured
	}
	base, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(base) == "" {
		return filepath.Join(".", ".cpa-billing-management")
	}
	return filepath.Join(base, "cliproxyapi", "cpa-billing-management")
}

// ResolveDataDir chooses an explicitly configured directory first, then the
// plugin-provided fallback directory, and finally the process default.
func ResolveDataDir(configured, fallback string) string {
	if configured = strings.TrimSpace(configured); configured != "" {
		return configured
	}
	if fallback = strings.TrimSpace(fallback); fallback != "" {
		return fallback
	}
	return defaultDataDir()
}

func (s *Store) databasePath() string { return filepath.Join(s.dataDir, "billing.db") }

func (s *Store) initDatabase() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS billing_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			state_json BLOB NOT NULL,
			updated_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("initialize billing database: %w", err)
	}
	return nil
}

func (s *Store) load() error {
	var raw []byte
	err := s.db.QueryRow(`SELECT state_json FROM billing_state WHERE id = 1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return s.persistLocked()
	}
	if err != nil {
		return fmt.Errorf("read billing state from database: %w", err)
	}
	var loaded State
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return fmt.Errorf("decode billing state: %w", err)
	}
	// Redact secret-like values before loading persisted state.
	for i := range loaded.Events {
		loaded.Events[i].Source = MaskSensitiveSource(loaded.Events[i].Source)
		if loaded.Events[i].APIKeyID == "" && loaded.Events[i].APIKey != "" {
			loaded.Events[i].APIKeyID = "legacy:" + loaded.Events[i].APIKey
		}
	}
	needsMigration := loaded.Version < stateVersion
	if needsMigration {
		loaded.Version = stateVersion
	}
	if loaded.Currency == "" {
		loaded.Currency = defaultCurrency
	}
	loaded.Rules = removeLegacyDefaultRules(loaded.Rules)
	if loaded.Aggregates == nil {
		loaded.Aggregates = map[string]*Aggregate{}
	}
	s.state = loaded
	if s.state.APIKeyAggregates == nil {
		s.rebuildAPIKeyAggregatesLocked()
		needsMigration = true
	}
	if needsMigration {
		if err := s.persistLocked(); err != nil {
			return fmt.Errorf("migrate billing state: %w", err)
		}
	}
	return nil
}

func (s *Store) persistLocked() error {
	s.state.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode billing state: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT INTO billing_state (id, state_json, updated_at) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET state_json = excluded.state_json, updated_at = excluded.updated_at
	`, raw, s.state.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("write billing state to database: %w", err)
	}
	return nil
}

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Events = nil
	s.state.Aggregates = map[string]*Aggregate{}
	s.state.APIKeyAggregates = map[string]*APIKeyAggregate{}
	if err := s.persistLocked(); err != nil {
		s.lastErr = err
		return err
	}
	return nil
}

func (s *Store) Currency() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Currency
}

func (s *Store) ConfigureYAML(raw []byte) {
	// The host sends plugin configuration as YAML. Pricing rules stay editable
	// through the model-cost page and are persisted in the SQLite database.
	var currency string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "currency:") {
			currency = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "currency:")), "\"'")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if currency != "" {
		s.state.Currency = currency
		if err := s.persistLocked(); err != nil {
			s.lastErr = err
		}
	}
}

func (s *Store) LastError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastErr == nil {
		return ""
	}
	return s.lastErr.Error()
}

// Close releases the SQLite connection. It is used when the plugin is
// reconfigured to point at a different data directory.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}
