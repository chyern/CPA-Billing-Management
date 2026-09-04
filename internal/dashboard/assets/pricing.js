const PRICING_API = '/v0/management/cpa-billing-management/prices';
const SYNC_API = PRICING_API + '/sync';
const SYNC_SOURCE_STORAGE_KEY = 'cpa-billing-pricing-source';
const priceFields = [
  'input_per_million',
  'output_per_million',
  'cache_read_per_million',
  'cache_creation_per_million',
];

const syncSource = document.getElementById('syncSource');
const syncButton = document.getElementById('sync');
const initial = JSON.parse(document.getElementById('initial').textContent);
let rules = normalizeRules(initial.rules || []);
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

function modelRuleKey(value) {
  const normalized = String(value || '').trim().toLowerCase();
  const slash = normalized.lastIndexOf('/');
  return slash >= 0 ? normalized.slice(slash + 1).trim() : normalized;
}

function normalizeRules(source) {
  const result = [];
  const indexes = new Map();
  (Array.isArray(source) ? source : []).forEach(rule => {
    const copy = Object.assign({}, rule);
    const rawMatch = String(copy.match || '').trim();
    if (!rawMatch) return;
    const match = rawMatch === '*' ? '*' : modelRuleKey(rawMatch);
    if (!match) return;
    copy.match = match;
    const key = match.toLowerCase();
    const existingIndex = indexes.get(key);
    if (existingIndex == null) {
      indexes.set(key, result.length);
      result.push(copy);
      return;
    }
    // When prefix removal collapses duplicate rows, prefer the row that has
    // an actual price while keeping the original table position.
    const existing = result[existingIndex];
    const hasPrice = item => priceFields.some(field => Number(item[field] || 0) > 0);
    if (!hasPrice(existing) && hasPrice(copy)) result[existingIndex] = copy;
  });
  return result;
}

function ruleMatchesModel(value, model) {
  const match = String(value || '').trim().toLowerCase();
  const modelName = String(model || '').trim().toLowerCase();
  return match === modelName || (match.includes('/') && modelRuleKey(match) === modelName);
}

function filterRules() {
  const input = document.getElementById('ruleSearch');
  const query = (input && input.value || '').trim().toLowerCase();
  document.querySelectorAll('#rules tbody tr').forEach(row => {
    const matchInput = row.querySelector('input.match');
    const text = (matchInput ? matchInput.value : '').toLowerCase();
    row.style.display = !query || text.includes(query) ? '' : 'none';
  });
}

function renderRules() {
  const rows = rules.map((rule, index) =>
    (() => {
      const change = syncChanges[String(rule.match || '').toLowerCase()];
      const badge = change ? '<span class="pill sync-pill">' + (change.action === 'add' ? '待新增' : '待更新') + '</span>' : '';
      const match = String(rule.match || '').trim().toLowerCase();
      const catalogRule = catalogModels.some(model => ruleMatchesModel(rule.match, model.model));
      const readonly = catalogRule ? ' readonly title="模型名称来自 CLIProxyAPI 模型列表"' : '';
      const removeButton = catalogRule
        ? '<button class="btn danger" disabled title="CLIProxyAPI 内置模型不能删除">删除</button>'
        : '<button class="btn danger" data-action="remove" data-index="' + index + '">删除</button>';
      const saveButton = '<button class="btn primary" data-action="save-row" data-index="' + index + '"' + (rule._dirty ? '' : ' disabled') + '>保存</button>';
      const trCls = change ? ' class="rule-row-changed"' : '';
      return '<tr data-i="' + index + '"' + trCls + '>' + '<td><div class="rule-match"><input class="match" data-k="match" placeholder="例如：gpt-4o" value="' + (rule._draft ? '' : escapeHTML(rule.match)) + '"' + readonly + '>' + badge + '</div></td>'
      + '<td><input data-k="input_per_million" type="number" min="0" step="0.000001" placeholder="例如：2.5" value="' + valueFor(rule, 'input_per_million') + '"></td>'
      + '<td><input data-k="output_per_million" type="number" min="0" step="0.000001" placeholder="例如：10" value="' + valueFor(rule, 'output_per_million') + '"></td>'
      + '<td><input data-k="cache_read_per_million" type="number" min="0" step="0.000001" placeholder="例如：0.25" value="' + valueFor(rule, 'cache_read_per_million') + '"></td>'
      + '<td><input data-k="cache_creation_per_million" type="number" min="0" step="0.000001" placeholder="例如：0.25" value="' + valueFor(rule, 'cache_creation_per_million') + '"></td>'
      + '<td><div class="row-actions">' + saveButton + removeButton + '</div></td>'
    + '</tr>';
    })(),
  ).join('');

  document.getElementById('rules').innerHTML = '<table><thead><tr><th>匹配</th><th class="num">输入 / 1M</th><th class="num">输出 / 1M</th><th class="num">缓存读取 / 1M</th><th class="num">缓存创建 / 1M</th><th></th></tr></thead><tbody>' + rows + '</tbody></table>';
  filterRules();
}

function catalogPriceValue(model, key) {
  const value = model.price && model.price[key];
  return Number.isFinite(Number(value)) ? Number(value) : 0;
}

function effectiveCatalogRule(model) {
  const rule = rules.find(item => ruleMatchesModel(item.match, model))
    || rules.find(item => String(item.match || '').trim() === '*');
  if (!rule) return {configured: false, price: {}};
  const configured = String(rule.match || '').trim() !== '*'
    || priceFields.some(field => Number(rule[field] || 0) > 0);
  return {configured, price: rule};
}

function refreshCatalogPrices() {
  catalogModels = catalogModels.map(model => {
    const effective = effectiveCatalogRule(model.model);
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
  if (element) {
    element.textContent = message;
    element.className = 'muted status' + (error ? ' error' : '');
  }
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

async function saveRules(nextRules) {
  const normalized = normalizeRules(nextRules);
  const data = await requestPricing(PRICING_API, {method: 'PUT', body: JSON.stringify({rules: normalized})});
  rules = normalizeRules(data.rules || normalized);
  syncChanges = {};
  rules.forEach(rule => { delete rule._dirty; });
  renderRules();
  renderCatalog();
  return data;
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

async function loadServedModels() {
  try {
    const config = await requestHostManagement('/v0/management/config');
    const configured = Array.isArray(config && config['api-keys']) ? config['api-keys'] : [];
    const keys = configured.map(key => String(key).trim()).filter(Boolean);
    const headersList = keys.length ? keys.map(key => ({Authorization: 'Bearer ' + key})) : [{}];
    let payload = null;
    for (const headers of headersList) {
      try {
        const response = await fetch('/v1/models', {credentials: 'same-origin', headers});
        if (response.ok) { payload = await response.json(); break; }
      } catch (_) {}
    }
    const models = payload && payload.data;
    if (!Array.isArray(models)) return false;
    const served = [];
    models.forEach(model => {
      const id = String(model && model.id || '').trim();
      if (!id) return;
      // owned_by is the provider reported by CLIProxyAPI; preserve it exactly.
      const provider = String(model.owned_by || '').trim();
      if (!provider) return;
      served.push({provider, model: id, configured: false, price: {}});
    });
    if (served.length === 0) return false;
    catalogModels = served;
    renderRules();
    renderCatalog();
    return true;
  } catch (_) {
    return false;
  }
}

document.getElementById('rules').addEventListener('input', event => {
  const input = event.target.closest('input');
  if (!input) return;
  const row = input.closest('tr');
  if (!row) return;
  const index = Number(row.dataset.i);
  if (!rules[index]) return;
  rules[index]._dirty = true;
  const saveButton = row.querySelector('[data-action="save-row"]');
  if (saveButton) saveButton.disabled = false;
});

document.getElementById('rules').addEventListener('click', async event => {
  const button = event.target.closest('[data-action="remove"]');
  const saveButton = event.target.closest('[data-action="save-row"]');
  if (!button && !saveButton) return;
  readRules();
  const index = Number((button || saveButton).dataset.index);
  if (saveButton) {
    if (!rules[index] || !rules[index]._dirty) return;
    try {
      const validationError = validateRules();
      if (validationError) throw new Error(validationError);
      await saveRules(rules);
      showStatus('第 ' + (index + 1) + ' 条模型费用已保存');
    } catch (error) {
      showStatus('保存失败：' + error.message, true);
    }
    return;
  }
  const label = rules[index] && rules[index].match || '这条模型费用规则';
  if (!window.confirm('确定要删除“' + label + '”吗？删除会立即生效。')) return;
  const removed = rules.splice(index, 1)[0];
  try {
    await saveRules(rules);
    showStatus('模型费用规则已删除');
  } catch (error) {
    rules.splice(index, 0, removed);
    renderRules();
    showStatus('删除失败：' + error.message, true);
  }
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
    rules = normalizeRules(data.rules || rules);
    syncChanges = Object.fromEntries((data.changes || []).map(change => [String(change.match || '').toLowerCase(), change]));
    rules.forEach(rule => {
      if (syncChanges[String(rule.match || '').toLowerCase()]) rule._dirty = true;
    });
    renderRules();
    renderCatalog();
    showStatus('');
  } catch (error) {
    showStatus('同步失败：' + error.message, true);
  } finally {
    syncButton.disabled = false;
    syncSource.disabled = false;
  }
};

document.getElementById('add').onclick = () => {
  readRules();
  const searchInput = document.getElementById('ruleSearch');
  if (searchInput && searchInput.value) {
    searchInput.value = '';
  }
  rules.push({_draft: true, match: '', input_per_million: null, output_per_million: null, cache_read_per_million: null, cache_creation_per_million: null});
  renderRules();
};

const ruleSearchInput = document.getElementById('ruleSearch');
if (ruleSearchInput) {
  ruleSearchInput.addEventListener('input', filterRules);
}

document.getElementById('save').onclick = async () => {
  try {
    readRules();
    const validationError = validateRules();
    if (validationError) {
      showStatus('保存失败：' + validationError, true);
      return;
    }
    await saveRules(rules);
    showStatus('模型费用已保存，新请求将使用最新价格，历史费用保持不变');
  } catch (error) {
    showStatus('保存失败：' + error.message, true);
  }
};

renderRules();
renderCatalog();
requestPricing().then(data => {
  rules = normalizeRules(data.rules || []);
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
  (async () => {
    await loadServedModels();
  })();
}).catch(error => showStatus('加载失败：' + error.message, true));
