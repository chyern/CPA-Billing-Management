package billing

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const (
	stateVersion    = 4
	maxCachedEvents = 10000
	defaultCurrency = "USD"
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
	s := &Store{dataDir: dataDir, db: db, state: emptyState()}
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

func emptyState() State {
	return State{
		Version: stateVersion, Currency: defaultCurrency, Rules: DefaultRules(),
		Aggregates: map[string]*Aggregate{}, APIKeyAggregates: map[string]*APIKeyAggregate{},
	}
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

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.persistResetLocked(); err != nil {
		s.lastErr = err
		return err
	}
	s.state.Events = nil
	s.state.Aggregates = map[string]*Aggregate{}
	s.state.APIKeyAggregates = map[string]*APIKeyAggregate{}
	return nil
}

func (s *Store) Currency() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Currency
}

func (s *Store) ConfigureYAML(raw []byte) error {
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
		next := s.state
		next.Currency = currency
		if err := s.persistSettingsLocked(next); err != nil {
			s.lastErr = err
			return err
		}
		s.state.Currency = currency
	}
	return nil
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
