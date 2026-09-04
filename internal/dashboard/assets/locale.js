(function () {
  const languageKey = 'cli-proxy-language';
  const translations = {
    en: {
      'CPA 费用统计': 'CPA Billing Statistics', 'CPA 模型费用': 'CPA Model Pricing', 'CPA 密钥余额': 'CPA Key Balances',
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
    },
    'zh-TW': {
      'CPA 费用统计': 'CPA 費用統計', 'CPA 模型费用': 'CPA 模型費用', 'CPA 密钥余额': 'CPA 金鑰餘額', '自动刷新': '自動重新整理', '不刷新': '不重新整理',
      '今天': '今天', '近 7 天': '近 7 天', '近 30 天': '近 30 天', '开始日期': '開始日期', '结束日期': '結束日期', '查询': '查詢',
      '总费用': '總費用', '请求数': '請求數', '总 token': '總 token', '失败请求': '失敗請求', ['按' + '模型' + '汇总']: '按模型彙總', '按 API Key 汇总': '按 API Key 彙總',
      ['最近' + '事件']: '最近' + '事件', '输入': '輸入', '缓存': '快取', '输出': '輸出', '费用': '費用', '时间': '時間', '状态': '狀態', '成功': '成功', '失败': '失敗',
      '未配置模型费用': '未設定模型費用', '上一页': '上一頁', '下一页': '下一頁', ['模型' + '价格' + '规则']: '模型價格規則', '新增规则': '新增規則', '保存模型费用': '儲存模型費用',
      '同步上游价格': '同步上游價格', '价格来源': '價格來源', '获取价格差异': '取得價格差異', '可选渠道': '可選來源', '添加': '新增', '保存': '儲存', '删除': '刪除', '操作': '操作',
      '备注': '備註', '当前余额': '目前餘額', '累计费用': '累計費用', '未设置': '未設定', '待保存': '待儲存', '已耗尽': '已耗盡', '正常': '正常', '输入完整 API Key': '輸入完整 API Key', '填写密钥用途': '填寫金鑰用途', '不跟踪': '不追蹤',
      '已更新': '已更新', '暂无 usage 事件': '暫無 usage 事件', '暂无 API Key 数据': '暫無 API Key 資料', ['暂无' + '最近' + '事件']: '暫無' + '最近' + '事件',
    },
    ru: {
      'CPA 费用统计': 'Статистика расходов CPA', 'CPA 模型费用': 'Цены моделей CPA', 'CPA 密钥余额': 'Баланс ключей CPA', '自动刷新': 'Автообновление', '不刷新': 'Выкл.',
      '今天': 'Сегодня', '近 7 天': 'Последние 7 дней', '近 30 天': 'Последние 30 дней', '开始日期': 'Дата начала', '结束日期': 'Дата окончания', '查询': 'Запросить',
      '总费用': 'Общие расходы', '请求数': 'Запросы', '总 token': 'Всего токенов', '失败请求': 'Ошибочные запросы', ['按' + '模型' + '汇总']: 'По моделям', '按 API Key 汇总': 'По API Key', ['最近' + '事件']: 'Последние события',
      '输入': 'Ввод', '缓存': 'Кэш', '输出': 'Вывод', '费用': 'Стоимость', '时间': 'Время', '状态': 'Статус', '成功': 'Успешно', '失败': 'Ошибка', '未配置模型费用': 'Цена модели не задана',
      '上一页': 'Назад', '下一页': 'Вперёд', ['模型' + '价格' + '规则']: 'Правила цен моделей', '新增规则': 'Добавить правило', '保存模型费用': 'Сохранить цены моделей', '同步上游价格': 'Синхронизировать цены', '价格来源': 'Источник цен', '获取价格差异': 'Получить различия', '可选渠道': 'Источник', '添加': 'Добавить', '保存': 'Сохранить', '删除': 'Удалить', '操作': 'Действия',
      '备注': 'Примечание', '当前余额': 'Текущий баланс', '累计费用': 'Общие расходы', '未设置': 'Не задано', '待保存': 'Ожидает сохранения', '已耗尽': 'Исчерпан', '正常': 'Норма', '输入完整 API Key': 'Введите полный API Key', '填写密钥用途': 'Назначение ключа', '不跟踪': 'Не отслеживать',
      '已更新': 'Обновлено', '暂无 usage 事件': 'Нет usage-событий', '暂无 API Key 数据': 'Нет данных API Key', ['暂无' + '最近' + '事件']: 'Нет последних событий',
    },
  };
  const getLanguage = () => {
    let value = '';
    try {
      // The management center keeps its language in an obfuscated storage
      // wrapper, but exposes the resolved locale on the parent HTML element.
      value = window.parent && window.parent.document.documentElement.lang || '';
      if (!value) value = window.parent && window.parent.document.documentElement.dataset.language || '';
      // Some management-center builds keep the html lang attribute at its
      // initial value. Detect the translated navigation labels as a fallback.
      if (!value || value === 'zh-CN') {
        const parentText = String(window.parent && window.parent.document.body && window.parent.document.body.innerText || '');
        if (/\bOPERATE\b|\bGATEWAY\b|\bPLUGINS\b/.test(parentText)) value = 'en';
        else if (/ОПЕРАЦИИ|ШЛЮЗ|ПЛАГИНЫ/.test(parentText)) value = 'ru';
        else if (/執行|閘道|外掛/.test(parentText)) value = 'zh-TW';
      }
    } catch (_) {}
    try { if (!value) value = window.parent && window.parent.localStorage.getItem(languageKey) || ''; } catch (_) {}
    if (!value) {
      try { value = localStorage.getItem(languageKey) || ''; } catch (_) {}
    }
    value = String(value || navigator.language || 'zh-CN');
    if (/^zh[-_]tw/i.test(value)) return 'zh-TW';
    if (/^en/i.test(value)) return 'en';
    if (/^ru/i.test(value)) return 'ru';
    return 'zh-CN';
  };
  const locale = getLanguage();
  const dictionary = translations[locale] || {};
  document.documentElement.lang = locale;
  window.cpaTranslate = value => dictionary[String(value)] || value;
  let translating = false;
  const translate = () => {
    if (translating || locale === 'zh-CN') return;
    translating = true;
    const replace = value => Object.keys(dictionary).sort((a, b) => b.length - a.length).reduce((text, key) => text.split(key).join(dictionary[key]), value);
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    const nodes = [];
    while (walker.nextNode()) nodes.push(walker.currentNode);
    nodes.forEach(node => { if (node.nodeValue.trim()) node.nodeValue = replace(node.nodeValue); });
    document.querySelectorAll('[placeholder],[title],[aria-label]').forEach(element => {
      ['placeholder', 'title', 'aria-label'].forEach(attribute => {
        if (element.hasAttribute(attribute)) element.setAttribute(attribute, replace(element.getAttribute(attribute)));
      });
    });
    translating = false;
  };
  const observer = new MutationObserver(translate);
  const start = () => { translate(); observer.observe(document.body, {childList: true, subtree: true, characterData: true}); };
  if (document.body) start(); else document.addEventListener('DOMContentLoaded', start, {once: true});
  setInterval(() => { if (getLanguage() !== locale) window.location.reload(); }, 1000);
})();
