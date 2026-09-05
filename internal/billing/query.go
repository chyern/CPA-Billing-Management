package billing

import (
	"fmt"
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
	summary, err := s.SummaryPageRangeStatus(page, pageSize, start, end, "all")
	if err != nil {
		s.mu.Lock()
		s.lastErr = err
		s.mu.Unlock()
	}
	return summary
}

// SummaryPageRangeStatus filters only event rows by success/failure. Top-level
// totals and groups always describe the complete date range. SQL queries read
// persisted history; the bounded in-memory event cache is never a total source.
func (s *Store) SummaryPageRangeStatus(page, pageSize int, start, end time.Time, eventStatus string) (Summary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return Summary{}, fmt.Errorf("billing database is closed")
	}
	status := strings.ToLower(strings.TrimSpace(eventStatus))
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "success" && status != "failed" {
		return Summary{}, fmt.Errorf("invalid event status %q", eventStatus)
	}
	page, pageSize = normalizePage(page, pageSize)
	where, args := eventDateWhere(start, end)
	var models []*Aggregate
	var apiKeys []*APIKeyAggregate
	var totals Totals
	var unpriced []string
	if start.IsZero() && end.IsZero() {
		models, totals, unpriced = s.modelSummaryLocked()
		apiKeys = s.apiKeySummaryLocked()
	} else {
		var err error
		models, apiKeys, totals, unpriced, err = s.summarizeDateRangeLocked(where, args)
		if err != nil {
			return Summary{}, err
		}
	}
	if status != "all" {
		where += " AND failed = ?"
		args = append(args, boolInt64(status == "failed"))
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM usage_events`+where, args...).Scan(&count); err != nil {
		return Summary{}, fmt.Errorf("count usage events: %w", err)
	}
	pages := 1
	if count > 0 {
		pages = 1 + (count-1)/pageSize
	}
	if page > pages {
		page = pages
	}
	pageArgs := append(append([]any(nil), args...), pageSize, (page-1)*pageSize)
	rows, err := s.db.Query(`SELECT `+eventColumns+` FROM usage_events`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, pageArgs...)
	if err != nil {
		return Summary{}, fmt.Errorf("query usage events: %w", err)
	}
	events, err := scanUsageEvents(rows)
	if err != nil {
		return Summary{}, err
	}
	// Preserve the API's chronological ordering inside each newest-first page.
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return Summary{Version: s.state.Version, Currency: s.state.Currency, UpdatedAt: s.state.UpdatedAt, Totals: totals, Models: models, APIKeys: apiKeys, RecentEvents: events, RecentEventsTotal: count, RecentEventsPage: page, RecentEventsPages: pages, RecentEventsPageSize: pageSize, UnpricedModels: unpriced}, nil
}

func eventDateWhere(start, end time.Time) (string, []any) {
	where := " WHERE 1=1"
	var args []any
	// Ingested timestamps are UTC RFC3339Nano. Removing the trailing Z makes
	// fractional seconds compare correctly against boundaries on whole seconds.
	if !start.IsZero() {
		where += " AND rtrim(requested_at, 'Z') >= ?"
		args = append(args, strings.TrimSuffix(start.UTC().Format(time.RFC3339Nano), "Z"))
	}
	if !end.IsZero() {
		where += " AND rtrim(requested_at, 'Z') < ?"
		args = append(args, strings.TrimSuffix(end.UTC().Format(time.RFC3339Nano), "Z"))
	}
	return where, args
}

func (s *Store) summarizeDateRangeLocked(where string, args []any) ([]*Aggregate, []*APIKeyAggregate, Totals, []string, error) {
	const sums = `COUNT(*), SUM(failed), SUM(input_tokens), SUM(output_tokens), SUM(reasoning_tokens), SUM(cached_tokens), SUM(total_tokens), SUM(cost)`
	rows, err := s.db.Query(`SELECT provider, model, `+sums+`, MIN(priced) FROM usage_events`+where+` GROUP BY lower(trim(provider)),lower(trim(model)) ORDER BY SUM(cost) DESC,provider,model`, args...)
	if err != nil {
		return nil, nil, Totals{}, nil, fmt.Errorf("summarize usage models: %w", err)
	}
	models := make([]*Aggregate, 0)
	var totals Totals
	missing := make(map[string]struct{})
	for rows.Next() {
		var a Aggregate
		if err := rows.Scan(&a.Provider, &a.Model, &a.Requests, &a.FailedRequests, &a.InputTokens, &a.OutputTokens, &a.ReasoningTokens, &a.CachedTokens, &a.TotalTokens, &a.Cost, &a.Priced); err != nil {
			rows.Close()
			return nil, nil, Totals{}, nil, fmt.Errorf("scan usage model summary: %w", err)
		}
		models = append(models, &a)
		totals.Requests += a.Requests
		totals.FailedRequests += a.FailedRequests
		totals.InputTokens += a.InputTokens
		totals.OutputTokens += a.OutputTokens
		totals.ReasoningTokens += a.ReasoningTokens
		totals.CachedTokens += a.CachedTokens
		totals.TotalTokens += a.TotalTokens
		totals.Cost += a.Cost
		if !a.Priced {
			missing[a.Model] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, Totals{}, nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, nil, Totals{}, nil, err
	}
	unpriced := make([]string, 0, len(missing))
	for model := range missing {
		unpriced = append(unpriced, model)
	}
	sort.Strings(unpriced)
	rows, err = s.db.Query(`SELECT api_key, `+sums+` FROM usage_events`+where+` GROUP BY api_key_id ORDER BY COUNT(*) DESC,api_key`, args...)
	if err != nil {
		return nil, nil, Totals{}, nil, fmt.Errorf("summarize usage API keys: %w", err)
	}
	defer rows.Close()
	keys := make([]*APIKeyAggregate, 0)
	for rows.Next() {
		var a APIKeyAggregate
		if err := rows.Scan(&a.APIKey, &a.Requests, &a.FailedRequests, &a.InputTokens, &a.OutputTokens, &a.ReasoningTokens, &a.CachedTokens, &a.TotalTokens, &a.Cost); err != nil {
			return nil, nil, Totals{}, nil, fmt.Errorf("scan usage API key summary: %w", err)
		}
		if strings.TrimSpace(a.APIKey) == "" {
			a.APIKey = "未提供"
		}
		keys = append(keys, &a)
	}
	return models, keys, totals, unpriced, rows.Err()
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
		priced := event.Priced
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
