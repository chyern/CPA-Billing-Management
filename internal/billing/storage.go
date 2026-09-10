package billing

import (
	"database/sql"
	"fmt"
	"time"
)

// EnsureSchema creates the normalized SQLite schema used by the plugin and
// standalone data tools.
func EnsureSchema(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='billing_settings'`).Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		err := tx.QueryRow(`SELECT schema_version FROM billing_settings WHERE id=1`).Scan(&version)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == nil && version != 4 && version != 5 && version != stateVersion {
			return fmt.Errorf("unsupported billing schema version %d", version)
		}
	}
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS billing_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			schema_version INTEGER NOT NULL,
			currency TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS pricing_rules (
			position INTEGER PRIMARY KEY,
			match TEXT NOT NULL COLLATE NOCASE UNIQUE,
			input_per_million REAL NOT NULL CHECK (input_per_million >= 0),
			output_per_million REAL NOT NULL CHECK (output_per_million >= 0),
			cache_read_per_million REAL NOT NULL CHECK (cache_read_per_million >= 0),
			cache_creation_per_million REAL NOT NULL CHECK (cache_creation_per_million >= 0)
		);
	`)
	if err != nil {
		return fmt.Errorf("initialize billing database: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE IF EXISTS model_aggregates`); err != nil {
		return err
	}
	if err := ensureAccountSchema(tx); err != nil {
		return err
	}
	if err := ensureEventSchemaTx(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE billing_settings SET schema_version=? WHERE id=1`, stateVersion); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) initDatabase() error { return EnsureSchema(s.db) }

func (s *Store) load() error {
	loaded := emptyState()
	var updatedAt string
	err := s.db.QueryRow(`SELECT schema_version, currency, updated_at FROM billing_settings WHERE id = 1`).Scan(&loaded.Version, &loaded.Currency, &updatedAt)
	if err == sql.ErrNoRows {
		s.state = loaded
		return s.persistFullStateLocked()
	}
	if err != nil {
		return fmt.Errorf("load billing settings: %w", err)
	}
	if loaded.Version != stateVersion {
		return fmt.Errorf("unsupported billing schema version %d, expected %d", loaded.Version, stateVersion)
	}
	loaded.UpdatedAt = parseDatabaseTime(updatedAt)

	if loaded.Rules, err = loadRules(s.db); err != nil {
		return err
	}
	if loaded.Events, err = loadEvents(s.db); err != nil {
		return err
	}
	if loaded.APIKeyAggregates, err = loadAPIKeyAggregates(s.db); err != nil {
		return err
	}
	s.state = loaded
	return nil
}

func loadRules(db *sql.DB) ([]PriceRule, error) {
	rows, err := db.Query(`SELECT match, input_per_million, output_per_million, cache_read_per_million, cache_creation_per_million FROM pricing_rules ORDER BY position`)
	if err != nil {
		return nil, fmt.Errorf("load pricing rules: %w", err)
	}
	defer rows.Close()
	var rules []PriceRule
	for rows.Next() {
		var rule PriceRule
		if err := rows.Scan(&rule.Match, &rule.InputPerMillion, &rule.OutputPerMillion, &rule.CacheReadPerMillion, &rule.CacheCreationPerMillion); err != nil {
			return nil, fmt.Errorf("scan pricing rule: %w", err)
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

const eventColumns = `requested_at, model, provider, domain, api_key, latency_ns, ttft_ns,
 input_tokens, cached_tokens, output_tokens, cost, currency, failed`

func loadEvents(db *sql.DB) ([]UsageEvent, error) {
	rows, err := db.Query(`SELECT `+eventColumns+` FROM (SELECT * FROM usage_events ORDER BY id DESC LIMIT ?) ORDER BY id`, maxCachedEvents)
	if err != nil {
		return nil, fmt.Errorf("load usage events: %w", err)
	}
	return scanUsageEvents(rows)
}

func scanUsageEvents(rows *sql.Rows) ([]UsageEvent, error) {
	defer rows.Close()
	events := make([]UsageEvent, 0)
	for rows.Next() {
		var event UsageEvent
		var requestedAt string
		if err := rows.Scan(&requestedAt, &event.Model, &event.Provider, &event.Domain, &event.APIKey, &event.LatencyNanos, &event.TTFTNanos, &event.InputTokens, &event.CachedTokens, &event.OutputTokens, &event.Cost, &event.Currency, &event.Failed); err != nil {
			return nil, fmt.Errorf("scan usage event: %w", err)
		}
		event.RequestedAt = parseDatabaseTime(requestedAt)
		event.TotalTokens = event.InputTokens + event.OutputTokens
		event.Priced = true

		events = append(events, event)
	}
	return events, rows.Err()
}

func loadAPIKeyAggregates(db *sql.DB) (map[string]*APIKeyAggregate, error) {
	rows, err := db.Query(`SELECT api_key_id, api_key, requests, cost FROM api_key_accounts WHERE requests <> 0 OR cost <> 0`)
	if err != nil {
		return nil, fmt.Errorf("load API key aggregates: %w", err)
	}
	defer rows.Close()
	result := map[string]*APIKeyAggregate{}
	for rows.Next() {
		var key string
		var aggregate APIKeyAggregate
		if err := rows.Scan(&key, &aggregate.APIKey, &aggregate.Requests, &aggregate.Cost); err != nil {
			return nil, fmt.Errorf("scan API key aggregate: %w", err)
		}
		result[key] = &aggregate
	}
	return result, rows.Err()
}

func parseDatabaseTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}
