package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	case path == balancesPath:
		return handleBalancesResource(req)
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/summary"):
		page := billing.ParseInt(queryValue(req.Query, "page"))
		pageSize := billing.ParseInt(queryValue(req.Query, "page_size"))
		start, end, err := summaryTimeRange(req.Query)
		if err != nil {
			return jsonManagementError(http.StatusBadRequest, err.Error())
		}
		return jsonManagementResponse(store.SummaryPageRange(int(page), int(pageSize), start, end))
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/prices"):
		return pricingSnapshotResponse(store)
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/prices/sync"):
		return handlePricingSync(store, req)
	case req.Method == http.MethodPut && strings.HasSuffix(path, "/prices"):
		return handlePricingUpdate(store, req.Body)
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/key-balances"):
		return keyBalancesResponse(store)
	case req.Method == http.MethodPut && strings.HasSuffix(path, "/key-balances"):
		return handleKeyBalancesUpdate(store, req.Body)
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/reset"):
		if err := store.Reset(); err != nil {
			return jsonManagementError(http.StatusInternalServerError, err.Error())
		}
		return jsonManagementResponse(map[string]any{"ok": true})
	default:
		return jsonManagementError(http.StatusNotFound, "unknown management route")
	}
}

func keyBalancesResponse(store *billing.Store) ([]byte, error) {
	balances, err := store.KeyBalances()
	if err != nil {
		return jsonManagementError(http.StatusInternalServerError, err.Error())
	}
	return jsonManagementResponse(map[string]any{"currency": store.Currency(), "balances": balances})
}

func handleKeyBalancesUpdate(store *billing.Store, body []byte) ([]byte, error) {
	var payload struct {
		Balances []billing.APIKeyBalance  `json:"balances"`
		Notes    *[]billing.APIKeyBalance `json:"notes"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return jsonManagementError(http.StatusBadRequest, err.Error())
	}
	if err := store.SetKeyBalances(payload.Balances); err != nil {
		return jsonManagementError(http.StatusBadRequest, err.Error())
	}
	if payload.Notes != nil {
		if err := store.SetKeyBalanceNotes(*payload.Notes); err != nil {
			return jsonManagementError(http.StatusBadRequest, err.Error())
		}
	}
	return keyBalancesResponse(store)
}

func summaryTimeRange(query map[string][]string) (time.Time, time.Time, error) {
	start, err := parseSummaryDate(queryValue(query, "start"))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start date: %w", err)
	}
	end, err := parseSummaryDate(queryValue(query, "end"))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end date: %w", err)
	}
	if !start.IsZero() && !end.IsZero() && end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("end date must not be before start date")
	}
	if !end.IsZero() {
		end = end.AddDate(0, 0, 1)
	}
	return start, end, nil
}

func parseSummaryDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	return time.ParseInLocation("2006-01-02", value, time.Local)
}

func pricingSnapshotResponse(store *billing.Store) ([]byte, error) {
	return jsonManagementResponse(map[string]any{
		"currency":        store.Currency(),
		"rules":           store.Rules(),
		"models":          []pricingCatalogModel{},
		"default_source":  defaultPricingSourceID,
		"pricing_sources": availablePricingSources(),
	})
}

type pricingCatalogModel struct {
	Provider   string            `json:"provider"`
	Model      string            `json:"model"`
	Configured bool              `json:"configured"`
	Price      billing.PriceRule `json:"price"`
}

func handlePricingSync(store *billing.Store, req abi.ManagementRequest) ([]byte, error) {
	sourceID := queryValue(req.Query, "source")
	if sourceID == "" {
		sourceID = defaultPricingSourceID
	}
	if _, ok := findPricingSource(sourceID); !ok {
		return jsonManagementError(http.StatusBadRequest, fmt.Sprintf("unknown pricing source %q", sourceID))
	}
	source, _ := findPricingSource(sourceID)
	preview := queryValue(req.Query, "preview") == "1"
	if !preview {
		return jsonManagementError(http.StatusBadRequest, "preview=1 is required; save the returned rules explicitly")
	}
	var result upstreamSyncResult
	rules := store.Rules()
	var served []syncModel
	if len(req.Body) > 0 {
		var payload struct {
			Rules  []billing.PriceRule   `json:"rules"`
			Models []pricingCatalogModel `json:"models"`
		}
		if decodeErr := json.Unmarshal(req.Body, &payload); decodeErr != nil {
			return jsonManagementError(http.StatusBadRequest, decodeErr.Error())
		}
		rules = payload.Rules
		for _, model := range payload.Models {
			if name := strings.TrimSpace(model.Model); name != "" {
				served = append(served, syncModel{Model: name})
			}
		}
	}
	result, err := previewUpstreamPricesFromSourceWithClient(store, &http.Client{Timeout: 15 * time.Second}, source, rules, served)
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
	if req.Method != http.MethodGet {
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
	switch req.Method {
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

func handleBalancesResource(req abi.ManagementRequest) ([]byte, error) {
	if req.Method != http.MethodGet {
		return jsonManagementError(http.StatusMethodNotAllowed, "method not allowed")
	}
	if resourceJSONRequested(req) {
		return jsonManagementError(http.StatusUnauthorized, "management login required")
	}
	page, err := dashboard.RenderBalances(dashboard.Data{})
	if err != nil {
		return nil, err
	}
	return htmlManagementResponse(page)
}

func resourceJSONRequested(req abi.ManagementRequest) bool {
	return queryValue(req.Query, "format") == "json"
}

func queryValue(query map[string][]string, key string) string {
	values := query[key]
	if len(values) > 0 {
		return strings.TrimSpace(values[0])
	}
	return ""
}
