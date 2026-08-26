package main

import (
	"encoding/json"
	"net/http"

	"github.com/chyern/CPA-Billing-Management/internal/abi"
)

func htmlManagementResponse(page []byte) ([]byte, error) {
	return okEnvelope(abi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers: map[string][]string{
			"Content-Type":  {"text/html; charset=utf-8"},
			"Cache-Control": {"no-store"},
		},
		Body: page,
	})
}

func jsonManagementResponse(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return okEnvelope(abi.ManagementResponse{StatusCode: http.StatusOK, Headers: map[string][]string{"Content-Type": {"application/json"}, "Cache-Control": {"no-store"}}, Body: body})
}

func jsonManagementError(status int, message string) ([]byte, error) {
	body, _ := json.Marshal(map[string]any{"error": message})
	return okEnvelope(abi.ManagementResponse{StatusCode: status, Headers: map[string][]string{"Content-Type": {"application/json"}}, Body: body})
}

func okEnvelope(value any) ([]byte, error) {
	result, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(abi.Envelope{OK: true, Result: json.RawMessage(result)})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(abi.Envelope{OK: false, Error: &abi.EnvelopeError{Code: code, Message: message}})
	return raw
}
