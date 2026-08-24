package billing

import (
	"fmt"
	"math"
	"os"
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
	if len(summary.RecentEvents) != 1 {
		t.Fatalf("recent events = %d, want 1", len(summary.RecentEvents))
	}
	event := summary.RecentEvents[0]
	if event.APIKey != "sk-t••••••-key" || event.LatencyNanos != int64(1500*time.Millisecond) || event.TTFTNanos != int64(250*time.Millisecond) {
		t.Fatalf("event identity and timing = %+v", event)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "data", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), apiKey) {
		t.Fatal("persistent billing state must not contain the complete API key")
	}
	reloaded, err := NewStore(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Summary().Totals.Cost; math.Abs(got-4.25) > 1e-12 {
		t.Fatalf("reloaded cost = %v, want 4.25", got)
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
	raw, err := os.ReadFile(filepath.Join(store.dataDir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "test-management-secret") || strings.Contains(string(raw), "management_key") {
		t.Fatal("management key must not be persisted or configured by billing state")
	}
}

func TestCalculateCostAndModelPriceFallback(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetRules([]PriceRule{{Match: "codex/gpt-test", InputPerMillion: 2, OutputPerMillion: 4, CacheReadPerMillion: 0.5, CacheCreationPerMillion: 1}}); err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(UsageRecord{
		Provider: "codex", Model: "gpt-test", InputTokens: 1_000_000, OutputTokens: 500_000,
		CachedTokens: 250_000, CacheReadTokens: 200_000, CacheCreationTokens: 50_000,
	})
	want := 750_000.0/1_000_000*2 + 500_000.0/1_000_000*4 + 200_000.0/1_000_000*0.5 + 50_000.0/1_000_000
	summary := store.Summary()
	if math.Abs(summary.Totals.Cost-want) > 1e-12 || len(summary.Models) != 1 || !summary.Models[0].Priced {
		t.Fatalf("model-price fallback summary = %+v, want cost %v", summary, want)
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
	// Simulate an event persisted before stable API key identifiers existed.
	store.mu.Lock()
	store.state.Events[0].APIKeyID = "legacy:" + store.state.Events[0].APIKey
	store.mu.Unlock()
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

func TestSetRulesRecalculatesOnlyEstimatedEvents(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(UsageRecord{Provider: "codex", Model: "future-model", InputTokens: 1_000_000, TotalTokens: 1_000_000})
	store.HandleUsage(UsageRecord{Provider: "codex", Model: "billed-model", InputTokens: 1_000_000, Cost: 2.5, CostProvided: true})
	if err := store.SetRules([]PriceRule{{Match: "future-model", InputPerMillion: 7}}); err != nil {
		t.Fatal(err)
	}
	if got := store.Summary().Totals.Cost; math.Abs(got-9.5) > 1e-12 {
		t.Fatalf("recalculated mixed cost = %v, want 9.5", got)
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
