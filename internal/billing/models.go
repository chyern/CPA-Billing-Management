package billing

import (
	"database/sql"
	"sync"
	"time"
)

// PriceRule is a price per one million tokens. Match accepts a model name,
// alias, or * for the default rule.
type PriceRule struct {
	Match                   string  `json:"match"`
	InputPerMillion         float64 `json:"input_per_million"`
	OutputPerMillion        float64 `json:"output_per_million"`
	CacheReadPerMillion     float64 `json:"cache_read_per_million"`
	CacheCreationPerMillion float64 `json:"cache_creation_per_million"`
}

type UsageRecord struct {
	Domain              string
	Provider            string
	ExecutorType        string
	Model               string
	ReasoningEffort     string
	Alias               string
	APIKey              string
	AuthID              string
	AuthIndex           string
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
	Currency            string    `json:"currency,omitempty"`
	Domain              string    `json:"domain,omitempty"`
	RequestedAt         time.Time `json:"requested_at"`
	Provider            string    `json:"provider,omitempty"`
	Model               string    `json:"model"`
	ReasoningEffort     string    `json:"reasoning_effort,omitempty"`
	Alias               string    `json:"-"`
	APIKey              string    `json:"api_key,omitempty"`
	APIKeyID            string    `json:"-"`
	AuthType            string    `json:"-"`
	AuthIndex           string    `json:"-"`
	Source              string    `json:"-"`
	LatencyNanos        int64     `json:"latency_ns,omitempty"`
	TTFTNanos           int64     `json:"ttft_ns,omitempty"`
	Failed              bool      `json:"failed"`
	InputTokens         int64     `json:"input_tokens"`
	OutputTokens        int64     `json:"output_tokens"`
	ReasoningTokens     int64     `json:"-"`
	CachedTokens        int64     `json:"cached_tokens"`
	CacheReadTokens     int64     `json:"-"`
	CacheCreationTokens int64     `json:"-"`
	TotalTokens         int64     `json:"-"`
	Cost                float64   `json:"cost"`
	PricedBy            string    `json:"-"`
	Priced              bool      `json:"-"`
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
	Version          int                         `json:"version"`
	Currency         string                      `json:"currency"`
	UpdatedAt        time.Time                   `json:"updated_at"`
	Rules            []PriceRule                 `json:"rules"`
	Events           []UsageEvent                `json:"events"`
	APIKeyAggregates map[string]*APIKeyAggregate `json:"api_key_aggregates"`
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
	mu      sync.RWMutex
	dataDir string
	db      *sql.DB
	state   State
	lastErr error
}
