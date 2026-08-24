// Command preview serves a sample dashboard for local UI development. It is
// not part of the CLIProxyAPI plugin runtime.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/chyern/CPA-Billing-Management/internal/billing"
	"github.com/chyern/CPA-Billing-Management/internal/dashboard"
)

func main() {
	summary := billing.Summary{
		Currency:  "USD",
		UpdatedAt: time.Now(),
		Totals: billing.Totals{
			Requests: 128, FailedRequests: 3, InputTokens: 8_420_000,
			OutputTokens: 1_260_000, TotalTokens: 9_680_000, Cost: 34.7284,
		},
		Models: []*billing.Aggregate{
			{Provider: "codex", Model: "gpt-5.5", Requests: 96, InputTokens: 6_400_000, OutputTokens: 980_000, TotalTokens: 7_380_000, Cost: 30.7, Priced: true},
			{Provider: "claude", Model: "claude-sonnet", Requests: 32, InputTokens: 2_020_000, OutputTokens: 280_000, TotalTokens: 2_300_000, Cost: 4.0284, Priced: true},
		},
		APIKeys: []*billing.APIKeyAggregate{
			{APIKey: "sk-a••••••demo", Requests: 96, FailedRequests: 1, InputTokens: 6_400_000, OutputTokens: 980_000, TotalTokens: 7_380_000, Cost: 30.7},
			{APIKey: "sk-b••••••test", Requests: 32, FailedRequests: 2, InputTokens: 2_020_000, OutputTokens: 280_000, TotalTokens: 2_300_000, Cost: 4.0284},
		},
		RecentEvents: []billing.UsageEvent{
			{RequestedAt: time.Now().Add(-2 * time.Minute), Provider: "codex", Model: "gpt-5.5", APIKey: "sk-a••••••demo", LatencyNanos: int64(1450 * time.Millisecond), TTFTNanos: int64(320 * time.Millisecond), InputTokens: 18_000, OutputTokens: 2_400, TotalTokens: 20_400, Cost: 0.081},
			{RequestedAt: time.Now().Add(-7 * time.Minute), Provider: "claude", Model: "claude-sonnet", APIKey: "sk-b••••••test", LatencyNanos: int64(820 * time.Millisecond), TTFTNanos: int64(180 * time.Millisecond), InputTokens: 12_000, OutputTokens: 1_800, TotalTokens: 13_800, Failed: true},
		},
		RecentEventsTotal: 42, RecentEventsPage: 1, RecentEventsPages: 3, RecentEventsPageSize: 20,
	}
	rules := []billing.PriceRule{
		{Match: "gpt-5.5", InputPerMillion: 2.5, OutputPerMillion: 15, CacheReadPerMillion: 0.25},
		{Match: "*"},
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v0/resource/plugins/cpa-billing-management/billing" && r.URL.Query().Get("format") == "json" {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				page = 1
			}
			if page > summary.RecentEventsPages {
				page = summary.RecentEventsPages
			}
			updated := summary
			updated.RecentEventsPage = page
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"summary": updated})
			return
		}
		var page []byte
		var err error
		if r.URL.Path == "/pricing" {
			page, err = dashboard.RenderPricing(dashboard.Data{Rules: rules, Currency: summary.Currency})
		} else {
			page, err = dashboard.RenderBilling(dashboard.Data{Summary: summary})
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
	log.Println("billing preview: http://127.0.0.1:4173; model costs: http://127.0.0.1:4173/pricing")
	log.Fatal(http.ListenAndServe("127.0.0.1:4173", nil))
}
