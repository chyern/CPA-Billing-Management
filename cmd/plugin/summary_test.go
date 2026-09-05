package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/chyern/CPA-Billing-Management/internal/abi"
	"github.com/chyern/CPA-Billing-Management/internal/billing"
)

func TestManagementSummaryFiltersOlderFailuresBeforePagination(t *testing.T) {
	s, err := billing.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 25; i++ {
		if err := s.HandleUsage(billing.UsageRecord{Model: "filter", Failed: i < 2, Cost: 1, CostProvided: true, RequestedAt: time.Date(2026, 9, 1, 0, 0, i, 0, time.UTC)}); err != nil {
			t.Fatal(err)
		}
	}
	request, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodGet, Path: "/cpa-billing-management/summary", Query: map[string][]string{
		"event_status": {"failed"}, "page_size": {"1"}, "page": {"2"}, "start": {"2026-09-01T00:00:00Z"}, "end": {"2026-09-01T00:00:00Z"},
	}})
	raw, err := handleManagement(s, request)
	if err != nil {
		t.Fatal(err)
	}
	if status := managementStatus(t, raw); status != http.StatusOK {
		t.Fatalf("summary status=%d", status)
	}
	var summary billing.Summary
	if err := json.Unmarshal(managementBody(t, raw), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Totals.Requests != 25 || summary.Totals.Cost != 25 || summary.RecentEventsTotal != 2 || summary.RecentEventsPages != 2 || len(summary.RecentEvents) != 1 || !summary.RecentEvents[0].Failed {
		t.Fatalf("summary filter/pagination=%+v", summary)
	}
	request, _ = json.Marshal(abi.ManagementRequest{Method: http.MethodGet, Path: "/cpa-billing-management/summary", Query: map[string][]string{"event_status": {"invalid"}}})
	raw, err = handleManagement(s, request)
	if err != nil {
		t.Fatal(err)
	}
	if status := managementStatus(t, raw); status != http.StatusBadRequest {
		t.Fatalf("invalid status returned %d", status)
	}
}
