# CPA Billing Management

CLIProxyAPI 自定义插件：接收 usage 事件，优先使用上游金额，否则按模型价格估算费用，并通过管理页面展示账单汇总。

## 当前功能

- 监听 CLIProxyAPI `UsagePlugin` 事件；
- 上游回调明确包含费用字段时直接使用该金额；
- 上游未返回金额时，按输入、输出、缓存读取和缓存创建 token 以及模型价格规则估算费用；
- 支持 `provider/model`、模型名、alias 和 `*` 通配价格规则；
- 使用 SQLite 持久化账单状态，包含按模型汇总、按 API Key 汇总和最近事件；数据库文件为数据目录下的 `billing.db`，默认目录是操作系统用户配置目录下的 `cliproxyapi/cpa-billing-management`；
- 在 CLIProxyAPI 管理页增加“费用统计”菜单，展示总费用、按模型汇总、按脱敏 API Key 汇总，以及最近请求的总耗时和首 Token 耗时；最近事件支持分页和可选的 5/10/15 秒自动刷新；
- 在独立的“模型费用”页面编辑价格规则；未匹配价格的事件费用为 0，并标记为“未配置模型费用”。
- 模型费用页默认不添加价格规则；支持从 [LiteLLM 公共模型价格目录](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json) 同步当前已使用且可识别的模型，未识别的模型仍可手动配置。

当前 CLIProxyAPI 的 `UsagePlugin` ABI 主要提供 token、耗时等字段，通常不包含金额，因此大多数文本模型会走模型价格估算。价格单位是配置币种/每百万 token，估算结果请以供应商账单为准。

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
https://raw.githubusercontent.com/chyern/CPA-Plugins/main/registry.json
```

保存配置后进入“插件商店”，刷新插件源并安装 **CPA Billing Management**。
插件安装包来自此仓库的 GitHub Release，第三方源通过固定版本、平台和 SHA-256 声明直接安装，
因此不会消耗 GitHub Releases API 的匿名请求额度。发布新版本时，GitHub Actions 会在正式
`registry.json` 独立维护在 [`CPA-Plugins`](https://github.com/chyern/CPA-Plugins) 项目中；
本项目的 Release Workflow 只负责测试、构建和发布 GitHub Release 产物。

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
  CPA_PLUGIN_DIR=/absolute/path/to/.cli-proxy-api/plugins/darwin/arm64 \\
  CPA_PLUGIN_VERSION=0.1.5
```

`install-local` 会把原有同版本文件移到 `backups` 目录，然后复制当前工作区的 `bin/cpa-billing-management.dylib`。以后重新执行 `make install-local` 并重启 CLIProxyAPI 即可加载新构建；账单数据和插件配置不会被覆盖。

启动 CLIProxyAPI 后，在管理页进入“费用统计”查看账单，或进入“模型费用”维护价格。管理 API 路由为：

- `GET /v0/management/cpa-billing-management/summary`
- `GET /v0/management/cpa-billing-management/prices`
- `PUT /v0/management/cpa-billing-management/prices`
- `POST /v0/management/cpa-billing-management/prices/sync`
- `POST /v0/management/cpa-billing-management/reset`

插件资源页面为：

`/v0/resource/plugins/cpa-billing-management/billing`

模型费用资源页面为：

`/v0/resource/plugins/cpa-billing-management/pricing`

资源页面会复用 CLIProxyAPI 管理中心的浏览器登录状态：从同源 `localStorage` 的
`cli-proxy-auth` 读取管理密钥，兼容管理中心的 `enc::v1::` 混淆格式，并通过
`Authorization: Bearer <management-key>` 调用插件管理 API。管理中心勾选“记住密码”后，
重新打开资源页会自动恢复；管理密钥缺失或 API 返回 401 时，页面会跳转到
`/management.html#/login`。插件不再在配置或资源 HTML 中注入第二份管理密钥。
