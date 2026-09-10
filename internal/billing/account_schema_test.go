package billing

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Build a real v4 database with overlapping and disjoint rows in all three
// wallet tables, including indistinguishable masked labels for different keys.
func legacyPageDatabase(t *testing.T, withScope bool) (string, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.HandleUsage(UsageRecord{Provider: "test", Model: "history", APIKey: "historical-key", Cost: 7, CostProvided: true, InputTokens: 40, OutputTokens: 10}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRules([]PriceRule{{Match: "history", InputPerMillion: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(dir, "billing.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	scopeColumn, scopeValue := "", ""
	if withScope {
		scopeColumn = ", caller_scope TEXT NOT NULL DEFAULT ''"
		scopeValue = ", '" + CallerScope("test-key") + "'"
	}
	_, err = db.Exec(`DROP TABLE api_key_accounts;
 UPDATE billing_settings SET schema_version=4;
 CREATE TABLE api_key_aggregates (aggregate_key TEXT PRIMARY KEY, api_key TEXT NOT NULL, requests INTEGER NOT NULL DEFAULT 0, failed_requests INTEGER NOT NULL DEFAULT 0, input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0, reasoning_tokens INTEGER NOT NULL DEFAULT 0, cached_tokens INTEGER NOT NULL DEFAULT 0, total_tokens INTEGER NOT NULL DEFAULT 0, cost REAL NOT NULL DEFAULT 0);
 CREATE TABLE api_key_balances (api_key_id TEXT PRIMARY KEY, api_key TEXT NOT NULL, balance REAL NOT NULL, updated_at TEXT NOT NULL` + scopeColumn + `);
 CREATE TABLE api_key_balance_notes (api_key_id TEXT PRIMARY KEY, api_key TEXT NOT NULL, note TEXT NOT NULL, updated_at TEXT NOT NULL);
 INSERT INTO api_key_aggregates(aggregate_key,api_key,requests,cost) VALUES ('overlap','same••••mask',100,12.5),('usage-only','same••••mask',8,2);
 INSERT INTO api_key_balances VALUES ('overlap','same••••mask',-2.5,'2026-09-10T01:00:00Z'` + scopeValue + `);
 INSERT INTO api_key_balances(api_key_id,api_key,balance,updated_at) VALUES ('zero','zero••••key',0,'2026-09-10T02:00:00Z');
 INSERT INTO api_key_balance_notes VALUES ('overlap','same••••mask','团队 A','2026-09-10T03:00:00Z'),('note-only','note••••key','仅备注','2026-09-10T04:00:00Z');
 CREATE TABLE model_aggregates (aggregate_key TEXT, provider TEXT, model TEXT);
 ALTER TABLE model_aggregates ADD COLUMN reasoning_tokens INTEGER DEFAULT 0;
 UPDATE model_aggregates SET aggregate_key='test/history',reasoning_tokens=99;
 CREATE TRIGGER forbid_snapshot_update BEFORE UPDATE ON usage_events BEGIN SELECT RAISE(ABORT,'snapshot changed'); END;
 CREATE TRIGGER forbid_snapshot_delete BEFORE DELETE ON usage_events BEGIN SELECT RAISE(ABORT,'snapshot deleted'); END;`)
	if err != nil {
		t.Fatal(err)
	}
	return dir, db
}

func TestPageSchemaMigrationPreservesIndependentData(t *testing.T) {
	for _, scope := range []bool{false, true} {
		name := "without scope"
		if scope {
			name = "with scope"
		}
		t.Run(name, func(t *testing.T) {
			dir, db := legacyPageDatabase(t, scope)
			s, err := NewStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			items, err := s.KeyBalances()
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 4 {
				t.Fatalf("accounts=%+v", items)
			}
			byID := map[string]APIKeyBalance{}
			for _, item := range items {
				byID[item.APIKeyID] = item
			}
			overlap := byID["overlap"]
			if !overlap.Configured || overlap.Balance != -2.5 || overlap.Note != "团队 A" || overlap.Requests != 100 || overlap.Cost != 12.5 || overlap.BalanceVersion != "2026-09-10T01:00:00Z" {
				t.Fatalf("merged account=%+v", overlap)
			}
			if scope && overlap.CallerScope != CallerScope("test-key") {
				t.Fatal("lost caller scope")
			}
			if !byID["zero"].Configured || byID["zero"].Balance != 0 || byID["note-only"].Configured || byID["note-only"].Note != "仅备注" || byID["usage-only"].Requests != 8 {
				t.Fatalf("disjoint accounts=%+v", byID)
			}
			sum := s.Summary()
			if sum.Totals.Requests != 1 || sum.Totals.Cost != 7 || sum.Totals.TotalTokens != 50 || len(sum.RecentEvents) != 1 || sum.RecentEvents[0].Cost != 7 || len(s.Rules()) != 1 {
				t.Fatalf("page data=%+v", sum)
			}
			var count, version int
			if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('api_key_aggregates','api_key_balances','api_key_balance_notes')`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("old tables remain: %d %v", count, err)
			}
			if err := db.QueryRow(`SELECT schema_version FROM billing_settings`).Scan(&version); err != nil || version != stateVersion {
				t.Fatalf("schema version=%d %v", version, err)
			}
			if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('model_aggregates') WHERE name IN ('reasoning_tokens','aggregate_key')`).Scan(&count); err != nil || count != 0 {
				t.Fatal("model columns not simplified")
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = NewStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			again, err := s.KeyBalances()
			if err != nil || !reflect.DeepEqual(items, again) {
				t.Fatalf("repeated migration changed accounts: %v", err)
			}
			// Account display reads the table directly, even when an in-memory cache is stale.
			if _, err := db.Exec(`UPDATE api_key_accounts SET requests=101 WHERE api_key_id='overlap'`); err != nil {
				t.Fatal(err)
			}
			again, err = s.KeyBalances()
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range again {
				if item.APIKeyID == "overlap" && item.Requests != 101 {
					t.Fatal("account page did not read table")
				}
			}
		})
	}
}

func TestPageMigrationRollsBackAllTablesOnDuplicateScope(t *testing.T) {
	dir, db := legacyPageDatabase(t, true)
	if _, err := db.Exec(`UPDATE api_key_balances SET caller_scope=? WHERE api_key_id='zero'`, CallerScope("test-key")); err != nil {
		t.Fatal(err)
	}
	if s, err := NewStore(dir); err == nil {
		s.Close()
		t.Fatal("conflicting scope accepted")
	}
	var version, count int
	if err := db.QueryRow(`SELECT schema_version FROM billing_settings`).Scan(&version); err != nil || version != 4 {
		t.Fatalf("version changed: %d %v", version, err)
	}
	for _, table := range []string{"api_key_balances", "api_key_balance_notes", "api_key_aggregates"} {
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("lost %s: %v", table, err)
		}
	}
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='api_key_accounts'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial account migration survived")
	}
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('model_aggregates') WHERE name='aggregate_key'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("model migration not rolled back")
	}
}

func TestFuturePageSchemaIsRejectedWithoutChanges(t *testing.T) {
	dir, db := legacyPageDatabase(t, true)
	if _, err := db.Exec(`UPDATE billing_settings SET schema_version=999`); err != nil {
		t.Fatal(err)
	}
	if s, err := NewStore(dir); err == nil {
		s.Close()
		t.Fatal("future schema accepted")
	} else if !strings.Contains(err.Error(), "unsupported") {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM api_key_balances`).Scan(&count); err != nil || count != 2 {
		t.Fatal("future schema was modified")
	}
}

func TestAccountStatisticsResetPreservesFundsAndNotes(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	key := "test-key"
	id := APIKeyIdentifier(key)
	if err := s.SetKeyBalances([]APIKeyBalance{{APIKeyID: id, APIKey: key, Balance: 10}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKeyBalanceNotes([]APIKeyBalance{{APIKeyID: id, APIKey: key, Note: "保留"}}); err != nil {
		t.Fatal(err)
	}
	use := UsageRecord{Provider: "test", Model: "m", APIKey: key, Cost: 2, CostProvided: true, RequestedAt: time.Now()}
	if err := s.HandleUsage(use); err != nil {
		t.Fatal(err)
	}
	before, err := s.KeyBalances()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	after, err := s.KeyBalances()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].Balance != 8 || after[0].Note != "保留" || after[0].Requests != 0 || after[0].Cost != 0 || after[0].BalanceVersion != before[0].BalanceVersion {
		t.Fatalf("reset account=%+v", after)
	}
	if err := s.HandleUsage(use); err != nil {
		t.Fatal(err)
	}
	after, err = s.KeyBalances()
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Requests != 1 || after[0].Cost != 2 || after[0].Balance != 6 {
		t.Fatalf("usage after reset=%+v", after)
	}
}

func TestModelPageIdentityKeepsProviderAndModelSeparate(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []UsageRecord{
		{Provider: "a/b", Model: "c", Cost: 1, CostProvided: true},
		{Provider: "a", Model: "b/c", Cost: 2, CostProvided: true},
		{Provider: " A/B ", Model: "C", Cost: 3, CostProvided: true},
	} {
		if err := s.HandleUsage(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sum := s.Summary()
	if len(sum.Models) != 2 || sum.Totals.Requests != 3 || sum.Totals.Cost != 6 {
		t.Fatalf("model identities=%+v", sum)
	}
	if sum.Models[0].Provider != "a/b" || sum.Models[0].Requests != 2 || sum.Models[0].Cost != 4 {
		t.Fatalf("case-insensitive model=%+v", sum.Models[0])
	}
}
