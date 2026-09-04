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
	for _, expected := range []string{"CPA 费用统计", "按 API Key 汇总", "api_keys", "failed_requests", "耗时/首字", "输入/缓存", "cached_tokens", "latency_ns", "ttft_ns", "自动刷新", "不刷新", "5 秒", "10 秒", "15 秒", "上一页", "下一页", "recent_events_total", "v0/management/cpa-billing-management/summary", "cli-proxy-auth", "enc::v1::", "Authorization:'Bearer '+MANAGEMENT_KEY", "/management.html#/login"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("rendered dashboard does not contain %q", expected)
		}
	}
	if strings.Contains(text, "X-Management-Key") || strings.Contains(text, "format=fallback-json") {
		t.Fatal("billing dashboard must use the management center auth contract without fallback credentials")
	}
	for _, unexpected := range []string{"价格规则", "保存价格", "数据直接读取自本机插件存储", "页面切换", "nav-tabs"} {
		if strings.Contains(text, unexpected) {
			t.Fatalf("billing dashboard must not contain %q", unexpected)
		}
	}
	if strings.Contains(text, `<button class="btn" id="refresh">刷新</button>`) {
		t.Fatal("billing dashboard must not contain a manual refresh button")
	}
	if !strings.Contains(text, `<option value="15" selected>15 秒</option>`) {
		t.Fatal("billing dashboard must default to 15-second auto refresh")
	}
}

func TestRenderContainsSeparateModelCostDashboard(t *testing.T) {
	raw, err := RenderPricing(Data{Rules: []any{}, Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{"CPA 模型费用", "模型价格规则", "同步上游价格", "价格来源", "LiteLLM 公共目录", "Models.dev 公共目录", "OpenRouter 模型 API", "cpa-billing-pricing-source", "source=", "新增规则", "保存模型费用", "输入 / 1M", "输出 / 1M", "v0/management/cpa-billing-management/prices", "/sync", "placeholder=\"例如：openai/gpt-4o\"", "Number.isFinite", "CLIProxyAPI 内置模型不能删除", "cli-proxy-auth", "enc::v1::", "Authorization:'Bearer '+MANAGEMENT_KEY", "/management.html#/login", "sync-panel", "sync-controls", "rules-panel", "--muted-bg: #262320", "background: var(--muted-bg)", "font-size: 12px", "font-weight: 500", "USD"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("rendered model-cost dashboard does not contain %q", expected)
		}
	}
	for _, unexpected := range []string{"最近事件", "按模型汇总", "X-Management-Key", "format=fallback-json", `id="refresh"`, "model-name", "页面切换", "nav-tabs"} {
		if strings.Contains(text, unexpected) {
			t.Fatalf("model-cost dashboard must not contain %q", unexpected)
		}
	}
}

func TestRenderContainsKeyBalanceDashboard(t *testing.T) {
	raw, err := RenderBalances(Data{})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{"CPA 密钥余额", "API Key 余额", "仅显示 CLIProxyAPI 当前配置的 API Key", "保持配置顺序", "备注", "填写密钥用途", "当前余额", "累计费用", "操作", "保存", "删除", "确定要删除", "添加", "new-key-input", "generateAPIKey", "getRandomValues", "pending", "disabled", "PUT", "/v0/management/api-keys", "key-balances", "crypto.subtle", "完整密钥不会写入账单数据库"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("rendered key-balance dashboard does not contain %q", expected)
		}
	}
	for _, unexpected := range []string{"页面切换", "nav-tabs", "模型价格规则", "最近事件"} {
		if strings.Contains(text, unexpected) {
			t.Fatalf("key-balance dashboard must not contain %q", unexpected)
		}
	}
}

func TestRenderDoesNotEmbedManagementKey(t *testing.T) {
	raw, err := RenderBilling(Data{})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{"readManagementKey", "AUTH_STORAGE_KEY", "Authorization:'Bearer '+MANAGEMENT_KEY", "cli-proxy-theme", "MutationObserver", "data-theme"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("rendered billing dashboard does not contain %q", expected)
		}
	}
	if strings.Contains(text, "test-management-secret") || strings.Contains(text, "management_key") {
		t.Fatal("rendered billing dashboard must not embed a configured management key")
	}
}

func TestRenderResolvesEmbeddedAssetPlaceholders(t *testing.T) {
	for name, render := range map[string]func(Data) ([]byte, error){
		"billing":  RenderBilling,
		"pricing":  RenderPricing,
		"balances": RenderBalances,
	} {
		raw, err := render(Data{})
		if err != nil {
			t.Fatalf("render %s page: %v", name, err)
		}
		for _, placeholder := range []string{"{{STYLES}}", "{{AUTH_SCRIPT}}", "{{PAGE_SCRIPT}}", "{{INITIAL_JSON}}"} {
			if strings.Contains(string(raw), placeholder) {
				t.Fatalf("rendered %s page still contains %s", name, placeholder)
			}
		}
	}
}

func TestRenderEscapesEmbeddedJSON(t *testing.T) {
	raw, err := RenderBilling(Data{Summary: map[string]any{"model": "</script><script>alert(1)</script>"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "</script><script>alert(1)</script>") || !strings.Contains(string(raw), `\u003c/script\u003e`) {
		t.Fatal("embedded dashboard JSON was not escaped")
	}
}

func TestPricingRuleHeadersAlignWithInputs(t *testing.T) {
	raw, err := RenderPricing(Data{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), ".rules th.num { text-align: left; }") {
		t.Fatal("pricing rule numeric headers must align to the leading edge of their inputs")
	}
}
