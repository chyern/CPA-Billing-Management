package billing

import (
	"database/sql"
	"fmt"
	"time"
)

// EnsureSchema creates the normalized SQLite schema used by the plugin and
// standalone data tools.
func EnsureSchema(db *sql.DB) error {
	_, err := db.Exec(`
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
		CREATE TABLE IF NOT EXISTS usage_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			requested_at TEXT NOT NULL,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			alias TEXT NOT NULL DEFAULT '',
			api_key TEXT NOT NULL DEFAULT '',
			api_key_id TEXT NOT NULL DEFAULT '',
			auth_type TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			latency_ns INTEGER NOT NULL DEFAULT 0,
			ttft_ns INTEGER NOT NULL DEFAULT 0,
			failed INTEGER NOT NULL DEFAULT 0 CHECK (failed IN (0, 1)),
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			reasoning_tokens INTEGER NOT NULL DEFAULT 0,
			cached_tokens INTEGER NOT NULL DEFAULT 0,
			cache_read_tokens INTEGER NOT NULL DEFAULT 0,
			cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0,
			priced_by TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_usage_events_requested_at ON usage_events(requested_at);
		CREATE INDEX IF NOT EXISTS idx_usage_events_provider_model ON usage_events(provider, model);
		CREATE INDEX IF NOT EXISTS idx_usage_events_api_key_id ON usage_events(api_key_id);
		CREATE TABLE IF NOT EXISTS model_aggregates (
			aggregate_key TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			requests INTEGER NOT NULL DEFAULT 0,
			failed_requests INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			reasoning_tokens INTEGER NOT NULL DEFAULT 0,
			cached_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0,
			priced INTEGER NOT NULL DEFAULT 0 CHECK (priced IN (0, 1))
		);
		CREATE TABLE IF NOT EXISTS api_key_aggregates (
			aggregate_key TEXT PRIMARY KEY,
			api_key TEXT NOT NULL,
			requests INTEGER NOT NULL DEFAULT 0,
			failed_requests INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			reasoning_tokens INTEGER NOT NULL DEFAULT 0,
			cached_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS api_key_balances (
			api_key_id TEXT PRIMARY KEY,
			api_key TEXT NOT NULL,
			balance REAL NOT NULL,
			caller_scope TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS api_key_balance_notes (
			api_key_id TEXT PRIMARY KEY,
			api_key TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("initialize normalized billing database: %w", err)
	}
	if err := ensureAPIKeyBalanceCallerScope(db); err != nil {
		return err
	}
	return nil
}

func ensureAPIKeyBalanceCallerScope(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(api_key_balances)`)
	if err != nil {
		return fmt.Errorf("inspect API key balance schema: %w", err)
	}
	hasCallerScope := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan API key balance schema: %w", err)
		}
		if name == "caller_scope" {
			hasCallerScope = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !hasCallerScope {
		if _, err := db.Exec(`ALTER TABLE api_key_balances ADD COLUMN caller_scope TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add API key caller scope: %w", err)
		}
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_key_balances_caller_scope ON api_key_balances(caller_scope) WHERE caller_scope <> ''`); err != nil {
		return fmt.Errorf("index API key caller scope: %w", err)
	}
	return nil
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
	if loaded.Aggregates, err = loadModelAggregates(s.db); err != nil {
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

func loadEvents(db *sql.DB) ([]UsageEvent, error) {
	rows, err := db.Query(`
		SELECT requested_at, provider, model, alias, api_key, api_key_id, auth_type, source,
		       latency_ns, ttft_ns, failed, input_tokens, output_tokens, reasoning_tokens,
		       cached_tokens, cache_read_tokens, cache_creation_tokens, total_tokens, cost, priced_by
		FROM usage_events ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("load usage events: %w", err)
	}
	defer rows.Close()
	var events []UsageEvent
	for rows.Next() {
		var event UsageEvent
		var requestedAt string
		var failed int
		if err := rows.Scan(
			&requestedAt, &event.Provider, &event.Model, &event.Alias, &event.APIKey, &event.APIKeyID,
			&event.AuthType, &event.Source, &event.LatencyNanos, &event.TTFTNanos, &failed,
			&event.InputTokens, &event.OutputTokens, &event.ReasoningTokens, &event.CachedTokens,
			&event.CacheReadTokens, &event.CacheCreationTokens, &event.TotalTokens, &event.Cost, &event.PricedBy,
		); err != nil {
			return nil, fmt.Errorf("scan usage event: %w", err)
		}
		event.RequestedAt = parseDatabaseTime(requestedAt)
		event.Failed = failed != 0
		event.Source = MaskSensitiveSource(event.Source)
		events = append(events, event)
	}
	return events, rows.Err()
}

func loadModelAggregates(db *sql.DB) (map[string]*Aggregate, error) {
	rows, err := db.Query(`SELECT aggregate_key, provider, model, requests, failed_requests, input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens, cost, priced FROM model_aggregates`)
	if err != nil {
		return nil, fmt.Errorf("load model aggregates: %w", err)
	}
	defer rows.Close()
	result := map[string]*Aggregate{}
	for rows.Next() {
		var key string
		var priced int
		var aggregate Aggregate
		if err := rows.Scan(&key, &aggregate.Provider, &aggregate.Model, &aggregate.Requests, &aggregate.FailedRequests, &aggregate.InputTokens, &aggregate.OutputTokens, &aggregate.ReasoningTokens, &aggregate.CachedTokens, &aggregate.TotalTokens, &aggregate.Cost, &priced); err != nil {
			return nil, fmt.Errorf("scan model aggregate: %w", err)
		}
		aggregate.Priced = priced != 0
		result[key] = &aggregate
	}
	return result, rows.Err()
}

func loadAPIKeyAggregates(db *sql.DB) (map[string]*APIKeyAggregate, error) {
	rows, err := db.Query(`SELECT aggregate_key, api_key, requests, failed_requests, input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens, cost FROM api_key_aggregates`)
	if err != nil {
		return nil, fmt.Errorf("load API key aggregates: %w", err)
	}
	defer rows.Close()
	result := map[string]*APIKeyAggregate{}
	for rows.Next() {
		var key string
		var aggregate APIKeyAggregate
		if err := rows.Scan(&key, &aggregate.APIKey, &aggregate.Requests, &aggregate.FailedRequests, &aggregate.InputTokens, &aggregate.OutputTokens, &aggregate.ReasoningTokens, &aggregate.CachedTokens, &aggregate.TotalTokens, &aggregate.Cost); err != nil {
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
