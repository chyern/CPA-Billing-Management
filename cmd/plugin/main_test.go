package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/chyern/CPA-Billing-Management/internal/abi"
	"github.com/chyern/CPA-Billing-Management/internal/billing"
)

func TestPluginBillingFlow(t *testing.T) {
	var err error
	store, err = billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	storeErr = nil

	registerRaw, err := handleMethod(abi.MethodPluginRegister, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resultContains(t, registerRaw, `"usage_plugin":true`) || !resultContains(t, registerRaw, `"management_api":true`) {
		t.Fatalf("registration is missing billing capabilities: %s", registerRaw)
	}

	if err := store.SetRules([]billing.PriceRule{{Match: "test-model", InputPerMillion: 1, OutputPerMillion: 2}}); err != nil {
		t.Fatal(err)
	}
	usage := []byte(`{"Provider":"test","Model":"test-model","Detail":{"InputTokens":1000000,"OutputTokens":1000000,"TotalTokens":2000000}}`)
	if _, err := handleMethod(abi.MethodUsageHandle, usage); err != nil {
		t.Fatal(err)
	}

	req, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodGet, Path: "/v0/management/cpa-billing-management/summary"})
	summaryRaw, err := handleMethod(abi.MethodManagementHandle, req)
	if err != nil {
		t.Fatal(err)
	}
	body := managementBody(t, summaryRaw)
	var summary billing.Summary
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Totals.Requests != 1 || summary.Totals.Cost != 3 {
		t.Fatalf("summary totals = %+v, want one request and cost 3", summary.Totals)
	}

	pricesBody, _ := json.Marshal(map[string]any{"rules": []billing.PriceRule{{Match: "test-model", InputPerMillion: 2, OutputPerMillion: 4}}})
	pricesReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodPut, Path: "/v0/management/cpa-billing-management/prices", Body: pricesBody})
	if _, err := handleMethod(abi.MethodManagementHandle, pricesReq); err != nil {
		t.Fatal(err)
	}
	summaryRaw, err = handleMethod(abi.MethodManagementHandle, req)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(managementBody(t, summaryRaw), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Totals.Cost != 6 {
		t.Fatalf("recalculated management cost = %v, want 6", summary.Totals.Cost)
	}

	resourceReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodGet, Path: "/v0/resource/plugins/cpa-billing-management/billing"})
	resourceRaw, err := handleMethod(abi.MethodManagementHandle, resourceReq)
	if err != nil {
		t.Fatal(err)
	}
	if resultContains(t, resourceRaw, "test-model") {
		t.Fatal("unauthenticated resource page must not embed billing records")
	}
}

func resultContains(t *testing.T, raw []byte, expected string) bool {
	t.Helper()
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	return contains(string(envelope.Result), expected)
}

func managementBody(t *testing.T, raw []byte) []byte {
	t.Helper()
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Body string `json:"Body"`
	}
	if err := json.Unmarshal(envelope.Result, &response); err != nil {
		t.Fatal(err)
	}
	body, err := base64.StdEncoding.DecodeString(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func contains(value, expected string) bool {
	for i := 0; i+len(expected) <= len(value); i++ {
		if value[i:i+len(expected)] == expected {
			return true
		}
	}
	return false
}
