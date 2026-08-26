package main

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/chyern/CPA-Billing-Management/internal/billing"
	_ "github.com/mattn/go-sqlite3"
)

func TestMigrateLegacySnapshotToNormalizedTables(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "billing.db")
	db, err := sql.Open("sqlite3", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE billing_state (id INTEGER PRIMARY KEY, state_json BLOB NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	legacy := billing.State{
		Version: 3, Currency: "USD",
		Rules: []billing.PriceRule{{Match: "codex/gpt-test", InputPerMillion: 2, OutputPerMillion: 4}},
		Events: []billing.UsageEvent{{
			RequestedAt: time.Now().UTC(), Provider: "codex", Model: "gpt-test", APIKey: "sk-t••••••-key",
			APIKeyID: "legacy:sk-t••••••-key", InputTokens: 100, OutputTokens: 20, TotalTokens: 120,
			Cost: 0.00028, PricedBy: "codex/gpt-test",
		}},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO billing_state (id, state_json, updated_at) VALUES (1, ?, ?)`, raw, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := migrate(databasePath, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.rules != 1 || result.events != 1 || result.models != 1 || result.apiKeys != 1 {
		t.Fatalf("migration result = %+v", result)
	}

	store, err := billing.NewStore(filepath.Dir(databasePath))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if rules := store.Rules(); len(rules) != 1 || rules[0].Match != "codex/gpt-test" {
		t.Fatalf("migrated rules = %+v", rules)
	}
	summary := store.Summary()
	if summary.Totals.Requests != 1 || len(summary.RecentEvents) != 1 || summary.RecentEvents[0].Model != "gpt-test" {
		t.Fatalf("migrated summary = %+v", summary)
	}

	verificationDB, err := sql.Open("sqlite3", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer verificationDB.Close()
	var legacyTableCount int
	if err := verificationDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'billing_state'`).Scan(&legacyTableCount); err != nil {
		t.Fatal(err)
	}
	if legacyTableCount != 0 {
		t.Fatal("legacy billing_state table was not removed")
	}
}
