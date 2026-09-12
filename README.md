# CPA Billing Management

CLIProxyAPI 自定义插件：接收 usage 事件，优先使用上游金额，否则按模型价格估算费用，并通过管理页面展示账单汇总。

## 当前功能

- 监听 CLIProxyAPI `UsagePlugin` 事件；
- 上游回调明确包含费用字段时直接使用该金额；
- 上游未返回金额时，按输入、输出、缓存读取和缓存创建 token 以及模型价格规则估算费用；
- 费用在 usage 事件写入时计算并固化，后续修改模型价格只影响新事件，不会联动修改历史费用；
- 支持模型名、alias 和 `*` 通配价格规则；
- 按页面职责使用 SQLite 表持久化设置、价格规则、usage 快照、模型累计统计和密钥账户；数据库文件为数据目录下的 `billing.db`，默认目录是插件动态库所在的安装目录；
- 在 CLIProxyAPI 管理页增加“费用统计”菜单，展示总费用、按模型汇总、按脱敏 API Key 汇总，以及最近请求的思考强度、上游、总耗时和首 Token 耗时；最近事件直接展示 `usage_events` 中的不可变快照；上游在写入时分别保存 `provider` 和 `domain`，页面按 `Provider(域名)` 展示；仅有 Provider 时显示 Provider，两者均缺失时显示“—”，不在查询时关联配置或补填历史值；最近事件支持分页和可选的 5/10/15 秒自动刷新；
- 在独立的“模型费用”页面编辑价格规则；费用页只列出 CLIProxyAPI `/v1/models` 当前暴露的模型，没有匹配价格规则的模型默认显示为 0，并标记为“未配置模型费用”。同步价格时先获取本地与上游的差异，确认后再保存，避免一次拉取直接覆盖人工调整。
- 模型费用页会将 CLIProxyAPI 当前暴露的模型加入编辑器作为零价占位（已有规则和通配规则保持不变）；支持按需从 [LiteLLM 公共模型价格目录](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json)、[Models.dev](https://models.dev/) 或 [OpenRouter Models API](https://openrouter.ai/docs/api-reference/list-available-models) 同步价格，未识别的模型仍可手动配置。
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

需要 Go 1.26、CGO 和本机 C 编译器；`make test` 还需要 Node.js 24 来运行页面回归测试：

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
      cpa_host_config_path: /absolute/path/to/cliproxyapi.conf
```

`cpa_billing_data_dir` 可直接在 CPA 管理页的插件配置中填写；留空时使用插件动态库所在的安装目录。
目录配置会一直用于后续请求，直到再次修改配置；清空该配置会切回默认目录。若新目录无法打开，插件保留原有可用存储并返回错误。

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
- `PATCH /v0/management/cpa-billing-management/key-balances`（按单个 Key 增量更新；余额变更带版本校验）
- `POST /v0/management/cpa-billing-management/reset`

`POST /v0/management/cpa-billing-management/prices/sync` 默认使用 LiteLLM；模型费用页面可选择 LiteLLM、Models.dev 或 OpenRouter，并通过 `source=litellm`、`source=models.dev` 或 `source=openrouter` 查询参数指定来源。接口必须传入 `preview=1`，只返回拟新增/拟更新规则，不写入数据库；用户确认后通过已有的 `PUT /prices` 保存。同步只下载公开价格目录，不会上传本地账单数据；不同来源的价格单位会统一转换为当前币种/每百万 token。

新接收的账单事件以表格快照保存在 SQLite 中；启动时内存只加载最近 10,000 条。`GET /summary` 支持 `start`、`end`、`page`、`page_size` 及 `event_status=all|success|failed`；状态只筛选最近事件及其分页数量，顶部汇总仍使用整个日期范围。全部时间与日期筛选统一从 `usage_events` 快照直接汇总，不使用模型累计表或内存累计缓存。修改价格规则不会重写历史事件。

`usage_events` 只保存行主键以及时间、模型、思考强度、Provider、域名、脱敏 API Key、总耗时、首字耗时、输入 token、缓存 token、输出 token、费用金额、费用币种和状态。升级会原子迁移旧事件表，保留已有可展示值并删除内部字段；未保存过的上游和币种留空，不回填。旧 `upstream` 快照会原子拆成 `provider` 与 `domain`，不读取当前配置。

最近事件直接查询此表，不联表，也不在页面读取上游配置。按 API Key 汇总直接以表中脱敏 `api_key` 分组（脱敏后相同的密钥合为一组），总 token 从输入加输出计算。所有时间范围的 Provider 分类直接使用快照中的 `provider`；没有上游快照的旧记录归入未记录分类。余额管理自己的密钥标识不参与事件表查询。

数据库版本 6 按页面保留以下业务表（不计 SQLite 自带的 `sqlite_sequence`）：

| 表 | 页面与职责 | 字段 |
| --- | --- | --- |
| `billing_settings` | 全局币种和结构版本 | `id`, `schema_version`, `currency`, `updated_at` |
| `pricing_rules` | 模型费用页的有序价格规则 | `position`, `match`, `input_per_million`, `output_per_million`, `cache_read_per_million`, `cache_creation_per_million` |
| `api_key_accounts` | 密钥余额页直接查询的账户记录 | `api_key_id`, `api_key`, `caller_scope`, `balance`, `note`, `requests`, `cost`, `balance_version`, `updated_at` |
| `usage_events` | 独立事件快照及按日期、脱敏密钥汇总 | 上述快照字段 |

版本 7 新增 `reasoning_effort` 思考强度快照，读取宿主 usage 事件的 `ReasoningEffort`；历史记录或宿主未提供该字段时留空，页面显示“—”。

版本 6 删除 `model_aggregates`。模型分组、请求数、失败数、token 和费用全部直接查询 `usage_events`；模型分组仍忽略首尾空格及英文大小写，总 token 使用输入加输出。历史缺失的 Provider 保持为空，不从旧汇总表补齐。快照未保存的计价状态也不根据当前规则推断。

`api_key_accounts` 合并旧 `api_key_aggregates`、`api_key_balances`、`api_key_balance_notes`，只保留余额页使用的请求数和累计费用，去掉其余 token 和失败计数。`api_key` 保存脱敏文本；`api_key_id` 是完整客户端密钥经 SHA-256 后前 8 字节的十六进制标识，用于准确扣款；`caller_scope` 用于宿主请求拦截，非空时唯一。这两个标识不关联事件快照。`balance=NULL` 表示未管理余额，零或负数表示已启用且耗尽。`balance_version` 只随余额变更更新，备注编辑独立；`updated_at` 记录账户更新时间。

升级在一个事务中迁移旧账户数据（如有）、删除旧账户表和模型累计表并更新版本，失败全部回滚。已有余额（含负值）、备注、累计请求、累计费用和余额版本原样保留；不重算或补写历史事件。清空统计仅清零账户请求数与累计费用，保留余额和备注。删除密钥账户设置会移除余额与备注；已有累计用量仍保留。

配置 `cpa_host_config_path` 为 CPA 主配置文件的绝对路径后，新 usage 事件会在写入前按宿主凭据标识核对配置，保存上游域名快照；完整上游密钥和凭据标识不写入事件表。支持 Codex、Claude、Gemini、xAI 和 OpenAI 兼容的 API Key 配置；未配置路径、不可读取、凭据类型不支持或不能准确匹配时域名留空，Provider 保存事件原值。修改配置、价格或币种不会重写已有快照。

余额、备注均持久化保存；每笔 usage 的事件、累计费用和余额扣减在同一个事务中写入，写入失败会返回 `accepted: false`。余额页使用 `PATCH /key-balances` 的 `updates` 数组，只提交当前行修改过的字段。修改 `balance`、设置 `configured: false`（取消跟踪）或 `delete: true` 时需携带 GET 返回的 `balance_version`，放在 `expected_balance_version` 中；版本冲突返回 HTTP 409。备注 `note` 可独立修改，不会覆盖并发扣费。

插件资源页面为：

`/v0/resource/plugins/cpa-billing-management/billing`

模型费用资源页面为：

`/v0/resource/plugins/cpa-billing-management/pricing`

密钥余额资源页面为：

`/v0/resource/plugins/cpa-billing-management/wallet`

资源页面会复用 CLIProxyAPI 管理中心的浏览器登录状态：从同源 `localStorage` 的
`cli-proxy-auth` 读取管理密钥，并通过
`Authorization: Bearer <management-key>` 调用插件管理 API。管理中心勾选“记住密码”后，
重新打开资源页会自动恢复；未勾选时，资源页会在当前页面提示再次输入管理密码并验证，
临时密钥只保存在同源管理中心窗口的内存中，切换插件菜单无需重复登录，刷新或关闭管理中心后失效。
独立打开插件页面或管理中心与插件跨源时，临时登录仅对当前插件页面有效。
插件不将临时密钥写入浏览器存储，也不在配置或资源 HTML 中注入管理密钥。
