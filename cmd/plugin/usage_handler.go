package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chyern/CPA-Billing-Management/internal/billing"
)

func handleUsage(store *billing.Store, raw []byte) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("decode usage request: %w", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("usage request must be an object")
	}
	if nested, ok := lookup(object, "record", "usage"); ok {
		if nestedObject, ok := nested.(map[string]any); ok {
			object = nestedObject
		}
	}
	record := usageRecordFromObject(object)
	if requestedAt := stringValue(object, "requested_at", "requestedat"); requestedAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, requestedAt); err == nil {
			record.RequestedAt = parsed
		}
	}
	store.HandleUsage(record)
	return nil
}

func usageRecordFromObject(object map[string]any) billing.UsageRecord {
	record := billing.UsageRecord{
		Provider: stringValue(object, "provider"), ExecutorType: stringValue(object, "executor_type", "executortype"),
		Model: stringValue(object, "model"), Alias: stringValue(object, "alias"), APIKey: stringValue(object, "api_key", "apikey"),
		AuthID: stringValue(object, "auth_id", "authid"), AuthType: stringValue(object, "auth_type", "authtype"),
		Source: stringValue(object, "source"), Latency: time.Duration(intValue(object, "latency", "duration", "elapsed")),
		TTFT: time.Duration(intValue(object, "ttft")), Failed: boolValue(object, "failed"),
		InputTokens: intValue(object, "input_tokens", "inputtokens"), OutputTokens: intValue(object, "output_tokens", "outputtokens"),
		ReasoningTokens: intValue(object, "reasoning_tokens", "reasoningtokens"), CachedTokens: intValue(object, "cached_tokens", "cachedtokens"),
		CacheReadTokens: intValue(object, "cache_read_tokens", "cachereadtokens"), CacheCreationTokens: intValue(object, "cache_creation_tokens", "cachecreationtokens"),
		TotalTokens: intValue(object, "total_tokens", "totaltokens"),
	}
	if cost, ok := upstreamCost(object); ok {
		record.Cost, record.CostProvided = cost, true
	}
	record.Currency = stringValue(object, "currency", "cost_currency", "costcurrency", "billing_currency", "billingcurrency")
	mergeUsageDetail(&record, object)
	return record
}

func mergeUsageDetail(record *billing.UsageRecord, object map[string]any) {
	detail, ok := lookup(object, "detail", "usage_detail", "usagedetail")
	if !ok {
		return
	}
	detailObject, ok := detail.(map[string]any)
	if !ok {
		return
	}
	record.InputTokens = firstInt(record.InputTokens, intValue(detailObject, "input_tokens", "inputtokens"))
	record.OutputTokens = firstInt(record.OutputTokens, intValue(detailObject, "output_tokens", "outputtokens"))
	record.ReasoningTokens = firstInt(record.ReasoningTokens, intValue(detailObject, "reasoning_tokens", "reasoningtokens"))
	record.CachedTokens = firstInt(record.CachedTokens, intValue(detailObject, "cached_tokens", "cachedtokens"))
	record.CacheReadTokens = firstInt(record.CacheReadTokens, intValue(detailObject, "cache_read_tokens", "cachereadtokens"))
	record.CacheCreationTokens = firstInt(record.CacheCreationTokens, intValue(detailObject, "cache_creation_tokens", "cachecreationtokens"))
	record.TotalTokens = firstInt(record.TotalTokens, intValue(detailObject, "total_tokens", "totaltokens"))
	if !record.CostProvided {
		if cost, found := upstreamCost(detailObject); found {
			record.Cost, record.CostProvided = cost, true
		}
	}
	if record.Currency == "" {
		record.Currency = stringValue(detailObject, "currency", "cost_currency", "costcurrency", "billing_currency", "billingcurrency")
	}
}

func normalizeKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "")
	return strings.ReplaceAll(value, "-", "")
}

func lookup(object map[string]any, keys ...string) (any, bool) {
	for key, value := range object {
		for _, wanted := range keys {
			if normalizeKey(key) == normalizeKey(wanted) {
				return value, true
			}
		}
	}
	return nil, false
}

func stringValue(object map[string]any, keys ...string) string {
	value, ok := lookup(object, keys...)
	if !ok {
		return ""
	}
	if stringValue, ok := value.(string); ok {
		return stringValue
	}
	return fmt.Sprint(value)
}

func intValue(object map[string]any, keys ...string) int64 {
	value, ok := lookup(object, keys...)
	if !ok {
		return 0
	}
	return billing.ParseInt(value)
}

func floatValue(object map[string]any, keys ...string) (float64, bool) {
	value, ok := lookup(object, keys...)
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func upstreamCost(object map[string]any) (float64, bool) {
	if cost, ok := floatValue(object, "actual_cost", "actualcost"); ok {
		return cost, true
	}
	if cost, ok := floatValue(object, "total_cost", "totalcost", "cost", "price", "total_price", "totalprice", "estimated_cost", "estimatedcost", "upstream_cost", "upstreamcost", "provider_cost", "providercost", "billing_cost", "billingcost", "amount"); ok {
		return cost, true
	}
	var total float64
	var found bool
	for _, keys := range [][]string{{"input_cost", "inputcost"}, {"output_cost", "outputcost"}, {"cache_creation_cost", "cachecreationcost"}, {"cache_read_cost", "cachereadcost"}} {
		if cost, ok := floatValue(object, keys...); ok {
			total += cost
			found = true
		}
	}
	return total, found
}

func boolValue(object map[string]any, keys ...string) bool {
	value, ok := lookup(object, keys...)
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func firstInt(current, fallback int64) int64 {
	if current != 0 {
		return current
	}
	return fallback
}
