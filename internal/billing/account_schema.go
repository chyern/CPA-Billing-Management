package billing

import (
	"database/sql"
	"fmt"
)

// Accounts are the balance page's source of truth. NULL balance means tracking
// is disabled; zero and negative balances remain configured and block requests.
// The version belongs only to the balance, so note edits do not conflict with it.
func ensureAccountSchema(tx *sql.Tx) error {
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS api_key_accounts (
		api_key_id TEXT PRIMARY KEY,
		api_key TEXT NOT NULL DEFAULT '',
		caller_scope TEXT NOT NULL DEFAULT '',
		balance REAL,
		note TEXT NOT NULL DEFAULT '',
		requests INTEGER NOT NULL DEFAULT 0,
		cost REAL NOT NULL DEFAULT 0,
		balance_version TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return err
	}
	for _, table := range []string{"api_key_aggregates", "api_key_balance_notes", "api_key_balances"} {
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			continue
		}
		var query string
		switch table {
		case "api_key_aggregates":
			query = `INSERT INTO api_key_accounts (api_key_id, api_key, requests, cost)
			 SELECT aggregate_key, api_key, requests, cost FROM api_key_aggregates WHERE true
			 ON CONFLICT(api_key_id) DO UPDATE SET api_key=excluded.api_key, requests=excluded.requests, cost=excluded.cost`
		case "api_key_balance_notes":
			query = `INSERT INTO api_key_accounts (api_key_id, api_key, note, updated_at)
			 SELECT api_key_id, api_key, note, updated_at FROM api_key_balance_notes WHERE true
			 ON CONFLICT(api_key_id) DO UPDATE SET api_key=CASE WHEN excluded.api_key='' THEN api_key_accounts.api_key ELSE excluded.api_key END,
			 note=excluded.note, updated_at=excluded.updated_at`
		case "api_key_balances":
			var hasScope int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('api_key_balances') WHERE name='caller_scope'`).Scan(&hasScope); err != nil {
				return err
			}
			scope := "''"
			if hasScope != 0 {
				scope = "caller_scope"
			}
			query = `INSERT INTO api_key_accounts (api_key_id, api_key, caller_scope, balance, balance_version, updated_at)
			 SELECT api_key_id, api_key, ` + scope + `, balance, updated_at, updated_at FROM api_key_balances WHERE true
			 ON CONFLICT(api_key_id) DO UPDATE SET api_key=CASE WHEN excluded.api_key='' THEN api_key_accounts.api_key ELSE excluded.api_key END,
			 caller_scope=excluded.caller_scope, balance=excluded.balance, balance_version=excluded.balance_version,
			 updated_at=max(api_key_accounts.updated_at,excluded.updated_at)`
		}
		if _, err := tx.Exec(query); err != nil {
			return fmt.Errorf("migrate %s into accounts: %w", table, err)
		}
		if _, err := tx.Exec(`DROP TABLE ` + table); err != nil {
			return err
		}
	}
	_, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_key_accounts_caller_scope ON api_key_accounts(caller_scope) WHERE caller_scope <> ''`)
	return err
}

// Clear only statistics; resetting the billing page must preserve wallet funds.
func clearAccountStatistics(tx *sql.Tx) error {
	if _, err := tx.Exec(`UPDATE api_key_accounts SET requests=0, cost=0`); err != nil {
		return err
	}
	return pruneEmptyAccounts(tx)
}

func pruneEmptyAccounts(tx *sql.Tx) error {
	_, err := tx.Exec(`DELETE FROM api_key_accounts WHERE balance IS NULL AND note='' AND requests=0 AND cost=0`)
	return err
}
