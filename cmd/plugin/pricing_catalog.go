package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type normalizedModelPrice struct {
	InputPerMillion         float64
	OutputPerMillion        float64
	CacheReadPerMillion     float64
	CacheCreationPerMillion float64
}

type normalizedPriceCatalog struct {
	exact            map[string]normalizedModelPrice
	aliases          map[string]normalizedModelPrice
	ambiguousAliases map[string]struct{}
}

type flexiblePrice float64

func (p *flexiblePrice) UnmarshalJSON(raw []byte) error {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		*p = 0
		return nil
	}
	if strings.HasPrefix(value, `"`) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		value = strings.TrimSpace(text)
		if value == "" {
			*p = 0
			return nil
		}
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("parse price %q: %w", value, err)
	}
	*p = flexiblePrice(parsed)
	return nil
}

type liteLLMModelPrice struct {
	InputCostPerToken           flexiblePrice `json:"input_cost_per_token"`
	OutputCostPerToken          flexiblePrice `json:"output_cost_per_token"`
	CacheReadInputTokenCost     flexiblePrice `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost flexiblePrice `json:"cache_creation_input_token_cost"`
}
type modelsDevProvider struct {
	ID     string                    `json:"id"`
	Models map[string]modelsDevModel `json:"models"`
}
type modelsDevModel struct {
	ID   string        `json:"id"`
	Cost modelsDevCost `json:"cost"`
}
type modelsDevCost struct {
	Input      flexiblePrice `json:"input"`
	Output     flexiblePrice `json:"output"`
	CacheRead  flexiblePrice `json:"cache_read"`
	CacheWrite flexiblePrice `json:"cache_write"`
}
type openRouterCatalog struct {
	Data []openRouterModel `json:"data"`
}
type openRouterModel struct {
	ID      string            `json:"id"`
	Pricing openRouterPricing `json:"pricing"`
}
type openRouterPricing struct {
	Prompt          flexiblePrice `json:"prompt"`
	Completion      flexiblePrice `json:"completion"`
	InputCacheRead  flexiblePrice `json:"input_cache_read"`
	InputCacheWrite flexiblePrice `json:"input_cache_write"`
}

func decodeLiteLLMCatalog(reader io.Reader) (*normalizedPriceCatalog, error) {
	var raw map[string]liteLLMModelPrice
	if err := json.NewDecoder(reader).Decode(&raw); err != nil {
		return nil, err
	}
	catalog := newNormalizedPriceCatalog()
	for model, price := range raw {
		catalog.addExact(model, normalizedModelPrice{InputPerMillion: float64(price.InputCostPerToken) * 1_000_000, OutputPerMillion: float64(price.OutputCostPerToken) * 1_000_000, CacheReadPerMillion: float64(price.CacheReadInputTokenCost) * 1_000_000, CacheCreationPerMillion: float64(price.CacheCreationInputTokenCost) * 1_000_000})
	}
	return catalog, nil
}

func decodeModelsDevCatalog(reader io.Reader) (*normalizedPriceCatalog, error) {
	var raw map[string]modelsDevProvider
	if err := json.NewDecoder(reader).Decode(&raw); err != nil {
		return nil, err
	}
	catalog := newNormalizedPriceCatalog()
	for providerKey, provider := range raw {
		for modelKey, model := range provider.Models {
			providerIDs, modelIDs := []string{providerKey, provider.ID}, []string{modelKey, model.ID}
			price := normalizedModelPrice{InputPerMillion: float64(model.Cost.Input), OutputPerMillion: float64(model.Cost.Output), CacheReadPerMillion: float64(model.Cost.CacheRead), CacheCreationPerMillion: float64(model.Cost.CacheWrite)}
			for _, providerID := range providerIDs {
				for _, modelID := range modelIDs {
					if strings.TrimSpace(providerID) != "" && strings.TrimSpace(modelID) != "" {
						catalog.addExact(providerID+"/"+modelID, price)
					}
				}
			}
			for _, modelID := range modelIDs {
				catalog.addAlias(modelID, price)
				if strings.Contains(modelID, "/") {
					catalog.addExact(modelID, price)
					catalog.addAlias(lastModelSegment(modelID), price)
				}
			}
		}
	}
	return catalog, nil
}

func decodeOpenRouterCatalog(reader io.Reader) (*normalizedPriceCatalog, error) {
	var raw openRouterCatalog
	if err := json.NewDecoder(reader).Decode(&raw); err != nil {
		return nil, err
	}
	catalog := newNormalizedPriceCatalog()
	for _, model := range raw.Data {
		price := normalizedModelPrice{InputPerMillion: float64(model.Pricing.Prompt) * 1_000_000, OutputPerMillion: float64(model.Pricing.Completion) * 1_000_000, CacheReadPerMillion: float64(model.Pricing.InputCacheRead) * 1_000_000, CacheCreationPerMillion: float64(model.Pricing.InputCacheWrite) * 1_000_000}
		catalog.addExact(model.ID, price)
		catalog.addAlias(lastModelSegment(model.ID), price)
	}
	return catalog, nil
}

func newNormalizedPriceCatalog() *normalizedPriceCatalog {
	return &normalizedPriceCatalog{exact: make(map[string]normalizedModelPrice), aliases: make(map[string]normalizedModelPrice), ambiguousAliases: make(map[string]struct{})}
}
func (catalog *normalizedPriceCatalog) addExact(key string, price normalizedModelPrice) {
	if key = normalizeCatalogKey(key); key != "" {
		catalog.exact[key] = price
	}
}
func (catalog *normalizedPriceCatalog) addAlias(key string, price normalizedModelPrice) {
	key = normalizeCatalogKey(key)
	if key == "" {
		return
	}
	if _, ambiguous := catalog.ambiguousAliases[key]; ambiguous {
		return
	}
	if existing, exists := catalog.aliases[key]; exists && existing != price {
		delete(catalog.aliases, key)
		catalog.ambiguousAliases[key] = struct{}{}
		return
	}
	catalog.aliases[key] = price
}
func (catalog *normalizedPriceCatalog) lookup(provider, model string) (normalizedModelPrice, bool) {
	model, provider = normalizeCatalogKey(model), normalizeCatalogKey(provider)
	if provider != "" && model != "" {
		if price, ok := catalog.exact[provider+"/"+model]; ok {
			return price, true
		}
	}
	if price, ok := catalog.exact[model]; ok {
		return price, true
	}
	if price, ok := catalog.aliases[model]; ok {
		return price, true
	}
	if price, ok := catalog.aliases[lastModelSegment(model)]; ok {
		return price, true
	}
	return normalizedModelPrice{}, false
}
func (catalog *normalizedPriceCatalog) empty() bool {
	return len(catalog.exact) == 0 && len(catalog.aliases) == 0
}
func normalizeCatalogKey(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
func lastModelSegment(value string) string {
	value = normalizeCatalogKey(value)
	if index := strings.LastIndex(value, "/"); index >= 0 {
		return value[index+1:]
	}
	return value
}
func hasNormalizedPrice(price normalizedModelPrice) bool {
	return price.InputPerMillion > 0 || price.OutputPerMillion > 0 || price.CacheReadPerMillion > 0 || price.CacheCreationPerMillion > 0
}
