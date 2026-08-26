// Command migrate-legacy converts the former single-row JSON snapshot into
// normalized SQLite tables. It is intentionally separate from plugin startup
// so legacy compatibility can be removed after the one-time migration.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/chyern/CPA-Billing-Management/internal/billing"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	databasePath := flag.String("database", "", "path to the legacy billing.db")
	replace := flag.Bool("replace", false, "replace rows already present in normalized tables")
	keepLegacy := flag.Bool("keep-legacy", false, "keep the billing_state JSON table after migration")
	flag.Parse()
	if strings.TrimSpace(*databasePath) == "" {
		log.Fatal("-database is required")
	}
	result, err := migrate(*databasePath, *replace, *keepLegacy)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("migrated %d pricing rules, %d usage events, %d model aggregates, and %d API key aggregates\n", result.rules, result.events, result.models, result.apiKeys)
}

type migrationResult struct {
	rules   int
	events  int
	models  int
	apiKeys int
}

func migrate(databasePath string, replace, keepLegacy bool) (migrationResult, error) {
	db, err := sql.Open("sqlite3", "file:"+databasePath+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return migrationResult{}, fmt.Errorf("open billing database: %w", err)
	}
	defer db.Close()

	var raw []byte
	if err := db.QueryRow(`SELECT state_json FROM billing_state WHERE id = 1`).Scan(&raw); err != nil {
		return migrationResult{}, fmt.Errorf("read legacy billing_state snapshot: %w", err)
	}
	var state billing.State
	if err := json.Unmarshal(raw, &state); err != nil {
		return migrationResult{}, fmt.Errorf("decode legacy billing_state snapshot: %w", err)
	}
	normalizeLegacyState(&state)

	if err := billing.EnsureSchema(db); err != nil {
		return migrationResult{}, err
	}
	var existing int
	if err := db.QueryRow(`SELECT COUNT(*) FROM billing_settings`).Scan(&existing); err != nil {
		return migrationResult{}, fmt.Errorf("inspect normalized billing settings: %w", err)
	}
	if existing > 0 && !replace {
		return migrationResult{}, fmt.Errorf("normalized billing tables already contain data; rerun with -replace to overwrite them")
	}
	if err := billing.ReplaceState(db, state); err != nil {
		return migrationResult{}, fmt.Errorf("write normalized billing data: %w", err)
	}
	if !keepLegacy {
		if _, err := db.Exec(`DROP TABLE billing_state`); err != nil {
			return migrationResult{}, fmt.Errorf("drop migrated billing_state table: %w", err)
		}
	}
	return migrationResult{rules: len(state.Rules), events: len(state.Events), models: len(state.Aggregates), apiKeys: len(state.APIKeyAggregates)}, nil
}

func normalizeLegacyState(state *billing.State) {
	if state.Currency == "" {
		state.Currency = "USD"
	}
	for index := range state.Events {
		event := &state.Events[index]
		event.Source = billing.MaskSensitiveSource(event.Source)
		if event.APIKeyID == "" && event.APIKey != "" {
			event.APIKeyID = "legacy:" + event.APIKey
		}
	}
	if state.Aggregates == nil {
		state.Aggregates = rebuildModelAggregates(state.Events)
	}
	if state.APIKeyAggregates == nil {
		state.APIKeyAggregates = rebuildAPIKeyAggregates(state.Events)
	}
}

func rebuildModelAggregates(events []billing.UsageEvent) map[string]*billing.Aggregate {
	result := map[string]*billing.Aggregate{}
	for _, event := range events {
		key := strings.ToLower(strings.TrimSpace(event.Provider)) + "/" + strings.ToLower(strings.TrimSpace(event.Model))
		aggregate := result[key]
		if aggregate == nil {
			aggregate = &billing.Aggregate{Provider: event.Provider, Model: event.Model, Priced: true}
			result[key] = aggregate
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
		aggregate.Priced = aggregate.Priced && (event.PricedBy != "" && (event.PricedBy != "*" || event.Cost > 0))
	}
	return result
}

func rebuildAPIKeyAggregates(events []billing.UsageEvent) map[string]*billing.APIKeyAggregate {
	result := map[string]*billing.APIKeyAggregate{}
	for _, event := range events {
		label := strings.TrimSpace(event.APIKey)
		if label == "" {
			label = "未提供"
		}
		key := event.APIKeyID
		if key == "" {
			key = "legacy:" + label
		}
		aggregate := result[key]
		if aggregate == nil {
			aggregate = &billing.APIKeyAggregate{APIKey: label}
			result[key] = aggregate
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
	return result
}
