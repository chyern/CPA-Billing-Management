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
	if !resultContains(t, managementRegisterRaw, `"Menu":"费用统计"`) || !resultContains(t, managementRegisterRaw, `"Menu":"模型费用"`) || !resultContains(t, managementRegisterRaw, `cpa-billing-management/prices`) {
		t.Fatalf("management registration is missing billing or model-cost routes: %s", managementRegisterRaw)
	}
	if err := store.SetRules([]billing.PriceRule{{Match: "test-model", InputPerMillion: 1, OutputPerMillion: 2}}); err != nil {
		t.Fatal(err)
	}
	const apiKey = "sk-test-sensitive-key"
	usage := []byte(`{"Provider":"test","Model":"test-model","APIKey":"sk-test-sensitive-key","ActualCost":3,"TotalCost":99,"Latency":1500000000,"TTFT":250000000,"Detail":{"InputTokens":1000000,"OutputTokens":1000000,"TotalTokens":2000000}}`)
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
		t.Fatalf("summary totals = %+v, want upstream cost 3", summary.Totals)
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
	if summary.Totals.Cost != 3 {
		t.Fatalf("upstream cost changed after price update: %v", summary.Totals.Cost)
	}

	localUsage := []byte(`{"Provider":"test","Model":"test-model","Detail":{"InputTokens":1000000,"OutputTokens":1000000,"TotalTokens":2000000}}`)
	if _, err := handleMethod(abi.MethodUsageHandle, localUsage); err != nil {
		t.Fatal(err)
	}
	summaryRaw, err = handleMethod(abi.MethodManagementHandle, req)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(managementBody(t, summaryRaw), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Totals.Requests != 2 || summary.Totals.Cost != 9 {
		t.Fatalf("hybrid summary = %+v, want upstream 3 plus model estimate 6", summary.Totals)
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
	if !contains(string(resourceBody), "API Key") || !contains(string(resourceBody), "耗时/首字") || !contains(string(resourceBody), "latency_ns") || !contains(string(resourceBody), "ttft_ns") {
		t.Fatal("resource page must show the masked API key, total latency, and TTFT")
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
		Query:  map[string][]string{"format": {"json"}, "page": {"2"}, "page_size": {"1"}},
	})
	refreshRaw, err := handleMethod(abi.MethodManagementHandle, refreshReq)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Summary billing.Summary `json:"summary"`
	}
	if err := json.Unmarshal(managementBody(t, refreshRaw), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Summary.Totals.Cost != 9 || snapshot.Summary.RecentEventsTotal != 2 || snapshot.Summary.RecentEventsPage != 2 || snapshot.Summary.RecentEventsPageSize != 1 {
		t.Fatalf("billing resource snapshot = %+v", snapshot.Summary.Totals)
	}
	billingPutReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodPut, Path: resourcePath})
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
	pricingPage := managementBody(t, pricingRaw)
	if !contains(string(pricingPage), "CPA 模型费用") || !contains(string(pricingPage), "模型价格规则") || !contains(string(pricingPage), "保存模型费用") || contains(string(pricingPage), "最近事件") {
		t.Fatal("model-cost resource page must contain only pricing controls")
	}

	localPricesBody, _ := json.Marshal(map[string]any{"rules": []billing.PriceRule{{Match: "test-model", InputPerMillion: 3, OutputPerMillion: 6}}})
	localPricesReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodPut, Path: pricingPath, Body: localPricesBody})
	localPricesRaw, err := handleMethod(abi.MethodManagementHandle, localPricesReq)
	if err != nil {
		t.Fatal(err)
	}
	var priceSnapshot struct {
		Rules []billing.PriceRule `json:"rules"`
	}
	if err := json.Unmarshal(managementBody(t, localPricesRaw), &priceSnapshot); err != nil {
		t.Fatal(err)
	}
	if len(priceSnapshot.Rules) != 1 || priceSnapshot.Rules[0].InputPerMillion != 3 {
		t.Fatalf("model-cost update response = %+v", priceSnapshot.Rules)
	}
	summaryRaw, err = handleMethod(abi.MethodManagementHandle, req)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(managementBody(t, summaryRaw), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Totals.Cost != 12 {
		t.Fatalf("recalculated hybrid cost = %v, want upstream 3 plus model estimate 9", summary.Totals.Cost)
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
