package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/chyern/CPA-Billing-Management/internal/abi"
	"github.com/chyern/CPA-Billing-Management/internal/billing"
)

func TestRegistrationUsesBuildVersion(t *testing.T) {
	previous := pluginVersion
	pluginVersion = "9.8.7"
	t.Cleanup(func() { pluginVersion = previous })

	if got := registration().Metadata.Version; got != "9.8.7" {
		t.Fatalf("registration version = %q, want build-injected version", got)
	}
}

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
	if resultContains(t, registerRaw, `"management_key"`) {
		t.Fatalf("registration must not expose a second management key config field: %s", registerRaw)
	}
	if !resultContains(t, registerRaw, `"cpa_billing_data_dir"`) {
		t.Fatalf("registration is missing the SQLite data directory config field: %s", registerRaw)
	}
	managementRegisterRaw, err := handleMethod(abi.MethodManagementRegister, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resultContains(t, managementRegisterRaw, `"Menu":"费用统计"`) || !resultContains(t, managementRegisterRaw, `"Menu":"模型费用"`) || !resultContains(t, managementRegisterRaw, `cpa-billing-management/prices`) || !resultContains(t, managementRegisterRaw, `cpa-billing-management/prices/sync`) {
		t.Fatalf("management registration is missing billing or model-cost routes: %s", managementRegisterRaw)
	}
	if err := store.SetRules([]billing.PriceRule{{Match: "test-model", InputPerMillion: 1, OutputPerMillion: 2}}); err != nil {
		t.Fatal(err)
	}
	const apiKey = "sk-test-sensitive-key"
	usage := []byte(`{"Provider":"test","Model":"test-model","APIKey":"sk-test-sensitive-key","ActualCost":3,"TotalCost":99,"Latency":1500000000,"TTFT":250000000,"Detail":{"InputTokens":1000000,"CachedTokens":250000,"OutputTokens":1000000,"TotalTokens":2000000}}`)
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
	if summary.Totals.Requests != 1 || summary.Totals.Cost != 3 || summary.Totals.CachedTokens != 250000 {
		t.Fatalf("summary totals = %+v, want upstream cost 3", summary.Totals)
	}
	if len(summary.Models) != 1 || summary.Models[0].CachedTokens != 250000 || len(summary.APIKeys) != 1 || summary.APIKeys[0].CachedTokens != 250000 {
		t.Fatalf("usage cache totals were not aggregated: models=%+v api_keys=%+v", summary.Models, summary.APIKeys)
	}
	if len(summary.RecentEvents) != 1 || summary.RecentEvents[0].APIKey != "sk-t••••••-key" || summary.RecentEvents[0].CachedTokens != 250000 || summary.RecentEvents[0].LatencyNanos != 1_500_000_000 || summary.RecentEvents[0].TTFTNanos != 250_000_000 {
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
	if contains(string(resourceBody), "test-model") {
		t.Fatal("resource page must not embed the local billing snapshot")
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
	if contains(string(resourceBody), "sk-t••••••-key") {
		t.Fatal("resource page must not expose the masked API key before authenticated API loading")
	}
	if !contains(string(resourceBody), "/v0/management/cpa-billing-management/summary") {
		t.Fatal("resource page must load data through the authenticated management API")
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
	if got := managementStatus(t, refreshRaw); got != http.StatusUnauthorized {
		t.Fatalf("billing resource JSON status = %d, want %d", got, http.StatusUnauthorized)
	}
	fallbackReq, _ := json.Marshal(abi.ManagementRequest{
		Method: http.MethodGet,
		Path:   resourcePath,
		Query:  map[string][]string{"format": {"fallback-json"}, "page": {"2"}, "page_size": {"1"}},
	})
	fallbackRaw, err := handleMethod(abi.MethodManagementHandle, fallbackReq)
	if err != nil {
		t.Fatal(err)
	}
	if got := managementStatus(t, fallbackRaw); got != http.StatusUnauthorized {
		t.Fatalf("billing resource fallback status = %d, want %d", got, http.StatusUnauthorized)
	}
	managementRefreshReq, _ := json.Marshal(abi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/cpa-billing-management/summary",
		Query:  map[string][]string{"page": {"2"}, "page_size": {"1"}},
	})
	managementRefreshRaw, err := handleMethod(abi.MethodManagementHandle, managementRefreshReq)
	if err != nil {
		t.Fatal(err)
	}
	var pagedSummary billing.Summary
	if err := json.Unmarshal(managementBody(t, managementRefreshRaw), &pagedSummary); err != nil {
		t.Fatal(err)
	}
	if pagedSummary.Totals.Cost != 9 || pagedSummary.RecentEventsTotal != 2 || pagedSummary.RecentEventsPage != 2 || pagedSummary.RecentEventsPageSize != 1 {
		t.Fatalf("authenticated management summary page = %+v", pagedSummary)
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
	if !contains(string(pricingPage), "CPA 模型费用") || !contains(string(pricingPage), "模型价格规则") || !contains(string(pricingPage), "保存模型费用") || !contains(string(pricingPage), "/v0/management/cpa-billing-management/prices") || contains(string(pricingPage), "最近事件") || contains(string(pricingPage), "test-model") {
		t.Fatal("model-cost resource page must contain only pricing controls")
	}
	pricingJSONReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodGet, Path: pricingPath, Query: map[string][]string{"format": {"json"}}})
	pricingJSONRaw, err := handleMethod(abi.MethodManagementHandle, pricingJSONReq)
	if err != nil {
		t.Fatal(err)
	}
	if got := managementStatus(t, pricingJSONRaw); got != http.StatusUnauthorized {
		t.Fatalf("pricing resource JSON status = %d, want %d", got, http.StatusUnauthorized)
	}
	pricingFallbackReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodGet, Path: pricingPath, Query: map[string][]string{"format": {"fallback-json"}}})
	pricingFallbackRaw, err := handleMethod(abi.MethodManagementHandle, pricingFallbackReq)
	if err != nil {
		t.Fatal(err)
	}
	if got := managementStatus(t, pricingFallbackRaw); got != http.StatusUnauthorized {
		t.Fatalf("pricing resource fallback status = %d, want %d", got, http.StatusUnauthorized)
	}

	localPricesBody, _ := json.Marshal(map[string]any{"rules": []billing.PriceRule{{Match: "test-model", InputPerMillion: 3, OutputPerMillion: 6}}})
	localPricesReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodPut, Path: pricingPath, Body: localPricesBody})
	localPricesRaw, err := handleMethod(abi.MethodManagementHandle, localPricesReq)
	if err != nil {
		t.Fatal(err)
	}
	if got := managementStatus(t, localPricesRaw); got != http.StatusMethodNotAllowed {
		t.Fatalf("pricing resource PUT status = %d, want %d", got, http.StatusMethodNotAllowed)
	}
	managementPricesReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodPut, Path: "/v0/management/cpa-billing-management/prices", Body: localPricesBody})
	localPricesRaw, err = handleMethod(abi.MethodManagementHandle, managementPricesReq)
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

func TestConfiguredDataDir(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want string
	}{
		{raw: "cpa_billing_data_dir: '/tmp/quoted'\n", want: "/tmp/quoted"},
		{raw: "currency: USD\n", want: ""},
	} {
		if got := configuredDataDir([]byte(test.raw)); got != test.want {
			t.Fatalf("configuredDataDir(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

func TestSyncUpstreamPricesAddsMatchedModels(t *testing.T) {
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(billing.UsageRecord{Provider: "openai", Model: "gpt-4o", InputTokens: 10, OutputTokens: 5, TotalTokens: 15})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gpt-4o":{"input_cost_per_token":0.0000025,"output_cost_per_token":0.00001,"cache_read_input_token_cost":0.000001}}`))
	}))
	defer server.Close()

	result, err := syncUpstreamPricesWithClient(store, server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 1 || result.Added != 1 {
		t.Fatalf("sync result = %+v, want one matched and added model", result)
	}
	rules := store.Rules()
	if len(rules) != 1 || rules[0].Match != "openai/gpt-4o" || rules[0].InputPerMillion != 2.5 || rules[0].CacheReadPerMillion != 1 {
		t.Fatalf("synced rules = %+v", rules)
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
