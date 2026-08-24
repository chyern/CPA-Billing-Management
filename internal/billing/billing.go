package billing

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	stateVersion       = 1
	maxPersistedEvents = 10000
	defaultCurrency    = "USD"
)

// PriceRule is a price per one million tokens. Match accepts a model name,
// alias, provider/model, or * for the fallback rule.
type PriceRule struct {
	Match                   string  `json:"match"`
	InputPerMillion         float64 `json:"input_per_million"`
	OutputPerMillion        float64 `json:"output_per_million"`
	CacheReadPerMillion     float64 `json:"cache_read_per_million"`
	CacheCreationPerMillion float64 `json:"cache_creation_per_million"`
}

type UsageRecord struct {
	Provider            string
	ExecutorType        string
	Model               string
	Alias               string
	APIKey              string
	AuthID              string
	AuthType            string
	Source              string
	RequestedAt         time.Time
	Latency             time.Duration
	TTFT                time.Duration
	Failed              bool
	InputTokens         int64
	OutputTokens        int64
	ReasoningTokens     int64
	CachedTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	TotalTokens         int64
	Cost                float64
	CostProvided        bool
	Currency            string
}

type UsageEvent struct {
	RequestedAt         time.Time `json:"requested_at"`
	Provider            string    `json:"provider"`
	Model               string    `json:"model"`
	Alias               string    `json:"alias,omitempty"`
	APIKey              string    `json:"api_key,omitempty"`
	APIKeyID            string    `json:"api_key_id,omitempty"`
	AuthType            string    `json:"auth_type,omitempty"`
	Source              string    `json:"source,omitempty"`
	LatencyNanos        int64     `json:"latency_ns,omitempty"`
	TTFTNanos           int64     `json:"ttft_ns,omitempty"`
	Failed              bool      `json:"failed"`
	InputTokens         int64     `json:"input_tokens"`
	OutputTokens        int64     `json:"output_tokens"`
	ReasoningTokens     int64     `json:"reasoning_tokens"`
	CachedTokens        int64     `json:"cached_tokens"`
	CacheReadTokens     int64     `json:"cache_read_tokens"`
	CacheCreationTokens int64     `json:"cache_creation_tokens"`
	TotalTokens         int64     `json:"total_tokens"`
	Cost                float64   `json:"cost"`
	PricedBy            string    `json:"priced_by,omitempty"`
}

type Aggregate struct {
	Provider        string  `json:"provider"`
	Model           string  `json:"model"`
	Requests        int64   `json:"requests"`
	FailedRequests  int64   `json:"failed_requests"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	ReasoningTokens int64   `json:"reasoning_tokens"`
	CachedTokens    int64   `json:"cached_tokens"`
	TotalTokens     int64   `json:"total_tokens"`
	Cost            float64 `json:"cost"`
	Priced          bool    `json:"priced"`
}

type APIKeyAggregate struct {
	APIKey          string  `json:"api_key"`
	Requests        int64   `json:"requests"`
	FailedRequests  int64   `json:"failed_requests"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	ReasoningTokens int64   `json:"reasoning_tokens"`
	CachedTokens    int64   `json:"cached_tokens"`
	TotalTokens     int64   `json:"total_tokens"`
	Cost            float64 `json:"cost"`
}

type Totals struct {
	Requests        int64   `json:"requests"`
	FailedRequests  int64   `json:"failed_requests"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	ReasoningTokens int64   `json:"reasoning_tokens"`
	CachedTokens    int64   `json:"cached_tokens"`
	TotalTokens     int64   `json:"total_tokens"`
	Cost            float64 `json:"cost"`
}

type State struct {
	Version    int                   `json:"version"`
	Currency   string                `json:"currency"`
	UpdatedAt  time.Time             `json:"updated_at"`
	Rules      []PriceRule           `json:"rules"`
	Events     []UsageEvent          `json:"events"`
	Aggregates map[string]*Aggregate `json:"aggregates"`
}

type Summary struct {
	Version              int                `json:"version"`
	Currency             string             `json:"currency"`
	UpdatedAt            time.Time          `json:"updated_at"`
	Totals               Totals             `json:"totals"`
	Models               []*Aggregate       `json:"models"`
	APIKeys              []*APIKeyAggregate `json:"api_keys"`
	RecentEvents         []UsageEvent       `json:"recent_events"`
	RecentEventsTotal    int                `json:"recent_events_total"`
	RecentEventsPage     int                `json:"recent_events_page"`
	RecentEventsPages    int                `json:"recent_events_pages"`
	RecentEventsPageSize int                `json:"recent_events_page_size"`
	UnpricedModels       []string           `json:"unpriced_models"`
}

type Store struct {
	mu            sync.RWMutex
	dataDir       string
	state         State
	lastErr       error
	managementKey string
}

func DefaultRules() []PriceRule {
	return []PriceRule{{Match: "*"}}
}

func NewStore(dataDir string) (*Store, error) {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = defaultDataDir()
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create billing data directory: %w", err)
	}
	s := &Store{dataDir: dataDir}
	s.state = State{Version: stateVersion, Currency: defaultCurrency, Rules: DefaultRules(), Aggregates: map[string]*Aggregate{}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func defaultDataDir() string {
	if configured := strings.TrimSpace(os.Getenv("CPA_BILLING_DATA_DIR")); configured != "" {
		return configured
	}
	base, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(base) == "" {
		return filepath.Join(".", ".cpa-billing-management")
	}
	return filepath.Join(base, "cliproxyapi", "cpa-billing-management")
}

func (s *Store) statePath() string { return filepath.Join(s.dataDir, "state.json") }

func (s *Store) load() error {
	raw, err := os.ReadFile(s.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return s.persistLocked()
	}
	if err != nil {
		return fmt.Errorf("read billing state: %w", err)
	}
	var loaded State
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return fmt.Errorf("decode billing state: %w", err)
	}
	// Older versions persisted Source verbatim.  Redact secret-like values
	// while loading so an existing state file is migrated on the next write.
	for i := range loaded.Events {
		loaded.Events[i].Source = MaskSensitiveSource(loaded.Events[i].Source)
		if loaded.Events[i].APIKeyID == "" && loaded.Events[i].APIKey != "" {
			loaded.Events[i].APIKeyID = "legacy:" + loaded.Events[i].APIKey
		}
	}
	if loaded.Version == 0 {
		loaded.Version = stateVersion
	}
	if loaded.Currency == "" {
		loaded.Currency = defaultCurrency
	}
	if len(loaded.Rules) == 0 {
		loaded.Rules = DefaultRules()
	}
	if loaded.Aggregates == nil {
		loaded.Aggregates = map[string]*Aggregate{}
	}
	s.state = loaded
	return nil
}

func (s *Store) persistLocked() error {
	s.state.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode billing state: %w", err)
	}
	tmp := s.statePath() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write billing state: %w", err)
	}
	if err := os.Rename(tmp, s.statePath()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace billing state: %w", err)
	}
	return nil
}

func (s *Store) HandleUsage(record UsageRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record.RequestedAt.IsZero() {
		record.RequestedAt = time.Now().UTC()
	} else {
		record.RequestedAt = record.RequestedAt.UTC()
	}
	if record.TotalTokens == 0 {
		record.TotalTokens = record.InputTokens + record.OutputTokens
	}
	if currency := strings.TrimSpace(record.Currency); currency != "" {
		s.state.Currency = currency
	}
	cost, priced, pricedBy := record.Cost, record.CostProvided, "upstream"
	if !record.CostProvided {
		rule, matched := s.matchRule(record)
		cost, priced, pricedBy = CalculateCost(record, rule), matched, rule.Match
	}
	if cost < 0 {
		cost = 0
	}
	event := UsageEvent{
		RequestedAt: record.RequestedAt, Provider: record.Provider, Model: record.Model,
		Alias: record.Alias, APIKey: MaskAPIKey(record.APIKey), APIKeyID: APIKeyIdentifier(record.APIKey), AuthType: record.AuthType,
		Source: MaskSensitiveSource(record.Source), LatencyNanos: nonNegativeDuration(record.Latency),
		TTFTNanos: nonNegativeDuration(record.TTFT), Failed: record.Failed, InputTokens: record.InputTokens,
		OutputTokens: record.OutputTokens, ReasoningTokens: record.ReasoningTokens,
		CachedTokens: record.CachedTokens, CacheReadTokens: record.CacheReadTokens,
		CacheCreationTokens: record.CacheCreationTokens, TotalTokens: record.TotalTokens,
		Cost: cost, PricedBy: pricedBy,
	}
	s.state.Events = append(s.state.Events, event)
	if len(s.state.Events) > maxPersistedEvents {
		s.state.Events = append([]UsageEvent(nil), s.state.Events[len(s.state.Events)-maxPersistedEvents:]...)
	}
	key := aggregateKey(record.Provider, record.Model)
	aggregate := s.state.Aggregates[key]
	if aggregate == nil {
		aggregate = &Aggregate{Provider: record.Provider, Model: record.Model}
		s.state.Aggregates[key] = aggregate
	}
	wasEmpty := aggregate.Requests == 0
	aggregate.Requests++
	if record.Failed {
		aggregate.FailedRequests++
	}
	aggregate.InputTokens += record.InputTokens
	aggregate.OutputTokens += record.OutputTokens
	aggregate.ReasoningTokens += record.ReasoningTokens
	aggregate.CachedTokens += record.CachedTokens
	aggregate.TotalTokens += record.TotalTokens
	aggregate.Cost += cost
	if wasEmpty {
		aggregate.Priced = priced
	} else {
		aggregate.Priced = aggregate.Priced && priced
	}
	if err := s.persistLocked(); err != nil {
		s.lastErr = err
	}
}

// MaskAPIKey returns a recognizable but non-secret API key label. The full key
// is deliberately discarded before a usage event reaches persistent storage.
func MaskAPIKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	switch {
	case len(runes) <= 2:
		return strings.Repeat("•", len(runes))
	case len(runes) <= 8:
		return string(runes[:1]) + strings.Repeat("•", len(runes)-2) + string(runes[len(runes)-1:])
	default:
		return string(runes[:4]) + "••••••" + string(runes[len(runes)-4:])
	}
}

// APIKeyIdentifier separates credentials that happen to have the same masked
// label without retaining the full key. It is used only as a grouping key.
func APIKeyIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:8])
}

// MaskSensitiveSource protects source values that are actually credentials in
// some CPA integrations, while preserving ordinary source labels.
func MaskSensitiveSource(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, '•') {
		return value
	}
	lower := strings.ToLower(value)
	secretLike := strings.HasPrefix(lower, "sk-") || strings.HasPrefix(lower, "rk-") ||
		strings.HasPrefix(lower, "pk-") || strings.HasPrefix(lower, "api-") ||
		strings.HasPrefix(lower, "bearer ")
	if secretLike {
		return MaskAPIKey(value)
	}
	return value
}

func nonNegativeDuration(value time.Duration) int64 {
	if value <= 0 {
		return 0
	}
	return int64(value)
}

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
	return (float64(input)/1_000_000)*rule.InputPerMillion +
		(float64(record.OutputTokens)/1_000_000)*rule.OutputPerMillion +
		(float64(cacheRead)/1_000_000)*rule.CacheReadPerMillion +
		(float64(cacheCreation)/1_000_000)*rule.CacheCreationPerMillion
}

func aggregateKey(provider, model string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "/" + strings.ToLower(strings.TrimSpace(model))
}

func (s *Store) matchRule(record UsageRecord) (PriceRule, bool) {
	keys := []string{
		strings.TrimSpace(record.Provider) + "/" + strings.TrimSpace(record.Model),
		record.Model,
		record.Alias,
		"*",
	}
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

func (s *Store) Summary() Summary {
	return s.SummaryPage(1, 20)
}

func (s *Store) SummaryPage(page, pageSize int) Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	if page < 1 {
		page = 1
	}
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
	apiKeyMap := make(map[string]*APIKeyAggregate)
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
		groupKey := event.APIKeyID
		if _, legacy := legacyLabels[label]; legacy {
			groupKey = "legacy:" + label
		} else if groupKey == "" {
			groupKey = "legacy:" + label
		}
		aggregate := apiKeyMap[groupKey]
		if aggregate == nil {
			aggregate = &APIKeyAggregate{APIKey: label}
			apiKeyMap[groupKey] = aggregate
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
	apiKeys := make([]*APIKeyAggregate, 0, len(apiKeyMap))
	for _, aggregate := range apiKeyMap {
		apiKeys = append(apiKeys, aggregate)
	}
	sort.Slice(apiKeys, func(i, j int) bool {
		if apiKeys[i].Requests == apiKeys[j].Requests {
			return apiKeys[i].APIKey < apiKeys[j].APIKey
		}
		return apiKeys[i].Requests > apiKeys[j].Requests
	})
	totalEvents := len(s.state.Events)
	pages := (totalEvents + pageSize - 1) / pageSize
	if pages == 0 {
		pages = 1
	}
	if page > pages {
		page = pages
	}
	end := totalEvents - (page-1)*pageSize
	if end < 0 {
		end = 0
	}
	start := end - pageSize
	if start < 0 {
		start = 0
	}
	events := append([]UsageEvent(nil), s.state.Events[start:end]...)
	unpricedModels := make([]string, 0, len(unpriced))
	for model := range unpriced {
		unpricedModels = append(unpricedModels, model)
	}
	sort.Strings(unpricedModels)
	return Summary{Version: s.state.Version, Currency: s.state.Currency, UpdatedAt: s.state.UpdatedAt,
		Totals: totals, Models: models, APIKeys: apiKeys, RecentEvents: events, RecentEventsTotal: totalEvents,
		RecentEventsPage: page, RecentEventsPages: pages, RecentEventsPageSize: pageSize,
		UnpricedModels: unpricedModels}
}

func (s *Store) Rules() []PriceRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]PriceRule(nil), s.state.Rules...)
}

func (s *Store) SetRules(rules []PriceRule) error {
	if len(rules) == 0 {
		return fmt.Errorf("at least one pricing rule is required")
	}
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
		if rules[i].InputPerMillion < 0 || rules[i].OutputPerMillion < 0 || rules[i].CacheReadPerMillion < 0 || rules[i].CacheCreationPerMillion < 0 {
			return fmt.Errorf("pricing rule %q contains a negative price", rules[i].Match)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Rules = append([]PriceRule(nil), rules...)
	s.recalculateLocked()
	if err := s.persistLocked(); err != nil {
		s.lastErr = err
		return err
	}
	return nil
}

func (s *Store) recalculateLocked() {
	s.state.Aggregates = map[string]*Aggregate{}
	for index := range s.state.Events {
		event := &s.state.Events[index]
		record := UsageRecord{
			Provider: event.Provider, Model: event.Model, Alias: event.Alias,
			InputTokens: event.InputTokens, OutputTokens: event.OutputTokens,
			ReasoningTokens: event.ReasoningTokens, CachedTokens: event.CachedTokens,
			CacheReadTokens: event.CacheReadTokens, CacheCreationTokens: event.CacheCreationTokens,
			TotalTokens: event.TotalTokens, Failed: event.Failed,
		}
		matched := true
		if !strings.EqualFold(event.PricedBy, "upstream") {
			rule, localMatched := s.matchRule(record)
			event.Cost = CalculateCost(record, rule)
			event.PricedBy = rule.Match
			matched = localMatched
		}
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
		aggregate.Priced = aggregate.Priced && matched
	}
}

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Events = nil
	s.state.Aggregates = map[string]*Aggregate{}
	if err := s.persistLocked(); err != nil {
		s.lastErr = err
		return err
	}
	return nil
}

func (s *Store) Currency() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Currency
}

func (s *Store) ConfigureYAML(raw []byte) {
	// The host sends plugin configuration as YAML. Pricing rules stay editable
	// through the model-cost page and are persisted in the plugin state file.
	var currency, managementKey string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "currency:") {
			currency = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "currency:")), "\"'")
		}
		if strings.HasPrefix(line, "management_key:") {
			managementKey = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "management_key:")), "\"'")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.managementKey = managementKey
	if currency != "" {
		s.state.Currency = currency
		if err := s.persistLocked(); err != nil {
			s.lastErr = err
		}
	}
}

// ManagementKey returns the optional key configured for resource-page API
// requests. It is kept in memory and is never persisted with billing data.
func (s *Store) ManagementKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.managementKey
}

func (s *Store) LastError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastErr == nil {
		return ""
	}
	return s.lastErr.Error()
}

func ParseInt(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	default:
		return 0
	}
}
