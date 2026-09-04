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
	record := usageRecordFromObject(object)
	if requestedAt := stringValue(object, "RequestedAt"); requestedAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, requestedAt); err == nil {
			record.RequestedAt = parsed
		}
	}
	store.HandleUsage(record)
	return nil
}

func usageRecordFromObject(object map[string]any) billing.UsageRecord {
	record := billing.UsageRecord{
		Provider: stringValue(object, "Provider"), ExecutorType: stringValue(object, "ExecutorType"),
		Model: stringValue(object, "Model"), Alias: stringValue(object, "Alias"), APIKey: stringValue(object, "APIKey"),
		AuthID: stringValue(object, "AuthID"), AuthType: stringValue(object, "AuthType"),
		Source: stringValue(object, "Source"), Latency: time.Duration(intValue(object, "Latency")),
		TTFT: time.Duration(intValue(object, "TTFT")), Failed: boolValue(object, "Failed"),
		InputTokens: intValue(object, "InputTokens"), OutputTokens: intValue(object, "OutputTokens"),
		ReasoningTokens: intValue(object, "ReasoningTokens"), CachedTokens: intValue(object, "CachedTokens"),
		CacheReadTokens: intValue(object, "CacheReadTokens"), CacheCreationTokens: intValue(object, "CacheCreationTokens"),
		TotalTokens: intValue(object, "TotalTokens"),
	}
	if cost, ok := floatValue(object, "ActualCost"); ok {
		record.Cost, record.CostProvided = cost, true
	}
	mergeUsageDetail(&record, object)
	return record
}

func mergeUsageDetail(record *billing.UsageRecord, object map[string]any) {
	detail, ok := lookup(object, "Detail")
	if !ok {
		return
	}
	detailObject, ok := detail.(map[string]any)
	if !ok {
		return
	}
	record.InputTokens = preferInt(record.InputTokens, intValue(detailObject, "InputTokens"))
	record.OutputTokens = preferInt(record.OutputTokens, intValue(detailObject, "OutputTokens"))
	record.ReasoningTokens = preferInt(record.ReasoningTokens, intValue(detailObject, "ReasoningTokens"))
	record.CachedTokens = preferInt(record.CachedTokens, intValue(detailObject, "CachedTokens"))
	record.CacheReadTokens = preferInt(record.CacheReadTokens, intValue(detailObject, "CacheReadTokens"))
	record.CacheCreationTokens = preferInt(record.CacheCreationTokens, intValue(detailObject, "CacheCreationTokens"))
	record.TotalTokens = preferInt(record.TotalTokens, intValue(detailObject, "TotalTokens"))
}

func lookup(object map[string]any, key string) (any, bool) {
	value, ok := object[key]
	if ok {
		return value, true
	}
	return nil, false
}

func stringValue(object map[string]any, key string) string {
	value, ok := lookup(object, key)
	if !ok {
		return ""
	}
	if stringValue, ok := value.(string); ok {
		return stringValue
	}
	return fmt.Sprint(value)
}

func intValue(object map[string]any, key string) int64 {
	value, ok := lookup(object, key)
	if !ok {
		return 0
	}
	return billing.ParseInt(value)
}

func floatValue(object map[string]any, key string) (float64, bool) {
	value, ok := lookup(object, key)
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

func boolValue(object map[string]any, key string) bool {
	value, ok := lookup(object, key)
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

func preferInt(current, alternate int64) int64 {
	if current != 0 {
		return current
	}
	return alternate
}
