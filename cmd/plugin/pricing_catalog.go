package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
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
	exact           map[string]normalizedModelPrice
	exactPriority   map[string]int
	aliases         map[string]normalizedModelPrice
	aliasesPriority map[string]int
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
	InputCostPerToken           *flexiblePrice `json:"input_cost_per_token"`
	OutputCostPerToken          *flexiblePrice `json:"output_cost_per_token"`
	CacheReadInputTokenCost     *flexiblePrice `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost *flexiblePrice `json:"cache_creation_input_token_cost"`
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
	Input      *flexiblePrice `json:"input"`
	Output     *flexiblePrice `json:"output"`
	CacheRead  *flexiblePrice `json:"cache_read"`
	CacheWrite *flexiblePrice `json:"cache_write"`
}
type openRouterCatalog struct {
	Data []openRouterModel `json:"data"`
}
type openRouterModel struct {
	ID      string            `json:"id"`
	Pricing openRouterPricing `json:"pricing"`
}
type openRouterPricing struct {
	Prompt          *flexiblePrice `json:"prompt"`
	Completion      *flexiblePrice `json:"completion"`
	InputCacheRead  *flexiblePrice `json:"input_cache_read"`
	InputCacheWrite *flexiblePrice `json:"input_cache_write"`
}

func decodeLiteLLMCatalog(reader io.Reader) (*normalizedPriceCatalog, error) {
	var raw map[string]liteLLMModelPrice
	if err := json.NewDecoder(reader).Decode(&raw); err != nil {
		return nil, err
	}
	catalog := newNormalizedPriceCatalog()
	for model, price := range raw {
		if price.InputCostPerToken == nil {
			continue
		}
		if normalized, ok := normalizePriceFields(1_000_000, price.InputCostPerToken, price.OutputCostPerToken, price.CacheReadInputTokenCost, price.CacheCreationInputTokenCost); ok {
			catalog.addExact(model, normalized)
		}
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
		priority := officialModelsDevProviderPriority(providerKey, provider.ID)
		for modelKey, model := range provider.Models {
			if model.Cost.Input == nil {
				continue
			}
			providerIDs, modelIDs := []string{providerKey, provider.ID}, []string{modelKey, model.ID}
			price, ok := normalizePriceFields(1, model.Cost.Input, model.Cost.Output, model.Cost.CacheRead, model.Cost.CacheWrite)
			if !ok {
				continue
			}
			for _, providerID := range providerIDs {
				for _, modelID := range modelIDs {
					if strings.TrimSpace(providerID) != "" && strings.TrimSpace(modelID) != "" {
						catalog.addExactWithPriority(providerID+"/"+modelID, price, priority)
					}
				}
			}
			for _, modelID := range modelIDs {
				catalog.addAliasWithPriority(modelID, price, priority)
				if strings.Contains(modelID, "/") {
					catalog.addExactWithPriority(modelID, price, priority)
					catalog.addAliasWithPriority(lastModelSegment(modelID), price, priority)
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
		if model.Pricing.Prompt == nil {
			continue
		}
		if float64(*model.Pricing.Prompt) == 0 && model.Pricing.Completion != nil && float64(*model.Pricing.Completion) > 0 {
			continue
		}
		price, ok := normalizePriceFields(1_000_000, model.Pricing.Prompt, model.Pricing.Completion, model.Pricing.InputCacheRead, model.Pricing.InputCacheWrite)
		if !ok {
			continue
		}
		catalog.addExact(model.ID, price)
		catalog.addAlias(lastModelSegment(model.ID), price)
	}
	return catalog, nil
}

func newNormalizedPriceCatalog() *normalizedPriceCatalog {
	return &normalizedPriceCatalog{
		exact:           make(map[string]normalizedModelPrice),
		exactPriority:   make(map[string]int),
		aliases:         make(map[string]normalizedModelPrice),
		aliasesPriority: make(map[string]int),
	}
}
func (catalog *normalizedPriceCatalog) addExact(key string, price normalizedModelPrice) {
	catalog.addExactWithPriority(key, price, 0)
}
func (catalog *normalizedPriceCatalog) addAlias(key string, price normalizedModelPrice) {
	catalog.addAliasWithPriority(key, price, 0)
}
func (catalog *normalizedPriceCatalog) addExactWithPriority(key string, price normalizedModelPrice, priority int) {
	key = normalizeCatalogKey(key)
	if key == "" {
		return
	}
	if existing, exists := catalog.exact[key]; exists {
		currentPriority := catalog.exactPriority[key]
		if priority < currentPriority || (priority == currentPriority && !preferNormalizedPrice(price, existing)) {
			return
		}
	}
	catalog.exact[key] = price
	catalog.exactPriority[key] = priority
}
func (catalog *normalizedPriceCatalog) addAliasWithPriority(key string, price normalizedModelPrice, priority int) {
	key = normalizeCatalogKey(key)
	if key == "" {
		return
	}
	if existing, exists := catalog.aliases[key]; exists {
		currentPriority := catalog.aliasesPriority[key]
		// Official model-owner entries always win. Price comparison is only a
		// deterministic fallback between entries at the same trust level.
		if priority < currentPriority || (priority == currentPriority && !preferNormalizedPrice(price, existing)) {
			return
		}
	}
	catalog.aliases[key] = price
	catalog.aliasesPriority[key] = priority
}

func officialModelsDevProviderPriority(providerIDs ...string) int {
	for _, providerID := range providerIDs {
		switch normalizeCatalogKey(providerID) {
		case "openai", "anthropic", "google", "mistral", "cohere", "xai", "deepseek":
			return 100
		}
	}
	return 0
}

func preferNormalizedPrice(next, current normalizedModelPrice) bool {
	nextNonZero := next.InputPerMillion > 0
	currentNonZero := current.InputPerMillion > 0
	if nextNonZero != currentNonZero {
		return nextNonZero
	}
	if nextNonZero && next.InputPerMillion != current.InputPerMillion {
		return next.InputPerMillion < current.InputPerMillion
	}
	if next.OutputPerMillion != current.OutputPerMillion {
		return next.OutputPerMillion < current.OutputPerMillion
	}
	if next.CacheReadPerMillion != current.CacheReadPerMillion {
		return next.CacheReadPerMillion < current.CacheReadPerMillion
	}
	return next.CacheCreationPerMillion < current.CacheCreationPerMillion
}
func (catalog *normalizedPriceCatalog) lookup(provider, model string) (normalizedModelPrice, bool) {
	model, provider = normalizeCatalogKey(model), normalizeCatalogKey(provider)
	if model != "" {
		for _, candidate := range catalogProviderCandidates(provider) {
			if price, ok := catalog.exact[candidate+"/"+model]; ok {
				return price, true
			}
		}
		if provider != "" {
			if price, ok := catalog.exact[provider+"/"+model]; ok {
				return price, true
			}
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

func catalogProviderCandidates(provider string) []string {
	provider = normalizeCatalogKey(provider)
	switch provider {
	case "codex":
		return []string{"openai"}
	case "claude":
		return []string{"anthropic"}
	case "gemini", "vertex", "aistudio", "antigravity":
		return []string{"google"}
	default:
		if provider != "" {
			return []string{provider}
		}
		return nil
	}
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
func normalizePriceFields(scale float64, fields ...*flexiblePrice) (normalizedModelPrice, bool) {
	values := make([]float64, len(fields))
	present := false
	for i, field := range fields {
		if field == nil {
			continue
		}
		present = true
		value := float64(*field) * scale
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return normalizedModelPrice{}, false
		}
		values[i] = value
	}
	if !present {
		return normalizedModelPrice{}, false
	}
	return normalizedModelPrice{InputPerMillion: values[0], OutputPerMillion: values[1], CacheReadPerMillion: values[2], CacheCreationPerMillion: values[3]}, true
}
