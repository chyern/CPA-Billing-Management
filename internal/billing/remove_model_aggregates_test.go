package billing

import (
	"reflect"
	"testing"
	"time"
)

func TestRemoveModelAggregatesUsesOnlySnapshots(t *testing.T) {
	s := testStore(t)
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	mustUsage(t, s, UsageRecord{RequestedAt: day, Model: "m", APIKey: "key", InputTokens: 10, OutputTokens: 2, TotalTokens: 999, Cost: 3, CostProvided: true})
	mustUsage(t, s, UsageRecord{RequestedAt: day, Provider: "codex", Model: "m", APIKey: "key", InputTokens: 20, OutputTokens: 4, Cost: 5, CostProvided: true, Failed: true})
	before := s.Summary()
	accounts, err := s.KeyBalances()
	if err != nil {
		t.Fatal(err)
	}
	// A stale v5 aggregate must not influence totals or fill missing providers.
	if _, err := s.db.Exec(`UPDATE billing_settings SET schema_version=5;
 CREATE TABLE model_aggregates (provider TEXT,model TEXT,requests INTEGER,total_tokens INTEGER,cost REAL,priced INTEGER);
 INSERT INTO model_aggregates VALUES ('guessed-provider','m',900,9000,900,0);
 CREATE TRIGGER forbid_snapshot_update BEFORE UPDATE ON usage_events BEGIN SELECT RAISE(ABORT,'snapshot changed'); END;
 CREATE TRIGGER forbid_snapshot_delete BEFORE DELETE ON usage_events BEGIN SELECT RAISE(ABORT,'snapshot deleted'); END;`); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(s.db); err != nil {
		t.Fatal(err)
	}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	all := s.Summary()
	ranged, err := s.SummaryPageRangeStatus(1, 20, day, day.AddDate(0, 0, 1), "all")
	if err != nil {
		t.Fatal(err)
	}
	if all.Totals.Requests != 2 || all.Totals.TotalTokens != 36 || all.Totals.Cost != 8 || all.Totals.FailedRequests != 1 {
		t.Fatalf("totals=%+v", all.Totals)
	}
	if !reflect.DeepEqual(all.Models, ranged.Models) || all.Totals != ranged.Totals {
		t.Fatal("all-time and date query disagree")
	}
	if len(all.Models) != 2 || all.Models[1].Provider != "" {
		t.Fatalf("provider inferred: %+v", all.Models)
	}
	if !reflect.DeepEqual(before.RecentEvents, all.RecentEvents) {
		t.Fatal("migration changed snapshot")
	}
	after, err := s.KeyBalances()
	if err != nil || !reflect.DeepEqual(accounts, after) {
		t.Fatal("migration changed accounts")
	}
	mustUsage(t, s, UsageRecord{RequestedAt: day, Model: "m", APIKey: "key", Cost: 1, CostProvided: true})
	if err := EnsureSchema(s.db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='model_aggregates'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("model table recreated")
	}
	if s.Summary().Totals.Requests != 3 {
		t.Fatal("new usage missing from summary")
	}
}

func TestRemoveModelAggregatesRollsBackWithVersionUpdate(t *testing.T) {
	s := testStore(t)
	if _, err := s.db.Exec(`UPDATE billing_settings SET schema_version=5;
 CREATE TABLE model_aggregates (requests INTEGER);
 INSERT INTO model_aggregates VALUES (7);
 CREATE TRIGGER fail_version BEFORE UPDATE ON billing_settings BEGIN SELECT RAISE(ABORT,'forced migration failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(s.db); err == nil {
		t.Fatal("migration unexpectedly succeeded")
	}
	var version, requests int
	if err := s.db.QueryRow(`SELECT schema_version FROM billing_settings`).Scan(&version); err != nil || version != 5 {
		t.Fatal("version not rolled back")
	}
	if err := s.db.QueryRow(`SELECT requests FROM model_aggregates`).Scan(&requests); err != nil || requests != 7 {
		t.Fatal("table drop not rolled back")
	}
}
