package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/chyern/CPA-Billing-Management/internal/abi"
)

// handleMethod is the application-level dispatcher behind the C ABI adapter.
// Each protocol capability delegates to its own handler module.
func handleMethod(method string, request []byte) ([]byte, error) {
	if method == abi.MethodPluginRegister || method == abi.MethodPluginReconfigure {
		return handleLifecycle(request)
	}
	lifecycleMu.RLock()
	defer lifecycleMu.RUnlock()
	switch method {
	case abi.MethodUsageHandle:
		billingStore, err := getBillingStore()
		if err != nil {
			return nil, err
		}
		if err := handleUsage(billingStore, request); err != nil {
			return okEnvelope(map[string]any{"accepted": false, "error": err.Error()})
		}
		return okEnvelope(map[string]any{"accepted": true})
	case abi.MethodRequestInterceptBefore:
		billingStore, err := getBillingStore()
		if err != nil {
			return nil, err
		}
		return handleRequestInterceptBefore(billingStore, request)
	case abi.MethodRequestInterceptAfter:
		return handleRequestInterceptAfter(request)
	case abi.MethodManagementRegister:
		return okEnvelope(managementRegistration())
	case abi.MethodManagementHandle:
		billingStore, err := getBillingStore()
		if err != nil {
			return nil, err
		}
		return handleManagement(billingStore, request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func handleLifecycle(request []byte) ([]byte, error) {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	var lifecycle abi.LifecycleRequest
	if len(request) > 0 {
		if err := json.Unmarshal(request, &lifecycle); err != nil {
			return nil, fmt.Errorf("decode lifecycle request: %w", err)
		}
	}
	if _, err := configureBillingStore(configuredDataDir(lifecycle.ConfigYAML), lifecycle.ConfigYAML); err != nil {
		return nil, err
	}
	return okEnvelope(registration())
}

func registration() abi.Registration {
	return abi.Registration{
		SchemaVersion: abi.SchemaVersion,
		Metadata: abi.Metadata{
			Name:             pluginID,
			Version:          pluginVersion,
			Author:           "CPA Billing Management",
			GitHubRepository: "https://github.com/chyern/CPA-Billing-Management",
			ConfigFields: []abi.ConfigField{
				{Name: "currency", Type: "string", Description: "费用展示币种，默认 USD；事件未携带币种时使用此值。"},
				{Name: "cpa_billing_data_dir", Type: "string", Description: "cpa_billing_data_dir：SQLite 账单数据库目录；留空时使用插件安装目录。"},
			},
		},
		Capabilities: abi.Capabilities{UsagePlugin: true, RequestInterceptor: true, ManagementAPI: true},
	}
}

func managementRegistration() abi.ManagementRegistrationResponse {
	return abi.ManagementRegistrationResponse{
		Resources: []abi.ResourceRoute{
			{Path: "/billing", Menu: "费用统计", Description: "查看 usage 事件、token 用量和费用汇总。"},
			{Path: "/pricing", Menu: "模型费用", Description: "配置模型每百万 token 的估算价格。"},
			{Path: "/wallet", Menu: "密钥余额", Description: "查看并维护客户端 API Key 的当前余额。"},
		},
		Routes: []abi.ManagementRoute{
			{Method: http.MethodGet, Path: "/cpa-billing-management/summary"},
			{Method: http.MethodGet, Path: "/cpa-billing-management/prices"},
			{Method: http.MethodPut, Path: "/cpa-billing-management/prices"},
			{Method: http.MethodPost, Path: "/cpa-billing-management/prices/sync"},
			{Method: http.MethodGet, Path: "/cpa-billing-management/key-balances"},
			{Method: http.MethodPatch, Path: "/cpa-billing-management/key-balances"},
			{Method: http.MethodPost, Path: "/cpa-billing-management/reset"},
		},
	}
}
