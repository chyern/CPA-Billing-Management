package billing

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStorePersistsUpstreamUsageCost(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	const apiKey = "sk-test-sensitive-key"
	store.HandleUsage(UsageRecord{
		Provider: "codex", Model: "gpt-5.5", APIKey: apiKey, RequestedAt: time.Now(),
		Latency: 1500 * time.Millisecond, TTFT: 250 * time.Millisecond,
		InputTokens: 1_000_000, OutputTokens: 2_000_000, TotalTokens: 3_000_000,
		Cost: 4.25, CostProvided: true, Currency: "USD",
	})
	summary := store.Summary()
	if summary.Totals.Requests != 1 || math.Abs(summary.Totals.Cost-4.25) > 1e-12 {
		t.Fatalf("summary = %+v, want one request and upstream cost 4.25", summary.Totals)
	}
	if len(summary.Models) != 1 || summary.Models[0].Requests != 1 || summary.Models[0].InputTokens != 1_000_000 {
		t.Fatalf("model aggregate = %+v, want persisted model totals", summary.Models)
	}
	if len(summary.APIKeys) != 1 || summary.APIKeys[0].Requests != 1 || summary.APIKeys[0].InputTokens != 1_000_000 {
		t.Fatalf("api key aggregate = %+v, want persisted API key totals", summary.APIKeys)
	}
	if len(summary.RecentEvents) != 1 {
		t.Fatalf("recent events = %d, want 1", len(summary.RecentEvents))
	}
	event := summary.RecentEvents[0]
	if event.APIKey != "sk-t••••••-key" || event.LatencyNanos != int64(1500*time.Millisecond) || event.TTFTNanos != int64(250*time.Millisecond) {
		t.Fatalf("event identity and timing = %+v", event)
	}
	raw := persistedDatabaseText(t, store)
	if strings.Contains(raw, apiKey) {
		t.Fatal("persistent billing state must not contain the complete API key")
	}
	var apiKeyAggregateCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM api_key_aggregates`).Scan(&apiKeyAggregateCount); err != nil {
		t.Fatal(err)
	}
	if apiKeyAggregateCount != 1 {
		t.Fatalf("persisted API key aggregates = %d, want 1", apiKeyAggregateCount)
	}
	reloaded, err := NewStore(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Summary().Totals.Cost; math.Abs(got-4.25) > 1e-12 {
		t.Fatalf("reloaded cost = %v, want 4.25", got)
	}
	reloadedSummary := reloaded.Summary()
	if len(reloadedSummary.Models) != 1 || reloadedSummary.Models[0].InputTokens != 1_000_000 {
		t.Fatalf("reloaded model aggregate = %+v, want persisted model totals", reloadedSummary.Models)
	}
	if len(reloadedSummary.APIKeys) != 1 || reloadedSummary.APIKeys[0].InputTokens != 1_000_000 {
		t.Fatalf("reloaded API key aggregate = %+v, want persisted API key totals", reloadedSummary.APIKeys)
	}
	if err := reloaded.SetRules([]PriceRule{{Match: "gpt-5.5", InputPerMillion: 99, OutputPerMillion: 99}}); err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Summary().Totals.Cost; math.Abs(got-4.25) > 1e-12 {
		t.Fatalf("upstream cost after model price change = %v, want 4.25", got)
	}
}

func TestSetRulesRejectsNonFinitePrices(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -0.1} {
		err := store.SetRules([]PriceRule{{Match: "example", InputPerMillion: value}})
		if err == nil {
			t.Fatalf("SetRules accepted invalid price %v", value)
		}
	}
}

func TestStoreStartsWithoutDefaultPricingRule(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	if rules := store.Rules(); len(rules) != 0 {
		t.Fatalf("default rules = %+v, want no default rule", rules)
	}
	if err := store.SetRules(nil); err != nil {
		t.Fatalf("clearing pricing rules: %v", err)
	}
}

func TestConfigureYAMLIgnoresManagementKey(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.ConfigureYAML([]byte("currency: CNY\nmanagement_key: test-management-secret\n"))
	if got := store.Currency(); got != "CNY" {
		t.Fatalf("currency = %q, want CNY", got)
	}
	raw := persistedDatabaseText(t, store)
	if strings.Contains(raw, "test-management-secret") || strings.Contains(raw, "management_key") {
		t.Fatal("management key must not be persisted or configured by billing state")
	}
}

func persistedDatabaseText(t *testing.T, store *Store) string {
	t.Helper()
	queries := []string{
		`SELECT currency FROM billing_settings`,
		`SELECT match FROM pricing_rules`,
		`SELECT provider || ' ' || model || ' ' || alias || ' ' || api_key || ' ' || api_key_id || ' ' || auth_type || ' ' || source || ' ' || priced_by FROM usage_events`,
		`SELECT provider || ' ' || model FROM model_aggregates`,
		`SELECT api_key FROM api_key_aggregates`,
	}
	var values []string
	for _, query := range queries {
		rows, err := store.db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				_ = rows.Close()
				t.Fatal(err)
			}
			values = append(values, value)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return strings.Join(values, "\n")
}

func TestStoreUsesNormalizedSQLiteTables(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"billing_settings", "pricing_rules", "usage_events", "model_aggregates", "api_key_aggregates"} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("normalized table %s is missing", table)
		}
	}
}

func TestCalculateCostAndModelPrice(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetRules([]PriceRule{{Match: "gpt-test", InputPerMillion: 2, OutputPerMillion: 4, CacheReadPerMillion: 0.5, CacheCreationPerMillion: 1}}); err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(UsageRecord{
		Provider: "codex", Model: "gpt-test", InputTokens: 1_000_000, OutputTokens: 500_000,
		CachedTokens: 250_000, CacheReadTokens: 200_000, CacheCreationTokens: 50_000,
	})
	want := 750_000.0/1_000_000*2 + 500_000.0/1_000_000*4 + 200_000.0/1_000_000*0.5 + 50_000.0/1_000_000
	summary := store.Summary()
	if math.Abs(summary.Totals.Cost-want) > 1e-12 || len(summary.Models) != 1 || !summary.Models[0].Priced {
		t.Fatalf("model-price summary = %+v, want cost %v", summary, want)
	}
}

func TestModelPriceIgnoresProvider(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetRules([]PriceRule{{Match: "gpt-test", InputPerMillion: 3}}); err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(UsageRecord{Provider: "codex", Model: "gpt-test", InputTokens: 1_000_000})
	if got := store.Summary().Totals.Cost; got != 3 {
		t.Fatalf("model-only price = %v, want 3", got)
	}

	if err := store.SetRules([]PriceRule{{Match: "openai/gpt-test", InputPerMillion: 4}}); err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(UsageRecord{Provider: "codex", Model: "gpt-test", InputTokens: 1_000_000})
	if got := store.Summary().Totals.Cost; got != 7 {
		t.Fatalf("provider/model rule should match its model segment, cost = %v, want 7", got)
	}
}

func TestMaskAPIKey(t *testing.T) {
	tests := map[string]string{
		"":                      "",
		"ab":                    "••",
		"abcd":                  "a••d",
		"abcdefgh":              "a••••••h",
		"sk-test-sensitive-key": "sk-t••••••-key",
	}
	for input, want := range tests {
		if got := MaskAPIKey(input); got != want {
			t.Errorf("MaskAPIKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSummaryAggregatesByAPIKeyAcrossAllEvents(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keyA := "sk-alpha-sensitive-one"
	keyB := "sk-beta-sensitive-two"
	store.HandleUsage(UsageRecord{APIKey: keyA, InputTokens: 10, OutputTokens: 2, TotalTokens: 12, Cost: 1, CostProvided: true})
	store.HandleUsage(UsageRecord{APIKey: keyB, InputTokens: 20, OutputTokens: 3, TotalTokens: 23, Cost: 2, CostProvided: true})
	store.HandleUsage(UsageRecord{APIKey: keyA, InputTokens: 30, OutputTokens: 4, TotalTokens: 34, Cost: 3, CostProvided: true, Failed: true})
	store.HandleUsage(UsageRecord{InputTokens: 40, TotalTokens: 40})

	summary := store.SummaryPage(1, 1)
	if len(summary.APIKeys) != 3 {
		t.Fatalf("api key aggregates = %d, want 3: %+v", len(summary.APIKeys), summary.APIKeys)
	}
	first := summary.APIKeys[0]
	if first.APIKey != MaskAPIKey(keyA) || first.Requests != 2 || first.FailedRequests != 1 || first.InputTokens != 40 || first.OutputTokens != 6 || first.TotalTokens != 46 || first.Cost != 4 {
		t.Fatalf("first api key aggregate = %+v", first)
	}
	if summary.APIKeys[1].Requests != 1 || summary.APIKeys[2].Requests != 1 {
		t.Fatalf("remaining api key aggregates = %+v", summary.APIKeys[1:])
	}
}

func TestMaskSensitiveSource(t *testing.T) {
	if got := MaskSensitiveSource("codex"); got != "codex" {
		t.Fatalf("ordinary source = %q, want codex", got)
	}
	if got := MaskSensitiveSource("sk-test-sensitive-key"); got != "sk-t••••••-key" {
		t.Fatalf("secret source = %q, want masked value", got)
	}
}

func TestMissingUpstreamCostIsMarkedUnpriced(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(UsageRecord{Provider: "unknown", Model: "new-model", InputTokens: 100, OutputTokens: 200, TotalTokens: 300})
	summary := store.Summary()
	if len(summary.UnpricedModels) != 1 || summary.UnpricedModels[0] != "new-model" {
		t.Fatalf("unpriced models = %v", summary.UnpricedModels)
	}
	if summary.Totals.Cost != 0 {
		t.Fatalf("unknown model cost = %v, want 0", summary.Totals.Cost)
	}
}

func TestSetRulesDoesNotRecalculateHistoricalEvents(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(UsageRecord{Provider: "codex", Model: "future-model", InputTokens: 1_000_000, TotalTokens: 1_000_000})
	store.HandleUsage(UsageRecord{Provider: "codex", Model: "billed-model", InputTokens: 1_000_000, Cost: 2.5, CostProvided: true})
	if err := store.SetRules([]PriceRule{{Match: "future-model", InputPerMillion: 7}}); err != nil {
		t.Fatal(err)
	}
	if got := store.Summary().Totals.Cost; math.Abs(got-2.5) > 1e-12 {
		t.Fatalf("historical mixed cost changed after price update = %v, want 2.5", got)
	}
	store.HandleUsage(UsageRecord{Provider: "codex", Model: "future-model", InputTokens: 1_000_000, TotalTokens: 1_000_000})
	if got := store.Summary().Totals.Cost; math.Abs(got-9.5) > 1e-12 {
		t.Fatalf("new event did not use updated price = %v, want 9.5 including historical 2.5", got)
	}
}

func TestSummaryPagePaginatesRecentEventsNewestFirstByPage(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 45; i++ {
		store.HandleUsage(UsageRecord{Provider: "test", Model: fmt.Sprintf("event-%02d", i)})
	}

	first := store.SummaryPage(1, 20)
	if first.RecentEventsTotal != 45 || first.RecentEventsPages != 3 || first.RecentEventsPage != 1 || first.RecentEventsPageSize != 20 {
		t.Fatalf("first page metadata = total %d pages %d page %d size %d", first.RecentEventsTotal, first.RecentEventsPages, first.RecentEventsPage, first.RecentEventsPageSize)
	}
	if len(first.RecentEvents) != 20 || first.RecentEvents[0].Model != "event-25" || first.RecentEvents[19].Model != "event-44" {
		t.Fatalf("first page events = %d, first %q, last %q", len(first.RecentEvents), first.RecentEvents[0].Model, first.RecentEvents[len(first.RecentEvents)-1].Model)
	}

	last := store.SummaryPage(99, 20)
	if last.RecentEventsPage != 3 || len(last.RecentEvents) != 5 || last.RecentEvents[0].Model != "event-00" || last.RecentEvents[4].Model != "event-04" {
		t.Fatalf("clamped last page = page %d, events %+v", last.RecentEventsPage, last.RecentEvents)
	}
}

func TestSummaryPageRangeFiltersEventsAndAggregates(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(UsageRecord{Provider: "test", Model: "yesterday", RequestedAt: time.Date(2026, 9, 2, 23, 59, 0, 0, time.Local), Cost: 1, CostProvided: true})
	store.HandleUsage(UsageRecord{Provider: "test", Model: "today", RequestedAt: time.Date(2026, 9, 3, 8, 0, 0, 0, time.Local), Cost: 2, CostProvided: true})
	start := time.Date(2026, 9, 3, 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 0, 1)
	summary := store.SummaryPageRange(1, 20, start, end)
	if summary.Totals.Requests != 1 || summary.Totals.Cost != 2 || summary.RecentEventsTotal != 1 || len(summary.Models) != 1 || summary.Models[0].Model != "today" {
		t.Fatalf("filtered summary = %+v", summary)
	}
}

func TestAPIKeyBalanceIsPersistedAndDecrementedByUsageCost(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	key := "sk-balance-test"
	id := APIKeyIdentifier(key)
	callerScope := CallerScope(key)
	if err := store.SetKeyBalances([]APIKeyBalance{{APIKeyID: id, APIKey: key, Balance: 10}}); err != nil {
		t.Fatal(err)
	}
	if balance, configured, err := store.BalanceForCallerScope(callerScope); err != nil || !configured || balance != 10 {
		t.Fatalf("balance lookup = %v, %v, %v; want 10, true, nil", balance, configured, err)
	}
	store.HandleUsage(UsageRecord{APIKey: key, Model: "priced-upstream", Cost: 2.5, CostProvided: true})
	balances, err := store.KeyBalances()
	if err != nil {
		t.Fatal(err)
	}
	if len(balances) != 1 || !balances[0].Configured || balances[0].CallerScope != callerScope || balances[0].Balance != 7.5 || balances[0].Cost != 2.5 || balances[0].APIKey != MaskAPIKey(key) {
		t.Fatalf("balances after usage = %+v", balances)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })
	balances, err = reloaded.KeyBalances()
	if err != nil {
		t.Fatal(err)
	}
	if len(balances) != 1 || balances[0].Balance != 7.5 {
		t.Fatalf("reloaded balances = %+v", balances)
	}
	if balance, configured, err := reloaded.BalanceForCallerScope(callerScope); err != nil || !configured || balance != 7.5 {
		t.Fatalf("reloaded balance lookup = %v, %v, %v; want 7.5, true, nil", balance, configured, err)
	}
}

func TestAPIKeyBalanceNotePersistsWithoutEnablingBalanceTracking(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	key := "sk-note-only"
	id := APIKeyIdentifier(key)
	if err := store.SetKeyBalanceNotes([]APIKeyBalance{{APIKeyID: id, APIKey: key, Note: "内部测试账号"}}); err != nil {
		t.Fatal(err)
	}
	balances, err := store.KeyBalances()
	if err != nil {
		t.Fatal(err)
	}
	if len(balances) != 1 || balances[0].Configured || balances[0].Note != "内部测试账号" || balances[0].APIKey != MaskAPIKey(key) {
		t.Fatalf("note-only balance row = %+v", balances)
	}
	if _, configured, err := store.BalanceForCallerScope(CallerScope(key)); err != nil || configured {
		t.Fatalf("note-only balance lookup configured = %v, err = %v; want false, nil", configured, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })
	balances, err = reloaded.KeyBalances()
	if err != nil {
		t.Fatal(err)
	}
	if len(balances) != 1 || balances[0].Note != "内部测试账号" || balances[0].Configured {
		t.Fatalf("reloaded note-only balance row = %+v", balances)
	}
}
