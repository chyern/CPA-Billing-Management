package billing

import (
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) withTransaction(operation func(*sql.Tx) error) error {
	if s.db == nil {
		return fmt.Errorf("billing database is closed")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin billing transaction: %w", err)
	}
	if err := operation(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit billing transaction: %w", err)
	}
	return nil
}

func (s *Store) persistSettingsLocked(next State) error {
	next.UpdatedAt = time.Now().UTC()
	if err := s.withTransaction(func(tx *sql.Tx) error { return writeSettings(tx, next) }); err != nil {
		return err
	}
	s.state.UpdatedAt = next.UpdatedAt
	return nil
}

func (s *Store) persistFullStateLocked() error {
	next := s.state
	next.UpdatedAt = time.Now().UTC()
	if err := s.withTransaction(func(tx *sql.Tx) error { return replaceState(tx, next) }); err != nil {
		return err
	}
	s.state.UpdatedAt = next.UpdatedAt
	return nil
}

func (s *Store) persistResetLocked() error {
	next := s.state
	next.UpdatedAt = time.Now().UTC()
	if err := s.withTransaction(func(tx *sql.Tx) error {
		if err := writeSettings(tx, next); err != nil {
			return err
		}
		for _, table := range []string{"usage_events"} {
			if _, err := tx.Exec(`DELETE FROM ` + table); err != nil {
				return fmt.Errorf("clear %s: %w", table, err)
			}
		}
		return clearAccountStatistics(tx)
	}); err != nil {
		return err
	}
	s.state.UpdatedAt = next.UpdatedAt
	return nil
}

func (s *Store) persistUsageLocked(event UsageEvent, next State, apiAgg *APIKeyAggregate) error {
	return s.withTransaction(func(tx *sql.Tx) error {
		if err := writeSettings(tx, next); err != nil {
			return err
		}
		if err := insertEvent(tx, event); err != nil {
			return err
		}
		if err := upsertAPIKeyAggregate(tx, event.APIKeyID, apiAgg); err != nil {
			return err
		}
		if event.Cost > 0 && event.APIKeyID != "" {
			if _, err := tx.Exec(`UPDATE api_key_accounts SET balance = balance - ?, balance_version = ?, updated_at = ? WHERE api_key_id = ? AND balance IS NOT NULL`, event.Cost, next.UpdatedAt.Format(time.RFC3339Nano), next.UpdatedAt.Format(time.RFC3339Nano), event.APIKeyID); err != nil {
				return fmt.Errorf("decrement API key balance %q: %w", event.APIKeyID, err)
			}
		}
		return nil
	})
}

func (s *Store) persistRulesLocked(rules []PriceRule) error {
	next := s.state
	next.Rules = append([]PriceRule(nil), rules...)
	next.UpdatedAt = time.Now().UTC()
	if err := s.withTransaction(func(tx *sql.Tx) error {
		if err := writeSettings(tx, next); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM pricing_rules`); err != nil {
			return fmt.Errorf("clear pricing rules: %w", err)
		}
		for i, r := range rules {
			if _, err := tx.Exec(`INSERT INTO pricing_rules (position, match, input_per_million, output_per_million, cache_read_per_million, cache_creation_per_million) VALUES (?, ?, ?, ?, ?, ?)`, i, r.Match, r.InputPerMillion, r.OutputPerMillion, r.CacheReadPerMillion, r.CacheCreationPerMillion); err != nil {
				return fmt.Errorf("insert pricing rule %q: %w", r.Match, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	s.state.Rules = append([]PriceRule(nil), rules...)
	s.state.UpdatedAt = next.UpdatedAt
	return nil
}

// ReplaceState atomically replaces all rows in the normalized billing schema.
func ReplaceState(db *sql.DB, state State) error {
	state.Version = stateVersion
	if state.Currency == "" {
		state.Currency = defaultCurrency
	}
	state.UpdatedAt = time.Now().UTC()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := replaceState(tx, state); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func replaceState(tx *sql.Tx, state State) error {
	if err := writeSettings(tx, state); err != nil {
		return err
	}
	for _, table := range []string{"pricing_rules", "usage_events"} {
		if _, err := tx.Exec(`DELETE FROM ` + table); err != nil {
			return fmt.Errorf("replace %s: %w", table, err)
		}
	}
	if err := clearAccountStatistics(tx); err != nil {
		return err
	}
	for position, rule := range state.Rules {
		if _, err := tx.Exec(`INSERT INTO pricing_rules (position, match, input_per_million, output_per_million, cache_read_per_million, cache_creation_per_million) VALUES (?, ?, ?, ?, ?, ?)`, position, rule.Match, rule.InputPerMillion, rule.OutputPerMillion, rule.CacheReadPerMillion, rule.CacheCreationPerMillion); err != nil {
			return fmt.Errorf("insert pricing rule %q: %w", rule.Match, err)
		}
	}
	for _, event := range state.Events {
		if err := insertEvent(tx, event); err != nil {
			return err
		}
	}
	for key, aggregate := range state.APIKeyAggregates {
		if err := upsertAPIKeyAggregate(tx, key, aggregate); err != nil {
			return err
		}
	}
	return nil
}

func writeSettings(tx *sql.Tx, state State) error {
	_, err := tx.Exec(`
		INSERT INTO billing_settings (id, schema_version, currency, updated_at) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET schema_version = excluded.schema_version, currency = excluded.currency, updated_at = excluded.updated_at
	`, stateVersion, state.Currency, state.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("write billing settings: %w", err)
	}
	return nil
}

func insertEvent(tx *sql.Tx, event UsageEvent) error {
	_, err := tx.Exec(`INSERT INTO usage_events (`+eventColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.RequestedAt.Format(time.RFC3339Nano), event.Model, event.ReasoningEffort, event.Provider, event.Domain, event.APIKey,
		event.LatencyNanos, event.TTFTNanos, event.InputTokens, event.CachedTokens, event.OutputTokens, event.Cost, event.Currency, event.Failed)
	if err != nil {
		return fmt.Errorf("insert usage event: %w", err)
	}
	return nil
}

func upsertAPIKeyAggregate(tx *sql.Tx, key string, aggregate *APIKeyAggregate) error {
	if aggregate == nil {
		return fmt.Errorf("API key aggregate %q is unavailable", key)
	}
	_, err := tx.Exec(`
		INSERT INTO api_key_accounts (api_key_id, api_key, requests, cost, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(api_key_id) DO UPDATE SET requests=excluded.requests, cost=excluded.cost,
		 api_key=CASE WHEN api_key_accounts.api_key='' THEN excluded.api_key ELSE api_key_accounts.api_key END,
		 updated_at=excluded.updated_at
	`, key, aggregate.APIKey, aggregate.Requests, aggregate.Cost, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("write API key aggregate %q: %w", key, err)
	}
	return nil
}
