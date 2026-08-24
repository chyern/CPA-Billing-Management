package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chyern/CPA-Billing-Management/internal/billing"
)

// LiteLLM maintains a public model-price catalog with per-token prices. The
// sync action only downloads this catalog; no local usage data is sent out.
const upstreamPricingCatalogURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

type upstreamModelPrice struct {
	InputCostPerToken           float64 `json:"input_cost_per_token"`
	OutputCostPerToken          float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost     float64 `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost float64 `json:"cache_creation_input_token_cost"`
}

type upstreamSyncResult struct {
	Source  string              `json:"source"`
	Matched int                 `json:"matched"`
	Added   int                 `json:"added"`
	Updated int                 `json:"updated"`
	Rules   []billing.PriceRule `json:"rules"`
}

func syncUpstreamPrices(store *billing.Store) (upstreamSyncResult, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	return syncUpstreamPricesWithClient(store, client, upstreamPricingCatalogURL)
}

func syncUpstreamPricesWithClient(store *billing.Store, client *http.Client, source string) (upstreamSyncResult, error) {
	result := upstreamSyncResult{Source: source}
	if store == nil {
		return result, fmt.Errorf("billing store is unavailable")
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Get(source)
	if err != nil {
		return result, fmt.Errorf("download upstream price catalog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return result, fmt.Errorf("upstream price catalog returned %s", resp.Status)
	}
	var catalog map[string]upstreamModelPrice
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&catalog); err != nil {
		return result, fmt.Errorf("decode upstream price catalog: %w", err)
	}

	rules := store.Rules()
	models := store.SummaryPage(1, 100).Models
	for _, model := range models {
		if model == nil {
			continue
		}
		price, ok := lookupUpstreamPrice(catalog, model.Provider, model.Model)
		if !ok || !hasUpstreamPrice(price) {
			continue
		}
		result.Matched++
		match := strings.TrimSpace(model.Model)
		if provider := strings.TrimSpace(model.Provider); provider != "" {
			match = provider + "/" + match
		}
		updatedRule := billing.PriceRule{
			Match:                   match,
			InputPerMillion:         price.InputCostPerToken * 1_000_000,
			OutputPerMillion:        price.OutputCostPerToken * 1_000_000,
			CacheReadPerMillion:     price.CacheReadInputTokenCost * 1_000_000,
			CacheCreationPerMillion: price.CacheCreationInputTokenCost * 1_000_000,
		}
		index := findRule(rules, match)
		if index >= 0 {
			if rules[index] != updatedRule {
				rules[index] = updatedRule
				result.Updated++
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
	}
	if result.Added > 0 || result.Updated > 0 {
		if err := store.SetRules(rules); err != nil {
			return result, err
		}
	}
	result.Rules = store.Rules()
	return result, nil
}

func lookupUpstreamPrice(catalog map[string]upstreamModelPrice, provider, model string) (upstreamModelPrice, bool) {
	keys := []string{model}
	if strings.TrimSpace(provider) != "" {
		keys = append([]string{provider + "/" + model}, keys...)
	}
	for _, key := range keys {
		for candidate, price := range catalog {
			if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(key)) {
				return price, true
			}
		}
	}
	return upstreamModelPrice{}, false
}

func hasUpstreamPrice(price upstreamModelPrice) bool {
	return price.InputCostPerToken > 0 || price.OutputCostPerToken > 0 || price.CacheReadInputTokenCost > 0 || price.CacheCreationInputTokenCost > 0
}

func findRule(rules []billing.PriceRule, match string) int {
	for i, rule := range rules {
		if strings.EqualFold(strings.TrimSpace(rule.Match), strings.TrimSpace(match)) {
			return i
		}
	}
	return -1
}
