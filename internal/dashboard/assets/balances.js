const BALANCES_API = '/v0/management/cpa-billing-management/key-balances';
let balances = [];
let currency = 'USD';

const escapeHTML = value => String(value ?? '').replace(
  /[&<>"']/g,
  character => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[character]),
);

function maskKey(value) {
  const chars = Array.from(String(value || '').trim());
  if (!chars.length) return '';
  if (chars.length <= 2) return '•'.repeat(chars.length);
  if (chars.length <= 8) return chars[0] + '•'.repeat(chars.length - 2) + chars[chars.length - 1];
  return chars.slice(0, 4).join('') + '••••••' + chars.slice(-4).join('');
}

async function keyIdentifier(value) {
  const bytes = new TextEncoder().encode(String(value || '').trim());
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', bytes));
  return [...digest.slice(0, 8)].map(byte => byte.toString(16).padStart(2, '0')).join('');
}

async function callerScope(value) {
  const bytes = new TextEncoder().encode('cli-proxy-api:caller-scope:v1\0' + String(value || '').trim());
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', bytes));
  return [...digest].map(byte => byte.toString(16).padStart(2, '0')).join('');
}

function showStatus(message, error = false) {
  const element = document.getElementById('status');
  const dot = document.getElementById('statusDot');
  element.textContent = message;
  element.className = 'muted status' + (error ? ' error' : '');
  dot.className = 'status-dot' + (error ? ' error' : '');
}

async function requestJSON(url, options = {}) {
  if (!requireManagementKey()) throw new Error('管理中心登录已失效');
  const response = await fetch(url, Object.assign({
    credentials: 'same-origin',
    headers: {'Content-Type': 'application/json', ...authHeaders()},
  }, options));
  if (response.status === 401) {
    redirectToManagementLogin();
    throw new Error('管理中心登录已失效');
  }
  if (!response.ok) throw new Error(await response.text() || response.statusText);
  return response.json();
}

function formatMoney(value) {
  return escapeHTML(currency) + ' ' + Number(value || 0).toFixed(6);
}

function renderCards() {
  const configured = balances.filter(item => item.configured);
  const remaining = configured.reduce((total, item) => total + Number(item.balance || 0), 0);
  const spent = balances.reduce((total, item) => total + Number(item.cost || 0), 0);
  const exhausted = configured.filter(item => Number(item.balance || 0) <= 0).length;
  const cards = [
    ['密钥数量', balances.length],
    ['已设置余额', configured.length],
    ['当前余额合计', formatMoney(remaining)],
    ['累计费用', formatMoney(spent)],
    ['余额耗尽', exhausted],
  ];
  document.getElementById('balanceCards').innerHTML = cards.map((card, index) =>
    '<div class="card' + (index === 2 ? ' primary-kpi' : '') + '"><div class="card-header-row"><span class="label">' + card[0] + '</span></div><div class="value">' + card[1] + '</div></div>'
  ).join('');
}

function renderBalances() {
  const rows = balances.map(item => {
    let status = '<span class="pill">未设置</span>';
    if (item.configured) {
      status = Number(item.balance || 0) > 0
        ? '<span class="pill success">正常</span>'
        : '<span class="pill danger">已耗尽</span>';
    }
    const value = item.configured ? Number(item.balance || 0) : '';
    return '<tr><td><span class="code-tag">' + escapeHTML(item.api_key || '未命名密钥') + '</span></td>'
      + '<td><input class="note-input" data-id="' + escapeHTML(item.api_key_id) + '" type="text" maxlength="200" placeholder="填写密钥用途" value="' + escapeHTML(item.note || '') + '"></td>'
      + '<td class="num">' + Number(item.requests || 0).toLocaleString('zh-CN') + '</td>'
      + '<td class="num">' + formatMoney(item.cost) + '</td>'
      + '<td class="num balance-cell"><input class="balance-input" data-id="' + escapeHTML(item.api_key_id) + '" type="number" min="0" step="0.000001" placeholder="不跟踪" value="' + value + '"></td>'
      + '<td class="status-cell">' + status + '</td></tr>';
  }).join('');
  document.getElementById('balances').innerHTML = rows
    ? '<table class="balance-table"><colgroup><col class="key-column"><col class="note-column"><col class="requests-column"><col class="cost-column"><col class="balance-column"><col class="status-column"></colgroup><thead><tr><th>API Key</th><th>备注</th><th class="num">请求数</th><th class="num">累计费用</th><th class="num">当前余额</th><th class="status-cell">状态</th></tr></thead><tbody>' + rows + '</tbody></table>'
    : '<div class="empty">暂无 API Key。配置客户端密钥或产生 usage 事件后会显示在这里。</div>';
  renderCards();
}

async function configuredKeys() {
  try {
    const config = await requestJSON('/v0/management/config');
    const raw = Array.isArray(config && (config.apiKeys || config['api-keys'])) ? (config.apiKeys || config['api-keys']) : [];
    const keys = raw.map(item => typeof item === 'string' ? item : String(item && (item.key || item.api_key || item.value) || '')).map(key => key.trim()).filter(Boolean);
    return Promise.all(keys.map(async key => ({
      api_key_id: await keyIdentifier(key),
      caller_scope: await callerScope(key),
      api_key: maskKey(key),
    })));
  } catch (_) {
    return [];
  }
}

async function loadBalances() {
  const [data, hostKeys] = await Promise.all([requestJSON(BALANCES_API), configuredKeys()]);
  currency = data.currency || 'USD';
  document.getElementById('currency').textContent = currency;
  const byID = new Map((data.balances || []).map(item => [String(item.api_key_id || ''), item]));
  hostKeys.forEach(key => {
    const current = byID.get(key.api_key_id);
    if (current) {
      if (!current.caller_scope) current.caller_scope = key.caller_scope;
      if (!current.api_key) current.api_key = key.api_key;
      return;
    }
    byID.set(key.api_key_id, Object.assign({balance: 0, configured: false, requests: 0, cost: 0}, key));
  });
  balances = [...byID.values()];
  renderBalances();
  showStatus('已更新');
}

document.getElementById('saveBalances').onclick = async () => {
  try {
    const configured = [];
    const notes = [];
    document.querySelectorAll('.balance-input').forEach(input => {
      const value = input.value.trim();
      if (value === '') return;
      const balance = Number(value);
      if (!Number.isFinite(balance) || balance < 0) throw new Error('余额必须是大于等于 0 的有效数字');
      const item = balances.find(candidate => candidate.api_key_id === input.dataset.id);
      configured.push({
        api_key_id: input.dataset.id,
        caller_scope: item && item.caller_scope || '',
        api_key: item && item.api_key || '',
        balance,
      });
    });
    document.querySelectorAll('.note-input').forEach(input => {
      const item = balances.find(candidate => candidate.api_key_id === input.dataset.id);
      notes.push({
        api_key_id: input.dataset.id,
        api_key: item && item.api_key || '',
        note: input.value.trim(),
      });
    });
    await requestJSON(BALANCES_API, {method: 'PUT', body: JSON.stringify({balances: configured, notes})});
    await loadBalances();
    showStatus('密钥余额已保存，余额耗尽后新请求将被拦截');
  } catch (error) {
    showStatus('保存失败：' + error.message, true);
  }
};

loadBalances().catch(error => showStatus('加载失败：' + error.message, true));
