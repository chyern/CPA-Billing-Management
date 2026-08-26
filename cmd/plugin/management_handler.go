package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/chyern/CPA-Billing-Management/internal/abi"
	"github.com/chyern/CPA-Billing-Management/internal/billing"
	"github.com/chyern/CPA-Billing-Management/internal/dashboard"
)

func handleManagement(store *billing.Store, raw []byte) ([]byte, error) {
	var req abi.ManagementRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("decode management request: %w", err)
	}
	path := strings.TrimRight(req.Path, "/")
	switch {
	case path == resourcePath:
		return handleBillingResource(req)
	case path == pricingPath:
		return handlePricingResource(req)
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/summary"):
		page := billing.ParseInt(queryValue(req.Query, "page"))
		pageSize := billing.ParseInt(queryValue(req.Query, "page_size"))
		return jsonManagementResponse(store.SummaryPage(int(page), int(pageSize)))
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/prices"):
		return pricingSnapshotResponse(store)
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/prices/sync"):
		return handlePricingSync(store, req)
	case req.Method == http.MethodPut && strings.HasSuffix(path, "/prices"):
		return handlePricingUpdate(store, req.Body)
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/reset"):
		if err := store.Reset(); err != nil {
			return jsonManagementError(http.StatusInternalServerError, err.Error())
		}
		return jsonManagementResponse(map[string]any{"ok": true})
	default:
		return jsonManagementError(http.StatusNotFound, "unknown management route")
	}
}

func pricingSnapshotResponse(store *billing.Store) ([]byte, error) {
	return jsonManagementResponse(map[string]any{
		"currency":        store.Currency(),
		"rules":           store.Rules(),
		"default_source":  defaultPricingSourceID,
		"pricing_sources": availablePricingSources(),
	})
}

func handlePricingSync(store *billing.Store, req abi.ManagementRequest) ([]byte, error) {
	sourceID := queryValue(req.Query, "source")
	if sourceID == "" {
		sourceID = defaultPricingSourceID
	}
	if _, ok := findPricingSource(sourceID); !ok {
		return jsonManagementError(http.StatusBadRequest, fmt.Sprintf("unknown pricing source %q", sourceID))
	}
	result, err := syncUpstreamPricesFromSource(store, sourceID)
	if err != nil {
		return jsonManagementError(http.StatusBadGateway, err.Error())
	}
	return jsonManagementResponse(result)
}

func handlePricingUpdate(store *billing.Store, body []byte) ([]byte, error) {
	var payload struct {
		Rules []billing.PriceRule `json:"rules"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return jsonManagementError(http.StatusBadRequest, err.Error())
	}
	if err := store.SetRules(payload.Rules); err != nil {
		return jsonManagementError(http.StatusBadRequest, err.Error())
	}
	return jsonManagementResponse(map[string]any{"ok": true, "rules": store.Rules()})
}

func handleBillingResource(req abi.ManagementRequest) ([]byte, error) {
	if resourceMethod(req) != http.MethodGet {
		return jsonManagementError(http.StatusMethodNotAllowed, "method not allowed")
	}
	if resourceJSONRequested(req) {
		return jsonManagementError(http.StatusUnauthorized, "management login required")
	}
	page, err := dashboard.RenderBilling(dashboard.Data{})
	if err != nil {
		return nil, err
	}
	return htmlManagementResponse(page)
}

func handlePricingResource(req abi.ManagementRequest) ([]byte, error) {
	switch resourceMethod(req) {
	case http.MethodGet:
		if resourceJSONRequested(req) {
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

func resourceMethod(req abi.ManagementRequest) string {
	if req.Method == "" {
		return http.MethodGet
	}
	return req.Method
}

func resourceJSONRequested(req abi.ManagementRequest) bool {
	format := queryValue(req.Query, "format")
	return strings.EqualFold(format, "json") || strings.EqualFold(format, "fallback-json")
}

func queryValue(query map[string][]string, key string) string {
	for candidate, values := range query {
		if strings.EqualFold(strings.TrimSpace(candidate), key) && len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}
