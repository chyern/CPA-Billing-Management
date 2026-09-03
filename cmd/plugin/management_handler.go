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
	rules := pricingRulesWithObservedModels(store)
	return jsonManagementResponse(map[string]any{
		"currency":        store.Currency(),
		"rules":           rules,
		"models":          observedPricingModels(store),
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

func observedPricingModels(store *billing.Store) []pricingCatalogModel {
	models := store.SummaryPage(1, 100).Models
	result := make([]pricingCatalogModel, 0, len(models))
	for _, model := range models {
		if model == nil || strings.TrimSpace(model.Model) == "" {
			continue
		}
		price, configured := store.ResolvePriceRule(model.Provider, model.Model)
		if !configured {
			price = billing.PriceRule{Match: model.Model}
		}
		result = append(result, pricingCatalogModel{Provider: model.Provider, Model: model.Model, Configured: configured, Price: price})
	}
	return result
}

// pricingRulesWithObservedModels keeps the editor useful before every model
// has a manual price. Unconfigured observed models are shown at zero, while
// existing exact, alias, and wildcard rules remain effective.
func pricingRulesWithObservedModels(store *billing.Store) []billing.PriceRule {
	rules := store.Rules()
	models := store.SummaryPage(1, 100).Models
	for _, model := range models {
		if model == nil {
			continue
		}
		if _, configured := store.ResolvePriceRule(model.Provider, model.Model); configured {
			continue
		}
		match := strings.TrimSpace(model.Model)
		if provider := strings.TrimSpace(model.Provider); provider != "" {
			match = provider + "/" + match
		}
		if findRule(rules, match) >= 0 {
			continue
		}
		zeroRule := billing.PriceRule{Match: match}
		insertAt := len(rules)
		for i, rule := range rules {
			if strings.TrimSpace(rule.Match) == "*" {
				insertAt = i
				break
			}
		}
		rules = append(rules, billing.PriceRule{})
		copy(rules[insertAt+1:], rules[insertAt:])
		rules[insertAt] = zeroRule
	}
	return rules
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
	preview := queryValue(req.Query, "preview") == "1" || strings.EqualFold(queryValue(req.Query, "preview"), "true")
	var result upstreamSyncResult
	var err error
	if preview {
		rules := store.Rules()
		if len(req.Body) > 0 {
			var payload struct {
				Rules []billing.PriceRule `json:"rules"`
			}
			if decodeErr := json.Unmarshal(req.Body, &payload); decodeErr != nil {
				return jsonManagementError(http.StatusBadRequest, decodeErr.Error())
			}
			rules = payload.Rules
		}
		result, err = previewUpstreamPricesFromSourceWithClient(store, &http.Client{Timeout: 15 * time.Second}, source, rules)
	} else {
		result, err = syncUpstreamPricesFromSource(store, sourceID)
	}
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
