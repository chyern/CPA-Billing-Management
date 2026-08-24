package billing

import (
	"math"
	"path/filepath"
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
	store.HandleUsage(UsageRecord{Provider: "codex", Model: "gpt-5.5", RequestedAt: time.Now(), InputTokens: 1_000_000, OutputTokens: 2_000_000, TotalTokens: 3_000_000})
	summary := store.Summary()
	if summary.Totals.Requests != 1 || math.Abs(summary.Totals.Cost-50) > 1e-12 {
		t.Fatalf("summary = %+v, want one request and cost 50", summary.Totals)
	}
	reloaded, err := NewStore(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Summary().Totals.Cost; math.Abs(got-50) > 1e-12 {
		t.Fatalf("reloaded cost = %v, want 50", got)
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
