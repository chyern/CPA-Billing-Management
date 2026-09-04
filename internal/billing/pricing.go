package billing

import (
	"fmt"
	"math"
	"strings"
)

func CalculateCost(record UsageRecord, rule PriceRule) float64 {
	cacheRead := record.CacheReadTokens
	cacheCreation := record.CacheCreationTokens
	cacheBreakdown := cacheRead + cacheCreation
	if record.CachedTokens > cacheBreakdown {
		cacheRead += record.CachedTokens - cacheBreakdown
		cacheBreakdown = record.CachedTokens
	}
	input := record.InputTokens - cacheBreakdown
	if input < 0 {
		input = 0
	}
	return (float64(input)/1_000_000)*rule.InputPerMillion + (float64(record.OutputTokens)/1_000_000)*rule.OutputPerMillion + (float64(cacheRead)/1_000_000)*rule.CacheReadPerMillion + (float64(cacheCreation)/1_000_000)*rule.CacheCreationPerMillion
}

func (s *Store) matchRule(record UsageRecord) (PriceRule, bool) {
	// 价格规则只按模型名匹配，provider 仅用于统计维度，不参与计费。
	// 规则表中如果仍有 provider/model 形式，取最后一个斜杠后的模型名，
	// 这样不会因为上游 provider 与 CLIProxyAPI 的 owned_by 不一致而漏计费。
	model := strings.TrimSpace(record.Model)
	keys := []string{model, strings.TrimSpace(record.Alias), "*"}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		for _, rule := range s.state.Rules {
			ruleMatch := strings.TrimSpace(rule.Match)
			matches := strings.EqualFold(ruleMatch, key)
			if !matches && key != "*" && strings.Contains(ruleMatch, "/") {
				matches = strings.EqualFold(modelNameFromRuleMatch(ruleMatch), key)
			}
			if matches {
				priced := key != "*" || rule.InputPerMillion > 0 || rule.OutputPerMillion > 0 || rule.CacheReadPerMillion > 0 || rule.CacheCreationPerMillion > 0
				return rule, priced
			}
		}
	}
	return PriceRule{Match: "*"}, false
}

func modelNameFromRuleMatch(match string) string {
	match = strings.TrimSpace(match)
	if index := strings.LastIndex(match, "/"); index >= 0 {
		return strings.TrimSpace(match[index+1:])
	}
	return match
}

func (s *Store) Rules() []PriceRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]PriceRule(nil), s.state.Rules...)
}

// ResolvePriceRule returns the effective rule for a model name. The provider
// argument is retained for callers that already pass a provider, but ignored.
func (s *Store) ResolvePriceRule(_ string, model string) (PriceRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.matchRule(UsageRecord{Model: model})
}

func (s *Store) SetRules(rules []PriceRule) error {
	if err := validateRules(rules); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Rules = append([]PriceRule{}, rules...)
	if err := s.persistFullStateLocked(); err != nil {
		s.lastErr = err
		return err
	}
	return nil
}

func validateRules(rules []PriceRule) error {
	seen := make(map[string]struct{}, len(rules))
	for i := range rules {
		rules[i].Match = strings.TrimSpace(rules[i].Match)
		if rules[i].Match == "" {
			return fmt.Errorf("pricing rule %d has an empty match", i)
		}
		key := strings.ToLower(rules[i].Match)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate pricing rule %q", rules[i].Match)
		}
		seen[key] = struct{}{}
		for _, price := range []float64{rules[i].InputPerMillion, rules[i].OutputPerMillion, rules[i].CacheReadPerMillion, rules[i].CacheCreationPerMillion} {
			if math.IsNaN(price) || math.IsInf(price, 0) || price < 0 {
				return fmt.Errorf("pricing rule %q contains an invalid price", rules[i].Match)
			}
		}
	}
	return nil
}

func (s *Store) addModelAggregateLocked(event UsageEvent, priced bool) {
	key := aggregateKey(event.Provider, event.Model)
	aggregate := s.state.Aggregates[key]
	if aggregate == nil {
		aggregate = &Aggregate{Provider: event.Provider, Model: event.Model, Priced: true}
		s.state.Aggregates[key] = aggregate
	}
	aggregate.Requests++
	if event.Failed {
		aggregate.FailedRequests++
	}
	aggregate.InputTokens += event.InputTokens
	aggregate.OutputTokens += event.OutputTokens
	aggregate.ReasoningTokens += event.ReasoningTokens
	aggregate.CachedTokens += event.CachedTokens
	aggregate.TotalTokens += event.TotalTokens
	aggregate.Cost += event.Cost
	aggregate.Priced = aggregate.Priced && priced
}
