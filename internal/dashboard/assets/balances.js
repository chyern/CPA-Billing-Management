const BALANCES_API = '/v0/management/cpa-billing-management/key-balances';
const t = value => typeof window.cpaTranslate === 'function' ? window.cpaTranslate(value) : value;
let balances = [];
let currency = 'USD';
let balanceLoadSequence = 0;

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

function generateAPIKey() {
  const bytes = new Uint8Array(24);
  crypto.getRandomValues(bytes);
  return 'sk-' + [...bytes].map(byte => byte.toString(16).padStart(2, '0')).join('');
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

let balanceSearchQuery = '';

function showToast(message, isError = false) {
  if (!message) return;
  const container = document.getElementById('toastContainer');
  if (!container) return;
  const toast = document.createElement('div');
  toast.className = 'cpa-toast ' + (isError ? 'error' : 'success');
  const icon = isError
    ? '<svg class="cpa-toast-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>'
    : '<svg class="cpa-toast-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>';
  toast.innerHTML = icon + '<span>' + escapeHTML(message) + '</span>';
  container.appendChild(toast);
  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateY(10px) scale(0.95)';
    setTimeout(() => toast.remove(), 200);
  }, 3000);
}

function copyToClipboard(text) {
  if (!text) return;
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(() => {
      showToast(t('已复制'));
    }).catch(() => {
      fallbackCopy(text);
    });
  } else {
    fallbackCopy(text);
  }
}

function fallbackCopy(text) {
  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.select();
  try {
    if (!document.execCommand('copy')) throw new Error('copy failed');
    showToast(t('已复制'));
  } catch (_) {
    showToast(t('复制失败'), true);
  }
  document.body.removeChild(textarea);
}

function showStatus(message, error = false) {
  const element = document.getElementById('status');
  const dot = document.getElementById('statusDot');
  if (element) {
    element.textContent = message;
    element.className = 'muted status' + (error ? ' error' : '');
  }
  if (dot) {
    dot.className = 'status-dot' + (error ? ' error' : '');
  }
  if (message) {
    showToast(message, error);
  }
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
  if (response.status === 409) throw new Error(t('余额已发生变化，请刷新页面后重试'));
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
    {
      label: '密钥数量',
      value: balances.length,
      icon: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>',
    },
    {
      label: '已设置余额',
      value: configured.length,
      icon: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"/></svg>',
    },
    {
      label: '当前余额合计',
      value: formatMoney(remaining),
      isPrimary: true,
      icon: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M19 5H5a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2Z"/><path d="M16 12h.01"/></svg>',
    },
    {
      label: '累计费用',
      value: formatMoney(spent),
      icon: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="1" x2="12" y2="23"/><path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/></svg>',
    },
    {
      label: '余额耗尽',
      value: exhausted,
      isAlert: true,
      hasFailed: exhausted > 0,
      icon: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
    },
  ];
  document.getElementById('balanceCards').innerHTML = cards.map(card => {
    let cls = 'card';
    if (card.isPrimary) cls += ' primary-kpi';
    if (card.isAlert && card.hasFailed) cls += ' alert-kpi has-failed';
    return '<div class="' + cls + '"><div class="card-header-row"><span class="label">' + card.label + '</span><span class="card-icon">' + card.icon + '</span></div><div class="value">' + card.value + '</div></div>';
  }).join('');
}

function renderBalances() {
  let displayBalances = balances;
  if (balanceSearchQuery) {
    const q = balanceSearchQuery.toLowerCase();
    displayBalances = balances.filter(item =>
      (item.api_key || '').toLowerCase().includes(q)
      || (item.note || '').toLowerCase().includes(q)
      || (item.api_key_value || '').toLowerCase().includes(q)
    );
  }

  const rows = displayBalances.map(item => {
    const itemID = escapeHTML(item.api_key_id);
    const keyCell = item.pending
      ? '<div class="code-tag-wrap"><input class="new-key-input" data-id="' + itemID + '" type="text" autocomplete="off" value="' + escapeHTML(item.api_key_value || '') + '" placeholder="输入完整 API Key"><button type="button" class="copy-btn" data-copy-val="' + escapeHTML(item.api_key_value || '') + '" title="复制完整 API Key" aria-label="复制 API Key"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg></button></div>'
      : '<div class="code-tag-wrap"><span class="code-tag">' + escapeHTML(item.api_key || '未命名密钥') + '</span>' + (item.api_key ? '<button type="button" class="copy-btn" data-copy-val="' + escapeHTML(item.api_key_value || item.api_key) + '" title="复制 API Key" aria-label="复制 API Key"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg></button>' : '') + '</div>';
    let status = '<span class="pill">未设置</span>';
    if (item.pending) {
      status = '<span class="pill sync-pill">待保存</span>';
    } else if (item.configured) {
      status = Number(item.balance || 0) > 0
        ? '<span class="pill success">正常</span>'
        : '<span class="pill danger">已耗尽</span>';
    }
    const value = item._draftBalance !== undefined ? item._draftBalance : (item.configured ? Number(item.balance || 0) : '');
    const noteValue = item._draftNote !== undefined ? item._draftNote : (item.note || '');
    return '<tr data-id="' + itemID + '"><td>' + keyCell + '</td>'
      + '<td><input class="note-input" data-id="' + itemID + '" type="text" maxlength="200" placeholder="填写密钥用途" value="' + escapeHTML(noteValue) + '"></td>'
      + '<td class="num">' + Number(item.requests || 0).toLocaleString('zh-CN') + '</td>'
      + '<td class="num">' + formatMoney(item.cost) + '</td>'
      + '<td class="num balance-cell"><input class="balance-input" data-id="' + itemID + '" type="number" min="0" step="0.000001" placeholder="不跟踪" value="' + escapeHTML(value) + '"></td>'
      + '<td class="status-cell">' + status + '</td>'
      + '<td class="actions-cell"><button class="btn primary btn-sm row-save" data-id="' + itemID + '">保存</button><button class="btn danger btn-sm row-delete" data-id="' + itemID + '"' + (item.pending ? ' disabled' : '') + '>删除</button></td></tr>';
  }).join('');

  const emptyView = '<div class="empty"><svg class="empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg><div class="empty-title">' + (balanceSearchQuery ? '未找到匹配项' : '暂无 API Key') + '</div><div class="empty-desc">仅显示 CLIProxyAPI 当前配置的 API Key。配置客户端密钥或产生 usage 事件后会显示在这里。</div></div>';

  document.getElementById('balances').innerHTML = rows
    ? '<table class="balance-table"><colgroup><col class="key-column"><col class="note-column"><col class="requests-column"><col class="cost-column"><col class="balance-column"><col class="status-column"><col class="actions-column"></colgroup><thead><tr><th>API Key</th><th>备注</th><th class="num">请求数</th><th class="num">累计费用</th><th class="num">当前余额</th><th class="status-cell">状态</th><th class="actions-cell">操作</th></tr></thead><tbody>' + rows + '</tbody></table>'
    : emptyView;
  renderCards();
}

async function configuredKeys() {
  const config = await requestJSON('/v0/management/api-keys');
  const raw = Array.isArray(config && config['api-keys']) ? config['api-keys'] : [];
  const keys = raw.map(key => String(key).trim()).filter(Boolean);
  return Promise.all(keys.map(async key => ({
    api_key_id: await keyIdentifier(key),
    caller_scope: await callerScope(key),
    api_key: maskKey(key),
    api_key_value: key,
  })));
}

async function configuredAPIKeys() {
  const config = await requestJSON('/v0/management/api-keys');
  return Array.isArray(config && config['api-keys']) ? config['api-keys'].map(key => String(key).trim()).filter(Boolean) : [];
}

async function patchBalance(update) {
  return requestJSON(BALANCES_API, {method: 'PATCH', body: JSON.stringify({updates: [update]})});
}

async function loadBalances() {
  const sequence = ++balanceLoadSequence;
  const [data, hostKeys] = await Promise.all([requestJSON(BALANCES_API), configuredKeys()]);
  if (sequence !== balanceLoadSequence) return;
  // Read drafts after the requests settle: typing while loading must survive.
  const pending = balances.filter(item => item.pending);
  const drafts = new Map(balances.map(item => [item.api_key_id, {
    note: item._draftNote, balance: item._draftBalance, version: item._expectedBalanceVersion,
  }]));
  currency = data.currency || 'USD';
  document.getElementById('currency').textContent = currency;
  const savedByID = new Map((data.balances || []).map(item => [String(item.api_key_id || ''), item]));
  const seen = new Set();
  balances = hostKeys.filter(key => {
    if (seen.has(key.api_key_id)) return false;
    seen.add(key.api_key_id);
    return true;
  }).map(key => {
    const item = Object.assign({balance: 0, configured: false, requests: 0, cost: 0, note: ''}, savedByID.get(key.api_key_id) || {}, key);
    const draft = drafts.get(key.api_key_id);
    if (draft) Object.assign(item, {_draftNote: draft.note, _draftBalance: draft.balance, _expectedBalanceVersion: draft.version});
    return item;
  }).concat(pending);
  renderBalances();
  showStatus('已更新');
}

function clearSavedDrafts(itemID, savedNote, savedBalance) {
  const item = balances.find(candidate => candidate.api_key_id === itemID);
  if (!item) return;
  if (item._draftNote === savedNote) delete item._draftNote;
  if (item._draftBalance === savedBalance) {
    delete item._draftBalance;
    delete item._expectedBalanceVersion;
  }
}

async function saveBalances(targetID) {
  try {
    const item = balances.find(candidate => candidate.api_key_id === targetID);
    if (!item) return;
    const noteInput = document.querySelector('.note-input[data-id="' + targetID + '"]');
    const balanceInput = document.querySelector('.balance-input[data-id="' + targetID + '"]');
    const note = (noteInput ? noteInput.value : (item._draftNote !== undefined ? item._draftNote : item.note || '')).trim();
    const value = String(balanceInput ? balanceInput.value : (item._draftBalance !== undefined ? item._draftBalance : item.configured ? item.balance : '')).trim();
    const update = {api_key_id: item.api_key_id, api_key: item.api_key || '', caller_scope: item.caller_scope || ''};
    let changed = false;
    if (note !== (item.note || '')) { update.note = note; changed = true; }
    if (value === '') {
      if (item.configured) { update.configured = false; update.expected_balance_version = item._expectedBalanceVersion !== undefined ? item._expectedBalanceVersion : (item.balance_version || ''); changed = true; }
    } else {
      const balance = Number(value);
      if (!item.configured || balance !== Number(item.balance)) {
        if (!Number.isFinite(balance) || balance < 0) throw new Error('余额必须是大于等于 0 的有效数字');
        update.balance = balance;
        update.expected_balance_version = item._expectedBalanceVersion !== undefined ? item._expectedBalanceVersion : (item.balance_version || '');
        changed = true;
      }
    }
    if (!changed) return showStatus('没有需要保存的更改');
    const savedNote = item._draftNote;
    const savedBalance = item._draftBalance;
    await patchBalance(update);
    clearSavedDrafts(targetID, savedNote, savedBalance);
    await loadBalances();
    showStatus('密钥余额已保存，余额耗尽后新请求将被拦截');
  } catch (error) {
    showStatus('保存失败：' + error.message, true);
  }
}

async function deleteAPIKey(item) {
  const label = item && item.api_key || '该 API Key';
  const confirmed = await (window.showConfirmDialog ? window.showConfirmDialog({
    title: t('确认删除 API Key'),
    message: t('确定要删除此 API Key 吗？'),
    target: label,
    detail: t('这会从 CLIProxyAPI 主配置中永久移除该 API Key，同时清除插件中的余额和备注。'),
    confirmText: t('删除'),
    cancelText: t('取消'),
    danger: true,
  }) : window.confirm(t('确定要删除 ') + label + t(' 吗？\n\n这会从 CLIProxyAPI 主配置中永久移除该 API Key，同时清除插件中的余额和备注。')));
  if (!confirmed) {
    return;
  }
  try {
    const value = item && item.api_key_value;
    if (!value) throw new Error('无法读取完整 API Key，未执行删除');
    const response = await fetch('/v0/management/api-keys?value=' + encodeURIComponent(value), {
      method: 'DELETE',
      credentials: 'same-origin',
      headers: authHeaders(),
    });
    if (response.status === 401) {
      redirectToManagementLogin();
      throw new Error('管理中心登录已失效');
    }
    if (!response.ok) throw new Error(await response.text() || response.statusText);
    await patchBalance({api_key_id: item.api_key_id, delete: true, expected_balance_version: item.balance_version || ''});
    await loadBalances();
    showStatus('API Key 已从 CLIProxyAPI 主配置删除');
  } catch (error) {
    showStatus('删除失败：' + error.message, true);
  }
}

async function savePendingAPIKey(item) {
  const keyInput = document.querySelector('.new-key-input[data-id="' + item.api_key_id + '"]');
  const noteInput = document.querySelector('.note-input[data-id="' + item.api_key_id + '"]');
  const balanceInput = document.querySelector('.balance-input[data-id="' + item.api_key_id + '"]');
  const value = keyInput && keyInput.value.trim();
  if (!value) throw new Error('请输入完整 API Key');
  const balanceValue = balanceInput ? balanceInput.value.trim() : '';
  const savedNote = item._draftNote;
  const savedBalance = item._draftBalance;
  const balance = balanceValue === '' ? 0 : Number(balanceValue);
  if (!Number.isFinite(balance) || balance < 0) throw new Error('余额必须是大于等于 0 的有效数字');
  const configured = await configuredAPIKeys();
  if (configured.includes(value)) throw new Error('该 API Key 已存在');
  configured.push(value);
  await requestJSON('/v0/management/api-keys', {method: 'PUT', body: JSON.stringify(configured)});
  item.pending = false;
  item.api_key_value = value;
  item.api_key = maskKey(value);
  item.api_key_id = await keyIdentifier(value);
  item.caller_scope = await callerScope(value);
  item.note = noteInput ? noteInput.value.trim() : '';
  item.configured = balanceValue !== '';
  item.balance = balance;
  item.balance_version = '';
  const update = {api_key_id: item.api_key_id, api_key: item.api_key, caller_scope: item.caller_scope};
  if (balanceValue !== '') update.balance = balance;
  if (item.note) update.note = item.note;
  if (balanceValue !== '') update.expected_balance_version = '';
  await patchBalance(update);
  clearSavedDrafts(item.api_key_id, savedNote, savedBalance);
  await loadBalances();
  showStatus('API Key 已添加到 CLIProxyAPI 主配置');
}

document.getElementById('balances').addEventListener('click', async event => {
  const saveButton = event.target.closest('.row-save');
  const deleteButton = event.target.closest('.row-delete');
  const button = saveButton || deleteButton;
  if (!button) return;
  const id = button.dataset.id;
  if (deleteButton) {
    const item = balances.find(candidate => candidate.api_key_id === id);
    if (item) await deleteAPIKey(item);
    return;
  }
  const item = balances.find(candidate => candidate.api_key_id === id);
  if (!item) return;
  if (item.pending) {
    try {
      await savePendingAPIKey(item);
    } catch (error) {
      showStatus('保存失败：' + error.message, true);
    }
    return;
  }
  await saveBalances(id);
});

document.getElementById('balances').addEventListener('input', event => {
  const input = event.target.closest('.note-input, .balance-input, .new-key-input');
  if (!input) return;
  const item = balances.find(candidate => candidate.api_key_id === input.dataset.id);
  if (!item) return;
  if (input.classList.contains('note-input')) item._draftNote = input.value;
  else if (input.classList.contains('balance-input')) {
    if (item._expectedBalanceVersion === undefined) item._expectedBalanceVersion = item.balance_version || '';
    item._draftBalance = input.value;
  } else item.api_key_value = input.value;
});

document.addEventListener('click', event => {
  const copyBtn = event.target.closest('.copy-btn');
  if (copyBtn) {
    const val = copyBtn.dataset.copyVal;
    if (val) {
      copyToClipboard(val);
    }
    return;
  }
});

const balanceSearchInput = document.getElementById('balanceSearch');
if (balanceSearchInput) {
  balanceSearchInput.addEventListener('input', e => {
    balanceSearchQuery = e.target.value.trim();
    renderBalances();
  });
}

document.getElementById('addAPIKey').addEventListener('click', () => {
  if (balances.some(item => item.pending)) {
    showStatus('请先保存当前待添加的 API Key', true);
    return;
  }
  const generated = generateAPIKey();
  balances.push({pending: true, api_key_id: 'pending-' + Date.now(), api_key: '', api_key_value: generated, balance: 0, configured: false, requests: 0, cost: 0, note: ''});
  renderBalances();
  const input = document.querySelector('.new-key-input');
  if (input) input.focus();
  showStatus('已新增待保存的 API Key');
});
loadBalances().catch(error => showStatus('加载失败：' + error.message, true));
