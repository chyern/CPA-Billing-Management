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
let rules = initial.rules || [];

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
    '<tr data-i="' + index + '">'
      + '<td><input class="match" data-k="match" placeholder="例如：openai/gpt-4o" value="' + (rule._draft ? '' : escapeHTML(rule.match)) + '"></td>'
      + '<td><input data-k="input_per_million" type="number" min="0" step="0.000001" placeholder="例如：2.5" value="' + valueFor(rule, 'input_per_million') + '"></td>'
      + '<td><input data-k="output_per_million" type="number" min="0" step="0.000001" placeholder="例如：10" value="' + valueFor(rule, 'output_per_million') + '"></td>'
      + '<td><input data-k="cache_read_per_million" type="number" min="0" step="0.000001" placeholder="例如：0.25" value="' + valueFor(rule, 'cache_read_per_million') + '"></td>'
      + '<td><input data-k="cache_creation_per_million" type="number" min="0" step="0.000001" placeholder="例如：0.25" value="' + valueFor(rule, 'cache_creation_per_million') + '"></td>'
      + '<td><button class="btn danger" data-action="remove" data-index="' + index + '">删除</button></td>'
    + '</tr>',
  ).join('');

  document.getElementById('rules').innerHTML = '<table><thead><tr><th>匹配</th><th class="num">输入 / 1M</th><th class="num">输出 / 1M</th><th class="num">缓存读取 / 1M</th><th class="num">缓存创建 / 1M</th><th></th></tr></thead><tbody>' + rows + '</tbody></table>';
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
  const source = syncSource.value;
  syncButton.disabled = true;
  syncSource.disabled = true;
  showStatus('正在从 ' + syncSource.options[syncSource.selectedIndex].text + ' 获取价格…');
  try {
    const data = await requestPricing(SYNC_API + '?source=' + encodeURIComponent(source), {method: 'POST'});
    rules = data.rules || rules;
    renderRules();
    showStatus('已从 ' + (data.source_name || source) + ' 同步：匹配 ' + Number(data.matched || 0) + ' 个，新增 ' + Number(data.added || 0) + ' 条，更新 ' + Number(data.updated || 0) + ' 条');
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
    renderRules();
    showStatus('模型费用已保存，历史估算已更新');
  } catch (error) {
    showStatus('保存失败：' + error.message, true);
  }
};

renderRules();
requestPricing().then(data => {
  rules = data.rules || [];
  document.getElementById('currency').textContent = data.currency || 'USD';
  renderRules();
  showStatus('已更新');
}).catch(error => showStatus('加载失败：' + error.message, true));
