package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/chyern/CPA-Billing-Management/internal/abi"
	"github.com/chyern/CPA-Billing-Management/internal/billing"
	"github.com/chyern/CPA-Billing-Management/internal/dashboard"
)

const (
	pluginID      = "cpa-billing-management"
	pluginVersion = "0.1.9"
	resourcePath  = "/v0/resource/plugins/" + pluginID + "/billing"
	pricingPath   = "/v0/resource/plugins/" + pluginID + "/pricing"
)

var (
	storeOnce sync.Once
	store     *billing.Store
	storeErr  error
)

func getBillingStore() (*billing.Store, error) {
	storeOnce.Do(func() {
		if store == nil {
			store, storeErr = billing.NewStore("")
		}
	})
	return store, storeErr
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(_ *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	plugin.abi_version = C.uint32_t(abi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	requestBytes := []byte(nil)
	if request != nil && requestLen > 0 {
		if uint64(requestLen) > uint64(1<<31-1) {
			writeResponse(response, errorEnvelope("invalid_request", "request is too large"))
			return 1
		}
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, err := handleMethod(C.GoString(method), requestBytes)
	if err != nil {
		writeResponse(response, errorEnvelope("plugin_error", err.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

func handleMethod(method string, request []byte) ([]byte, error) {
	billingStore, err := getBillingStore()
	if err != nil {
		return nil, err
	}
	switch method {
	case abi.MethodPluginRegister, abi.MethodPluginReconfigure:
		var lifecycle abi.LifecycleRequest
		if len(request) > 0 && json.Unmarshal(request, &lifecycle) == nil {
			billingStore.ConfigureYAML(lifecycle.ConfigYAML)
		}
		return okEnvelope(registration())
	case abi.MethodUsageHandle:
		if err := handleUsage(billingStore, request); err != nil {
			// Usage callbacks have no error channel in the public plugin API. Return
			// an error envelope for diagnostics while keeping the host request alive.
			return okEnvelope(map[string]any{"accepted": false, "error": err.Error()})
		}
		return okEnvelope(map[string]any{"accepted": true})
	case abi.MethodManagementRegister:
		return okEnvelope(abi.ManagementRegistrationResponse{
			Resources: []abi.ResourceRoute{
				{Path: "/billing", Menu: "费用统计", Description: "查看 usage 事件、token 用量和费用汇总。"},
				{Path: "/pricing", Menu: "模型费用", Description: "配置模型每百万 token 的估算价格。"},
			},
			Routes: []abi.ManagementRoute{
				{Method: http.MethodGet, Path: "/cpa-billing-management/summary"},
				{Method: http.MethodGet, Path: "/cpa-billing-management/prices"},
				{Method: http.MethodPut, Path: "/cpa-billing-management/prices"},
				{Method: http.MethodPost, Path: "/cpa-billing-management/prices/sync"},
				{Method: http.MethodPost, Path: "/cpa-billing-management/reset"},
			},
		})
	case abi.MethodManagementHandle:
		return handleManagement(billingStore, request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func registration() abi.Registration {
	return abi.Registration{
		SchemaVersion: abi.SchemaVersion,
		Metadata: abi.Metadata{
			Name:             pluginID,
			Version:          pluginVersion,
			Author:           "CPA Billing Management",
			GitHubRepository: "https://github.com/chyern/CPA-Billing-Management",
			ConfigFields: []abi.ConfigField{
				{Name: "currency", Type: "string", Description: "费用展示币种，默认 USD；事件未携带币种时使用此值。"},
			},
		},
		Capabilities: abi.Capabilities{UsagePlugin: true, ManagementAPI: true},
	}
}

func handleUsage(store *billing.Store, raw []byte) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("decode usage request: %w", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("usage request must be an object")
	}
	if nested, ok := lookup(object, "record", "usage"); ok {
		if nestedObject, ok := nested.(map[string]any); ok {
			object = nestedObject
		}
	}
	record := billing.UsageRecord{
		Provider: stringValue(object, "provider"), ExecutorType: stringValue(object, "executor_type", "executortype"),
		Model: stringValue(object, "model"), Alias: stringValue(object, "alias"), APIKey: stringValue(object, "api_key", "apikey"),
		AuthID: stringValue(object, "auth_id", "authid"), AuthType: stringValue(object, "auth_type", "authtype"),
		Source: stringValue(object, "source"), Latency: time.Duration(intValue(object, "latency", "duration", "elapsed")),
		TTFT: time.Duration(intValue(object, "ttft")), Failed: boolValue(object, "failed"),
		InputTokens: intValue(object, "input_tokens", "inputtokens"), OutputTokens: intValue(object, "output_tokens", "outputtokens"),
		ReasoningTokens: intValue(object, "reasoning_tokens", "reasoningtokens"), CachedTokens: intValue(object, "cached_tokens", "cachedtokens"),
		CacheReadTokens: intValue(object, "cache_read_tokens", "cachereadtokens"), CacheCreationTokens: intValue(object, "cache_creation_tokens", "cachecreationtokens"),
		TotalTokens: intValue(object, "total_tokens", "totaltokens"),
	}
	if cost, ok := upstreamCost(object); ok {
		record.Cost, record.CostProvided = cost, true
	}
	record.Currency = stringValue(object, "currency", "cost_currency", "costcurrency", "billing_currency", "billingcurrency")
	if detail, ok := lookup(object, "detail", "usage_detail", "usagedetail"); ok {
		if detailObject, ok := detail.(map[string]any); ok {
			record.InputTokens = firstInt(record.InputTokens, intValue(detailObject, "input_tokens", "inputtokens"))
			record.OutputTokens = firstInt(record.OutputTokens, intValue(detailObject, "output_tokens", "outputtokens"))
			record.ReasoningTokens = firstInt(record.ReasoningTokens, intValue(detailObject, "reasoning_tokens", "reasoningtokens"))
			record.CachedTokens = firstInt(record.CachedTokens, intValue(detailObject, "cached_tokens", "cachedtokens"))
			record.CacheReadTokens = firstInt(record.CacheReadTokens, intValue(detailObject, "cache_read_tokens", "cachereadtokens"))
			record.CacheCreationTokens = firstInt(record.CacheCreationTokens, intValue(detailObject, "cache_creation_tokens", "cachecreationtokens"))
			record.TotalTokens = firstInt(record.TotalTokens, intValue(detailObject, "total_tokens", "totaltokens"))
			if !record.CostProvided {
				if cost, ok := upstreamCost(detailObject); ok {
					record.Cost, record.CostProvided = cost, true
				}
			}
			if record.Currency == "" {
				record.Currency = stringValue(detailObject, "currency", "cost_currency", "costcurrency", "billing_currency", "billingcurrency")
			}
		}
	}
	if requestedAt := stringValue(object, "requested_at", "requestedat"); requestedAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, requestedAt); err == nil {
			record.RequestedAt = parsed
		}
	}
	store.HandleUsage(record)
	return nil
}

func handleManagement(store *billing.Store, raw []byte) ([]byte, error) {
	var req abi.ManagementRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("decode management request: %w", err)
	}
	path := strings.TrimRight(req.Path, "/")
	switch {
	case path == resourcePath:
		return handleBillingResource(store, req)
	case path == pricingPath:
		return handlePricingResource(store, req)
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/summary"):
		page := billing.ParseInt(queryValue(req.Query, "page"))
		pageSize := billing.ParseInt(queryValue(req.Query, "page_size"))
		return jsonManagementResponse(store.SummaryPage(int(page), int(pageSize)))
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/prices"):
		return jsonManagementResponse(map[string]any{"currency": store.Currency(), "rules": store.Rules()})
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/prices/sync"):
		result, err := syncUpstreamPrices(store)
		if err != nil {
			return jsonManagementError(http.StatusBadGateway, err.Error())
		}
		return jsonManagementResponse(result)
	case req.Method == http.MethodPut && strings.HasSuffix(path, "/prices"):
		var payload struct {
			Rules []billing.PriceRule `json:"rules"`
		}
		if err := json.Unmarshal(req.Body, &payload); err != nil {
			return jsonManagementError(http.StatusBadRequest, err.Error())
		}
		if err := store.SetRules(payload.Rules); err != nil {
			return jsonManagementError(http.StatusBadRequest, err.Error())
		}
		return jsonManagementResponse(map[string]any{"ok": true, "rules": store.Rules()})
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/reset"):
		if err := store.Reset(); err != nil {
			return jsonManagementError(http.StatusInternalServerError, err.Error())
		}
		return jsonManagementResponse(map[string]any{"ok": true})
	default:
		return jsonManagementError(http.StatusNotFound, "unknown management route")
	}
}

func handleBillingResource(store *billing.Store, req abi.ManagementRequest) ([]byte, error) {
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet:
		format := queryValue(req.Query, "format")
		if strings.EqualFold(format, "json") || strings.EqualFold(format, "fallback-json") {
			return jsonManagementError(http.StatusUnauthorized, "management login required")
		}
		page, err := dashboard.RenderBilling(dashboard.Data{})
		if err != nil {
			return nil, err
		}
		return htmlManagementResponse(page)
	default:
		return jsonManagementError(http.StatusMethodNotAllowed, "method not allowed")
	}
}

func handlePricingResource(store *billing.Store, req abi.ManagementRequest) ([]byte, error) {
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet:
		format := queryValue(req.Query, "format")
		if strings.EqualFold(format, "json") || strings.EqualFold(format, "fallback-json") {
			return jsonManagementError(http.StatusUnauthorized, "management login required")
		}
		page, err := dashboard.RenderPricing(dashboard.Data{})
		if err != nil {
			return nil, err
		}
		return htmlManagementResponse(page)
	case http.MethodPut:
		return jsonManagementError(http.StatusMethodNotAllowed, "resource writes are disabled; use the authenticated management API")
	default:
		return jsonManagementError(http.StatusMethodNotAllowed, "method not allowed")
	}
}

func htmlManagementResponse(page []byte) ([]byte, error) {
	return okEnvelope(abi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers: map[string][]string{
			"Content-Type":  {"text/html; charset=utf-8"},
			"Cache-Control": {"no-store"},
		},
		Body: page,
	})
}

func queryValue(query map[string][]string, key string) string {
	for candidate, values := range query {
		if strings.EqualFold(strings.TrimSpace(candidate), key) && len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

func jsonManagementResponse(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return okEnvelope(abi.ManagementResponse{StatusCode: http.StatusOK, Headers: map[string][]string{"Content-Type": {"application/json"}, "Cache-Control": {"no-store"}}, Body: body})
}

func jsonManagementError(status int, message string) ([]byte, error) {
	body, _ := json.Marshal(map[string]any{"error": message})
	return okEnvelope(abi.ManagementResponse{StatusCode: status, Headers: map[string][]string{"Content-Type": {"application/json"}}, Body: body})
}

func okEnvelope(value any) ([]byte, error) {
	result, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(abi.Envelope{OK: true, Result: json.RawMessage(result)})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(abi.Envelope{OK: false, Error: &abi.EnvelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}

func normalizeKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

func lookup(object map[string]any, keys ...string) (any, bool) {
	for key, value := range object {
		for _, wanted := range keys {
			if normalizeKey(key) == normalizeKey(wanted) {
				return value, true
			}
		}
	}
	return nil, false
}

func stringValue(object map[string]any, keys ...string) string {
	value, ok := lookup(object, keys...)
	if !ok {
		return ""
	}
	if stringValue, ok := value.(string); ok {
		return stringValue
	}
	return fmt.Sprint(value)
}

func intValue(object map[string]any, keys ...string) int64 {
	value, ok := lookup(object, keys...)
	if !ok {
		return 0
	}
	return billing.ParseInt(value)
}

func floatValue(object map[string]any, keys ...string) (float64, bool) {
	value, ok := lookup(object, keys...)
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func upstreamCost(object map[string]any) (float64, bool) {
	if cost, ok := floatValue(object, "actual_cost", "actualcost"); ok {
		return cost, true
	}
	if cost, ok := floatValue(object, "total_cost", "totalcost", "cost", "price", "total_price", "totalprice", "estimated_cost", "estimatedcost", "upstream_cost", "upstreamcost", "provider_cost", "providercost", "billing_cost", "billingcost", "amount"); ok {
		return cost, true
	}
	var total float64
	var found bool
	for _, keys := range [][]string{
		{"input_cost", "inputcost"},
		{"output_cost", "outputcost"},
		{"cache_creation_cost", "cachecreationcost"},
		{"cache_read_cost", "cachereadcost"},
	} {
		if cost, ok := floatValue(object, keys...); ok {
			total += cost
			found = true
		}
	}
	return total, found
}

func boolValue(object map[string]any, keys ...string) bool {
	value, ok := lookup(object, keys...)
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func firstInt(current, fallback int64) int64 {
	if current != 0 {
		return current
	}
	return fallback
}
