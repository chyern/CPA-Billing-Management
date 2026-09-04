# CPA Billing Management

CLIProxyAPI 自定义插件：接收 usage 事件，优先使用上游金额，否则按模型价格估算费用，并通过管理页面展示账单汇总。

## 当前功能

- 监听 CLIProxyAPI `UsagePlugin` 事件；
- 上游回调明确包含费用字段时直接使用该金额；
- 上游未返回金额时，按输入、输出、缓存读取和缓存创建 token 以及模型价格规则估算费用；
- 费用在 usage 事件写入时计算并固化，后续修改模型价格只影响新事件，不会联动修改历史费用；
- 支持模型名、alias 和 `*` 通配价格规则；
- 使用结构化 SQLite 表持久化设置、价格规则、usage 事件、按模型汇总和按 API Key 汇总；数据库文件为数据目录下的 `billing.db`，默认目录是操作系统用户配置目录下的 `cliproxyapi/cpa-billing-management`；
- 在 CLIProxyAPI 管理页增加“费用统计”菜单，展示总费用、按模型汇总、按脱敏 API Key 汇总，以及最近请求的总耗时和首 Token 耗时；最近事件支持分页和可选的 5/10/15 秒自动刷新；
- 在独立的“模型费用”页面编辑价格规则；费用页只列出 CLIProxyAPI `/v1/models` 当前暴露的模型，没有匹配价格规则的模型默认显示为 0，并标记为“未配置模型费用”。同步价格时先获取本地与上游的差异，确认后再保存，避免一次拉取直接覆盖人工调整。
- 模型费用页默认不添加价格规则；支持按需从 [LiteLLM 公共模型价格目录](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json)、[Models.dev](https://models.dev/) 或 [OpenRouter Models API](https://openrouter.ai/docs/api-reference/list-available-models) 同步当前已使用且可识别的模型，未识别的模型仍可手动配置。
- 提供独立的“密钥余额”页面，可为客户端 API Key 设置当前余额；后续 usage 事件产生费用时自动扣减，并展示累计请求、累计费用和余额状态。已设置且余额耗尽的密钥会在访问上游前返回 HTTP 402；未设置余额的密钥继续放行。完整密钥不会写入账单数据库。

当前 CLIProxyAPI 的 `UsagePlugin` ABI 主要提供 token、耗时等字段，通常不包含金额，因此大多数文本模型会走模型价格估算。价格单位是配置币种/每百万 token，估算结果请以供应商账单为准。

## 项目结构

项目按协议适配、应用编排和领域能力分层，新增功能时应放入对应模块，避免继续扩张入口文件：

- `cmd/plugin/main.go`：仅保留 C ABI 边界、插件存储实例和内存释放；
- `cmd/plugin/dispatcher.go`：插件方法分发及生命周期注册；
- `cmd/plugin/usage_handler.go`：将宿主 usage JSON 转换为领域记录；
- `cmd/plugin/management_handler.go`：管理 API 和资源页路由；
- `cmd/plugin/pricing_sync.go`、`pricing_catalog.go`：上游价格同步流程与目录格式解析；
- `internal/billing`：账单领域层，分别维护模型、存储、用量聚合、定价、查询和敏感信息脱敏；
- `internal/dashboard`：页面资源装配；`assets/` 下分别维护公共 CSS、鉴权脚本、费用页和模型费用页的 HTML/JS，构建时通过 `go:embed` 嵌入插件；
- `internal/abi`：CLIProxyAPI 插件协议的数据结构。

## 构建

需要 Go 1.26、CGO 和本机 C 编译器：

```bash
make test
make build
```

构建产物为 `bin/cpa-billing-management.dylib`。Linux 发布包会包含
`cpa-billing-management.so`，当前提供 `linux/amd64` 架构。

插件版本由发布 Tag 注入插件元数据：GitHub Actions 从 `vMAJOR.MINOR.PATCH`
Tag 构建时写入对应的 `MAJOR.MINOR.PATCH`；本地非 Tag 构建显示为 `dev`，在
精确 Tag checkout 上运行 `make build` 则会自动使用该 Tag 版本。这样源码、构建产物
和发布版本不会再依赖手工同步的版本常量。

## 安装与配置

### 通过第三方插件源安装

在 CLIProxyAPI 管理页的“配置面板 → 高级与实验 → 第三方插件源”中添加：

```text
https://raw.githubusercontent.com/chyern/CPA-Billing-Management/main/registry.json
```

保存配置后进入“插件商店”，刷新插件源并安装 **CPA Billing Management**。
插件安装包来自此仓库的 GitHub Release，第三方源通过固定版本、平台和 SHA-256 声明直接安装，
因此不会消耗 GitHub Releases API 的匿名请求额度。发布新版本时，GitHub Actions 会在正式
Release 构建完成后自动计算产物 SHA-256，并使用 GitHub App 直接更新本项目 `main` 分支上的
`registry.json`。

当前发布流程提供 Darwin arm64（Apple Silicon Mac）和 Linux amd64 产物。
本项目的 GitHub Actions 只响应 `v*` 发布标签；推送普通分支、提交 Pull Request 或手动操作不会触发构建。
推送 `v*` 标签时会自动测试、构建两个平台的产物并创建正式 GitHub Release。

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
      cpa_billing_data_dir: /absolute/path/to/billing-data
```

`cpa_billing_data_dir` 可直接在 CPA 管理页的插件配置中填写；留空时使用插件动态库所在的安装目录。

### 本地目录安装（开发/快速更新）

如果插件源码就在本机，不需要通过插件商店下载。先构建并运行 ABI 冒烟测试，再把版本化插件文件复制到 CLIProxyAPI 的本地插件目录：

```bash
make smoke
make install-local \\
  CPA_PLUGIN_DIR=/absolute/path/to/.cli-proxy-api/plugins/darwin/arm64
```

`install-local` 会直接覆盖当前 Tag 对应的同版本插件文件，不创建本地备份。以后重新执行 `make install-local` 并重启 CLIProxyAPI 即可加载新构建；账单数据库和插件配置不会被修改。

启动 CLIProxyAPI 后，在管理页进入“费用统计”查看账单，或进入“模型费用”维护价格。管理 API 路由为：

- `GET /v0/management/cpa-billing-management/summary`
- `GET /v0/management/cpa-billing-management/prices`
- `PUT /v0/management/cpa-billing-management/prices`
- `POST /v0/management/cpa-billing-management/prices/sync`
- `GET /v0/management/cpa-billing-management/key-balances`
- `PUT /v0/management/cpa-billing-management/key-balances`
- `POST /v0/management/cpa-billing-management/reset`

`POST /v0/management/cpa-billing-management/prices/sync` 默认使用 LiteLLM；模型费用页面可选择 LiteLLM、Models.dev 或 OpenRouter，并通过 `source=litellm`、`source=models.dev` 或 `source=openrouter` 查询参数指定来源。接口必须传入 `preview=1`，只返回拟新增/拟更新规则，不写入数据库；用户确认后通过已有的 `PUT /prices` 保存。同步只下载公开价格目录，不会上传本地账单数据；不同来源的价格单位会统一转换为当前币种/每百万 token。

插件资源页面为：

`/v0/resource/plugins/cpa-billing-management/billing`

模型费用资源页面为：

`/v0/resource/plugins/cpa-billing-management/pricing`

密钥余额资源页面为：

`/v0/resource/plugins/cpa-billing-management/wallet`

资源页面会复用 CLIProxyAPI 管理中心的浏览器登录状态：从同源 `localStorage` 的
`cli-proxy-auth` 读取管理密钥，兼容管理中心的 `enc::v1::` 混淆格式，并通过
`Authorization: Bearer <management-key>` 调用插件管理 API。管理中心勾选“记住密码”后，
重新打开资源页会自动恢复；管理密钥缺失或 API 返回 401 时，页面会跳转到
`/management.html#/login`。插件不再在配置或资源 HTML 中注入第二份管理密钥。
