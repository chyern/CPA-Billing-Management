(function () {
  const languageKey = 'cli-proxy-language';
  const translations = {
    en: {
      'cpa-billing-management': 'CPA Billing Management', '费用统计': 'Billing Statistics', '模型费用': 'Model Pricing', '密钥余额': 'Key Balances',
      'CPA 费用统计': 'CPA Billing Statistics', 'CPA 模型费用': 'CPA Model Pricing', 'CPA 密钥余额': 'CPA Key Balances',
      '模型': 'Model',
      '自动刷新': 'Auto refresh', '不刷新': 'Off', '今天': 'Today', '近 7 天': 'Last 7 days', '近 30 天': 'Last 30 days', '5 秒': '5 sec', '10 秒': '10 sec', '15 秒': '15 sec', '不足 1 ms': '<1 ms', '首字': 'TTFT',
      '开始日期': 'Start date', '结束日期': 'End date', '查询': 'Query', '总费用': 'Total cost', '请求数': 'Requests',
      '总 token': 'Total tokens', '失败请求': 'Failed requests', ['按' + '模型' + '汇总']: 'By model', '按 API Key 汇总': 'By API Key',
      ['最近' + '事件']: 'Recent events', 'Provider': 'Provider', 'Model': 'Model', '输入': 'Input', '缓存': 'Cache', '输出': 'Output',
      '费用': 'Cost', '时间': 'Time', '耗时/首字': 'Latency/TTFT', '输入/缓存': 'Input/Cache', '状态': 'Status',
      '成功': 'Success', '失败': 'Failed', '未配置模型费用': 'Model price not configured', '暂无 usage 事件': 'No usage events',
      '暂无 API Key 数据': 'No API Key data', ['暂无' + '最近' + '事件']: 'No recent events', '上一页': 'Previous', '下一页': 'Next',
      '第 ': 'Page ', ' / ': ' / ', ' 页 · 共 ': ' · ', ' 条': ' records', ['模型' + '价格' + '规则']: 'Model pricing rules',
      '新增规则': 'Add rule', '保存模型费用': 'Save model pricing', '同步上游价格': 'Sync upstream prices', '价格来源': 'Price source',
      '获取价格差异': 'Get price differences', '可选渠道': 'Optional source', 'LiteLLM 公共目录': 'LiteLLM catalog',
      'Models.dev 公共目录': 'Models.dev catalog', 'OpenRouter 模型 API': 'OpenRouter Models API', '例如：gpt-4o': 'e.g. gpt-4o',
      '与 CLIProxyAPI 首页“模型”卡片相同，列出当前代理端点暴露的模型；未配置价格的模型按 0 展示。': 'Shows the models exposed by the current CLIProxyAPI endpoint; models without a configured price are shown as 0.',
      '匹配优先级：模型名 → alias → *。价格单位为当前币种 / 1M token，费用在事件生成时计算，后续修改价格不会影响历史事件。': 'Match priority: model name → alias → *. Prices are per 1M tokens in the current currency; costs are fixed when events are created.',
      'API Key 余额': 'API Key balances', '备注': 'Note', '添加': 'Add', '保存': 'Save', '删除': 'Delete', '操作': 'Actions',
      '未设置': 'Not set', '待保存': 'Pending', '正常': '正常', '已耗尽': 'Exhausted', '当前余额': 'Current balance',
      '累计费用': 'Total cost', '密钥数量': 'Keys', '已设置余额': 'With balance', '当前余额合计': 'Total balance', '余额耗尽': 'Exhausted',
      '输入完整 API Key': 'Enter full API Key', '填写密钥用途': 'Describe this key', '不跟踪': 'Not tracked', '可用模型目录': 'Available models', '暂无可用模型': 'No models available', '未配置，按 0 计': 'Not configured; counted as 0', '按模型名称或通配快速过滤...': 'Filter by model name or wildcard…', '估算费用仅供参考': 'Estimated costs are for reference only', '模型费用为估算值，请以供应商账单为准': 'Model costs are estimates; refer to your provider bill',
      '仅显示 CLIProxyAPI 当前配置的 API Key，并保持配置顺序。留空表示不跟踪余额，完整密钥不会写入账单数据库。': 'Only API Keys configured in CLIProxyAPI are shown, in configuration order. Leave balance empty to disable tracking; full keys are never stored in the billing database.',
      '为客户端 API Key 设置当前余额；新 usage 事件产生费用时会自动扣减，余额耗尽后的请求会在访问上游前被拦截。': 'Set the current balance for client API Keys. New usage costs are deducted automatically; requests are blocked before reaching the upstream when a balance is exhausted.',
      '当前币种：': 'Currency: ', '未设置余额的密钥不拦截': 'Keys without a balance set are not blocked',
      '暂无 API Key。配置客户端密钥或产生 usage 事件后会显示在这里。': 'No API Keys. Configure a client key or generate a usage event to show it here.',
      '费用计算说明': 'Cost calculation details', '上游明确返回金额时优先使用，否则按“模型费用”中的每百万 token 价格估算': 'Use the upstream amount when provided; otherwise estimate using the per-million-token price in “Model pricing”.',
      '已更新': 'Updated', '正在从 ': 'Loading from ', ' 获取价格…': ' prices…', '保存失败：': 'Save failed: ', '删除失败：': 'Delete failed: ',
      '加载失败：': 'Load failed: ', '更新失败：': 'Update failed: ', '查询失败：结束日期不能早于开始日期': 'Query failed: end date cannot be before start date',
      '模型费用已保存，新请求将使用最新价格，历史费用保持不变': 'Model pricing saved. New requests use the latest prices; historical costs are unchanged.',
      '模型费用规则已删除': 'Model pricing rule deleted', '确定要删除“': 'Delete “', '”吗？删除会立即生效。': '”? This takes effect immediately.',
      '密钥余额已保存，余额耗尽后新请求将被拦截': 'Key balances saved. New requests are blocked when a balance is exhausted.',
      'API Key 已添加到 CLIProxyAPI 主配置': 'API Key added to CLIProxyAPI configuration', 'API Key 已从 CLIProxyAPI 主配置删除': 'API Key removed from CLIProxyAPI configuration',
      '请先保存当前待添加的 API Key': 'Save the pending API Key first', '已新增待保存的 API Key': 'A new API Key is ready to save', '余额必须是大于等于 0 的有效数字': 'Balance must be a valid number greater than or equal to 0', '请输入完整 API Key': 'Enter a complete API Key', '该 API Key 已存在': 'This API Key already exists',
      '昨天': 'Yesterday', '全部': 'All', '重置': 'Reset', '仅成功': 'Success only', '仅失败': 'Failed only',
      '复制 API Key': 'Copy API Key', '已复制': 'Copied', '搜索模型...': 'Search models...', '搜索 API Key...': 'Search API Keys...',
      '搜索 Key 或备注...': 'Search Key or note...', '默认兜底': 'Default fallback', '未找到匹配项': 'No matching items found', '失败率': 'Failure rate',
    },
    'zh-TW': {
      'cpa-billing-management': 'CPA 帳務管理', '费用统计': '費用統計', '模型费用': '模型費用', '密钥余额': '金鑰餘額',
      'CPA 费用统计': 'CPA 費用統計', 'CPA 模型费用': 'CPA 模型費用', 'CPA 密钥余额': 'CPA 金鑰餘額', '自动刷新': '自動重新整理', '不刷新': '不重新整理',
      '模型': '模型',
      '今天': '今天', '昨天': '昨天', '近 7 天': '近 7 天', '近 30 天': '近 30 天', '开始日期': '開始日期', '结束日期': '結束日期', '查询': '查詢', '重置': '重設',
      '总费用': '總費用', '请求数': '請求數', '总 token': '總 token', '失败请求': '失敗請求', ['按' + '模型' + '汇总']: '按模型彙總', '按 API Key 汇总': '按 API Key 彙總',
      ['最近' + '事件']: '最近' + '事件', '输入': '輸入', '缓存': '快取', '输出': '輸出', '费用': '費用', '时间': '時間', '状态': '狀態', '成功': '成功', '失败': '失敗',
      '全部': '全部', '仅成功': '僅成功', '仅失败': '僅失敗', '复制 API Key': '複製 API Key', '已复制': '已複製', '默认兜底': '預設兜底',
      '搜索模型...': '搜尋模型...', '搜索 API Key...': '搜尋 API Key...', '搜索 Key 或备注...': '搜尋 Key 或備註...', '未找到匹配项': '未找到相符項目', '失败率': '失敗率',
      '未配置模型费用': '未設定模型費用', '上一页': '上一頁', '下一页': '下一頁', ['模型' + '价格' + '规则']: '模型價格規則', '新增规则': '新增規則', '保存模型费用': '儲存模型費用',
      '同步上游价格': '同步上游價格', '价格来源': '價格來源', '获取价格差异': '取得價格差異', '可选渠道': '可選來源', '添加': '新增', '保存': '儲存', '删除': '刪除', '操作': '操作',
      '备注': '備註', '当前余额': '目前餘額', '累计费用': '累計費用', '未设置': '未設定', '待保存': '待儲存', '已耗尽': '已耗盡', '正常': '正常', '输入完整 API Key': '輸入完整 API Key', '填写密钥用途': '填寫金鑰用途', '不跟踪': '不追蹤',
      '已更新': '已更新', '暂无 usage 事件': '暫無 usage 事件', '暂无 API Key 数据': '暫無 API Key 資料', ['暂无' + '最近' + '事件']: '暫無' + '最近' + '事件',
    },
    ru: {
      'cpa-billing-management': 'Управление биллингом CPA', '费用统计': 'Статистика расходов', '模型费用': 'Цены моделей', '密钥余额': 'Баланс ключей',
      'CPA 费用统计': 'Статистика расходов CPA', 'CPA 模型费用': 'Цены моделей CPA', 'CPA 密钥余额': 'Баланс ключей CPA', '自动刷新': 'Автообновление', '不刷新': 'Выкл.',
      '模型': 'Модель',
      '今天': 'Сегодня', '昨天': 'Вчера', '近 7 天': 'Последние 7 дней', '近 30 天': 'Последние 30 дней', '开始日期': 'Дата начала', '结束日期': 'Дата окончания', '查询': 'Запросить', '重置': 'Сбросить',
      '总费用': 'Общие расходы', '请求数': 'Запросы', '总 token': 'Всего токенов', '失败请求': 'Ошибочные запросы', ['按' + '模型' + '汇总']: 'По моделям', '按 API Key 汇总': 'По API Key', ['最近' + '事件']: 'Последние события',
      '输入': 'Ввод', '缓存': 'Кэш', '输出': 'Вывод', '费用': 'Стоимость', '时间': 'Время', '状态': 'Статус', '成功': 'Успешно', '失败': 'Ошибка', '未配置模型费用': 'Цена модели не задана',
      '全部': 'Все', '仅成功': 'Только успешно', '仅失败': 'Только ошибки', '复制 API Key': 'Копировать API Key', '已复制': 'Скопировано', '默认兜底': 'По умолчанию',
      '搜索模型...': 'Поиск моделей...', '搜索 API Key...': 'Поиск API Key...', '搜索 Key 或备注...': 'Поиск по ключу...', '未找到匹配项': 'Совпадений не найдено', '失败率': 'Ошибки',
      '上一页': 'Назад', '下一页': 'Вперёд', ['模型' + '价格' + '规则']: 'Правила цен моделей', '新增规则': 'Добавить правило', '保存模型费用': 'Сохранить цены моделей', '同步上游价格': 'Синхронизировать цены', '价格来源': 'Источник цен', '获取价格差异': 'Получить различия', '可选渠道': 'Источник', '添加': 'Добавить', '保存': 'Сохранить', '删除': 'Удалить', '操作': 'Действия',
      '备注': 'Примечание', '当前余额': 'Текущий баланс', '累计费用': 'Общие расходы', '未设置': 'Не задано', '待保存': 'Ожидает сохранения', '已耗尽': 'Исчерпан', '正常': 'Норма', '输入完整 API Key': 'Введите полный API Key', '填写密钥用途': 'Назначение ключа', '不跟踪': 'Не отслеживать',
      '已更新': 'Обновлено', '暂无 usage 事件': 'Нет usage-событий', '暂无 API Key 数据': 'Нет данных API Key', ['暂无' + '最近' + '事件']: 'Нет последних событий',
    },
  };

  // The dashboard pages also render status messages and table empty states
  // dynamically. Keep these strings in the same dictionary so newly rendered
  // content follows the selected language as well.
  const extraTranslations = {
    en: {
      '产生模型调用事件后将自动在此汇总': 'Model usage events will be summarized here automatically',
      '仅显示 CLIProxyAPI 当前配置的 API Key。配置客户端密钥或产生 usage 事件后会显示在这里。': 'Only API Keys configured in CLIProxyAPI are shown. Configure a client key or generate a usage event to show it here.',
      '按模型名称或通配快速过滤...': 'Filter by model name or wildcard...', '复制完整 API Key': 'Copy full API Key', '未命名密钥': 'Unnamed key', '未提供': 'Not provided',
      'LiteLLM 公共目录': 'LiteLLM catalog', 'Models.dev 公共目录': 'Models.dev catalog', 'OpenRouter 模型 API': 'OpenRouter Models API',
      '与 CLIProxyAPI 首页“模型”卡片相同，列出当前代理端点暴露的模型；未配置价格的模型按 0 展示。': 'Shows the models exposed by the current CLIProxyAPI endpoint; models without a configured price are shown as 0.',
      '匹配优先级：模型名 → alias → *。价格单位为当前币种 / 1M token，费用在事件生成时计算，后续修改价格不会影响历史事件。': 'Match priority: model name → alias → *. Prices are per 1M tokens in the current currency; costs are fixed when events are created.',
      '费用计算说明': 'Cost calculation details', '上游明确返回金额时优先使用，否则按“模型费用”中的每百万 token 价格估算': 'Use the upstream amount when provided; otherwise estimate using the per-million-token price in “Model pricing”.',
      '当前币种：': 'Currency: ', '估算费用仅供参考': 'Estimated costs are for reference only', '未设置余额的密钥不拦截': 'Keys without a balance set are not blocked',
      '当前余额合计': 'Total balance', '余额必须是大于等于 0 的有效数字': 'Balance must be a valid number greater than or equal to 0', '管理中心登录已失效': 'Management session expired',
      '无法读取完整 API Key，未执行删除': 'Unable to read the full API Key; deletion was not performed', '请输入完整 API Key': 'Enter a complete API Key', '该 API Key 已存在': 'This API Key already exists',
      '同步失败：': 'Sync failed: ', '请求失败': 'Request failed', '请求 ': 'Requests ', '失败 ': 'Failed ', '输入 ': 'Input ', '缓存 ': 'Cache ', '输出 ': 'Output ', '费用 ': 'Cost ',
      '缓存读取 / 1M': 'Cache read / 1M', '缓存创建 / 1M': 'Cache creation / 1M', '匹配': 'Match', ['暂无' + '价格' + '规则']: 'No pricing rules', '待新增': 'Pending add', '待更新': 'Pending update',
      '模型名称来自 CLIProxyAPI 模型列表': 'Model name comes from the CLIProxyAPI model list', 'CLIProxyAPI 内置模型不能删除': 'Built-in CLIProxyAPI models cannot be deleted', '暂无可用模型': 'No models available', '未配置，按 0 计': 'Not configured; counted as 0',
      '第 ': 'Page ', ' 条规则的匹配不能为空': ' match cannot be empty', ' 条规则与其他规则重复：': ' duplicates another rule: ', ' 条规则的价格必须是大于等于 0 的有效数字': ' price must be a valid number greater than or equal to 0',
      ' 条模型费用已保存': ' model pricing saved', '这条模型费用规则': 'this model pricing rule', '正在从 ': 'Loading prices from ', '获取价格…': 'Get prices…',
      '最近处理的 API 请求事件会实时出现在这里': 'Recently processed API request events appear here in real time', '确定要删除“': 'Delete “', '”吗？删除会立即生效。': '”? This takes effect immediately.',
      '确定要删除 ': 'Delete ', ' 吗？\n\n这会从 CLIProxyAPI 主配置中永久移除该 API Key，同时清除插件中的余额和备注。': ' ?\n\nThis permanently removes the API Key from the CLIProxyAPI configuration and clears its plugin balance and note.',
      ['按' + '模型' + '汇总' + ' ']: 'By model ', '按 API Key 汇总 ': 'By API Key ', ['最近' + '事件' + ' ']: 'Recent events ', '费用统计 ': 'Billing statistics ', '密钥余额 ': 'Key balances ', '模型费用 ': 'Model pricing ', '正常': 'Normal',
    },
    'zh-TW': {
      '产生模型调用事件后将自动在此汇总': '模型呼叫事件會自動在此彙總',
      '仅显示 CLIProxyAPI 当前配置的 API Key。配置客户端密钥或产生 usage 事件后会显示在这里。': '僅顯示 CLIProxyAPI 目前設定的 API Key。設定用戶端金鑰或產生 usage 事件後會顯示在這裡。',
      '按模型名称或通配快速过滤...': '按模型名稱或萬用字元快速篩選...', '复制完整 API Key': '複製完整 API Key', '未命名密钥': '未命名金鑰', '未提供': '未提供',
      'LiteLLM 公共目录': 'LiteLLM 公共目錄', 'Models.dev 公共目录': 'Models.dev 公共目錄', 'OpenRouter 模型 API': 'OpenRouter 模型 API',
      '与 CLIProxyAPI 首页“模型”卡片相同，列出当前代理端点暴露的模型；未配置价格的模型按 0 展示。': '與 CLIProxyAPI 首頁「模型」卡片相同，列出目前代理端點公開的模型；未設定價格的模型按 0 顯示。',
      '匹配优先级：模型名 → alias → *。价格单位为当前币种 / 1M token，费用在事件生成时计算，后续修改价格不会影响历史事件。': '比對優先順序：模型名稱 → alias → *。價格單位為目前幣別 / 1M token，費用在事件產生時計算，後續修改價格不會影響歷史事件。',
      '费用计算说明': '費用計算說明', '上游明确返回金额时优先使用，否则按“模型费用”中的每百万 token 价格估算': '上游明確回傳金額時優先使用，否則按「模型費用」中的每百萬 token 價格估算。',
      '当前币种：': '目前幣別：', '估算费用仅供参考': '估算費用僅供參考', '未设置余额的密钥不拦截': '未設定餘額的金鑰不攔截',
      '当前余额合计': '目前餘額合計', '余额必须是大于等于 0 的有效数字': '餘額必須是大於等於 0 的有效數字', '管理中心登录已失效': '管理中心登入已失效',
      '无法读取完整 API Key，未执行删除': '無法讀取完整 API Key，未執行刪除', '请输入完整 API Key': '請輸入完整 API Key', '该 API Key 已存在': '此 API Key 已存在',
      '同步失败：': '同步失敗：', '请求失败': '請求失敗', '请求 ': '請求 ', '失败 ': '失敗 ', '输入 ': '輸入 ', '缓存 ': '快取 ', '输出 ': '輸出 ', '费用 ': '費用 ',
      '缓存读取 / 1M': '快取讀取 / 1M', '缓存创建 / 1M': '快取建立 / 1M', '匹配': '比對', ['暂无' + '价格' + '规则']: '暫無價格規則', '待新增': '待新增', '待更新': '待更新',
      '模型名称来自 CLIProxyAPI 模型列表': '模型名稱來自 CLIProxyAPI 模型清單', 'CLIProxyAPI 内置模型不能删除': '無法刪除 CLIProxyAPI 內建模型', '暂无可用模型': '暫無可用模型', '未配置，按 0 计': '未設定，按 0 計',
      '第 ': '第 ', ' 条规则的匹配不能为空': ' 條規則的比對不可為空', ' 条规则与其他规则重复：': ' 條規則與其他規則重複：', ' 条规则的价格必须是大于等于 0 的有效数字': ' 條規則的價格必須是大於等於 0 的有效數字',
      ' 条模型费用已保存': ' 條模型費用已儲存', '这条模型费用规则': '這條模型費用規則', '正在从 ': '正在從 ', '获取价格…': '取得價格…',
      '最近处理的 API 请求事件会实时出现在这里': '最近處理的 API 請求事件會即時出現在這裡', '确定要删除“': '確定要刪除「', '”吗？删除会立即生效。': '」嗎？刪除會立即生效。',
      '确定要删除 ': '確定要刪除 ', ' 吗？\n\n这会从 CLIProxyAPI 主配置中永久移除该 API Key，同时清除插件中的余额和备注。': ' 嗎？\n\n這會從 CLIProxyAPI 主設定中永久移除該 API Key，同時清除外掛中的餘額和備註。',
      ['按' + '模型' + '汇总' + ' ']: '按模型彙總 ', '按 API Key 汇总 ': '按 API Key 彙總 ', ['最近' + '事件' + ' ']: ['最近' + '事件' + ' '], '费用统计 ': '費用統計 ', '密钥余额 ': '金鑰餘額 ', '模型费用 ': '模型費用 ', '正常': '正常',
    },
    ru: {
      '产生模型调用事件后将自动在此汇总': 'События использования моделей будут собираться здесь автоматически',
      '仅显示 CLIProxyAPI 当前配置的 API Key。配置客户端密钥或产生 usage 事件后会显示在这里。': 'Здесь отображаются только ключи API, настроенные в CLIProxyAPI. Настройте ключ или создайте событие использования.',
      '按模型名称或通配快速过滤...': 'Фильтр по имени модели или шаблону...', '复制完整 API Key': 'Копировать полный API Key', '未命名密钥': 'Ключ без имени', '未提供': 'Не указан',
      'LiteLLM 公共目录': 'Каталог LiteLLM', 'Models.dev 公共目录': 'Каталог Models.dev', 'OpenRouter 模型 API': 'API моделей OpenRouter',
      '与 CLIProxyAPI 首页“模型”卡片相同，列出当前代理端点暴露的模型；未配置价格的模型按 0 展示。': 'Показывает модели, доступные через текущую конечную точку CLIProxyAPI; модели без заданной цены отображаются как 0.',
      '匹配优先级：模型名 → alias → *。价格单位为当前币种 / 1M token，费用在事件生成时计算，后续修改价格不会影响历史事件。': 'Приоритет сопоставления: имя модели → alias → *. Цены указаны за 1M токенов в текущей валюте; стоимость фиксируется при создании события.',
      '费用计算说明': 'Расчёт стоимости', '上游明确返回金额时优先使用，否则按“模型费用”中的每百万 token 价格估算': 'Используется сумма от провайдера; иначе стоимость оценивается по цене за миллион токенов из «Цен моделей».',
      '当前币种：': 'Валюта: ', '估算费用仅供参考': 'Оценка стоимости приведена только для справки', '未设置余额的密钥不拦截': 'Ключи без заданного баланса не блокируются',
      '当前余额合计': 'Общий текущий баланс', '余额必须是大于等于 0 的有效数字': 'Баланс должен быть допустимым числом не меньше 0', '管理中心登录已失效': 'Сеанс управления истёк',
      '无法读取完整 API Key，未执行删除': 'Не удалось прочитать полный API Key; удаление не выполнено', '请输入完整 API Key': 'Введите полный API Key', '该 API Key 已存在': 'Этот API Key уже существует',
      '同步失败：': 'Ошибка синхронизации: ', '请求失败': 'Ошибка запроса', '请求 ': 'Запросы ', '失败 ': 'Ошибки ', '输入 ': 'Ввод ', '缓存 ': 'Кэш ', '输出 ': 'Вывод ', '费用 ': 'Стоимость ',
      '缓存读取 / 1M': 'Чтение кэша / 1M', '缓存创建 / 1M': 'Создание кэша / 1M', '匹配': 'Сопоставление', ['暂无' + '价格' + '规则']: 'Нет правил цен', '待新增': 'Добавление', '待更新': 'Обновление',
      '模型名称来自 CLIProxyAPI 模型列表': 'Имя модели взято из списка моделей CLIProxyAPI', 'CLIProxyAPI 内置模型不能删除': 'Встроенные модели CLIProxyAPI нельзя удалить', '暂无可用模型': 'Нет доступных моделей', '未配置，按 0 计': 'Не задано; считается как 0',
      '第 ': 'Страница ', ' 条规则的匹配不能为空': ': поле сопоставления не может быть пустым', ' 条规则与其他规则重复：': ' дублирует другое правило: ', ' 条规则的价格必须是大于等于 0 的有效数字': ': цена должна быть допустимым числом не меньше 0',
      ' 条模型费用已保存': ': цены модели сохранены', '这条模型费用规则': 'это правило цены модели', '正在从 ': 'Загрузка цен из ', '获取价格…': 'Получить цены…',
      '最近处理的 API 请求事件会实时出现在这里': 'Недавние события API-запросов появляются здесь в реальном времени', '确定要删除“': 'Удалить «', '”吗？删除会立即生效。': '»? Изменение вступит в силу немедленно.',
      '确定要删除 ': 'Удалить ', ' 吗？\n\n这会从 CLIProxyAPI 主配置中永久移除该 API Key，同时清除插件中的余额和备注。': '?\n\nКлюч будет навсегда удалён из конфигурации CLIProxyAPI, а баланс и примечание плагина очищены.',
      ['按' + '模型' + '汇总' + ' ']: 'По моделям ', '按 API Key 汇总 ': 'По API Key ', ['最近' + '事件' + ' ']: 'Последние события ', '费用统计 ': 'Статистика расходов ', '密钥余额 ': 'Баланс ключей ', '模型费用 ': 'Цены моделей ', '正常': 'Норма',
    },
  };
  Object.keys(extraTranslations).forEach((language) => Object.assign(translations[language], extraTranslations[language]));

  const normalizeLanguage = value => {
    if (value === null || value === undefined) return '';
    let candidate = String(value).trim();
    if (!candidate) return '';
    // Zustand persist stores { state: { language: 'en' } } in localStorage.
    try {
      const parsed = JSON.parse(candidate);
      candidate = parsed && (parsed.state && parsed.state.language || parsed.language || parsed.locale) || candidate;
    } catch (_) {}
    candidate = String(candidate || '').replace('_', '-').toLowerCase();
    if (candidate === 'zh-tw' || candidate === 'zh-hk' || candidate === 'zh-mo' || candidate === 'zh-hant') return 'zh-TW';
    if (candidate === 'zh' || candidate.startsWith('zh-cn') || candidate.startsWith('zh-hans')) return 'zh-CN';
    if (candidate === 'en' || candidate.startsWith('en-')) return 'en';
    if (candidate === 'ru' || candidate.startsWith('ru-')) return 'ru';
    return '';
  };
  const readStorageLanguage = storage => {
    try {
      return normalizeLanguage(storage && storage.getItem(languageKey));
    } catch (_) {
      return '';
    }
  };
  const getLanguage = () => {
    const embedded = window.parent && window.parent !== window;
    let value = '';
    // Storage is the source of truth. The parent html lang can remain at its
    // initial zh-CN value for a short time while the React shell hydrates.
    if (embedded) {
      try { value = readStorageLanguage(window.parent.localStorage); } catch (_) {}
    }
    if (!value) {
      try { value = readStorageLanguage(window.localStorage); } catch (_) {}
    }
    if (!value && embedded) {
      try { value = normalizeLanguage(window.parent.document.documentElement.dataset.language); } catch (_) {}
      try { if (!value) value = normalizeLanguage(window.parent.document.documentElement.lang); } catch (_) {}
    }
    if (!value) value = normalizeLanguage(new URLSearchParams(window.location.search).get('lang'));
    if (!value) value = normalizeLanguage(navigator.languages && navigator.languages[0] || navigator.language);
    return value || 'zh-CN';
  };
  const locale = getLanguage();
  const dictionary = translations[locale] || {};
  document.documentElement.lang = locale;
  window.cpaTranslate = value => dictionary[String(value)] || value;
  const replace = value => Object.keys(dictionary).sort((a, b) => b.length - a.length).reduce((text, key) => text.split(key).join(dictionary[key]), value);
  let translating = false;
  const translate = () => {
    if (translating || locale === 'zh-CN') return;
    translating = true;
    const title = document.querySelector('title');
    if (title) {
      const nextTitle = replace(title.textContent || '');
      if (nextTitle !== title.textContent) title.textContent = nextTitle;
    }
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    const nodes = [];
    while (walker.nextNode()) nodes.push(walker.currentNode);
    nodes.forEach(node => {
      if (!node.nodeValue.trim()) return;
      const next = replace(node.nodeValue);
      // Avoid writing an unchanged value: characterData observers fire even
      // when nodeValue is assigned the same string, which otherwise creates a
      // tight MutationObserver loop on translated pages.
      if (next !== node.nodeValue) node.nodeValue = next;
    });
    document.querySelectorAll('[placeholder],[title],[aria-label]').forEach(element => {
      ['placeholder', 'title', 'aria-label'].forEach(attribute => {
        if (element.hasAttribute(attribute)) {
          const current = element.getAttribute(attribute);
          const next = replace(current);
          if (next !== current) element.setAttribute(attribute, next);
        }
      });
    });
    translating = false;
  };
  const observer = new MutationObserver(translate);
  const start = () => { translate(); observer.observe(document.body, {childList: true, subtree: true, characterData: true}); };
  if (document.body) start(); else document.addEventListener('DOMContentLoaded', start, {once: true});

  // Resource routes are rendered by the host management shell, outside this
  // iframe. Translate only the plugin's own sidebar labels so the shell's
  // other navigation remains under its own localization control.
  const translateParentNavigation = () => {
    if (locale === 'zh-CN' || !window.parent || window.parent === window) return;
    try {
      const parentDocument = window.parent.document;
      const sidebar = parentDocument.querySelector('aside.sidebar');
      if (!sidebar) return;
      const labels = new Set(['cpa-billing-management', '费用统计', '模型费用', '密钥余额']);
      const walker = parentDocument.createTreeWalker(sidebar, NodeFilter.SHOW_TEXT);
      const nodes = [];
      while (walker.nextNode()) nodes.push(walker.currentNode);
      nodes.forEach(node => {
        const raw = node.nodeValue || '';
        const label = raw.trim();
        if (!labels.has(label)) return;
        const translated = dictionary[label];
        if (!translated || translated === label) return;
        node.nodeValue = raw.replace(label, translated);
      });
    } catch (_) {}
  };
  let parentNavigationObserver;
  const startParentNavigation = () => {
    if (locale === 'zh-CN' || !window.parent || window.parent === window) return;
    try {
      const parentDocument = window.parent.document;
      translateParentNavigation();
      parentNavigationObserver = new MutationObserver(translateParentNavigation);
      parentNavigationObserver.observe(parentDocument.body, {childList: true, subtree: true, characterData: true});
    } catch (_) {}
  };
  startParentNavigation();
  const reloadIfLanguageChanged = () => {
    if (getLanguage() !== locale) window.location.reload();
  };
  // React updates the parent html lang after changing language. Reacting to
  // that mutation (and to storage events) removes the old one-second window
  // where the embedded page could display the previous locale.
  window.addEventListener('storage', reloadIfLanguageChanged);
  try {
    if (window.parent && window.parent !== window) {
      const parentRoot = window.parent.document.documentElement;
      new MutationObserver(reloadIfLanguageChanged).observe(parentRoot, {attributes: true, attributeFilter: ['lang', 'data-language']});
    }
  } catch (_) {}
  setInterval(reloadIfLanguageChanged, 1500);
})();
