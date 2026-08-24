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

func TestCalculateCostSeparatesCacheReadTokens(t *testing.T) {
	record := UsageRecord{InputTokens: 1_000_000, OutputTokens: 500_000, CacheReadTokens: 200_000, CacheCreationTokens: 50_000}
	rule := PriceRule{InputPerMillion: 2, OutputPerMillion: 4, CacheReadPerMillion: 0.5, CacheCreationPerMillion: 1}
	want := 750_000.0/1_000_000*2 + 500_000.0/1_000_000*4 + 200_000.0/1_000_000*0.5 + 50_000.0/1_000_000
	if got := CalculateCost(record, rule); math.Abs(got-want) > 1e-12 {
		t.Fatalf("CalculateCost() = %v, want %v", got, want)
	}
}

func TestStorePersistsUsageAndMatchesProviderModel(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetRules([]PriceRule{{Match: "codex/gpt-5.5", InputPerMillion: 10, OutputPerMillion: 20}}); err != nil {
		t.Fatal(err)
	}
	const apiKey = "sk-test-sensitive-key"
	store.HandleUsage(UsageRecord{
		Provider: "codex", Model: "gpt-5.5", APIKey: apiKey, RequestedAt: time.Now(),
		Latency: 1500 * time.Millisecond, TTFT: 250 * time.Millisecond,
		InputTokens: 1_000_000, OutputTokens: 2_000_000, TotalTokens: 3_000_000,
	})
	summary := store.Summary()
	if summary.Totals.Requests != 1 || math.Abs(summary.Totals.Cost-50) > 1e-12 {
		t.Fatalf("summary = %+v, want one request and cost 50", summary.Totals)
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
	if got := reloaded.Summary().Totals.Cost; math.Abs(got-50) > 1e-12 {
		t.Fatalf("reloaded cost = %v, want 50", got)
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

func TestUnknownModelUsesWildcardAndIsMarkedUnpriced(t *testing.T) {
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

func TestSetRulesRecalculatesHistoricalEvents(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(UsageRecord{Provider: "codex", Model: "future-model", InputTokens: 1_000_000, TotalTokens: 1_000_000})
	if got := store.Summary().Totals.Cost; got != 0 {
		t.Fatalf("initial cost = %v, want 0", got)
	}
	if err := store.SetRules([]PriceRule{{Match: "future-model", InputPerMillion: 7}}); err != nil {
		t.Fatal(err)
	}
	summary := store.Summary()
	if got := summary.Totals.Cost; math.Abs(got-7) > 1e-12 {
		t.Fatalf("recalculated cost = %v, want 7", got)
	}
	if len(summary.Models) != 1 || !summary.Models[0].Priced {
		t.Fatalf("model should be marked priced: %+v", summary.Models)
	}
}
