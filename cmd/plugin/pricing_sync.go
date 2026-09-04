package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chyern/CPA-Billing-Management/internal/billing"
)

const (
	defaultPricingSourceID   = "litellm"
	maxPricingCatalogBytes   = 10 << 20
	pricingRequestRetryCount = 3
)

type pricingSource struct {
	ID     string
	Name   string
	URL    string
	Decode func(io.Reader) (*normalizedPriceCatalog, error)
}

type pricingSourceDescriptor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var pricingSources = []pricingSource{
	{ID: "litellm", Name: "LiteLLM", URL: "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json", Decode: decodeLiteLLMCatalog},
	{ID: "models.dev", Name: "Models.dev", URL: "https://models.dev/api.json", Decode: decodeModelsDevCatalog},
	{ID: "openrouter", Name: "OpenRouter", URL: "https://openrouter.ai/api/v1/models", Decode: decodeOpenRouterCatalog},
}

type upstreamSyncResult struct {
	Source     string              `json:"source"`
	SourceID   string              `json:"source_id"`
	SourceName string              `json:"source_name"`
	Applied    bool                `json:"applied"`
	Matched    int                 `json:"matched"`
	Added      int                 `json:"added"`
	Updated    int                 `json:"updated"`
	Unchanged  int                 `json:"unchanged"`
	Changes    []pricingSyncChange `json:"changes"`
	Rules      []billing.PriceRule `json:"rules"`
}

type pricingSyncChange struct {
	Action   string             `json:"action"`
	Match    string             `json:"match"`
	Current  *billing.PriceRule `json:"current,omitempty"`
	Upstream billing.PriceRule  `json:"upstream"`
}

func availablePricingSources() []pricingSourceDescriptor {
	result := make([]pricingSourceDescriptor, 0, len(pricingSources))
	for _, source := range pricingSources {
		result = append(result, pricingSourceDescriptor{ID: source.ID, Name: source.Name})
	}
	return result
}

func findPricingSource(id string) (pricingSource, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		id = defaultPricingSourceID
	}
	for _, source := range pricingSources {
		if strings.EqualFold(source.ID, id) {
			return source, true
		}
	}
	return pricingSource{}, false
}

func syncUpstreamPrices(store *billing.Store) (upstreamSyncResult, error) {
	return syncUpstreamPricesFromSource(store, defaultPricingSourceID)
}

func syncUpstreamPricesFromSource(store *billing.Store, sourceID string) (upstreamSyncResult, error) {
	source, ok := findPricingSource(sourceID)
	if !ok {
		return upstreamSyncResult{}, fmt.Errorf("unknown pricing source %q", sourceID)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	return syncUpstreamPricesFromSourceWithClient(store, client, source)
}

func syncUpstreamPricesFromSourceWithClient(store *billing.Store, client *http.Client, source pricingSource) (upstreamSyncResult, error) {
	var rules []billing.PriceRule
	if store != nil {
		rules = store.Rules()
	}
	return reconcileUpstreamPricesFromSourceWithClient(store, client, source, rules, true)
}

func previewUpstreamPricesFromSourceWithClient(store *billing.Store, client *http.Client, source pricingSource, rules []billing.PriceRule) (upstreamSyncResult, error) {
	return reconcileUpstreamPricesFromSourceWithClient(store, client, source, rules, false)
}

func reconcileUpstreamPricesFromSourceWithClient(store *billing.Store, client *http.Client, source pricingSource, rules []billing.PriceRule, apply bool) (upstreamSyncResult, error) {
	result := upstreamSyncResult{Source: source.URL, SourceID: source.ID, SourceName: source.Name}
	if store == nil {
		return result, fmt.Errorf("billing store is unavailable")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if source.Decode == nil {
		return result, fmt.Errorf("pricing source %q has no decoder", source.ID)
	}

	var response *http.Response
	var err error
	for attempt := 0; attempt < pricingRequestRetryCount; attempt++ {
		request, requestErr := http.NewRequest(http.MethodGet, source.URL, nil)
		if requestErr != nil {
			return result, fmt.Errorf("create %s price request: %w", source.Name, requestErr)
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", "cpa-billing-management/"+pluginVersion)
		response, err = client.Do(request)
		if err == nil {
			break
		}
		if attempt+1 < pricingRequestRetryCount {
			time.Sleep(time.Duration(200*(1<<attempt)) * time.Millisecond)
		}
	}
	if err != nil {
		return result, fmt.Errorf("download %s price catalog: %w", source.Name, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return result, fmt.Errorf("%s price catalog returned %s", source.Name, response.Status)
	}
	catalog, err := source.Decode(io.LimitReader(response.Body, maxPricingCatalogBytes))
	if err != nil {
		return result, fmt.Errorf("decode %s price catalog: %w", source.Name, err)
	}
	if catalog == nil || catalog.empty() {
		return result, fmt.Errorf("%s price catalog is empty", source.Name)
	}

	rules = append([]billing.PriceRule(nil), rules...)
	models := syncModels(rules)
	for _, model := range models {
		price, ok := catalog.lookup(model.Model)
		if !ok {
			continue
		}
		result.Matched++
		updatedRule := billing.PriceRule{
			Match:                   model.Model,
			InputPerMillion:         price.InputPerMillion,
			OutputPerMillion:        price.OutputPerMillion,
			CacheReadPerMillion:     price.CacheReadPerMillion,
			CacheCreationPerMillion: price.CacheCreationPerMillion,
		}
		index := findRuleByModel(rules, model.Model)
		if index >= 0 {
			// Preserve an existing rule's spelling while updating its price. This
			// avoids rewriting user data; matching itself is model-name based.
			match := strings.TrimSpace(rules[index].Match)
			updatedRule.Match = match
			if rules[index] != updatedRule {
				current := rules[index]
				rules[index] = updatedRule
				result.Updated++
				result.Changes = append(result.Changes, pricingSyncChange{Action: "update", Match: match, Current: &current, Upstream: updatedRule})
			} else {
				result.Unchanged++
			}
			continue
		}
		insertAt := len(rules)
		for i, rule := range rules {
			if strings.TrimSpace(rule.Match) == "*" {
				insertAt = i
				break
			}
		}
		rules = append(rules, billing.PriceRule{})
		copy(rules[insertAt+1:], rules[insertAt:])
		rules[insertAt] = updatedRule
		result.Added++
		result.Changes = append(result.Changes, pricingSyncChange{Action: "add", Match: model.Model, Upstream: updatedRule})
	}
	if apply && (result.Added > 0 || result.Updated > 0) {
		if err := store.SetRules(rules); err != nil {
			return result, err
		}
	}
	result.Applied = apply
	result.Rules = rules
	return result, nil
}

type syncModel struct {
	Model string
}

// syncModels includes only models currently present in the pricing editor.
// Usage events are intentionally independent from pricing-rule maintenance.
func syncModels(rules []billing.PriceRule) []syncModel {
	result := make([]syncModel, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		key := strings.ToLower(model)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		result = append(result, syncModel{Model: model})
	}
	for _, rule := range rules {
		match := strings.TrimSpace(rule.Match)
		if match == "" || match == "*" {
			continue
		}
		if index := strings.LastIndex(match, "/"); index >= 0 {
			match = strings.TrimSpace(match[index+1:])
		}
		add(match)
	}
	return result
}

func findRuleByModel(rules []billing.PriceRule, model string) int {
	model = strings.TrimSpace(model)
	for i, rule := range rules {
		match := strings.TrimSpace(rule.Match)
		if match == "" || match == "*" {
			continue
		}
		if strings.EqualFold(match, model) {
			return i
		}
		if index := strings.LastIndex(match, "/"); index >= 0 {
			match = strings.TrimSpace(match[index+1:])
		}
		if strings.EqualFold(match, model) {
			return i
		}
	}
	return -1
}
