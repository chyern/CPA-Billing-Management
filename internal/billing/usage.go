package billing

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// HandleUsage records one usage event atomically. On persistence failure no
// in-memory aggregate or balance is changed, and the error is returned.
func (s *Store) HandleUsage(record UsageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	normalizeUsageRecord(&record)
	if err := validateUsageRecord(record); err != nil {
		return err
	}
	cost, priced, pricedBy := s.priceUsageRecord(record)
	if !finite(cost) {
		return fmt.Errorf("calculated usage cost must be finite")
	}
	event := usageEventFromRecord(record, cost, pricedBy, priced)
	modelKey := aggregateKey(event.Provider, event.Model)
	modelAgg := cloneAggregate(s.state.Aggregates[modelKey])
	if modelAgg == nil {
		modelAgg = &Aggregate{Provider: event.Provider, Model: event.Model, Priced: true}
	}
	applyModelAggregate(modelAgg, event, priced)
	apiKey := s.nextAPIKeyAggregate(event)
	if !finite(modelAgg.Cost) || !finite(apiKey.Cost) {
		return fmt.Errorf("usage aggregate cost exceeds the supported range")
	}
	nextCurrency := s.state.Currency
	if currency := strings.TrimSpace(record.Currency); currency != "" {
		nextCurrency = currency
	}
	nextState := s.state
	nextState.Currency = nextCurrency
	nextState.UpdatedAt = time.Now().UTC()
	if err := s.persistUsageLocked(event, nextState, modelKey, modelAgg, apiKey); err != nil {
		s.lastErr = err
		return err
	}
	s.state.Currency = nextCurrency
	s.state.UpdatedAt = nextState.UpdatedAt
	s.state.Aggregates[modelKey] = modelAgg
	if s.state.APIKeyAggregates == nil {
		s.state.APIKeyAggregates = make(map[string]*APIKeyAggregate)
	}
	s.state.APIKeyAggregates[event.APIKeyID] = apiKey
	s.state.Events = append(s.state.Events, event)
	if len(s.state.Events) > maxCachedEvents {
		s.state.Events = s.state.Events[len(s.state.Events)-maxCachedEvents:]
	}
	return nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func validateUsageRecord(r UsageRecord) error {
	if !finite(r.Cost) {
		return fmt.Errorf("usage cost must be finite")
	}
	return nil
}
func cloneAggregate(a *Aggregate) *Aggregate {
	if a == nil {
		return nil
	}
	c := *a
	return &c
}
func normalizeUsageRecord(record *UsageRecord) {
	if record.RequestedAt.IsZero() {
		record.RequestedAt = time.Now().UTC()
	} else {
		record.RequestedAt = record.RequestedAt.UTC()
	}
	if record.TotalTokens == 0 {
		record.TotalTokens = record.InputTokens + record.OutputTokens
	}
}
func (s *Store) priceUsageRecord(record UsageRecord) (float64, bool, string) {
	cost, priced, pricedBy := record.Cost, record.CostProvided, "upstream"
	if !record.CostProvided {
		rule, matched := s.matchRule(record)
		cost, priced, pricedBy = CalculateCost(record, rule), matched, rule.Match
	}
	if cost < 0 {
		cost = 0
	}
	return cost, priced, pricedBy
}
func usageEventFromRecord(record UsageRecord, cost float64, pricedBy string, priced bool) UsageEvent {
	return UsageEvent{RequestedAt: record.RequestedAt, Provider: record.Provider, Model: record.Model, Alias: record.Alias, APIKey: MaskAPIKey(record.APIKey), APIKeyID: APIKeyIdentifier(record.APIKey), AuthType: record.AuthType, Source: MaskSensitiveSource(record.Source), LatencyNanos: nonNegativeDuration(record.Latency), TTFTNanos: nonNegativeDuration(record.TTFT), Failed: record.Failed, InputTokens: record.InputTokens, OutputTokens: record.OutputTokens, ReasoningTokens: record.ReasoningTokens, CachedTokens: record.CachedTokens, CacheReadTokens: record.CacheReadTokens, CacheCreationTokens: record.CacheCreationTokens, TotalTokens: record.TotalTokens, Cost: cost, PricedBy: pricedBy, Priced: priced}
}
func applyModelAggregate(a *Aggregate, event UsageEvent, priced bool) {
	a.Requests++
	if event.Failed {
		a.FailedRequests++
	}
	a.InputTokens += event.InputTokens
	a.OutputTokens += event.OutputTokens
	a.ReasoningTokens += event.ReasoningTokens
	a.CachedTokens += event.CachedTokens
	a.TotalTokens += event.TotalTokens
	a.Cost += event.Cost
	a.Priced = a.Priced && priced
}
func (s *Store) nextAPIKeyAggregate(event UsageEvent) *APIKeyAggregate {
	var a APIKeyAggregate
	if old := s.state.APIKeyAggregates[event.APIKeyID]; old != nil {
		a = *old
	}
	if a.APIKey == "" {
		a.APIKey = event.APIKey
		if strings.TrimSpace(a.APIKey) == "" {
			a.APIKey = "未提供"
		}
	}
	a.Requests++
	if event.Failed {
		a.FailedRequests++
	}
	a.InputTokens += event.InputTokens
	a.OutputTokens += event.OutputTokens
	a.ReasoningTokens += event.ReasoningTokens
	a.CachedTokens += event.CachedTokens
	a.TotalTokens += event.TotalTokens
	a.Cost += event.Cost
	return &a
}
func aggregateKey(provider, model string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "/" + strings.ToLower(strings.TrimSpace(model))
}
