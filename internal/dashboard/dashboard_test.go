package dashboard

import (
	"strings"
	"testing"
)

func TestRenderContainsBillingDashboard(t *testing.T) {
	raw, err := Render(Data{Summary: map[string]any{"currency": "USD"}, Rules: []any{}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{"CPA 费用统计", "价格规则", "v0/management/cpa-billing-management"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("rendered dashboard does not contain %q", expected)
		}
	}
}
