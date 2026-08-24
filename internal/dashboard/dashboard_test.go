package dashboard

import (
	"strings"
	"testing"
)

func TestRenderContainsBillingDashboard(t *testing.T) {
	raw, err := RenderBilling(Data{Summary: map[string]any{"currency": "USD"}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{"CPA 费用统计", "API Key", "耗时", "latency_ns", "自动刷新", "不刷新", "5 秒", "10 秒", "15 秒", "上一页", "下一页", "recent_events_total", "v0/resource/plugins/cpa-billing-management/billing"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("rendered dashboard does not contain %q", expected)
		}
	}
	if strings.Contains(text, "管理 API Token") {
		t.Fatal("rendered dashboard must not request a management API token")
	}
	for _, unexpected := range []string{"价格规则", "保存价格", "数据直接读取自本机插件存储"} {
		if strings.Contains(text, unexpected) {
			t.Fatalf("billing dashboard must not contain %q", unexpected)
		}
	}
	if strings.Contains(text, `<button class="btn" id="refresh">刷新</button>`) {
		t.Fatal("billing dashboard must not contain a manual refresh button")
	}
}

func TestRenderContainsSeparatePricingDashboard(t *testing.T) {
	raw, err := RenderPricing(Data{Rules: []any{}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{"CPA 价格配置", "价格规则", "保存价格", "v0/resource/plugins/cpa-billing-management/pricing"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("rendered pricing dashboard does not contain %q", expected)
		}
	}
	for _, unexpected := range []string{"最近事件", "按模型汇总", "数据直接读取自本机插件存储"} {
		if strings.Contains(text, unexpected) {
			t.Fatalf("pricing dashboard must not contain %q", unexpected)
		}
	}
}
