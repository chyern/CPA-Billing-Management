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
	managementRegisterRaw, err := handleMethod(abi.MethodManagementRegister, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resultContains(t, managementRegisterRaw, `"Menu":"费用统计"`) || !resultContains(t, managementRegisterRaw, `"Menu":"价格配置"`) {
		t.Fatalf("management registration is missing separate billing and pricing pages: %s", managementRegisterRaw)
	}

	if err := store.SetRules([]billing.PriceRule{{Match: "test-model", InputPerMillion: 1, OutputPerMillion: 2}}); err != nil {
		t.Fatal(err)
	}
	const apiKey = "sk-test-sensitive-key"
	usage := []byte(`{"Provider":"test","Model":"test-model","APIKey":"sk-test-sensitive-key","Latency":1500000000,"TTFT":250000000,"Detail":{"InputTokens":1000000,"OutputTokens":1000000,"TotalTokens":2000000}}`)
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
	if len(summary.RecentEvents) != 1 || summary.RecentEvents[0].APIKey != "sk-t••••••-key" || summary.RecentEvents[0].LatencyNanos != 1_500_000_000 || summary.RecentEvents[0].TTFTNanos != 250_000_000 {
		t.Fatalf("usage identity and timing were not preserved safely: %+v", summary.RecentEvents)
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
	resourceBody := managementBody(t, resourceRaw)
	if !contains(string(resourceBody), "test-model") {
		t.Fatal("resource page must embed the current local billing snapshot")
	}
	if !contains(string(resourceBody), "API Key") || !contains(string(resourceBody), "耗时") || !contains(string(resourceBody), "latency_ns") {
		t.Fatal("resource page must show the masked API key and request latency")
	}
	if contains(string(resourceBody), "价格规则") || contains(string(resourceBody), "数据直接读取自本机插件存储") {
		t.Fatal("billing resource page must not contain pricing controls or the local-storage notice")
	}
	if contains(string(resourceBody), apiKey) {
		t.Fatal("resource page must not expose the complete API key")
	}
	if contains(string(resourceBody), "管理 API Token") {
		t.Fatal("local resource page must not request a management API token")
	}

	refreshReq, _ := json.Marshal(abi.ManagementRequest{
		Method: http.MethodGet,
		Path:   resourcePath,
		Query:  map[string][]string{"format": {"json"}},
	})
	refreshRaw, err := handleMethod(abi.MethodManagementHandle, refreshReq)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Summary billing.Summary     `json:"summary"`
		Rules   []billing.PriceRule `json:"rules"`
	}
	if err := json.Unmarshal(managementBody(t, refreshRaw), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Summary.Totals.Cost != 6 || len(snapshot.Rules) != 0 {
		t.Fatalf("local resource snapshot = %+v, rules = %+v", snapshot.Summary.Totals, snapshot.Rules)
	}
	billingPutReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodPut, Path: resourcePath, Body: pricesBody})
	billingPutRaw, err := handleMethod(abi.MethodManagementHandle, billingPutReq)
	if err != nil {
		t.Fatal(err)
	}
	if got := managementStatus(t, billingPutRaw); got != http.StatusMethodNotAllowed {
		t.Fatalf("billing resource PUT status = %d, want %d", got, http.StatusMethodNotAllowed)
	}

	pricingReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodGet, Path: pricingPath})
	pricingRaw, err := handleMethod(abi.MethodManagementHandle, pricingReq)
	if err != nil {
		t.Fatal(err)
	}
	pricingBody := managementBody(t, pricingRaw)
	if !contains(string(pricingBody), "CPA 价格配置") || !contains(string(pricingBody), "价格规则") || contains(string(pricingBody), "最近事件") {
		t.Fatal("pricing resource page must contain only pricing controls")
	}

	localPricesBody, _ := json.Marshal(map[string]any{"rules": []billing.PriceRule{{Match: "test-model", InputPerMillion: 3, OutputPerMillion: 6}}})
	localPricesReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodPut, Path: pricingPath, Body: localPricesBody})
	localPricesRaw, err := handleMethod(abi.MethodManagementHandle, localPricesReq)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(managementBody(t, localPricesRaw), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Rules) != 1 || snapshot.Rules[0].InputPerMillion != 3 {
		t.Fatalf("local price update response = %+v", snapshot.Rules)
	}
	summaryRaw, err = handleMethod(abi.MethodManagementHandle, req)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(managementBody(t, summaryRaw), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Totals.Cost != 9 {
		t.Fatalf("local price update cost = %v, want 9", summary.Totals.Cost)
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

func managementStatus(t *testing.T, raw []byte) int {
	t.Helper()
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	var response struct {
		StatusCode int `json:"StatusCode"`
	}
	if err := json.Unmarshal(envelope.Result, &response); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode
}

func contains(value, expected string) bool {
	for i := 0; i+len(expected) <= len(value); i++ {
		if value[i:i+len(expected)] == expected {
			return true
		}
	}
	return false
}
