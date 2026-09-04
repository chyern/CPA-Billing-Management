package main

import (
	"encoding/json"
	"net/http"

	"github.com/chyern/CPA-Billing-Management/internal/abi"
)

// handleMethod is the application-level dispatcher behind the C ABI adapter.
// Each protocol capability delegates to its own handler module.
func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case abi.MethodPluginRegister, abi.MethodPluginReconfigure:
		return handleLifecycle(request)
	case abi.MethodUsageHandle:
		billingStore, err := getBillingStore("")
		if err != nil {
			return nil, err
		}
		if err := handleUsage(billingStore, request); err != nil {
			return okEnvelope(map[string]any{"accepted": false, "error": err.Error()})
		}
		return okEnvelope(map[string]any{"accepted": true})
	case abi.MethodRequestInterceptBefore:
		billingStore, err := getBillingStore("")
		if err != nil {
			return nil, err
		}
		return handleRequestInterceptBefore(billingStore, request)
	case abi.MethodRequestInterceptAfter:
		return handleRequestInterceptAfter(request)
	case abi.MethodManagementRegister:
		return okEnvelope(managementRegistration())
	case abi.MethodManagementHandle:
		billingStore, err := getBillingStore("")
		if err != nil {
			return nil, err
		}
		return handleManagement(billingStore, request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func handleLifecycle(request []byte) ([]byte, error) {
	var lifecycle abi.LifecycleRequest
	dataDir := ""
	if len(request) > 0 && json.Unmarshal(request, &lifecycle) == nil {
		dataDir = configuredDataDir(lifecycle.ConfigYAML)
	}
	billingStore, err := getBillingStore(dataDir)
	if err != nil {
		return nil, err
	}
	if len(request) > 0 {
		billingStore.ConfigureYAML(lifecycle.ConfigYAML)
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
			{Method: http.MethodPut, Path: "/cpa-billing-management/key-balances"},
			{Method: http.MethodPost, Path: "/cpa-billing-management/reset"},
		},
	}
}
