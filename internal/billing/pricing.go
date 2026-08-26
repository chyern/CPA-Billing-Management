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
	keys := []string{strings.TrimSpace(record.Provider) + "/" + strings.TrimSpace(record.Model), record.Model, record.Alias, "*"}
	for _, key := range keys {
		for _, rule := range s.state.Rules {
			if strings.EqualFold(strings.TrimSpace(rule.Match), strings.TrimSpace(key)) {
				priced := key != "*" || rule.InputPerMillion > 0 || rule.OutputPerMillion > 0 || rule.CacheReadPerMillion > 0 || rule.CacheCreationPerMillion > 0
				return rule, priced
			}
		}
	}
	return PriceRule{Match: "*"}, false
}

func (s *Store) Rules() []PriceRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]PriceRule(nil), s.state.Rules...)
}

func (s *Store) SetRules(rules []PriceRule) error {
	if err := validateRules(rules); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Rules = append([]PriceRule{}, rules...)
	s.recalculateLocked()
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

func (s *Store) recalculateLocked() {
	s.state.Aggregates = map[string]*Aggregate{}
	s.state.APIKeyAggregates = map[string]*APIKeyAggregate{}
	for index := range s.state.Events {
		event := &s.state.Events[index]
		record := UsageRecord{Provider: event.Provider, Model: event.Model, Alias: event.Alias, InputTokens: event.InputTokens, OutputTokens: event.OutputTokens, ReasoningTokens: event.ReasoningTokens, CachedTokens: event.CachedTokens, CacheReadTokens: event.CacheReadTokens, CacheCreationTokens: event.CacheCreationTokens, TotalTokens: event.TotalTokens, Failed: event.Failed}
		matched := true
		if !strings.EqualFold(event.PricedBy, "upstream") {
			rule, localMatched := s.matchRule(record)
			event.Cost, event.PricedBy, matched = CalculateCost(record, rule), rule.Match, localMatched
		}
		s.addModelAggregateLocked(*event, matched)
		s.addAPIKeyAggregateLocked(*event)
	}
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
