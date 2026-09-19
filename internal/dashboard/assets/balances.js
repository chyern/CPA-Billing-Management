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
  const response = await managementFetch(url, Object.assign({
    credentials: 'same-origin',
    headers: {'Content-Type': 'application/json', ...authHeaders()},
  }, options));
  if (!response) throw new Error('管理中心登录已取消');
  if (response.status === 409) throw new Error(t('余额已发生变化，请刷新页面后重试'));
  if (!response.ok) throw new Error(await response.text() || response.statusText);
  return response.json();
}

function formatMoney(value) {
  return escapeHTML(currency) + ' ' + Number(value || 0).toFixed(3);
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
      icon: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>',
    },
    {
      label: '已设置余额',
      value: configured.length,
      icon: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>',
    },
    {
      label: '当前余额合计',
      value: formatMoney(remaining),
      isPrimary: true,
      icon: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 5H5a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2Z"/><path d="M16 12h.01"/></svg>',
    },
    {
      label: '累计费用',
      value: formatMoney(spent),
      icon: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="1" x2="12" y2="23"/><path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/></svg>',
    },
    {
      label: '余额耗尽',
      value: exhausted,
      isAlert: true,
      hasFailed: exhausted > 0,
      icon: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
    },
  ];
  document.getElementById('balanceCards').innerHTML = cards.map(card => {
    let cls = 'card';
    if (card.isPrimary) cls += ' primary-kpi';
    if (card.isAlert && card.hasFailed) cls += ' alert-kpi has-failed';
    return '<div class="' + cls + '"><div class="card-header-row"><span class="label">' + card.label + '</span><span class="card-icon">' + card.icon + '</span></div><div class="value">' + card.value + '</div></div>';
  }).join('');
}

function hasBalanceChanges(item) {
  if (item.pending) return true;
  if (item._draftNote !== undefined && item._draftNote.trim() !== (item.note || '')) return true;
  if (item._draftBalance !== undefined) {
    const value = String(item._draftBalance).trim();
    if (value === '' ? Boolean(item.configured) : !item.configured || Number(value) !== Number(item.balance)) return true;
  }
  if (item._draftRechargeAmount !== undefined || item._draftRechargeFrequency !== undefined || item._draftRechargeTime !== undefined || item._draftRechargeDay !== undefined || item._draftRechargeCron !== undefined || item._draftRechargeMode !== undefined) {
    const amount = String(item._draftRechargeAmount ?? (item.recharge_configured ? item.recharge_amount : '')).trim();
    const settings = rechargeSettings(item);
    const cron = buildRechargeCron(String(item._draftRechargeFrequency ?? settings.frequency), String(item._draftRechargeTime ?? settings.time), String(item._draftRechargeDay ?? settings.day), String(item._draftRechargeCron ?? settings.cron));
    const mode = String(item._draftRechargeMode ?? (item.recharge_configured ? item.recharge_mode : 'add')).trim();
    if (amount !== (item.recharge_configured ? String(item.recharge_amount) : '') || cron !== (item.recharge_configured ? item.recharge_cron : '') || (item.recharge_configured && mode !== item.recharge_mode)) return true;
  }
  return false;
}

function rechargeSettings(item) {
  const expression = String(item.recharge_cron || '').trim();
  const parts = expression.split(/\s+/);
  if (parts.length === 5) {
    const [minute, hour, dom, month, dow] = parts;
    const time = /^\d+$/.test(hour) && /^\d+$/.test(minute) ? String(hour).padStart(2, '0') + ':' + String(minute).padStart(2, '0') : '00:00';
    if (minute === '0' && hour === '*' && dom === '*' && month === '*' && dow === '*') return {frequency: 'hourly', time, day: '1', cron: expression};
    if (/^\d+$/.test(hour) && /^\d+$/.test(minute) && dom === '*' && month === '*' && dow === '*') return {frequency: 'daily', time, day: '1', cron: expression};
    if (/^\d+$/.test(hour) && /^\d+$/.test(minute) && dom === '*' && month === '*' && /^\d+$/.test(dow)) return {frequency: 'weekly', time, day: dow, cron: expression};
    if (/^\d+$/.test(hour) && /^\d+$/.test(minute) && /^\d+$/.test(dom) && month === '*' && dow === '*') return {frequency: 'monthly', time, day: dom, cron: expression};
  }
  return {frequency: expression ? 'custom' : 'daily', time: '00:00', day: '1', cron: expression};
}

function buildRechargeCron(frequency, time, day, custom) {
  if (frequency === 'custom') return String(custom || '').trim();
  if (frequency === 'hourly') return '0 * * * *';
  const match = /^(\d{1,2}):(\d{1,2})$/.exec(time || '00:00');
  const hour = match ? Math.min(23, Number(match[1])) : 0;
  const minute = match ? Math.min(59, Number(match[2])) : 0;
  if (frequency === 'weekly') return minute + ' ' + hour + ' * * ' + (Number(day) || 1);
  if (frequency === 'monthly') return minute + ' ' + hour + ' ' + Math.min(31, Math.max(1, Number(day) || 1)) + ' * *';
  if (frequency === 'daily') return minute + ' ' + hour + ' * * *';
  return '';
}

function formatRechargeSummary(item) {
  const amountVal = item._draftRechargeAmount !== undefined
    ? item._draftRechargeAmount
    : (item.recharge_configured ? item.recharge_amount : '');
  if (amountVal === '' || Number(amountVal) <= 0) return null;

  const settings = rechargeSettings(item);
  const freq = item._draftRechargeFrequency || settings.frequency;
  const time = item._draftRechargeTime || settings.time || '00:00';
  const day = item._draftRechargeDay || settings.day || '1';
  const mode = item._draftRechargeMode || (item.recharge_configured ? item.recharge_mode : 'add') || 'add';
  const modeText = mode === 'reset' ? t('重置为') : t('增加');
  const numVal = Number(amountVal);
  const amountFormatted = numVal % 1 === 0 ? numVal.toFixed(2) : numVal.toFixed(3);

  const weekdays = {'1': '周一', '2': '周二', '3': '周三', '4': '周四', '5': '周五', '6': '周六', '7': '周日'};
  let freqText = '';
  if (freq === 'daily') freqText = t('每天') + ' ' + time;
  else if (freq === 'weekly') freqText = (weekdays[day] ? '每' + weekdays[day] : t('每周一')) + ' ' + time;
  else if (freq === 'monthly') freqText = t('每月') + ' ' + day + ' ' + t('日') + ' ' + time;
  else if (freq === 'hourly') freqText = t('每小时整点');
  else if (freq === 'custom') freqText = 'Cron (' + (item._draftRechargeCron || settings.cron) + ')';

  return freqText + ' · ' + modeText + ' ' + escapeHTML(currency) + ' ' + amountFormatted;
}

function rechargeControls(item, itemID) {
  const summary = formatRechargeSummary(item);
  const isDraft = item._draftRechargeAmount !== undefined;
  const next = item.recharge_configured && item.recharge_next_at && summary && !isDraft
    ? '<div class="recharge-next">下次：' + escapeHTML(new Date(item.recharge_next_at).toLocaleString('zh-CN')) + '</div>'
    : (isDraft && summary ? '<div class="recharge-next draft-hint">待保存</div>' : '');

  if (summary) {
    return '<div class="recharge-summary-wrap"><button type="button" class="recharge-badge configured btn-recharge-config" data-id="' + itemID + '" title="' + t('点击修改定时额度') + '"><svg class="recharge-icon" viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg><span class="recharge-text">' + escapeHTML(summary) + '</span><span class="recharge-edit-link">' + t('编辑') + '</span></button>' + next + '</div>';
  }

  return '<div class="recharge-summary-wrap"><button type="button" class="btn-setup-recharge btn-recharge-config" data-id="' + itemID + '"><svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg><span>' + t('+ 设置定时额度') + '</span></button></div>';
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
    const value = item._draftBalance !== undefined ? item._draftBalance : (item.configured ? Number(item.balance || 0) : '');
    const noteValue = item._draftNote !== undefined ? item._draftNote : (item.note || '');
    const saveIcon = '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>';
    const deleteIcon = '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>';
    return '<tr data-id="' + itemID + '"><td>' + keyCell + '</td>'
      + '<td><input class="note-input" data-id="' + itemID + '" type="text" maxlength="200" placeholder="填写密钥用途" value="' + escapeHTML(noteValue) + '"></td>'
      + '<td class="num">' + Number(item.requests || 0).toLocaleString('zh-CN') + '</td>'
      + '<td class="num">' + formatMoney(item.cost) + '</td>'
      + '<td class="num balance-cell"><input class="balance-input" data-id="' + itemID + '" type="number" min="0" step="0.001" placeholder="不跟踪" value="' + escapeHTML(value) + '"></td>'
      + '<td class="recharge-cell">' + rechargeControls(item, itemID) + '</td>'
      + '<td class="actions-cell"><button class="btn primary btn-sm row-save" data-id="' + itemID + '"' + (hasBalanceChanges(item) ? '' : ' disabled') + '>' + saveIcon + '<span>保存</span></button><button class="btn danger btn-sm row-delete" data-id="' + itemID + '"' + (item.pending ? ' disabled' : '') + '>' + deleteIcon + '<span>删除</span></button></td></tr>';
  }).join('');

  const emptyView = '<div class="empty"><svg class="empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg><div class="empty-title">' + (balanceSearchQuery ? '未找到匹配项' : '暂无 API Key') + '</div><div class="empty-desc">仅显示 CLIProxyAPI 当前配置的 API Key。配置客户端密钥或产生 usage 事件后会显示在这里。</div></div>';

  document.getElementById('balances').innerHTML = rows
    ? '<table class="balance-table"><colgroup><col class="key-column"><col class="note-column"><col class="requests-column"><col class="cost-column"><col class="balance-column"><col class="recharge-column"><col class="actions-column"></colgroup><thead><tr><th>API Key</th><th>备注</th><th class="num">请求数</th><th class="num">累计费用</th><th class="num">当前余额</th><th>定时额度 <span class="header-sub">(定期充值)</span></th><th class="actions-cell">操作</th></tr></thead><tbody>' + rows + '</tbody></table>'
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
    rechargeAmount: item._draftRechargeAmount, rechargeFrequency: item._draftRechargeFrequency, rechargeTime: item._draftRechargeTime, rechargeDay: item._draftRechargeDay, rechargeCron: item._draftRechargeCron, rechargeMode: item._draftRechargeMode,
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
    if (draft) Object.assign(item, {_draftNote: draft.note, _draftBalance: draft.balance, _expectedBalanceVersion: draft.version, _draftRechargeAmount: draft.rechargeAmount, _draftRechargeFrequency: draft.rechargeFrequency, _draftRechargeTime: draft.rechargeTime, _draftRechargeDay: draft.rechargeDay, _draftRechargeCron: draft.rechargeCron, _draftRechargeMode: draft.rechargeMode});
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
	delete item._draftRechargeAmount;
	delete item._draftRechargeFrequency;
	delete item._draftRechargeTime;
	delete item._draftRechargeDay;
	delete item._draftRechargeCron;
  delete item._draftRechargeMode;
}

async function saveBalances(targetID) {
  try {
    const item = balances.find(candidate => candidate.api_key_id === targetID);
    if (!item) return;
    const noteInput = document.querySelector('.note-input[data-id="' + targetID + '"]');
    const balanceInput = document.querySelector('.balance-input[data-id="' + targetID + '"]');
    const rechargeAmountInput = document.querySelector('.recharge-amount[data-id="' + targetID + '"]');
    const rechargeFrequencyInput = document.querySelector('.recharge-frequency[data-id="' + targetID + '"]');
    const rechargeTimeInput = document.querySelector('.recharge-time[data-id="' + targetID + '"]');
    const rechargeDayInput = document.querySelector('.recharge-day[data-id="' + targetID + '"]');
    const rechargeCronInput = document.querySelector('.recharge-cron[data-id="' + targetID + '"]');
    const rechargeModeInput = document.querySelector('.recharge-mode[data-id="' + targetID + '"]');
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
    const rechargeAmountText = String(rechargeAmountInput ? rechargeAmountInput.value : (item._draftRechargeAmount ?? (item.recharge_configured ? item.recharge_amount : ''))).trim();
    const rechargeSettingsValue = rechargeSettings(item);
    const rechargeFrequency = String(rechargeFrequencyInput ? rechargeFrequencyInput.value : (item._draftRechargeFrequency ?? rechargeSettingsValue.frequency)).trim();
    const rechargeTime = String(rechargeTimeInput ? rechargeTimeInput.value : (item._draftRechargeTime ?? rechargeSettingsValue.time)).trim();
    const rechargeDay = String(rechargeDayInput ? rechargeDayInput.value : (item._draftRechargeDay ?? rechargeSettingsValue.day)).trim();
    const rechargeCron = buildRechargeCron(rechargeFrequency, rechargeTime, rechargeDay, rechargeCronInput ? rechargeCronInput.value : (item._draftRechargeCron ?? rechargeSettingsValue.cron));
    const rechargeMode = String(rechargeModeInput ? rechargeModeInput.value : (item._draftRechargeMode ?? (item.recharge_configured ? item.recharge_mode : 'add'))).trim();
    const oldRechargeAmount = item.recharge_configured ? String(item.recharge_amount) : '';
    const rechargeChanged = rechargeAmountText !== oldRechargeAmount || (item.recharge_configured && (rechargeCron !== item.recharge_cron || rechargeMode !== item.recharge_mode));
    if (rechargeChanged) {
      if (rechargeAmountText === '') {
        update.recharge_amount = 0; update.recharge_cron = ''; update.recharge_mode = 'add';
      } else {
        const rechargeAmount = Number(rechargeAmountText);
        if (!Number.isFinite(rechargeAmount) || rechargeAmount < 0) throw new Error('充值额度必须是大于等于 0 的有效数字');
        update.recharge_amount = rechargeAmount; update.recharge_cron = rechargeCron; update.recharge_mode = rechargeMode;
      }
      update.expected_balance_version = item._expectedBalanceVersion !== undefined ? item._expectedBalanceVersion : (item.balance_version || '');
      changed = true;
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
    const response = await managementFetch('/v0/management/api-keys?value=' + encodeURIComponent(value), {
      method: 'DELETE',
      credentials: 'same-origin',
      headers: authHeaders(),
    });
    if (!response) throw new Error('管理中心登录已取消');
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
  const rechargeAmountInput = document.querySelector('.recharge-amount[data-id="' + item.api_key_id + '"]');
  const rechargeFrequencyInput = document.querySelector('.recharge-frequency[data-id="' + item.api_key_id + '"]');
  const rechargeTimeInput = document.querySelector('.recharge-time[data-id="' + item.api_key_id + '"]');
  const rechargeDayInput = document.querySelector('.recharge-day[data-id="' + item.api_key_id + '"]');
  const rechargeCronInput = document.querySelector('.recharge-cron[data-id="' + item.api_key_id + '"]');
  const rechargeModeInput = document.querySelector('.recharge-mode[data-id="' + item.api_key_id + '"]');
  const value = keyInput && keyInput.value.trim();
  if (!value) throw new Error('请输入完整 API Key');
  const balanceValue = balanceInput ? balanceInput.value.trim() : '';
  const rechargeAmountValue = rechargeAmountInput ? rechargeAmountInput.value.trim() : '';
  const rechargeSettingsValue = rechargeSettings(item);
  const rechargeFrequency = rechargeFrequencyInput ? rechargeFrequencyInput.value : rechargeSettingsValue.frequency;
  const rechargeTime = rechargeTimeInput ? rechargeTimeInput.value : rechargeSettingsValue.time;
  const rechargeDay = rechargeDayInput ? rechargeDayInput.value : rechargeSettingsValue.day;
  const rechargeCron = buildRechargeCron(rechargeFrequency, rechargeTime, rechargeDay, rechargeCronInput ? rechargeCronInput.value : rechargeSettingsValue.cron);
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
  if (rechargeAmountValue !== '') {
    const rechargeAmount = Number(rechargeAmountValue);
    if (!Number.isFinite(rechargeAmount) || rechargeAmount < 0) throw new Error('充值额度必须是大于等于 0 的有效数字');
    update.recharge_amount = rechargeAmount;
    update.recharge_cron = rechargeCron;
    update.recharge_mode = rechargeModeInput ? rechargeModeInput.value : 'add';
    update.expected_balance_version = '';
  }
  await patchBalance(update);
  clearSavedDrafts(item.api_key_id, savedNote, savedBalance);
  await loadBalances();
  showStatus('API Key 已添加到 CLIProxyAPI 主配置');
}

let currentEditingRechargeKey = null;

function populateMonthlyDays(selectedDay) {
  const sel = document.getElementById('modalRechargeDayMonthly');
  if (!sel) return;
  sel.innerHTML = Array.from({length: 28}, (_, i) => {
    const d = String(i + 1);
    return '<option value="' + d + '"' + (String(selectedDay) === d ? ' selected' : '') + '>' + d + ' 日</option>';
  }).join('');
}

function updateModalToggleState(enabled) {
  const container = document.getElementById('rechargeConfigBody');
  const clearBtn = document.getElementById('modalBtnClearRecharge');
  if (!container) return;
  if (enabled) {
    container.style.opacity = '1';
    container.style.pointerEvents = 'auto';
    container.classList.remove('disabled');
  } else {
    container.style.opacity = '0.35';
    container.style.pointerEvents = 'none';
    container.classList.add('disabled');
  }
  if (clearBtn) {
    const isConfigured = currentEditingRechargeKey && (
      currentEditingRechargeKey.recharge_configured ||
      (currentEditingRechargeKey._draftRechargeAmount !== undefined && Number(currentEditingRechargeKey._draftRechargeAmount) > 0)
    );
    clearBtn.style.display = isConfigured ? 'inline-flex' : 'none';
  }
}

function setModalMode(mode) {
  const hiddenSelect = document.getElementById('modalRechargeMode');
  if (hiddenSelect) hiddenSelect.value = mode;
  document.querySelectorAll('input[name="modalRechargeModeRadio"]').forEach(radio => {
    radio.checked = radio.value === mode;
    const card = radio.closest('.mode-card');
    if (card) card.classList.toggle('active', radio.value === mode);
  });
}

function setModalFrequency(freq, values = {}) {
  const hiddenSelect = document.getElementById('modalRechargeFrequency');
  if (hiddenSelect) hiddenSelect.value = freq;

  document.querySelectorAll('.freq-tab-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.frequency === freq);
  });

  const sections = {
    daily: document.getElementById('scheduleSectionDaily'),
    weekly: document.getElementById('scheduleSectionWeekly'),
    monthly: document.getElementById('scheduleSectionMonthly'),
    hourly: document.getElementById('scheduleSectionHourly'),
    custom: document.getElementById('scheduleSectionCustom'),
  };

  Object.entries(sections).forEach(([key, el]) => {
    if (el) el.style.display = (key === freq) ? 'flex' : 'none';
  });

  if (values.time) {
    const dailyTime = document.getElementById('modalRechargeTimeDaily');
    const weeklyTime = document.getElementById('modalRechargeTimeWeekly');
    const monthlyTime = document.getElementById('modalRechargeTimeMonthly');
    if (dailyTime) dailyTime.value = values.time;
    if (weeklyTime) weeklyTime.value = values.time;
    if (monthlyTime) monthlyTime.value = values.time;
  }
  if (values.day) {
    const weeklyDay = document.getElementById('modalRechargeDayWeekly');
    const monthlyDay = document.getElementById('modalRechargeDayMonthly');
    if (weeklyDay) weeklyDay.value = values.day;
    if (monthlyDay) monthlyDay.value = values.day;
  }
  if (values.customCron !== undefined) {
    const cronInput = document.getElementById('modalRechargeCron');
    if (cronInput) cronInput.value = values.customCron;
  }
}

function calculateNextRunEstimate(frequency, time, day) {
  const now = new Date();
  const [hourStr, minStr] = (time || '00:00').split(':');
  const targetHour = Number(hourStr) || 0;
  const targetMin = Number(minStr) || 0;

  if (frequency === 'hourly') {
    const next = new Date(now);
    next.setMinutes(0, 0, 0);
    next.setHours(next.getHours() + 1);
    return next;
  }

  if (frequency === 'daily') {
    const next = new Date(now);
    next.setHours(targetHour, targetMin, 0, 0);
    if (next <= now) next.setDate(next.getDate() + 1);
    return next;
  }

  if (frequency === 'weekly') {
    const targetDow = Number(day) || 1;
    const currentDow = now.getDay() === 0 ? 7 : now.getDay();
    let daysToAdd = targetDow - currentDow;
    const next = new Date(now);
    next.setHours(targetHour, targetMin, 0, 0);
    if (daysToAdd < 0 || (daysToAdd === 0 && next <= now)) {
      daysToAdd += 7;
    }
    next.setDate(next.getDate() + daysToAdd);
    return next;
  }

  if (frequency === 'monthly') {
    const targetDom = Math.min(28, Math.max(1, Number(day) || 1));
    const next = new Date(now.getFullYear(), now.getMonth(), targetDom, targetHour, targetMin, 0, 0);
    if (next <= now) {
      next.setMonth(next.getMonth() + 1);
    }
    return next;
  }

  return null;
}

function updateModalPreview() {
  const isEnabled = document.getElementById('rechargeEnabledToggle')?.checked;
  const summaryEl = document.getElementById('modalPreviewRuleSummary');
  const nextRunEl = document.getElementById('modalPreviewNextRun');
  if (!summaryEl || !nextRunEl) return;

  if (!isEnabled) {
    summaryEl.textContent = '未启用定时额度';
    summaryEl.className = 'preview-rule-text disabled';
    nextRunEl.textContent = '保存后将清除此 API Key 的定时自动充值/重置规则';
    return;
  }

  const amountVal = document.getElementById('modalRechargeAmount')?.value || '0';
  const mode = document.getElementById('modalRechargeMode')?.value || 'add';
  const freq = document.getElementById('modalRechargeFrequency')?.value || 'daily';
  let time = '00:00';
  let day = '1';
  let customCron = '';

  if (freq === 'daily') {
    time = document.getElementById('modalRechargeTimeDaily')?.value || '00:00';
  } else if (freq === 'weekly') {
    day = document.getElementById('modalRechargeDayWeekly')?.value || '1';
    time = document.getElementById('modalRechargeTimeWeekly')?.value || '00:00';
  } else if (freq === 'monthly') {
    day = document.getElementById('modalRechargeDayMonthly')?.value || '1';
    time = document.getElementById('modalRechargeTimeMonthly')?.value || '00:00';
  } else if (freq === 'custom') {
    customCron = document.getElementById('modalRechargeCron')?.value || '';
  }

  const modeText = mode === 'reset' ? '自动重置为' : '自动增加';
  const numVal = Number(amountVal) || 0;
  const formattedAmount = currency + ' ' + (numVal % 1 === 0 ? numVal.toFixed(2) : numVal.toFixed(3));
  const weekdays = {'1': '周一', '2': '周二', '3': '周三', '4': '周四', '5': '周五', '6': '周六', '7': '周日'};

  let freqText = '';
  if (freq === 'daily') freqText = '每天 ' + time;
  else if (freq === 'weekly') freqText = (weekdays[day] ? '每' + weekdays[day] : '每周一') + ' ' + time;
  else if (freq === 'monthly') freqText = '每月 ' + day + ' 日 ' + time;
  else if (freq === 'hourly') freqText = '每小时整点（00分）';
  else if (freq === 'custom') freqText = customCron ? 'Cron 规则 (' + customCron + ')' : '自定义 Cron 表达式';

  summaryEl.textContent = freqText + ' ' + modeText + ' ' + formattedAmount;
  summaryEl.className = 'preview-rule-text';

  const nextDate = calculateNextRunEstimate(freq, time, day);
  if (nextDate) {
    nextRunEl.textContent = '下次预计执行时间：' + nextDate.toLocaleString('zh-CN');
  } else if (freq === 'custom' && currentEditingRechargeKey && currentEditingRechargeKey.recharge_next_at) {
    nextRunEl.textContent = '下次执行时间：' + new Date(currentEditingRechargeKey.recharge_next_at).toLocaleString('zh-CN');
  } else {
    nextRunEl.textContent = '保存后系统将根据 Cron 规则自动计算下次执行时间';
  }
}

function openRechargeModal(itemID) {
  const item = balances.find(c => c.api_key_id === itemID);
  if (!item) return;
  currentEditingRechargeKey = item;

  const keyDisplay = item.api_key || item.api_key_value || '待保存密钥';
  document.getElementById('modalTargetKey').textContent = keyDisplay;
  const noteDisplay = item._draftNote !== undefined ? item._draftNote : (item.note || '');
  document.getElementById('modalTargetNote').textContent = noteDisplay ? '（' + noteDisplay + '）' : '';
  document.querySelectorAll('.recharge-currency-label').forEach(el => el.textContent = currency);

  const hasRecharge = (item._draftRechargeAmount !== undefined)
    ? (item._draftRechargeAmount !== '' && Number(item._draftRechargeAmount) > 0)
    : (Boolean(item.recharge_configured) && Number(item.recharge_amount) > 0);

  const amount = item._draftRechargeAmount !== undefined
    ? item._draftRechargeAmount
    : (item.recharge_configured ? item.recharge_amount : '');

  const settings = rechargeSettings(item);
  const frequency = item._draftRechargeFrequency || settings.frequency || 'daily';
  const time = item._draftRechargeTime || settings.time || '00:00';
  const day = item._draftRechargeDay || settings.day || '1';
  const customCron = item._draftRechargeCron !== undefined ? item._draftRechargeCron : (settings.cron || '');
  const mode = item._draftRechargeMode || (item.recharge_configured ? item.recharge_mode : 'add') || 'add';

  populateMonthlyDays(day);

  const switchInput = document.getElementById('rechargeEnabledToggle');
  if (switchInput) {
    switchInput.checked = hasRecharge;
  }
  updateModalToggleState(hasRecharge);

  const amountInput = document.getElementById('modalRechargeAmount');
  if (amountInput) {
    amountInput.value = amount || (hasRecharge ? '10' : '');
  }

  setModalMode(mode);
  setModalFrequency(frequency, {time, day, customCron});
  updateModalPreview();

  const modal = document.getElementById('rechargeModal');
  if (modal) {
    modal.classList.add('active');
    document.body.style.overflow = 'hidden';
  }
}

function closeRechargeModal() {
  const modal = document.getElementById('rechargeModal');
  if (modal) {
    modal.classList.remove('active');
    document.body.style.overflow = '';
  }
  currentEditingRechargeKey = null;
}

async function handleModalSave() {
  if (!currentEditingRechargeKey) return;
  const item = currentEditingRechargeKey;
  const isEnabled = document.getElementById('rechargeEnabledToggle')?.checked;

  if (!isEnabled) {
    item._draftRechargeAmount = '';
    item._draftRechargeFrequency = 'daily';
    item._draftRechargeTime = '00:00';
    item._draftRechargeDay = '1';
    item._draftRechargeCron = '';
    item._draftRechargeMode = 'add';
  } else {
    const amountVal = document.getElementById('modalRechargeAmount')?.value.trim();
    if (!amountVal || Number(amountVal) <= 0) {
      showToast('请输入大于 0 的有效充值额度', true);
      document.getElementById('modalRechargeAmount')?.focus();
      return;
    }
    const freq = document.getElementById('modalRechargeFrequency')?.value || 'daily';
    const mode = document.getElementById('modalRechargeMode')?.value || 'add';
    let time = '00:00';
    let day = '1';
    let customCron = '';

    if (freq === 'daily') {
      time = document.getElementById('modalRechargeTimeDaily')?.value || '00:00';
    } else if (freq === 'weekly') {
      day = document.getElementById('modalRechargeDayWeekly')?.value || '1';
      time = document.getElementById('modalRechargeTimeWeekly')?.value || '00:00';
    } else if (freq === 'monthly') {
      day = document.getElementById('modalRechargeDayMonthly')?.value || '1';
      time = document.getElementById('modalRechargeTimeMonthly')?.value || '00:00';
    } else if (freq === 'custom') {
      customCron = document.getElementById('modalRechargeCron')?.value.trim() || '';
      if (!customCron) {
        showToast('请输入有效的 Cron 表达式', true);
        document.getElementById('modalRechargeCron')?.focus();
        return;
      }
    }

    item._draftRechargeAmount = amountVal;
    item._draftRechargeFrequency = freq;
    item._draftRechargeTime = time;
    item._draftRechargeDay = day;
    item._draftRechargeCron = customCron;
    item._draftRechargeMode = mode;
  }

  closeRechargeModal();

  if (item.pending) {
    renderBalances();
    const saveButton = document.querySelector('.row-save[data-id="' + item.api_key_id + '"]');
    if (saveButton) saveButton.disabled = !hasBalanceChanges(item);
    showToast(isEnabled ? '已配置定时额度，请保存行以完成创建' : '已取消定时额度');
  } else {
    await saveBalances(item.api_key_id);
  }
}

async function handleModalClear() {
  if (!currentEditingRechargeKey) return;
  const item = currentEditingRechargeKey;
  item._draftRechargeAmount = '';
  item._draftRechargeFrequency = 'daily';
  item._draftRechargeTime = '00:00';
  item._draftRechargeDay = '1';
  item._draftRechargeCron = '';
  item._draftRechargeMode = 'add';
  closeRechargeModal();
  if (item.pending) {
    renderBalances();
    const saveButton = document.querySelector('.row-save[data-id="' + item.api_key_id + '"]');
    if (saveButton) saveButton.disabled = !hasBalanceChanges(item);
    showToast('已清除定时额度');
  } else {
    await saveBalances(item.api_key_id);
  }
}

function initRechargeModal() {
  const modal = document.getElementById('rechargeModal');
  if (!modal) return;

  document.getElementById('closeRechargeModal')?.addEventListener('click', closeRechargeModal);
  document.getElementById('modalBtnCancel')?.addEventListener('click', closeRechargeModal);

  modal.addEventListener('click', event => {
    if (event.target === modal) closeRechargeModal();
  });

  document.addEventListener('keydown', event => {
    if (event.key === 'Escape' && modal.classList.contains('active')) {
      closeRechargeModal();
    }
  });

  document.getElementById('rechargeEnabledToggle')?.addEventListener('change', event => {
    updateModalToggleState(event.target.checked);
    updateModalPreview();
  });

  document.getElementById('modalRechargeAmount')?.addEventListener('input', updateModalPreview);

  document.querySelectorAll('#rechargeModal .preset-pill').forEach(btn => {
    btn.addEventListener('click', () => {
      const addVal = Number(btn.dataset.amount || 0);
      const input = document.getElementById('modalRechargeAmount');
      const curVal = Number(input.value || 0);
      input.value = (curVal + addVal).toFixed(2).replace(/\.00$/, '');
      updateModalPreview();
    });
  });

  document.querySelectorAll('#rechargeModal .mode-card').forEach(card => {
    card.addEventListener('click', () => {
      setModalMode(card.dataset.mode);
      updateModalPreview();
    });
  });

  document.querySelectorAll('#rechargeModal .freq-tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      setModalFrequency(btn.dataset.frequency);
      updateModalPreview();
    });
  });

  ['modalRechargeTimeDaily', 'modalRechargeTimeWeekly', 'modalRechargeTimeMonthly', 'modalRechargeDayWeekly', 'modalRechargeDayMonthly', 'modalRechargeCron'].forEach(id => {
    const el = document.getElementById(id);
    el?.addEventListener('input', updateModalPreview);
    el?.addEventListener('change', updateModalPreview);
  });

  document.querySelectorAll('#rechargeModal .cron-pill').forEach(btn => {
    btn.addEventListener('click', () => {
      const input = document.getElementById('modalRechargeCron');
      if (input) input.value = btn.dataset.cron;
      updateModalPreview();
    });
  });

  document.getElementById('modalBtnSave')?.addEventListener('click', handleModalSave);
  document.getElementById('modalBtnClearRecharge')?.addEventListener('click', handleModalClear);
}

document.getElementById('balances').addEventListener('click', async event => {
  const configBtn = event.target.closest('.btn-recharge-config');
  if (configBtn) {
    openRechargeModal(configBtn.dataset.id);
    return;
  }
  const saveButton = event.target.closest('.row-save');
  const deleteButton = event.target.closest('.row-delete');
  const button = saveButton || deleteButton;
  if (!button || button.disabled) return;
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

const handleBalanceFieldChange = event => {
  const input = event.target.closest('.note-input, .balance-input, .new-key-input, .recharge-amount, .recharge-frequency, .recharge-time, .recharge-day, .recharge-cron, .recharge-mode');
  if (!input) return;
  const item = balances.find(candidate => candidate.api_key_id === input.dataset.id);
  if (!item) return;
  if (input.classList.contains('note-input')) item._draftNote = input.value;
  else if (input.classList.contains('balance-input')) {
    if (item._expectedBalanceVersion === undefined) item._expectedBalanceVersion = item.balance_version || '';
    item._draftBalance = input.value;
  } else if (input.classList.contains('recharge-amount')) item._draftRechargeAmount = input.value;
  else if (input.classList.contains('recharge-frequency')) item._draftRechargeFrequency = input.value;
  else if (input.classList.contains('recharge-time')) item._draftRechargeTime = input.value;
  else if (input.classList.contains('recharge-day')) item._draftRechargeDay = input.value;
  else if (input.classList.contains('recharge-cron')) item._draftRechargeCron = input.value;
  else if (input.classList.contains('recharge-mode')) item._draftRechargeMode = input.value;
  else item.api_key_value = input.value;
  const saveButton = document.querySelector('.row-save[data-id="' + item.api_key_id + '"]');
  if (saveButton) saveButton.disabled = !hasBalanceChanges(item);
};
document.getElementById('balances').addEventListener('input', handleBalanceFieldChange);
document.getElementById('balances').addEventListener('change', handleBalanceFieldChange);

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
initRechargeModal();
loadBalances().catch(error => showStatus('加载失败：' + error.message, true));
