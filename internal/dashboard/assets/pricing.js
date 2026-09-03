const PRICING_API = '/v0/management/cpa-billing-management/prices';
const SYNC_API = PRICING_API + '/sync';
const SYNC_SOURCE_STORAGE_KEY = 'cpa-billing-pricing-source';
// CLIProxyAPI 的模型注册表由 router-for-me/models 维护。认证文件为空时，
// /v0/management/auth-files 无法提供模型，因此这里同时读取该完整目录。
const CLIPROXY_MODELS_URL = 'https://raw.githubusercontent.com/router-for-me/models/main/models.json';
const priceFields = [
  'input_per_million',
  'output_per_million',
  'cache_read_per_million',
  'cache_creation_per_million',
];

const syncSource = document.getElementById('syncSource');
const syncButton = document.getElementById('sync');
const initial = JSON.parse(document.getElementById('initial').textContent);
let rules = initial.rules || [];
let catalogModels = initial.models || [];
let syncChanges = {};

document.getElementById('currency').textContent = initial.currency || 'USD';

try {
  const remembered = localStorage.getItem(SYNC_SOURCE_STORAGE_KEY);
  if ([...syncSource.options].some(option => option.value === remembered)) {
    syncSource.value = remembered;
  }
} catch (_) {}

const escapeHTML = value => String(value ?? '').replace(
  /[&<>"']/g,
  character => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[character]),
);

function valueFor(rule, key) {
  return rule._draft || rule[key] == null || !Number.isFinite(Number(rule[key]))
    ? ''
    : Number(rule[key]);
}

function renderRules() {
  const rows = rules.map((rule, index) =>
    (() => {
      const change = syncChanges[String(rule.match || '').toLowerCase()];
      const badge = change ? '<span class="pill sync-pill">' + (change.action === 'add' ? '待新增' : '待更新') + '</span>' : '';
      const match = String(rule.match || '').trim().toLowerCase();
      const catalogRule = catalogModels.some(model => {
        const providerModel = String(model.provider || '').trim().toLowerCase() + '/' + String(model.model || '').trim().toLowerCase();
        return match === providerModel || match === String(model.model || '').trim().toLowerCase();
      });
      const readonly = catalogRule ? ' readonly title="模型名称来自 CLIProxyAPI 模型列表"' : '';
      return '<tr data-i="' + index + '">' + '<td><div class="rule-match"><input class="match" data-k="match" placeholder="例如：openai/gpt-4o" value="' + (rule._draft ? '' : escapeHTML(rule.match)) + '"' + readonly + '>' + badge + '</div></td>'
      + '<td><input data-k="input_per_million" type="number" min="0" step="0.000001" placeholder="例如：2.5" value="' + valueFor(rule, 'input_per_million') + '"></td>'
      + '<td><input data-k="output_per_million" type="number" min="0" step="0.000001" placeholder="例如：10" value="' + valueFor(rule, 'output_per_million') + '"></td>'
      + '<td><input data-k="cache_read_per_million" type="number" min="0" step="0.000001" placeholder="例如：0.25" value="' + valueFor(rule, 'cache_read_per_million') + '"></td>'
      + '<td><input data-k="cache_creation_per_million" type="number" min="0" step="0.000001" placeholder="例如：0.25" value="' + valueFor(rule, 'cache_creation_per_million') + '"></td>'
      + '<td><button class="btn danger" data-action="remove" data-index="' + index + '">删除</button></td>'
    + '</tr>';
    })(),
  ).join('');

  document.getElementById('rules').innerHTML = '<table><thead><tr><th>匹配</th><th class="num">输入 / 1M</th><th class="num">输出 / 1M</th><th class="num">缓存读取 / 1M</th><th class="num">缓存创建 / 1M</th><th></th></tr></thead><tbody>' + rows + '</tbody></table>';
}

function catalogPriceValue(model, key) {
  const value = model.price && model.price[key];
  return Number.isFinite(Number(value)) ? Number(value) : 0;
}

function effectiveCatalogRule(provider, model) {
  const providerModel = String(provider || '').trim().toLowerCase() + '/' + String(model || '').trim().toLowerCase();
  const modelKey = String(model || '').trim().toLowerCase();
  const rule = rules.find(item => String(item.match || '').trim().toLowerCase() === providerModel)
    || rules.find(item => String(item.match || '').trim().toLowerCase() === modelKey)
    || rules.find(item => String(item.match || '').trim() === '*');
  if (!rule) return {configured: false, price: {}};
  const configured = String(rule.match || '').trim() !== '*'
    || priceFields.some(field => Number(rule[field] || 0) > 0);
  return {configured, price: rule};
}

function refreshCatalogPrices() {
  catalogModels = catalogModels.map(model => {
    const effective = effectiveCatalogRule(model.provider, model.model);
    return Object.assign({}, model, effective);
  });
}

function renderCatalog() {
  refreshCatalogPrices();
  const rows = catalogModels
    .slice()
    .sort((a, b) => String(a.model || '').localeCompare(String(b.model || '')))
    .map(model => '<tr><td>' + escapeHTML(model.provider || '-') + '</td><td>' + escapeHTML(model.model || '-') + (model.configured ? '' : ' <span class="pill">未配置，按 0 计</span>') + '</td><td class="num">' + catalogPriceValue(model, 'input_per_million') + '</td><td class="num">' + catalogPriceValue(model, 'output_per_million') + '</td><td class="num">' + catalogPriceValue(model, 'cache_read_per_million') + '</td><td class="num">' + catalogPriceValue(model, 'cache_creation_per_million') + '</td></tr>')
    .join('');
  document.getElementById('catalog').innerHTML = rows
    ? '<table><thead><tr><th>Provider</th><th>Model</th><th class="num">输入 / 1M</th><th class="num">输出 / 1M</th><th class="num">缓存读取 / 1M</th><th class="num">缓存创建 / 1M</th></tr></thead><tbody>' + rows + '</tbody></table>'
    : '<div class="empty">暂无可用模型</div>';
}

function mergeCatalogIntoRules() {
  const existing = new Set(rules.map(rule => String(rule.match || '').trim().toLowerCase()));
  catalogModels.forEach(model => {
    const provider = String(model.provider || '').trim();
    const name = String(model.model || '').trim();
    if (!name) return;
    const match = (provider ? provider + '/' : '') + name;
    if (existing.has(match.toLowerCase()) || existing.has(name.toLowerCase())) return;
    const effective = effectiveCatalogRule(provider, name);
    const price = effective.price || {};
    rules.push({
      match,
      input_per_million: Number.isFinite(Number(price.input_per_million)) ? Number(price.input_per_million) : 0,
      output_per_million: Number.isFinite(Number(price.output_per_million)) ? Number(price.output_per_million) : 0,
      cache_read_per_million: Number.isFinite(Number(price.cache_read_per_million)) ? Number(price.cache_read_per_million) : 0,
      cache_creation_per_million: Number.isFinite(Number(price.cache_creation_per_million)) ? Number(price.cache_creation_per_million) : 0,
    });
    existing.add(match.toLowerCase());
  });
}

function readRules() {
  document.querySelectorAll('#rules tbody tr').forEach(row => {
    const index = Number(row.dataset.i);
    row.querySelectorAll('input').forEach(input => {
      const key = input.dataset.k;
      const value = input.value.trim();
      rules[index][key] = key === 'match' ? value : (value === '' ? NaN : Number(value));
    });
    delete rules[index]._draft;
  });
}

function validateRules() {
  const seen = new Set();
  for (let index = 0; index < rules.length; index++) {
    const rule = rules[index];
    const match = String(rule.match || '').trim();
    const key = match.toLowerCase();
    if (!match) return '第 ' + (index + 1) + ' 条规则的匹配不能为空';
    if (seen.has(key)) return '第 ' + (index + 1) + ' 条规则与其他规则重复：' + match;
    seen.add(key);
    for (const field of priceFields) {
      if (!Number.isFinite(rule[field]) || rule[field] < 0) {
        return '第 ' + (index + 1) + ' 条规则的价格必须是大于等于 0 的有效数字';
      }
    }
  }
  return '';
}

function showStatus(message, error = false) {
  const element = document.getElementById('status');
  element.textContent = message;
  element.className = 'muted status' + (error ? ' error' : '');
}

async function requestPricing(url = PRICING_API, options = {}) {
  if (!requireManagementKey()) throw new Error('管理中心登录已失效');
  const request = Object.assign({
    credentials: 'same-origin',
    headers: {'Content-Type': 'application/json', ...authHeaders()},
  }, options);
  const response = await fetch(url, request);
  if (response.status === 401) {
    redirectToManagementLogin();
    throw new Error('管理中心登录已失效');
  }
  if (!response.ok) throw new Error(await response.text() || response.statusText);
  return response.json();
}

async function requestHostManagement(url) {
  if (!requireManagementKey()) throw new Error('管理中心登录已失效');
  const response = await fetch(url, {credentials: 'same-origin', headers: authHeaders()});
  if (response.status === 401) {
    redirectToManagementLogin();
    throw new Error('管理中心登录已失效');
  }
  if (!response.ok) throw new Error(await response.text() || response.statusText);
  return response.json();
}

async function loadHostModels() {
  try {
    const filesPayload = await requestHostManagement('/v0/management/auth-files');
    const files = filesPayload.files || [];
    const modelsByKey = new Map(catalogModels.map(model => [String(model.provider || '') + '/' + String(model.model || ''), model]));
    const payloads = await Promise.all(files.map(file => {
      const name = file.name || file.id;
      return name ? requestHostManagement('/v0/management/auth-files/models?name=' + encodeURIComponent(name)).catch(() => null) : null;
    }));
    payloads.forEach((payload, index) => {
      if (!payload) return;
      const file = files[index] || {};
      (payload.models || []).forEach(model => {
        const id = String(model.id || '').trim();
        if (!id) return;
        const provider = String(file.provider || file.type || model.type || model.owned_by || 'unknown').trim();
        const key = provider + '/' + id;
        if (!modelsByKey.has(key)) modelsByKey.set(key, {provider, model: id, configured: false, price: {}});
      });
    });
    catalogModels = [...modelsByKey.values()];
    mergeCatalogIntoRules();
    renderRules();
    renderCatalog();
  } catch (_) {
    // The plugin remains usable when the host model-management endpoint is unavailable.
  }
}

async function loadCliProxyCatalog() {
  const controller = typeof AbortController === 'function' ? new AbortController() : null;
  const timeout = setTimeout(() => controller && controller.abort(), 8000);
  try {
    const response = await fetch(CLIPROXY_MODELS_URL, {
      credentials: 'omit',
      mode: 'cors',
      signal: controller ? controller.signal : undefined,
    });
    if (!response.ok) throw new Error('model catalog returned ' + response.status);
    const payload = await response.json();
    const modelsByKey = new Map(catalogModels.map(model => [String(model.provider || '').toLowerCase() + '/' + String(model.model || '').toLowerCase(), model]));
    Object.entries(payload || {}).forEach(([sourceProvider, models]) => {
      const provider = sourceProvider.startsWith('codex-') ? 'codex' : sourceProvider;
      (Array.isArray(models) ? models : []).forEach(model => {
        const id = String(model && (model.id || model.name) || '').trim();
        if (!id) return;
        const key = provider.toLowerCase() + '/' + id.toLowerCase();
        if (!modelsByKey.has(key)) modelsByKey.set(key, {provider, model: id, configured: false, price: {}});
      });
    });
    catalogModels = [...modelsByKey.values()];
    renderCatalog();
    const count = Object.values(payload || {}).reduce((total, models) => total + (Array.isArray(models) ? models.length : 0), 0);
    showStatus('已加载 CLIProxyAPI 模型目录（' + count + ' 个）');
  } catch (_) {
    // 远程目录不可用时保留插件本地的 observed/auth-files 模型集合。
  } finally {
    clearTimeout(timeout);
  }
}

async function loadServedModels() {
  try {
    const config = await requestHostManagement('/v0/management/config');
    const configured = Array.isArray(config && (config.apiKeys || config['api-keys'])) ? (config.apiKeys || config['api-keys']) : [];
    const keys = configured.map(item => {
      if (typeof item === 'string') return item.trim();
      if (item && typeof item === 'object') return String(item.key || item.api_key || item.value || '').trim();
      return '';
    }).filter(Boolean);
    const headersList = keys.length ? keys.map(key => ({Authorization: 'Bearer ' + key})) : [{}];
    let payload = null;
    for (const headers of headersList) {
      try {
        const response = await fetch('/v1/models', {credentials: 'same-origin', headers});
        if (response.ok) { payload = await response.json(); break; }
      } catch (_) {}
    }
    const models = payload && (payload.data || payload.models);
    if (!Array.isArray(models)) return false;
    const served = [];
    models.forEach(model => {
      const id = String(model && model.id || '').trim();
      if (!id) return;
      // /v1/models 的 owned_by=openai 对应 CLIProxyAPI 的 Codex 聚合入口。
      const owner = String(model.provider || model.owned_by || 'openai').trim().toLowerCase();
      const provider = owner === 'openai' ? 'codex' : owner;
      served.push({provider, model: id, configured: false, price: {}});
    });
    if (served.length === 0) return false;
    catalogModels = served;
    mergeCatalogIntoRules();
    renderRules();
    renderCatalog();
    showStatus('已加载 CLIProxyAPI 当前暴露模型（' + served.length + ' 个）');
    return true;
  } catch (_) {
    // /v1/models 需要可用的 API Key；不可用时继续使用静态目录。
    return false;
  }
}

document.getElementById('rules').addEventListener('click', event => {
  const button = event.target.closest('[data-action="remove"]');
  if (!button) return;
  readRules();
  rules.splice(Number(button.dataset.index), 1);
  renderRules();
});

syncSource.onchange = () => {
  try { localStorage.setItem(SYNC_SOURCE_STORAGE_KEY, syncSource.value); } catch (_) {}
};

syncButton.onclick = async () => {
  try {
    readRules();
    const validationError = validateRules();
    if (validationError) {
      showStatus('同步失败：' + validationError, true);
      return;
    }
  } catch (error) {
    showStatus('同步失败：' + error.message, true);
    return;
  }
  const source = syncSource.value;
  syncButton.disabled = true;
  syncSource.disabled = true;
  showStatus('正在从 ' + syncSource.options[syncSource.selectedIndex].text + ' 获取价格…');
  try {
    const data = await requestPricing(SYNC_API + '?source=' + encodeURIComponent(source) + '&preview=1', {method: 'POST', body: JSON.stringify({rules})});
    rules = data.rules || rules;
    syncChanges = Object.fromEntries((data.changes || []).map(change => [String(change.match || '').toLowerCase(), change]));
    renderRules();
    renderCatalog();
    showStatus('已从 ' + (data.source_name || source) + ' 获取差异：匹配 ' + Number(data.matched || 0) + ' 个，待新增 ' + Number(data.added || 0) + ' 条，待更新 ' + Number(data.updated || 0) + ' 条；请确认后保存');
  } catch (error) {
    showStatus('同步失败：' + error.message, true);
  } finally {
    syncButton.disabled = false;
    syncSource.disabled = false;
  }
};

document.getElementById('add').onclick = () => {
  readRules();
  rules.push({_draft: true, match: '', input_per_million: null, output_per_million: null, cache_read_per_million: null, cache_creation_per_million: null});
  renderRules();
};

document.getElementById('save').onclick = async () => {
  try {
    readRules();
    const validationError = validateRules();
    if (validationError) {
      showStatus('保存失败：' + validationError, true);
      return;
    }
    const data = await requestPricing(PRICING_API, {method: 'PUT', body: JSON.stringify({rules})});
    rules = data.rules || rules;
    syncChanges = {};
    renderRules();
    renderCatalog();
    showStatus('模型费用已保存，新请求将使用最新价格，历史费用保持不变');
  } catch (error) {
    showStatus('保存失败：' + error.message, true);
  }
};

renderRules();
renderCatalog();
requestPricing().then(data => {
  rules = data.rules || [];
  catalogModels = data.models || catalogModels;
  if (Array.isArray(data.pricing_sources) && data.pricing_sources.length > 0) {
    const selected = syncSource.value || data.default_source;
    syncSource.innerHTML = data.pricing_sources.map(source => '<option value="' + escapeHTML(source.id) + '">' + escapeHTML(source.name) + '</option>').join('');
    if ([...syncSource.options].some(option => option.value === selected)) {
      syncSource.value = selected;
    } else if ([...syncSource.options].some(option => option.value === data.default_source)) {
      syncSource.value = data.default_source;
    }
  }
  document.getElementById('currency').textContent = data.currency || 'USD';
  renderRules();
  renderCatalog();
  showStatus('已更新');
  (async () => {
    const served = await loadServedModels();
    if (!served) {
      await loadHostModels();
      await loadCliProxyCatalog();
    }
  })();
}).catch(error => showStatus('加载失败：' + error.message, true));
