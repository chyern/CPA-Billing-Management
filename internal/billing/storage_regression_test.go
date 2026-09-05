package billing

import (
	"database/sql"
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func mustUsage(t *testing.T, s *Store, r UsageRecord) {
	t.Helper()
	if err := s.HandleUsage(r); err != nil {
		t.Fatal(err)
	}
}
func stateJSON(t *testing.T, s *Store) string {
	t.Helper()
	b, err := json.Marshal(s.state)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUsageTransactionFailurePreservesMemoryAndBalance(t *testing.T) {
	s := testStore(t)
	key := "sk-atomic-test"
	if err := s.SetKeyBalances([]APIKeyBalance{{APIKeyID: APIKeyIdentifier(key), APIKey: key, Balance: 10}}); err != nil {
		t.Fatal(err)
	}
	mustUsage(t, s, UsageRecord{Model: "known", APIKey: key, Cost: 1, CostProvided: true})
	before := stateJSON(t, s)
	if _, err := s.db.Exec(`CREATE TRIGGER fail_balance_debit BEFORE UPDATE ON api_key_balances BEGIN SELECT RAISE(ABORT, 'forced debit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.HandleUsage(UsageRecord{Model: "known", APIKey: key, Cost: 2, CostProvided: true, Currency: "CNY"}); err == nil {
		t.Fatal("expected persistence failure")
	}
	if got := stateJSON(t, s); got != before {
		t.Fatal("failed usage changed in-memory state")
	}
	var events int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM usage_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("failed transaction left %d events", events)
	}
	balance, configured, err := s.BalanceForCallerScope(CallerScope(key))
	if err != nil || !configured || balance != 9 {
		t.Fatalf("balance=%v configured=%v err=%v", balance, configured, err)
	}
	if _, err := s.db.Exec(`DROP TRIGGER fail_balance_debit`); err != nil {
		t.Fatal(err)
	}
	mustUsage(t, s, UsageRecord{Model: "known", APIKey: key, Cost: 2, CostProvided: true})
	if s.Summary().Totals.Requests != 2 || s.Summary().Totals.Cost != 3 {
		t.Fatal("retry did not record exactly one event")
	}
}

func TestUsageRejectsNonFiniteCostsAndOverflow(t *testing.T) {
	s := testStore(t)
	before := stateJSON(t, s)
	for _, cost := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := s.HandleUsage(UsageRecord{Cost: cost, CostProvided: true}); err == nil {
			t.Fatalf("accepted %v", cost)
		}
		if stateJSON(t, s) != before {
			t.Fatal("rejected cost changed state")
		}
	}
	if err := s.SetRules([]PriceRule{{Match: "overflow", InputPerMillion: math.MaxFloat64}}); err != nil {
		t.Fatal(err)
	}
	before = stateJSON(t, s)
	if err := s.HandleUsage(UsageRecord{Model: "overflow", InputTokens: 2_000_000}); err == nil {
		t.Fatal("accepted computed infinite cost")
	}
	if stateJSON(t, s) != before {
		t.Fatal("overflow changed state")
	}
	mustUsage(t, s, UsageRecord{Model: "aggregate", Cost: math.MaxFloat64, CostProvided: true})
	before = stateJSON(t, s)
	if err := s.HandleUsage(UsageRecord{Model: "aggregate", Cost: math.MaxFloat64, CostProvided: true}); err == nil {
		t.Fatal("accepted overflowing aggregate")
	}
	if stateJSON(t, s) != before {
		t.Fatal("aggregate overflow changed state")
	}
}

func TestSummaryRetainsHistoryBeyondEventCache(t *testing.T) {
	s := testStore(t)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	// Seed a full persistent history efficiently, then use the real ingestion
	// path across the cache boundary. The oldest 17 events precede the cache.
	state := emptyState()
	state.Events = make([]UsageEvent, maxCachedEvents+17)
	for i := range state.Events {
		stamp := start
		if i >= 17 {
			stamp = start.AddDate(0, 0, 1)
		}
		state.Events[i] = UsageEvent{RequestedAt: stamp, Provider: "test", Model: "history", APIKey: "masked", APIKeyID: "history-id", InputTokens: 1, TotalTokens: 1, Cost: 1, PricedBy: "upstream", Priced: true, Failed: i%100 == 0}
	}
	models, keys, _, _ := summarizeEvents(state.Events)
	state.Aggregates[aggregateKey("test", "history")] = models[0]
	state.APIKeyAggregates["history-id"] = keys[0]
	if err := ReplaceState(s.db, state); err != nil {
		t.Fatal(err)
	}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	if len(s.state.Events) != maxCachedEvents {
		t.Fatalf("cache=%d", len(s.state.Events))
	}
	mustUsage(t, s, UsageRecord{RequestedAt: start.AddDate(0, 0, 1), Provider: "test", Model: "history", Cost: 1, CostProvided: true})
	total := maxCachedEvents + 18
	all, err := s.SummaryPageRangeStatus(999, 100, time.Time{}, time.Time{}, "all")
	if err != nil {
		t.Fatal(err)
	}
	if all.Totals.Requests != int64(total) || all.Totals.Cost != float64(total) || all.RecentEventsTotal != total || len(all.RecentEvents) != 18 {
		t.Fatalf("all-time summary=%+v", all)
	}
	first, err := s.SummaryPageRangeStatus(1, 100, start, start.AddDate(0, 0, 1), "all")
	if err != nil {
		t.Fatal(err)
	}
	if first.Totals.Requests != 17 || first.Totals.Cost != 17 || len(first.APIKeys) != 1 || first.APIKeys[0].Requests != 17 {
		t.Fatalf("old date lost: %+v", first)
	}
	var firstID int64
	if err := s.db.QueryRow(`SELECT MIN(id) FROM usage_events`).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRules([]PriceRule{{Match: "history", InputPerMillion: 500}}); err != nil {
		t.Fatal(err)
	}
	var count int
	var minID int64
	if err := s.db.QueryRow(`SELECT COUNT(*), MIN(id) FROM usage_events`).Scan(&count, &minID); err != nil {
		t.Fatal(err)
	}
	if count != total || minID != firstID {
		t.Fatalf("SetRules rewrote history: count=%d min=%d", count, minID)
	}
	// Reopening must keep bounded memory while full SQL history remains queryable.
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	if len(s.state.Events) != maxCachedEvents || s.Summary().Totals.Requests != int64(total) {
		t.Fatal("reload truncated history totals")
	}
}

func TestSummaryStatusFiltersBeforePaginationAndKeepsDateTotals(t *testing.T) {
	s := testStore(t)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 45; i++ {
		mustUsage(t, s, UsageRecord{Model: "status", RequestedAt: start.Add(time.Duration(i) * time.Nanosecond), Failed: i%10 == 0, Cost: 1, CostProvided: true})
	}
	summary, err := s.SummaryPageRangeStatus(2, 2, start, start.AddDate(0, 0, 1), "failed")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Totals.Requests != 45 || summary.Totals.FailedRequests != 5 || summary.RecentEventsTotal != 5 || summary.RecentEventsPages != 3 || len(summary.RecentEvents) != 2 {
		t.Fatalf("summary=%+v", summary)
	}
	for _, event := range summary.RecentEvents {
		if !event.Failed {
			t.Fatal("status page included success")
		}
	}
	success, err := s.SummaryPageRangeStatus(1, 100, start, start.AddDate(0, 0, 1), "success")
	if err != nil {
		t.Fatal(err)
	}
	if success.RecentEventsTotal != 40 || success.Totals.Cost != 45 {
		t.Fatalf("success summary=%+v", success)
	}
	if _, err := s.SummaryPageRangeStatus(1, 20, start, time.Time{}, "garbage"); err == nil {
		t.Fatal("invalid status accepted")
	}
	// An end boundary at an exact nanosecond is exclusive, including whole-second events correctly.
	narrow, err := s.SummaryPageRangeStatus(1, 100, start, start.Add(2*time.Nanosecond), "all")
	if err != nil {
		t.Fatal(err)
	}
	if narrow.Totals.Requests != 2 {
		t.Fatalf("fractional date boundary requests=%d", narrow.Totals.Requests)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SummaryPageRangeStatus(1, 20, time.Time{}, time.Time{}, "all"); err == nil {
		t.Fatal("closed database query hid error")
	}
}

func TestWildcardPricingPersistsIncludingZeroTokenUsage(t *testing.T) {
	s := testStore(t)
	if err := s.SetRules([]PriceRule{{Match: "*", InputPerMillion: 2}}); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	mustUsage(t, s, UsageRecord{Model: "wildcard", RequestedAt: day, InputTokens: 1_000_000})
	mustUsage(t, s, UsageRecord{Model: "wildcard-zero", RequestedAt: day})
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	sum, err := s.SummaryPageRangeStatus(1, 20, day, day.AddDate(0, 0, 1), "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.UnpricedModels) != 0 || sum.Totals.Cost != 2 {
		t.Fatalf("wildcard summary=%+v", sum)
	}
	for _, event := range sum.RecentEvents {
		if !event.Priced {
			t.Fatal("wildcard event not priced")
		}
	}
}

func TestStoreRequiresCurrentSchemaColumns(t *testing.T) {
	for _, tc := range []struct {
		table, column, prepare string
	}{
		{"usage_events", "priced", ""},
		{"api_key_balances", "caller_scope", "DROP INDEX idx_api_key_balances_caller_scope"},
	} {
		t.Run(tc.table+"."+tc.column, func(t *testing.T) {
			dataDir := t.TempDir()
			s, err := NewStore(dataDir)
			if err != nil {
				t.Fatal(err)
			}
			if tc.prepare != "" {
				if _, err := s.db.Exec(tc.prepare); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.db.Exec("ALTER TABLE " + tc.table + " DROP COLUMN " + tc.column); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			if reopened, err := NewStore(dataDir); err == nil {
				_ = reopened.Close()
				t.Fatal("opened database with missing current schema column")
			}
			db, err := sql.Open("sqlite3", filepath.Join(dataDir, "billing.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var columns int
			if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", tc.table, tc.column).Scan(&columns); err != nil {
				t.Fatal(err)
			}
			if columns != 0 {
				t.Fatal("opening a database added a missing schema column")
			}
		})
	}
}

func TestFailedRulesResetAndConfigurationKeepMemory(t *testing.T) {
	s := testStore(t)
	if err := s.SetRules([]PriceRule{{Match: "old", InputPerMillion: 1}}); err != nil {
		t.Fatal(err)
	}
	mustUsage(t, s, UsageRecord{Model: "old", InputTokens: 1_000_000})
	for _, tc := range []struct {
		name, trigger, drop string
		run                 func() error
	}{
		{"rules", `CREATE TRIGGER fail_rules BEFORE INSERT ON pricing_rules BEGIN SELECT RAISE(ABORT,'forced'); END`, `DROP TRIGGER fail_rules`, func() error { return s.SetRules([]PriceRule{{Match: "new", InputPerMillion: 2}}) }},
		{"reset", `CREATE TRIGGER fail_reset BEFORE DELETE ON usage_events BEGIN SELECT RAISE(ABORT,'forced'); END`, `DROP TRIGGER fail_reset`, s.Reset},
		{"config", `CREATE TRIGGER fail_settings BEFORE UPDATE ON billing_settings BEGIN SELECT RAISE(ABORT,'forced'); END`, `DROP TRIGGER fail_settings`, func() error { return s.ConfigureYAML([]byte("currency: CNY")) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := stateJSON(t, s)
			if _, err := s.db.Exec(tc.trigger); err != nil {
				t.Fatal(err)
			}
			if err := tc.run(); err == nil {
				t.Fatal("expected failure")
			}
			if before != stateJSON(t, s) {
				t.Fatal("failed transaction changed memory")
			}
			if _, err := s.db.Exec(tc.drop); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUsageQueriesUseDateAndStatusIndexes(t *testing.T) {
	s := testStore(t)
	for _, tc := range []struct {
		name, query, index string
		args               []any
	}{
		{"date", `EXPLAIN QUERY PLAN SELECT COUNT(*) FROM usage_events WHERE rtrim(requested_at,'Z') >= ? AND rtrim(requested_at,'Z') < ?`, "idx_usage_events_date_range", []any{"2026-09-01T00:00:00", "2026-09-02T00:00:00"}},
		{"status", `EXPLAIN QUERY PLAN SELECT id FROM usage_events WHERE failed = ? ORDER BY id DESC LIMIT ? OFFSET ?`, "idx_usage_events_status", []any{1, 20, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := s.db.Query(tc.query, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var plans []string
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				plans = append(plans, detail)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(plans, "\n"), tc.index) {
				t.Fatalf("expected %s, got plan %v", tc.index, plans)
			}
		})
	}
}
