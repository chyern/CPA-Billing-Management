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
	for _, expected := range []string{"CPA 费用统计", "价格规则", "API Key", "耗时", "latency_ns", "v0/resource/plugins/cpa-billing-management/billing", "数据直接读取自本机插件存储"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("rendered dashboard does not contain %q", expected)
		}
	}
	if strings.Contains(text, "管理 API Token") {
		t.Fatal("rendered dashboard must not request a management API token")
	}
}
