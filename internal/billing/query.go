package billing

import "sort"

func (s *Store) Summary() Summary { return s.SummaryPage(1, 20) }

func (s *Store) SummaryPage(page, pageSize int) Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page, pageSize = normalizePage(page, pageSize)
	models, totals, unpricedModels := s.modelSummaryLocked()
	apiKeys := s.apiKeySummaryLocked()
	events, page, pages := paginateEvents(s.state.Events, page, pageSize)
	return Summary{Version: s.state.Version, Currency: s.state.Currency, UpdatedAt: s.state.UpdatedAt, Totals: totals, Models: models, APIKeys: apiKeys, RecentEvents: events, RecentEventsTotal: len(s.state.Events), RecentEventsPage: page, RecentEventsPages: pages, RecentEventsPageSize: pageSize, UnpricedModels: unpricedModels}
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
