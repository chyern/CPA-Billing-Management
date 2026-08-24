# CPA Billing Management

CLIProxyAPI 自定义插件：接收 usage 事件，按模型价格计算费用，并通过管理页面展示账单汇总。

## 当前功能

- 监听 CLIProxyAPI `UsagePlugin` 事件；
- 按输入、输出、缓存读取和缓存创建 token 计算估算费用；
- 支持 `provider/model`、模型名、alias 和 `*` 通配价格规则；
- 将账单状态持久化到插件数据目录，默认是操作系统用户配置目录下的 `cliproxyapi/cpa-billing-management`；
- 在 CLIProxyAPI 管理页增加“费用统计”菜单，展示总费用、按模型汇总和最近请求；
- 在管理页面编辑价格规则；未知模型默认费用为 0，并标记为“未定价”。

费用公式为：

```text
费用 = (输入 token - 缓存读取 token - 缓存创建 token) / 1,000,000 × 输入单价
     + 输出 token / 1,000,000 × 输出单价
     + 缓存读取 token / 1,000,000 × 缓存读取单价
     + 缓存创建 token / 1,000,000 × 缓存创建单价
```

价格单位是配置币种/每百万 token。此结果是估算值，最终账单应以供应商账单为准。

## 构建

需要 Go 1.26、CGO 和本机 C 编译器：

```bash
make test
make build
```

构建产物为 `bin/cpa-billing-management.dylib`。Linux 上可按 CLIProxyAPI 的插件加载约定将输出文件名改为 `.so`。

## 安装与配置

### 通过第三方插件源安装

在 CLIProxyAPI 管理页的“配置面板 → 高级与实验 → 第三方插件源”中添加：

```text
https://raw.githubusercontent.com/chyern/CPA-Billing-Management/main/registry.json
```

保存配置后进入“插件商店”，刷新插件源并安装 **CPA Billing Management**。
插件安装包来自此仓库的 GitHub Release，第三方源通过固定版本、平台和 SHA-256 声明直接安装，
因此不会消耗 GitHub Releases API 的匿名请求额度。发布新版本时需要同步更新 `registry.json`。

当前发布流程提供 Darwin arm64 产物，适用于 Apple Silicon Mac。

### 手动安装

将动态库放入 CLIProxyAPI 的插件目录，并在配置中启用：

```yaml
plugins:
  enabled: true
  dir: /absolute/path/to/plugins
  configs:
    cpa-billing-management:
      enabled: true
      priority: 1
      currency: USD
```

如果需要指定账单数据目录，可以设置环境变量：

```bash
export CPA_BILLING_DATA_DIR=/var/lib/cliproxyapi/billing
```

启动 CLIProxyAPI 后，在管理页进入“费用统计”。管理 API 路由为：

- `GET /v0/management/cpa-billing-management/summary`
- `GET /v0/management/cpa-billing-management/prices`
- `PUT /v0/management/cpa-billing-management/prices`
- `POST /v0/management/cpa-billing-management/reset`

插件资源页面为：

`/v0/resource/plugins/cpa-billing-management/billing`

资源页面直接读取插件本地账单存储，不需要重复输入管理 API Token；刷新和价格修改也通过插件资源路由完成。
带管理认证的 API 仍保留给外部自动化使用。若 CLIProxyAPI 对公网开放，请同时限制插件资源路由的网络访问，
因为资源页面会展示账单数据并允许修改本地价格规则。
