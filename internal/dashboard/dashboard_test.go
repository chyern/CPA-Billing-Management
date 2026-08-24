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
	for _, expected := range []string{"CPA 费用统计", "按 API Key 汇总", "api_keys", "failed_requests", "耗时/首字", "输入", "缓存", "cached_tokens", "latency_ns", "ttft_ns", "自动刷新", "不刷新", "5 秒", "10 秒", "15 秒", "上一页", "下一页", "recent_events_total", "v0/management/cpa-billing-management/summary", "format=fallback-json", "X-Management-Key"} {
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

func TestRenderContainsSeparateModelCostDashboard(t *testing.T) {
	raw, err := RenderPricing(Data{Rules: []any{}, Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{"CPA 模型费用", "模型价格规则", "保存模型费用", "输入 / 1M", "输出 / 1M", "v0/management/cpa-billing-management/prices", "format=fallback-json", "X-Management-Key", "USD"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("rendered model-cost dashboard does not contain %q", expected)
		}
	}
	for _, unexpected := range []string{"最近事件", "按模型汇总", "管理 API Token"} {
		if strings.Contains(text, unexpected) {
			t.Fatalf("model-cost dashboard must not contain %q", unexpected)
		}
	}
}

func TestRenderUsesConfiguredManagementKey(t *testing.T) {
	raw, err := RenderBilling(Data{ManagementKey: "test-management-secret"})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{"test-management-secret", "Authorization", "Bearer '+MANAGEMENT_KEY"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("rendered billing dashboard does not contain %q", expected)
		}
	}
}
