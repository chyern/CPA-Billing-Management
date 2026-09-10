package billing

import (
	"database/sql"
	"fmt"
	"strings"
)

const eventSchema = `(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 requested_at TEXT NOT NULL,
 model TEXT NOT NULL,
 provider TEXT NOT NULL DEFAULT '',
 domain TEXT NOT NULL DEFAULT '',
 api_key TEXT NOT NULL DEFAULT '',
 latency_ns INTEGER NOT NULL DEFAULT 0,
 ttft_ns INTEGER NOT NULL DEFAULT 0,
 input_tokens INTEGER NOT NULL DEFAULT 0,
 cached_tokens INTEGER NOT NULL DEFAULT 0,
 output_tokens INTEGER NOT NULL DEFAULT 0,
 cost REAL NOT NULL DEFAULT 0,
 currency TEXT NOT NULL DEFAULT '',
 failed INTEGER NOT NULL DEFAULT 0 CHECK (failed IN (0, 1))
)`

// This table is the immutable dashboard row, not the original usage payload.
// Rebuild older schemas transactionally, preserving row IDs and visible values.
// Never infer an upstream for records that did not already store a snapshot.
func ensureEventSchemaTx(tx *sql.Tx) error {
	var err error
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS usage_events ` + eventSchema); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT name FROM pragma_table_info('usage_events')`)
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	expected := strings.Split(strings.ReplaceAll("id,"+eventColumns, "\n", ""), ",")
	rebuild := len(columns) != len(expected)
	for _, name := range expected {
		if !columns[strings.TrimSpace(name)] {
			rebuild = true
		}
	}
	if rebuild {
		var version int
		err := tx.QueryRow(`SELECT schema_version FROM billing_settings WHERE id = 1`).Scan(&version)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == nil && version != 4 && version != 5 && version != stateVersion {
			return fmt.Errorf("unsupported billing schema version %d", version)
		}
		if _, err = tx.Exec(`CREATE TABLE usage_events_snapshot ` + eventSchema); err != nil {
			return err
		}
		if err = migrateEventSnapshots(tx, columns); err != nil {
			return err
		}

		if _, err = tx.Exec(`DROP TABLE usage_events; ALTER TABLE usage_events_snapshot RENAME TO usage_events`); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_usage_events_requested_at ON usage_events(requested_at);
 CREATE INDEX IF NOT EXISTS idx_usage_events_date_range ON usage_events(rtrim(requested_at, 'Z'));
 CREATE INDEX IF NOT EXISTS idx_usage_events_status ON usage_events(failed, id);`); err != nil {
		return err
	}
	return nil
}

// Split only a value already saved in the row; never consult current config.
func migrateEventSnapshots(tx *sql.Tx, columns map[string]bool) error {
	column := func(name string) string {
		if columns[name] {
			return name
		}
		return "''"
	}
	rows, err := tx.Query(`SELECT id, requested_at, model, ` + column("upstream") + `, ` + column("provider") + `, ` + column("domain") + `, api_key,
        latency_ns, ttft_ns, input_tokens, cached_tokens, output_tokens, cost, ` + column("currency") + `, failed FROM usage_events ORDER BY id`)
	if err != nil {
		return fmt.Errorf("read event snapshots for migration: %w", err)
	}
	defer rows.Close()
	insert, err := tx.Prepare(`INSERT INTO usage_events_snapshot (id, ` + eventColumns + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insert.Close()
	for rows.Next() {
		var id int64
		var requestedAt, combined string
		var event UsageEvent
		if err := rows.Scan(&id, &requestedAt, &event.Model, &combined, &event.Provider, &event.Domain, &event.APIKey,
			&event.LatencyNanos, &event.TTFTNanos, &event.InputTokens, &event.CachedTokens, &event.OutputTokens, &event.Cost, &event.Currency, &event.Failed); err != nil {
			return err
		}
		if columns["upstream"] {
			event.Provider, event.Domain = "", ""
			if combined != "" {
				index := strings.LastIndexByte(combined, '(')
				if index <= 0 || !strings.HasSuffix(combined, ")") {
					return fmt.Errorf("cannot split upstream snapshot for event %d", id)
				}
				event.Provider, event.Domain = combined[:index], combined[index+1:len(combined)-1]
			}
		}
		if _, err := insert.Exec(id, requestedAt, event.Model, event.Provider, event.Domain, event.APIKey,
			event.LatencyNanos, event.TTFTNanos, event.InputTokens, event.CachedTokens, event.OutputTokens, event.Cost, event.Currency, event.Failed); err != nil {
			return err
		}
	}
	return rows.Err()
}
