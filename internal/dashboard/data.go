package dashboard

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Data struct {
	Summary  any `json:"summary,omitempty"`
	Rules    any `json:"rules,omitempty"`
	Currency any `json:"currency,omitempty"`
}

// Render is kept as the billing-page entry point for preview integrations.
func Render(data Data) ([]byte, error) { return RenderBilling(data) }

func initialJSON(data Data) (string, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("encode dashboard data: %w", err)
	}
	// JSON is embedded in a script element. Escape HTML-significant bytes so a
	// model name cannot terminate the element and inject markup.
	return strings.NewReplacer("<", "\\u003c", ">", "\\u003e", "&", "\\u0026").Replace(string(raw)), nil
}
