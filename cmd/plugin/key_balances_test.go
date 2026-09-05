package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/chyern/CPA-Billing-Management/internal/abi"
	"github.com/chyern/CPA-Billing-Management/internal/billing"
)

func TestKeyBalancePatchManagement(t *testing.T) {
	store, err := billing.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registered := false
	for _, route := range managementRegistration().Routes {
		if route.Method == http.MethodPatch && route.Path == "/cpa-billing-management/key-balances" {
			registered = true
		}
	}
	if !registered {
		t.Fatal("PATCH route not registered")
	}
	for _, route := range managementRegistration().Routes {
		if route.Method == http.MethodPut && route.Path == "/cpa-billing-management/key-balances" {
			t.Fatal("legacy PUT key-balances route must not be registered")
		}
	}
	legacyRequest, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodPut, Path: "/v0/management/cpa-billing-management/key-balances"})
	legacyRaw, err := handleManagement(store, legacyRequest)
	if err != nil {
		t.Fatal(err)
	}
	if status := managementStatus(t, legacyRaw); status != http.StatusNotFound {
		t.Fatalf("legacy key balance PUT status = %d, want %d", status, http.StatusNotFound)
	}
	key := "sk-patch-management"
	id := billing.APIKeyIdentifier(key)
	if err := store.SetKeyBalances([]billing.APIKeyBalance{{APIKeyID: id, APIKey: key, Balance: 10}}); err != nil {
		t.Fatal(err)
	}
	rows, err := store.KeyBalances()
	if err != nil {
		t.Fatal(err)
	}
	version := rows[0].BalanceVersion
	store.HandleUsage(billing.UsageRecord{APIKey: key, Cost: 2, CostProvided: true})
	for _, tc := range []struct {
		update map[string]any
		status int
	}{
		{map[string]any{"api_key_id": id, "note": "changed"}, http.StatusOK},
		{map[string]any{"api_key_id": id, "balance": 12, "expected_balance_version": version}, http.StatusConflict},
		{map[string]any{"api_key_id": id, "balance": 12}, http.StatusBadRequest},
	} {
		body, _ := json.Marshal(map[string]any{"updates": []any{tc.update}})
		request, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodPatch, Path: "/v0/management/cpa-billing-management/key-balances", Body: body})
		raw, err := handleManagement(store, request)
		if err != nil {
			t.Fatal(err)
		}
		if status := managementStatus(t, raw); status != tc.status {
			t.Fatalf("PATCH status=%d want=%d body=%s", status, tc.status, managementBody(t, raw))
		}
	}
	rows, err = store.KeyBalances()
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Balance != 8 || rows[0].Note != "changed" {
		t.Fatalf("unexpected PATCH state: %+v", rows)
	}
}
