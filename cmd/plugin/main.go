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
	pluginVersion = "0.1.4"
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
				{Path: "/billing", Menu: "费用统计", Description: "按模型查看 CLIProxyAPI token 用量和估算费用。"},
				{Path: "/pricing", Menu: "价格配置", Description: "配置模型的 token 估算价格。"},
			},
			Routes: []abi.ManagementRoute{
				{Method: http.MethodGet, Path: "/cpa-billing-management/summary"},
				{Method: http.MethodGet, Path: "/cpa-billing-management/prices"},
				{Method: http.MethodPut, Path: "/cpa-billing-management/prices"},
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
				{Name: "currency", Type: "string", Description: "费用展示币种，默认 USD。价格数值按该币种计。"},
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
	if detail, ok := lookup(object, "detail", "usage_detail", "usagedetail"); ok {
		if detailObject, ok := detail.(map[string]any); ok {
			record.InputTokens = firstInt(record.InputTokens, intValue(detailObject, "input_tokens", "inputtokens"))
			record.OutputTokens = firstInt(record.OutputTokens, intValue(detailObject, "output_tokens", "outputtokens"))
			record.ReasoningTokens = firstInt(record.ReasoningTokens, intValue(detailObject, "reasoning_tokens", "reasoningtokens"))
			record.CachedTokens = firstInt(record.CachedTokens, intValue(detailObject, "cached_tokens", "cachedtokens"))
			record.CacheReadTokens = firstInt(record.CacheReadTokens, intValue(detailObject, "cache_read_tokens", "cachereadtokens"))
			record.CacheCreationTokens = firstInt(record.CacheCreationTokens, intValue(detailObject, "cache_creation_tokens", "cachecreationtokens"))
			record.TotalTokens = firstInt(record.TotalTokens, intValue(detailObject, "total_tokens", "totaltokens"))
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
		return jsonManagementResponse(store.Summary())
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/prices"):
		return jsonManagementResponse(map[string]any{"currency": store.Currency(), "rules": store.Rules()})
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
		pageNumber := billing.ParseInt(queryValue(req.Query, "page"))
		pageSize := billing.ParseInt(queryValue(req.Query, "page_size"))
		summary := store.SummaryPage(int(pageNumber), int(pageSize))
		if strings.EqualFold(queryValue(req.Query, "format"), "json") {
			return jsonManagementResponse(map[string]any{"summary": summary})
		}
		page, err := dashboard.RenderBilling(dashboard.Data{Summary: summary})
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
	snapshot := func() map[string]any { return map[string]any{"currency": store.Currency(), "rules": store.Rules()} }
	switch method {
	case http.MethodGet:
		if strings.EqualFold(queryValue(req.Query, "format"), "json") {
			return jsonManagementResponse(snapshot())
		}
		page, err := dashboard.RenderPricing(dashboard.Data{Rules: store.Rules()})
		if err != nil {
			return nil, err
		}
		return htmlManagementResponse(page)
	case http.MethodPut:
		var payload struct {
			Rules []billing.PriceRule `json:"rules"`
		}
		if err := json.Unmarshal(req.Body, &payload); err != nil {
			return jsonManagementError(http.StatusBadRequest, err.Error())
		}
		if err := store.SetRules(payload.Rules); err != nil {
			return jsonManagementError(http.StatusBadRequest, err.Error())
		}
		return jsonManagementResponse(snapshot())
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
