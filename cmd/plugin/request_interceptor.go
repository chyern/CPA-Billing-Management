package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/chyern/CPA-Billing-Management/internal/abi"
	"github.com/chyern/CPA-Billing-Management/internal/billing"
)

const callerScopeMetadataKey = "caller_scope"

func handleRequestInterceptBefore(store *billing.Store, raw []byte) ([]byte, error) {
	var req abi.RequestInterceptRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("decode request interception: %w", err)
	}
	callerScope, _ := req.Metadata[callerScopeMetadataKey].(string)
	callerScope = strings.TrimSpace(callerScope)
	if callerScope == "" {
		return okEnvelope(abi.RequestInterceptResponse{})
	}
	balance, configured, err := store.BalanceForCallerScope(callerScope)
	if err != nil {
		return nil, err
	}
	if !configured || balance > 0 {
		return okEnvelope(abi.RequestInterceptResponse{})
	}
	body, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"type":    "insufficient_api_key_balance",
			"message": "API key balance is insufficient",
		},
	})
	if err != nil {
		return nil, err
	}
	return okEnvelope(abi.RequestInterceptResponse{
		Terminate:       true,
		StatusCode:      http.StatusPaymentRequired,
		ResponseHeaders: map[string][]string{"Content-Type": {"application/json; charset=utf-8"}},
		ResponseBody:    body,
	})
}

func handleRequestInterceptAfter(raw []byte) ([]byte, error) {
	var req abi.RequestInterceptRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("decode request interception: %w", err)
	}
	return okEnvelope(abi.RequestInterceptResponse{})
}
