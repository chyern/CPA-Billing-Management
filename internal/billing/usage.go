package billing

import (
	"strings"
	"time"
)

func (s *Store) HandleUsage(record UsageRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	normalizeUsageRecord(&record)
	if currency := strings.TrimSpace(record.Currency); currency != "" {
		s.state.Currency = currency
	}
	cost, priced, pricedBy := s.priceUsageRecord(record)
	event := usageEventFromRecord(record, cost, pricedBy)
	s.state.Events = append(s.state.Events, event)
	if len(s.state.Events) > maxPersistedEvents {
		s.state.Events = append([]UsageEvent(nil), s.state.Events[len(s.state.Events)-maxPersistedEvents:]...)
	}
	s.addModelAggregateLocked(event, priced)
	s.addAPIKeyAggregateLocked(event)
	if err := s.persistLocked(); err != nil {
		s.lastErr = err
	}
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

func usageEventFromRecord(record UsageRecord, cost float64, pricedBy string) UsageEvent {
	return UsageEvent{
		RequestedAt: record.RequestedAt, Provider: record.Provider, Model: record.Model, Alias: record.Alias,
		APIKey: MaskAPIKey(record.APIKey), APIKeyID: APIKeyIdentifier(record.APIKey), AuthType: record.AuthType,
		Source: MaskSensitiveSource(record.Source), LatencyNanos: nonNegativeDuration(record.Latency), TTFTNanos: nonNegativeDuration(record.TTFT),
		Failed: record.Failed, InputTokens: record.InputTokens, OutputTokens: record.OutputTokens, ReasoningTokens: record.ReasoningTokens,
		CachedTokens: record.CachedTokens, CacheReadTokens: record.CacheReadTokens, CacheCreationTokens: record.CacheCreationTokens,
		TotalTokens: record.TotalTokens, Cost: cost, PricedBy: pricedBy,
	}
}

func (s *Store) addAPIKeyAggregateLocked(event UsageEvent) {
	if s.state.APIKeyAggregates == nil {
		s.state.APIKeyAggregates = map[string]*APIKeyAggregate{}
	}
	label := strings.TrimSpace(event.APIKey)
	if label == "" {
		label = "未提供"
	}
	legacyKey, groupKey := "legacy:"+label, event.APIKeyID
	if _, legacyExists := s.state.APIKeyAggregates[legacyKey]; legacyExists || groupKey == "" {
		groupKey = legacyKey
	}
	aggregate := s.state.APIKeyAggregates[groupKey]
	if aggregate == nil {
		aggregate = &APIKeyAggregate{APIKey: label}
		s.state.APIKeyAggregates[groupKey] = aggregate
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
}

func (s *Store) rebuildAPIKeyAggregatesLocked() {
	s.state.APIKeyAggregates = map[string]*APIKeyAggregate{}
	legacyLabels := make(map[string]struct{})
	for _, event := range s.state.Events {
		if strings.HasPrefix(event.APIKeyID, "legacy:") && event.APIKey != "" {
			legacyLabels[event.APIKey] = struct{}{}
		}
	}
	for _, event := range s.state.Events {
		label := strings.TrimSpace(event.APIKey)
		if label == "" {
			label = "未提供"
		}
		if _, legacy := legacyLabels[label]; legacy {
			event.APIKeyID = "legacy:" + label
		}
		s.addAPIKeyAggregateLocked(event)
	}
}

func aggregateKey(provider, model string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "/" + strings.ToLower(strings.TrimSpace(model))
}
