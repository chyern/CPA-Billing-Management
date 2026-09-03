package billing

import (
	"sort"
	"strings"
	"time"
)

func (s *Store) Summary() Summary { return s.SummaryPage(1, 20) }

func (s *Store) SummaryPage(page, pageSize int) Summary {
	return s.SummaryPageRange(page, pageSize, time.Time{}, time.Time{})
}

// SummaryPageRange returns a summary limited to [start, end). Zero boundaries
// disable the corresponding limit. Event costs are already fixed at ingestion.
func (s *Store) SummaryPageRange(page, pageSize int, start, end time.Time) Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page, pageSize = normalizePage(page, pageSize)
	filtered := s.state.Events
	if !start.IsZero() || !end.IsZero() {
		filtered = make([]UsageEvent, 0, len(s.state.Events))
		for _, event := range s.state.Events {
			if !start.IsZero() && event.RequestedAt.Before(start) {
				continue
			}
			if !end.IsZero() && !event.RequestedAt.Before(end) {
				continue
			}
			filtered = append(filtered, event)
		}
	}
	models, apiKeys, totals, unpricedModels := summarizeEvents(filtered)
	events, page, pages := paginateEvents(filtered, page, pageSize)
	return Summary{Version: s.state.Version, Currency: s.state.Currency, UpdatedAt: s.state.UpdatedAt, Totals: totals, Models: models, APIKeys: apiKeys, RecentEvents: events, RecentEventsTotal: len(filtered), RecentEventsPage: page, RecentEventsPages: pages, RecentEventsPageSize: pageSize, UnpricedModels: unpricedModels}
}

func summarizeEvents(events []UsageEvent) ([]*Aggregate, []*APIKeyAggregate, Totals, []string) {
	modelsByKey := make(map[string]*Aggregate)
	keysByID := make(map[string]*APIKeyAggregate)
	var totals Totals
	unpriced := make(map[string]struct{})
	for _, event := range events {
		totals.Requests++
		totals.FailedRequests += boolInt64(event.Failed)
		totals.InputTokens += event.InputTokens
		totals.OutputTokens += event.OutputTokens
		totals.ReasoningTokens += event.ReasoningTokens
		totals.CachedTokens += event.CachedTokens
		totals.TotalTokens += event.TotalTokens
		totals.Cost += event.Cost
		modelKey := aggregateKey(event.Provider, event.Model)
		aggregate := modelsByKey[modelKey]
		if aggregate == nil {
			aggregate = &Aggregate{Provider: event.Provider, Model: event.Model, Priced: true}
			modelsByKey[modelKey] = aggregate
		}
		aggregate.Requests++
		aggregate.FailedRequests += boolInt64(event.Failed)
		aggregate.InputTokens += event.InputTokens
		aggregate.OutputTokens += event.OutputTokens
		aggregate.ReasoningTokens += event.ReasoningTokens
		aggregate.CachedTokens += event.CachedTokens
		aggregate.TotalTokens += event.TotalTokens
		aggregate.Cost += event.Cost
		priced := strings.TrimSpace(event.PricedBy) != "" && event.PricedBy != "*"
		aggregate.Priced = aggregate.Priced && priced
		if !priced {
			unpriced[event.Model] = struct{}{}
		}
		keyID := event.APIKeyID
		keyAggregate := keysByID[keyID]
		if keyAggregate == nil {
			label := event.APIKey
			if strings.TrimSpace(label) == "" {
				label = "未提供"
			}
			keyAggregate = &APIKeyAggregate{APIKey: label}
			keysByID[keyID] = keyAggregate
		}
		keyAggregate.Requests++
		keyAggregate.FailedRequests += boolInt64(event.Failed)
		keyAggregate.InputTokens += event.InputTokens
		keyAggregate.OutputTokens += event.OutputTokens
		keyAggregate.ReasoningTokens += event.ReasoningTokens
		keyAggregate.CachedTokens += event.CachedTokens
		keyAggregate.TotalTokens += event.TotalTokens
		keyAggregate.Cost += event.Cost
	}
	models := make([]*Aggregate, 0, len(modelsByKey))
	for _, aggregate := range modelsByKey {
		models = append(models, aggregate)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Cost > models[j].Cost })
	apiKeys := make([]*APIKeyAggregate, 0, len(keysByID))
	for _, aggregate := range keysByID {
		apiKeys = append(apiKeys, aggregate)
	}
	sort.Slice(apiKeys, func(i, j int) bool {
		if apiKeys[i].Requests == apiKeys[j].Requests {
			return apiKeys[i].APIKey < apiKeys[j].APIKey
		}
		return apiKeys[i].Requests > apiKeys[j].Requests
	})
	unpricedModels := make([]string, 0, len(unpriced))
	for model := range unpriced {
		unpricedModels = append(unpricedModels, model)
	}
	sort.Strings(unpricedModels)
	return models, apiKeys, totals, unpricedModels
}

func boolInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func normalizePage(page, pageSize int) (int, int) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	if page < 1 {
		page = 1
	}
	return page, pageSize
}

func (s *Store) modelSummaryLocked() ([]*Aggregate, Totals, []string) {
	models := make([]*Aggregate, 0, len(s.state.Aggregates))
	var totals Totals
	unpriced := make(map[string]struct{})
	for _, aggregate := range s.state.Aggregates {
		copy := *aggregate
		models = append(models, &copy)
		totals.Requests += aggregate.Requests
		totals.FailedRequests += aggregate.FailedRequests
		totals.InputTokens += aggregate.InputTokens
		totals.OutputTokens += aggregate.OutputTokens
		totals.ReasoningTokens += aggregate.ReasoningTokens
		totals.CachedTokens += aggregate.CachedTokens
		totals.TotalTokens += aggregate.TotalTokens
		totals.Cost += aggregate.Cost
		if !aggregate.Priced {
			unpriced[aggregate.Model] = struct{}{}
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Cost > models[j].Cost })
	unpricedModels := make([]string, 0, len(unpriced))
	for model := range unpriced {
		unpricedModels = append(unpricedModels, model)
	}
	sort.Strings(unpricedModels)
	return models, totals, unpricedModels
}

func (s *Store) apiKeySummaryLocked() []*APIKeyAggregate {
	apiKeys := make([]*APIKeyAggregate, 0, len(s.state.APIKeyAggregates))
	for _, aggregate := range s.state.APIKeyAggregates {
		copy := *aggregate
		apiKeys = append(apiKeys, &copy)
	}
	sort.Slice(apiKeys, func(i, j int) bool {
		if apiKeys[i].Requests == apiKeys[j].Requests {
			return apiKeys[i].APIKey < apiKeys[j].APIKey
		}
		return apiKeys[i].Requests > apiKeys[j].Requests
	})
	return apiKeys
}

func paginateEvents(all []UsageEvent, page, pageSize int) ([]UsageEvent, int, int) {
	pages := (len(all) + pageSize - 1) / pageSize
	if pages == 0 {
		pages = 1
	}
	if page > pages {
		page = pages
	}
	end := len(all) - (page-1)*pageSize
	if end < 0 {
		end = 0
	}
	start := end - pageSize
	if start < 0 {
		start = 0
	}
	return append([]UsageEvent(nil), all[start:end]...), page, pages
}
