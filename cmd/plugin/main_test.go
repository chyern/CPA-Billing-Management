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
	if !resultContains(t, registerRaw, `"usage_plugin":true`) || !resultContains(t, registerRaw, `"request_interceptor":true`) || !resultContains(t, registerRaw, `"management_api":true`) {
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
	if !resultContains(t, managementRegisterRaw, `"Menu":"费用统计"`) || !resultContains(t, managementRegisterRaw, `"Menu":"模型费用"`) || !resultContains(t, managementRegisterRaw, `"Menu":"密钥余额"`) || !resultContains(t, managementRegisterRaw, `cpa-billing-management/prices`) || !resultContains(t, managementRegisterRaw, `cpa-billing-management/prices/sync`) || !resultContains(t, managementRegisterRaw, `cpa-billing-management/key-balances`) {
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
	if summary.Totals.Cost != 9 {
		t.Fatalf("historical hybrid cost changed = %v, want original estimate 6 plus upstream 3", summary.Totals.Cost)
	}
}

func TestRequestInterceptorRejectsOnlyConfiguredExhaustedKeys(t *testing.T) {
	billingStore, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = billingStore.Close() })
	key := "sk-exhausted-balance"
	callerScope := billing.CallerScope(key)
	if err := billingStore.SetKeyBalances([]billing.APIKeyBalance{{APIKeyID: billing.APIKeyIdentifier(key), APIKey: key, Balance: 0}}); err != nil {
		t.Fatal(err)
	}

	raw, _ := json.Marshal(abi.RequestInterceptRequest{RequestID: "request-1", Metadata: map[string]any{"caller_scope": callerScope}})
	responseRaw, err := handleRequestInterceptBefore(billingStore, raw)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(responseRaw, &envelope); err != nil {
		t.Fatal(err)
	}
	var response abi.RequestInterceptResponse
	if err := json.Unmarshal(envelope.Result, &response); err != nil {
		t.Fatal(err)
	}
	if !response.Terminate || response.StatusCode != http.StatusPaymentRequired || !contains(string(response.ResponseBody), "insufficient_api_key_balance") {
		t.Fatalf("exhausted response = %+v", response)
	}

	unconfiguredRaw, _ := json.Marshal(abi.RequestInterceptRequest{RequestID: "request-2", Metadata: map[string]any{"caller_scope": billing.CallerScope("sk-unconfigured")}})
	unconfiguredResponseRaw, err := handleRequestInterceptBefore(billingStore, unconfiguredRaw)
	if err != nil {
		t.Fatal(err)
	}
	if resultContains(t, unconfiguredResponseRaw, `"Terminate":true`) {
		t.Fatalf("unconfigured key must be allowed: %s", unconfiguredResponseRaw)
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
	if err := store.SetRules([]billing.PriceRule{{Match: "openai/gpt-4o"}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gpt-4o":{"input_cost_per_token":0.0000025,"output_cost_per_token":0.00001,"cache_read_input_token_cost":0.000001}}`))
	}))
	defer server.Close()

	source, _ := findPricingSource("litellm")
	source.URL = server.URL
	result, err := syncUpstreamPricesFromSourceWithClient(store, server.Client(), source)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 1 || result.Updated != 1 {
		t.Fatalf("sync result = %+v, want one matched and updated model", result)
	}
	rules := store.Rules()
	if len(rules) != 1 || rules[0].Match != "openai/gpt-4o" || rules[0].InputPerMillion != 2.5 || rules[0].CacheReadPerMillion != 1 {
		t.Fatalf("synced rules = %+v", rules)
	}
}

func TestPreviewUpstreamPricesDoesNotPersistChanges(t *testing.T) {
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(billing.UsageRecord{Provider: "openai", Model: "gpt-4o", InputTokens: 10, OutputTokens: 5})
	if err := store.SetRules([]billing.PriceRule{{Match: "openai/gpt-4o"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRules([]billing.PriceRule{{Match: "openai/gpt-4o", InputPerMillion: 1, OutputPerMillion: 2}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gpt-4o":{"input_cost_per_token":0.0000025,"output_cost_per_token":0.00001}}`))
	}))
	defer server.Close()

	source, _ := findPricingSource("litellm")
	result, err := previewUpstreamPricesFromSourceWithClient(store, server.Client(), pricingSource{ID: source.ID, Name: source.Name, URL: server.URL, Decode: source.Decode}, store.Rules())
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied || result.Updated != 1 || len(result.Changes) != 1 {
		t.Fatalf("preview result = %+v", result)
	}
	if got := store.Rules()[0].InputPerMillion; got != 1 {
		t.Fatalf("preview changed persisted input price to %v", got)
	}
	if got := result.Rules[0].InputPerMillion; got != 2.5 {
		t.Fatalf("preview rules input price = %v, want 2.5", got)
	}
}

func TestPreviewSyncIncludesModelsAlreadyInRules(t *testing.T) {
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	rules := []billing.PriceRule{{Match: "codex/gpt-5.6-terra"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gpt-5.6-terra":{"input_cost_per_token":0.000002,"output_cost_per_token":0.00001}}`))
	}))
	defer server.Close()
	source, _ := findPricingSource("litellm")
	result, err := previewUpstreamPricesFromSourceWithClient(store, server.Client(), pricingSource{ID: source.ID, Name: source.Name, URL: server.URL, Decode: source.Decode}, rules)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 1 || result.Updated != 1 || len(result.Changes) != 1 || result.Rules[0].Match != "codex/gpt-5.6-terra" {
		t.Fatalf("preview rules-model result = %+v", result)
	}
}

func TestPreviewSyncIncludesServedModelsWhenRulesEmpty(t *testing.T) {
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gpt-4o":{"input_cost_per_token":0.0000025,"output_cost_per_token":0.00001}}`))
	}))
	defer server.Close()
	source, _ := findPricingSource("litellm")
	served := []syncModel{{Model: "gpt-4o"}}
	result, err := previewUpstreamPricesFromSourceWithClient(store, server.Client(), pricingSource{ID: source.ID, Name: source.Name, URL: server.URL, Decode: source.Decode}, nil, served)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 1 || result.Added != 1 || len(result.Rules) != 1 || result.Rules[0].Match != "gpt-4o" {
		t.Fatalf("served-model preview result = %+v", result)
	}
}

func TestSyncPreservesFreeModelsAndSkipsNegativeSentinels(t *testing.T) {
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(billing.UsageRecord{Model: "free-model", InputTokens: 1})
	store.HandleUsage(billing.UsageRecord{Model: "dynamic-model", InputTokens: 1})
	if err := store.SetRules([]billing.PriceRule{{Match: "free-model"}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"free-model":{"input_cost_per_token":0,"output_cost_per_token":0},"dynamic-model":{"input_cost_per_token":-1,"output_cost_per_token":-1}}`))
	}))
	defer server.Close()

	source, _ := findPricingSource("litellm")
	source.URL = server.URL
	result, err := syncUpstreamPricesFromSourceWithClient(store, server.Client(), source)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 1 || result.Unchanged != 1 {
		t.Fatalf("sync result = %+v", result)
	}
	if rules := store.Rules(); len(rules) != 1 || rules[0].Match != "free-model" {
		t.Fatalf("synced free-model rules = %+v", rules)
	}
}

func TestSyncModelsDevPricesUsesPerMillionValues(t *testing.T) {
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(billing.UsageRecord{Provider: "openai", Model: "gpt-4o", InputTokens: 10, OutputTokens: 5})
	if err := store.SetRules([]billing.PriceRule{{Match: "openai/gpt-4o"}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"openai":{"id":"openai","models":{"gpt-4o":{"id":"gpt-4o","cost":{"input":2.5,"output":10,"cache_read":1,"cache_write":1.5}}}}}`))
	}))
	defer server.Close()

	source, ok := findPricingSource("models.dev")
	if !ok {
		t.Fatal("models.dev pricing source is not registered")
	}
	source.URL = server.URL
	result, err := syncUpstreamPricesFromSourceWithClient(store, server.Client(), source)
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceID != "models.dev" || result.Matched != 1 || result.Updated != 1 {
		t.Fatalf("sync result = %+v", result)
	}
	rules := store.Rules()
	if len(rules) != 1 || rules[0].Match != "openai/gpt-4o" || rules[0].InputPerMillion != 2.5 || rules[0].OutputPerMillion != 10 || rules[0].CacheReadPerMillion != 1 || rules[0].CacheCreationPerMillion != 1.5 {
		t.Fatalf("models.dev rules = %+v", rules)
	}
}

func TestModelsDevAliasChoosesCheapestNonZeroPrice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"expensive":{"models":{"gpt-test":{"cost":{"input":10,"output":20}}}},
			"free":{"models":{"gpt-test":{"cost":{"input":0,"output":0}}}},
			"cheap":{"models":{"gpt-test":{"cost":{"input":2,"output":4}}}}
		}`))
	}))
	defer server.Close()
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(billing.UsageRecord{Provider: "codex", Model: "gpt-test", InputTokens: 1})
	if err := store.SetRules([]billing.PriceRule{{Match: "codex/gpt-test"}}); err != nil {
		t.Fatal(err)
	}
	source := pricingSource{ID: "models.dev", Name: "Models.dev", URL: server.URL, Decode: decodeModelsDevCatalog}
	result, err := syncUpstreamPricesFromSourceWithClient(store, server.Client(), source)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 1 || len(store.Rules()) != 1 || store.Rules()[0].InputPerMillion != 2 {
		t.Fatalf("models.dev alias result = %+v, rules=%+v", result, store.Rules())
	}
}

func TestModelsDevCodexAliasPrefersOfficialOpenAIPrice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"xpersona":{"id":"xpersona","models":{"gpt-5.6-sol":{"id":"gpt-5.6-sol","cost":{"input":1.5,"output":12,"cache_read":0.15}}}},
			"reseller":{"id":"reseller","models":{"sol":{"id":"openai/gpt-5.6-sol","cost":{"input":2,"output":10,"cache_read":0.2}}}},
			"openai":{"id":"openai","models":{"gpt-5.6-sol":{"id":"gpt-5.6-sol","cost":{"input":4,"output":20,"cache_read":0.4,"cache_write":5}}}}
		}`))
	}))
	defer server.Close()
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(billing.UsageRecord{Provider: "codex", Model: "gpt-5.6-sol", InputTokens: 1})
	if err := store.SetRules([]billing.PriceRule{{Match: "codex/gpt-5.6-sol"}}); err != nil {
		t.Fatal(err)
	}
	source := pricingSource{ID: "models.dev", Name: "Models.dev", URL: server.URL, Decode: decodeModelsDevCatalog}
	result, err := syncUpstreamPricesFromSourceWithClient(store, server.Client(), source)
	if err != nil {
		t.Fatal(err)
	}
	rules := store.Rules()
	if result.Matched != 1 || len(rules) != 1 {
		t.Fatalf("models.dev official result = %+v, rules=%+v", result, rules)
	}
	price := rules[0]
	if price.Match != "codex/gpt-5.6-sol" || price.InputPerMillion != 4 || price.OutputPerMillion != 20 || price.CacheReadPerMillion != 0.4 || price.CacheCreationPerMillion != 5 {
		t.Fatalf("official OpenAI price was not preferred: %+v", price)
	}
}

func TestSyncOpenRouterPricesConvertsPerTokenValues(t *testing.T) {
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(billing.UsageRecord{Provider: "openai", Model: "gpt-4o", InputTokens: 10, OutputTokens: 5})
	if err := store.SetRules([]billing.PriceRule{{Match: "openai/gpt-4o"}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"openai/gpt-4o","pricing":{"prompt":"0.0000025","completion":"0.00001","input_cache_read":"0.000001","input_cache_write":"0.0000015"}}]}`))
	}))
	defer server.Close()

	source, ok := findPricingSource("openrouter")
	if !ok {
		t.Fatal("openrouter pricing source is not registered")
	}
	source.URL = server.URL
	result, err := syncUpstreamPricesFromSourceWithClient(store, server.Client(), source)
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceID != "openrouter" || result.Matched != 1 || result.Updated != 1 {
		t.Fatalf("sync result = %+v", result)
	}
	rules := store.Rules()
	if len(rules) != 1 || rules[0].InputPerMillion != 2.5 || rules[0].OutputPerMillion != 10 || rules[0].CacheReadPerMillion != 1 || rules[0].CacheCreationPerMillion != 1.5 {
		t.Fatalf("openrouter rules = %+v", rules)
	}
}

func TestManagementSyncRejectsUnknownPricingSource(t *testing.T) {
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	req, _ := json.Marshal(abi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/cpa-billing-management/prices/sync",
		Query:  map[string][]string{"source": {"unknown"}},
	})
	raw, err := handleManagement(store, req)
	if err != nil {
		t.Fatal(err)
	}
	if got := managementStatus(t, raw); got != http.StatusBadRequest {
		t.Fatalf("unknown source status = %d, want %d", got, http.StatusBadRequest)
	}
}

func TestManagementSyncSelectsRequestedPricingSource(t *testing.T) {
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(billing.UsageRecord{Provider: "openai", Model: "gpt-4o", InputTokens: 10, OutputTokens: 5})
	if err := store.SetRules([]billing.PriceRule{{Match: "openai/gpt-4o"}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gpt-4o":{"input_cost_per_token":0.000003,"output_cost_per_token":0.000012}}`))
	}))
	defer server.Close()

	previousSources := pricingSources
	pricingSources = []pricingSource{{ID: "fixture", Name: "Fixture", URL: server.URL, Decode: decodeLiteLLMCatalog}}
	t.Cleanup(func() { pricingSources = previousSources })
	body, _ := json.Marshal(map[string]any{"rules": store.Rules()})
	req, _ := json.Marshal(abi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/cpa-billing-management/prices/sync",
		Query:  map[string][]string{"source": {"fixture"}, "preview": {"1"}},
		Body:   body,
	})
	raw, err := handleManagement(store, req)
	if err != nil {
		t.Fatal(err)
	}
	if got := managementStatus(t, raw); got != http.StatusOK {
		t.Fatalf("selected source status = %d, want %d", got, http.StatusOK)
	}
	var result upstreamSyncResult
	if err := json.Unmarshal(managementBody(t, raw), &result); err != nil {
		t.Fatal(err)
	}
	if result.SourceID != "fixture" || result.SourceName != "Fixture" || result.Matched != 1 {
		t.Fatalf("selected source result = %+v", result)
	}
}

func TestManagementPreviewSyncDoesNotWriteRules(t *testing.T) {
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(billing.UsageRecord{Provider: "openai", Model: "gpt-4o", InputTokens: 10, OutputTokens: 5})
	if err := store.SetRules([]billing.PriceRule{{Match: "openai/gpt-4o", InputPerMillion: 1, OutputPerMillion: 2}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gpt-4o":{"input_cost_per_token":0.0000025,"output_cost_per_token":0.00001}}`))
	}))
	defer server.Close()

	previousSources := pricingSources
	pricingSources = []pricingSource{{ID: "fixture", Name: "Fixture", URL: server.URL, Decode: decodeLiteLLMCatalog}}
	t.Cleanup(func() { pricingSources = previousSources })
	body, _ := json.Marshal(map[string]any{"rules": store.Rules()})
	req, _ := json.Marshal(abi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/cpa-billing-management/prices/sync",
		Query:  map[string][]string{"source": {"fixture"}, "preview": {"1"}},
		Body:   body,
	})
	raw, err := handleManagement(store, req)
	if err != nil {
		t.Fatal(err)
	}
	if got := managementStatus(t, raw); got != http.StatusOK {
		t.Fatalf("preview sync status = %d, want %d", got, http.StatusOK)
	}
	var result upstreamSyncResult
	if err := json.Unmarshal(managementBody(t, raw), &result); err != nil {
		t.Fatal(err)
	}
	if result.Applied || result.Updated != 1 || len(result.Changes) != 1 {
		t.Fatalf("preview sync result = %+v", result)
	}
	if got := store.Rules()[0].InputPerMillion; got != 1 {
		t.Fatalf("management preview changed persisted input price to %v", got)
	}
}

func TestPricingManagementListsAvailableSources(t *testing.T) {
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	req, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodGet, Path: "/v0/management/cpa-billing-management/prices"})
	raw, err := handleManagement(store, req)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		DefaultSource  string                    `json:"default_source"`
		PricingSources []pricingSourceDescriptor `json:"pricing_sources"`
	}
	if err := json.Unmarshal(managementBody(t, raw), &response); err != nil {
		t.Fatal(err)
	}
	if response.DefaultSource != defaultPricingSourceID || len(response.PricingSources) != 3 {
		t.Fatalf("pricing sources response = %+v", response)
	}
	for _, source := range response.PricingSources {
		if source.ID == "" || source.Name == "" {
			t.Fatalf("invalid pricing source descriptor = %+v", source)
		}
	}
}

func TestPricingManagementDoesNotListUsageModels(t *testing.T) {
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	store.HandleUsage(billing.UsageRecord{Provider: "openai", Model: "gpt-unpriced", InputTokens: 1})
	req, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodGet, Path: "/v0/management/cpa-billing-management/prices"})
	raw, err := handleManagement(store, req)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Rules  []billing.PriceRule   `json:"rules"`
		Models []pricingCatalogModel `json:"models"`
	}
	if err := json.Unmarshal(managementBody(t, raw), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Rules) != 0 || len(response.Models) != 0 {
		t.Fatalf("usage model leaked into pricing snapshot = rules=%+v models=%+v", response.Rules, response.Models)
	}
}

func TestKeyBalanceManagementAndResource(t *testing.T) {
	store, err := billing.NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	key := "sk-management-balance"
	id := billing.APIKeyIdentifier(key)
	body, _ := json.Marshal(map[string]any{
		"balances": []billing.APIKeyBalance{{APIKeyID: id, APIKey: key, Balance: 12}},
		"notes":    []billing.APIKeyBalance{{APIKeyID: id, APIKey: key, Note: "生产调用"}},
	})
	putReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodPut, Path: "/v0/management/cpa-billing-management/key-balances", Body: body})
	putRaw, err := handleManagement(store, putReq)
	if err != nil {
		t.Fatal(err)
	}
	if got := managementStatus(t, putRaw); got != http.StatusOK {
		t.Fatalf("key balance PUT status = %d", got)
	}
	store.HandleUsage(billing.UsageRecord{APIKey: key, Model: "upstream-priced", Cost: 3, CostProvided: true})
	getReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodGet, Path: "/v0/management/cpa-billing-management/key-balances"})
	getRaw, err := handleManagement(store, getReq)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Balances []billing.APIKeyBalance `json:"balances"`
	}
	if err := json.Unmarshal(managementBody(t, getRaw), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Balances) != 1 || snapshot.Balances[0].Balance != 9 || snapshot.Balances[0].APIKey != billing.MaskAPIKey(key) || snapshot.Balances[0].Note != "生产调用" {
		t.Fatalf("key balance snapshot = %+v", snapshot)
	}
	resourceReq, _ := json.Marshal(abi.ManagementRequest{Method: http.MethodGet, Path: balancesPath})
	resourceRaw, err := handleManagement(store, resourceReq)
	if err != nil {
		t.Fatal(err)
	}
	page := string(managementBody(t, resourceRaw))
	if !contains(page, "CPA 密钥余额") || !contains(page, "key-balances") || contains(page, key) {
		t.Fatal("key balance resource page must contain balance controls without secrets")
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
