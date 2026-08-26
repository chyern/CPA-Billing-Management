package billing

import (
	"database/sql"
	"sync"
	"time"
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
	Version          int                         `json:"version"`
	Currency         string                      `json:"currency"`
	UpdatedAt        time.Time                   `json:"updated_at"`
	Rules            []PriceRule                 `json:"rules"`
	Events           []UsageEvent                `json:"events"`
	Aggregates       map[string]*Aggregate       `json:"aggregates"`
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
