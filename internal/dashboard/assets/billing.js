const SUMMARY_API = '/v0/management/cpa-billing-management/summary';
const PAGE_SIZE = 20;

let refreshTimer = null;
let modelSearchQuery = '';
let keySearchQuery = '';
let eventStatusFilterVal = 'all';
let modelSortField = '';
let modelSortAsc = false;
let keySortField = '';
let keySortAsc = false;

const localDate = date => {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return year + '-' + month + '-' + day;
};
const startDate = document.getElementById('startDate');
const endDate = document.getElementById('endDate');
const queryButton = document.getElementById('query');
const resetQueryButton = document.getElementById('resetQuery');
const today = localDate(new Date());
startDate.value = today;
endDate.value = today;
let state = JSON.parse(document.getElementById('initial').textContent).summary || {
  currency: 'USD',
  totals: {},
  models: [],
  api_keys: [],
  recent_events: [],
  recent_events_total: 0,
  recent_events_page: 1,
  recent_events_pages: 1,
  recent_events_page_size: PAGE_SIZE,
};

const escapeHTML = value => String(value ?? '').replace(
  /[&<>"']/g,
  character => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[character]),
);
const formatNumber = value => Number(value || 0).toLocaleString('zh-CN');
const formatMoney = value => escapeHTML(state.currency || 'USD') + ' ' + Number(value || 0).toFixed(6);
const costHelp = '<span class="cost-help"><button type="button" class="cost-help-button" data-action="cost-help" aria-label="费用计算说明" aria-expanded="false">?</button><span class="cost-help-tooltip" role="tooltip" hidden>上游明确返回金额时优先使用，否则按“模型费用”中的每百万 token 价格估算</span></span>';

function formatDuration(nanoseconds) {
  const milliseconds = Number(nanoseconds || 0) / 1e6;
  if (!Number.isFinite(milliseconds) || milliseconds <= 0) return '-';
  if (milliseconds < 1) return '不足 1 ms';
  if (milliseconds < 1000) return Math.round(milliseconds) + ' ms';
  if (milliseconds < 10000) return (milliseconds / 1000).toFixed(2) + ' s';
  return (milliseconds / 1000).toFixed(1) + ' s';
}

function showToast(message, isError = false) {
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
      showToast('已复制：' + text);
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
    document.execCommand('copy');
    showToast('已复制：' + text);
  } catch (_) {
    showToast('复制失败', true);
  }
  document.body.removeChild(textarea);
}

function renderCards() {
  const totals = state.totals || {};
  const totalReq = Number(totals.requests || 0);
  const failed = Number(totals.failed_requests || 0);
  const failRate = totalReq > 0 ? (failed / totalReq * 100).toFixed(1) : '0.0';
  const cards = [
    {
      label: '总费用',
      value: formatMoney(totals.cost),
      isPrimary: true,
      icon: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2v20M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/></svg>',
    },
    {
      label: '请求数',
      value: formatNumber(totals.requests),
      icon: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>',
    },
    {
      label: '总 token',
      value: formatNumber(totals.total_tokens),
      icon: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="4" width="16" height="16" rx="2"/><rect x="9" y="9" width="6" height="6"/><path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 15h3M1 9h3M1 15h3"/></svg>',
    },
    {
      label: '失败请求',
      value: formatNumber(totals.failed_requests),
      isAlert: true,
      hasFailed: failed > 0,
      sub: failed > 0
        ? '<div class="card-sub"><span class="badge-rate">' + failRate + '% 失败率</span></div>'
        : '<div class="card-sub"><span class="badge-rate good">0% 失败率</span></div>',
      icon: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
    },
  ];
  document.getElementById('cards').innerHTML = cards
    .map(card => {
      let cls = 'card';
      if (card.isPrimary) cls += ' primary-kpi';
      if (card.isAlert) cls += ' alert-kpi' + (card.hasFailed ? ' has-failed' : '');
      return '<div class="' + cls + '"><div class="card-header-row"><span class="label">' + card.label + '</span><span class="card-icon">' + card.icon + '</span></div><div class="value">' + card.value + '</div>' + (card.sub || '') + '</div>';
    })
    .join('');
}

function sortArrow(active, asc) {
  if (!active) return '<span class="sort-icon">▲▼</span>';
  return '<span class="sort-icon" style="opacity:1">' + (asc ? '▲' : '▼') + '</span>';
}

function renderModels() {
  let models = (state.models || []).slice();
  const countBadge = document.getElementById('modelsCount');
  if (countBadge) countBadge.textContent = models.length ? String(models.length) : '';

  if (modelSearchQuery) {
    const q = modelSearchQuery.toLowerCase();
    models = models.filter(m => (m.model || '').toLowerCase().includes(q) || (m.provider || '').toLowerCase().includes(q));
  }

  if (modelSortField) {
    models.sort((a, b) => {
      let valA = a[modelSortField] ?? 0;
      let valB = b[modelSortField] ?? 0;
      if (typeof valA === 'string') {
        return modelSortAsc ? valA.localeCompare(valB) : valB.localeCompare(valA);
      }
      return modelSortAsc ? (Number(valA) - Number(valB)) : (Number(valB) - Number(valA));
    });
  }

  const emptyView = '<div class="empty"><svg class="empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg><div class="empty-title">' + (modelSearchQuery ? '未找到匹配项' : '暂无 usage 事件') + '</div><div class="empty-desc">产生模型调用事件后将自动在此汇总</div></div>';

  document.getElementById('models').innerHTML = models.length
    ? '<table><thead><tr>'
      + '<th class="sortable" data-sort-model="provider">Provider ' + sortArrow(modelSortField === 'provider', modelSortAsc) + '</th>'
      + '<th class="sortable" data-sort-model="model">Model ' + sortArrow(modelSortField === 'model', modelSortAsc) + '</th>'
      + '<th class="num sortable" data-sort-model="requests">请求 ' + sortArrow(modelSortField === 'requests', modelSortAsc) + '</th>'
      + '<th class="num sortable" data-sort-model="input_tokens">输入 ' + sortArrow(modelSortField === 'input_tokens', modelSortAsc) + '</th>'
      + '<th class="num sortable" data-sort-model="cached_tokens">缓存 ' + sortArrow(modelSortField === 'cached_tokens', modelSortAsc) + '</th>'
      + '<th class="num sortable" data-sort-model="output_tokens">输出 ' + sortArrow(modelSortField === 'output_tokens', modelSortAsc) + '</th>'
      + '<th class="num sortable" data-sort-model="total_tokens">总 token ' + sortArrow(modelSortField === 'total_tokens', modelSortAsc) + '</th>'
      + '<th class="num sortable" data-sort-model="cost">费用 ' + costHelp + ' ' + sortArrow(modelSortField === 'cost', modelSortAsc) + '</th>'
      + '</tr></thead><tbody>'
      + models.map(model => '<tr><td><span class="provider-badge">' + escapeHTML(model.provider) + '</span></td><td>' + escapeHTML(model.model) + '</td><td class="num">' + formatNumber(model.requests) + '</td><td class="num">' + formatNumber(model.input_tokens) + '</td><td class="num">' + formatNumber(model.cached_tokens) + '</td><td class="num">' + formatNumber(model.output_tokens) + '</td><td class="num">' + formatNumber(model.total_tokens) + '</td><td class="num">' + formatMoney(model.cost) + '</td></tr>').join('')
      + '</tbody></table>'
    : emptyView;
}

function renderAPIKeys() {
  let apiKeys = (state.api_keys || []).slice();
  const countBadge = document.getElementById('keysCount');
  if (countBadge) countBadge.textContent = apiKeys.length ? String(apiKeys.length) : '';

  if (keySearchQuery) {
    const q = keySearchQuery.toLowerCase();
    apiKeys = apiKeys.filter(k => (k.api_key || '').toLowerCase().includes(q));
  }

  if (keySortField) {
    apiKeys.sort((a, b) => {
      let valA = a[keySortField] ?? 0;
      let valB = b[keySortField] ?? 0;
      if (typeof valA === 'string') {
        return keySortAsc ? valA.localeCompare(valB) : valB.localeCompare(valA);
      }
      return keySortAsc ? (Number(valA) - Number(valB)) : (Number(valB) - Number(valA));
    });
  }

  const emptyView = '<div class="empty"><svg class="empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg><div class="empty-title">' + (keySearchQuery ? '未找到匹配项' : '暂无 API Key 数据') + '</div><div class="empty-desc">配置客户端密钥或产生调用事件后会在此汇总</div></div>';

  document.getElementById('apiKeys').innerHTML = apiKeys.length
    ? '<table><thead><tr>'
      + '<th class="sortable" data-sort-key="api_key">API Key ' + sortArrow(keySortField === 'api_key', keySortAsc) + '</th>'
      + '<th class="num sortable" data-sort-key="requests">请求 ' + sortArrow(keySortField === 'requests', keySortAsc) + '</th>'
      + '<th class="num sortable" data-sort-key="failed_requests">失败 ' + sortArrow(keySortField === 'failed_requests', keySortAsc) + '</th>'
      + '<th class="num sortable" data-sort-key="input_tokens">输入 ' + sortArrow(keySortField === 'input_tokens', keySortAsc) + '</th>'
      + '<th class="num sortable" data-sort-key="cached_tokens">缓存 ' + sortArrow(keySortField === 'cached_tokens', keySortAsc) + '</th>'
      + '<th class="num sortable" data-sort-key="output_tokens">输出 ' + sortArrow(keySortField === 'output_tokens', keySortAsc) + '</th>'
      + '<th class="num sortable" data-sort-key="total_tokens">总 token ' + sortArrow(keySortField === 'total_tokens', keySortAsc) + '</th>'
      + '<th class="num sortable" data-sort-key="cost">费用 ' + costHelp + ' ' + sortArrow(keySortField === 'cost', keySortAsc) + '</th>'
      + '</tr></thead><tbody>'
      + apiKeys.map(key => '<tr><td><div class="code-tag-wrap"><span class="code-tag">' + escapeHTML(key.api_key || '未提供') + '</span><button type="button" class="copy-btn" data-copy="' + escapeHTML(key.api_key || '') + '" title="复制 API Key" aria-label="复制 API Key"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg></button></div></td><td class="num">' + formatNumber(key.requests) + '</td><td class="num">' + formatNumber(key.failed_requests) + '</td><td class="num">' + formatNumber(key.input_tokens) + '</td><td class="num">' + formatNumber(key.cached_tokens) + '</td><td class="num">' + formatNumber(key.output_tokens) + '</td><td class="num">' + formatNumber(key.total_tokens) + '</td><td class="num">' + formatMoney(key.cost) + '</td></tr>').join('')
      + '</tbody></table>'
    : emptyView;
}

function renderEvents() {
  let events = (state.recent_events || []).slice().reverse();
  const countBadge = document.getElementById('eventsCount');
  if (countBadge) countBadge.textContent = state.recent_events_total ? String(state.recent_events_total) : '';

  if (eventStatusFilterVal === 'success') {
    events = events.filter(e => !e.failed);
  } else if (eventStatusFilterVal === 'failed') {
    events = events.filter(e => e.failed);
  }

  const emptyView = '<div class="empty"><svg class="empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg><div class="empty-title">暂无最近事件</div><div class="empty-desc">最近处理的 API 请求事件会实时出现在这里</div></div>';

  const eventTable = events.length
      ? '<table><thead><tr><th>时间</th><th>模型</th><th>API Key</th><th class="num">耗时/首字</th><th class="num">输入/缓存</th><th class="num">输出</th><th class="num">费用 ' + costHelp + '</th><th>状态</th></tr></thead><tbody>'
      + events.map(event => '<tr>'
        + '<td>' + escapeHTML(new Date(event.requested_at).toLocaleString()) + '</td>'
        + '<td>' + escapeHTML(event.model || '-') + ((!event.priced_by || event.priced_by === '*') ? ' <span class="pill">未配置模型费用</span>' : '') + '</td>'
        + '<td><div class="code-tag-wrap"><span class="code-tag">' + escapeHTML(event.api_key || '-') + '</span>' + (event.api_key ? '<button type="button" class="copy-btn" data-copy="' + escapeHTML(event.api_key) + '" title="复制 API Key" aria-label="复制 API Key"><svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg></button>' : '') + '</div></td>'
        + '<td class="num"><div class="dual-metric"><span class="dual-metric-primary">' + formatDuration(event.latency_ns) + '</span><span class="dual-metric-secondary">首字 ' + formatDuration(event.ttft_ns) + '</span></div></td>'
        + '<td class="num"><div class="dual-metric"><span class="dual-metric-primary">' + formatNumber(event.input_tokens) + '</span><span class="dual-metric-secondary">缓存 ' + formatNumber(event.cached_tokens) + '</span></div></td>'
        + '<td class="num">' + formatNumber(event.output_tokens) + '</td>'
        + '<td class="num">' + formatMoney(event.cost) + '</td>'
        + '<td>' + (event.failed ? '<span class="pill danger">失败</span>' : '<span class="pill success">成功</span>') + '</td>'
      + '</tr>').join('')
      + '</tbody></table>'
    : emptyView;
  const page = Number(state.recent_events_page || 1);
  const pages = Math.max(1, Number(state.recent_events_pages || 1));
  const total = Number(state.recent_events_total || 0);
  document.getElementById('events').innerHTML = eventTable
    + '<div class="pager"><button class="btn" id="prevPage" ' + (page <= 1 ? 'disabled' : '') + '>上一页</button><span class="muted">第 ' + page + ' / ' + pages + ' 页 · 共 ' + formatNumber(total) + ' 条</span><button class="btn" id="nextPage" ' + (page >= pages ? 'disabled' : '') + '>下一页</button></div>';
  document.getElementById('prevPage').onclick = () => loadPage(page - 1);
  document.getElementById('nextPage').onclick = () => loadPage(page + 1);
}

function render() {
  renderCards();
  renderModels();
  renderAPIKeys();
  renderEvents();
}

function closeCostHelp() {
  document.querySelectorAll('[data-action="cost-help"]').forEach(button => {
    button.setAttribute('aria-expanded', 'false');
    const tooltip = button.parentElement.querySelector('.cost-help-tooltip');
    if (tooltip) tooltip.hidden = true;
  });
}

document.addEventListener('click', event => {
  const copyBtn = event.target.closest('.copy-btn');
  if (copyBtn) {
    copyToClipboard(copyBtn.dataset.copy);
    return;
  }

  const modelSortHeader = event.target.closest('[data-sort-model]');
  if (modelSortHeader) {
    const field = modelSortHeader.dataset.sortModel;
    if (modelSortField === field) {
      modelSortAsc = !modelSortAsc;
    } else {
      modelSortField = field;
      modelSortAsc = false;
    }
    renderModels();
    return;
  }

  const keySortHeader = event.target.closest('[data-sort-key]');
  if (keySortHeader) {
    const field = keySortHeader.dataset.sortKey;
    if (keySortField === field) {
      keySortAsc = !keySortAsc;
    } else {
      keySortField = field;
      keySortAsc = false;
    }
    renderAPIKeys();
    return;
  }

  const button = event.target.closest('[data-action="cost-help"]');
  if (!button) {
    if (!event.target.closest('.cost-help')) closeCostHelp();
    return;
  }
  event.stopPropagation();
  const tooltip = button.parentElement.querySelector('.cost-help-tooltip');
  if (!tooltip) return;
  const expanded = button.getAttribute('aria-expanded') === 'true';
  closeCostHelp();
  if (!expanded) {
    button.setAttribute('aria-expanded', 'true');
    tooltip.hidden = false;
  }
});

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
}

async function loadPage(page) {
  if (!requireManagementKey()) return;
  const pageNumber = Math.max(1, page);
  try {
    const params = new URLSearchParams({page: String(pageNumber), page_size: String(PAGE_SIZE)});
    if (startDate.value) params.set('start', startDate.value);
    if (endDate.value) params.set('end', endDate.value);
    const response = await fetch(SUMMARY_API + '?' + params.toString(), {
      credentials: 'same-origin',
      headers: authHeaders(),
    });
    if (response.status === 401) {
      redirectToManagementLogin();
      return;
    }
    if (!response.ok) throw new Error(await response.text() || response.statusText);
    const payload = await response.json();
    state = payload.summary || payload;
    render();
    showStatus('已更新');
  } catch (error) {
    const msg = error.message || '请求失败';
    showStatus('更新失败：' + msg, true);
    showToast('更新失败：' + msg, true);
  }
}

queryButton.onclick = () => {
  if (startDate.value && endDate.value && endDate.value < startDate.value) {
    showStatus('查询失败：结束日期不能早于开始日期', true);
    showToast('查询失败：结束日期不能早于开始日期', true);
    return;
  }
  if (quickDates) {
    quickDates.querySelectorAll('.pill-btn').forEach(btn => btn.classList.remove('active'));
  }
  loadPage(1);
};

if (resetQueryButton) {
  resetQueryButton.onclick = () => {
    startDate.value = today;
    endDate.value = today;
    if (quickDates) {
      quickDates.querySelectorAll('.pill-btn').forEach(btn => btn.classList.remove('active'));
      const todayBtn = quickDates.querySelector('[data-days="0"]');
      if (todayBtn) todayBtn.classList.add('active');
    }
    loadPage(1);
  };
}

const quickDates = document.getElementById('quickDates');
if (quickDates) {
  quickDates.addEventListener('click', event => {
    const target = event.target.closest('[data-days]');
    if (!target) return;
    quickDates.querySelectorAll('.pill-btn').forEach(btn => btn.classList.remove('active'));
    target.classList.add('active');
    const days = target.dataset.days;
    if (days === 'all') {
      startDate.value = '';
      endDate.value = '';
    } else if (days === '-1') {
      const yesterday = new Date();
      yesterday.setDate(yesterday.getDate() - 1);
      const yStr = localDate(yesterday);
      startDate.value = yStr;
      endDate.value = yStr;
    } else {
      const numDays = Number(days);
      const end = new Date();
      const start = new Date();
      if (numDays > 0) {
        start.setDate(start.getDate() - (numDays - 1));
      }
      startDate.value = localDate(start);
      endDate.value = localDate(end);
    }
    loadPage(1);
  });
}

const modelSearchInput = document.getElementById('modelSearch');
if (modelSearchInput) {
  modelSearchInput.addEventListener('input', e => {
    modelSearchQuery = e.target.value.trim();
    renderModels();
  });
}

const keySearchInput = document.getElementById('keySearch');
if (keySearchInput) {
  keySearchInput.addEventListener('input', e => {
    keySearchQuery = e.target.value.trim();
    renderAPIKeys();
  });
}

const eventStatusFilter = document.getElementById('eventStatusFilter');
if (eventStatusFilter) {
  eventStatusFilter.addEventListener('click', e => {
    const btn = e.target.closest('.filter-pill');
    if (!btn) return;
    eventStatusFilter.querySelectorAll('.filter-pill').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    eventStatusFilterVal = btn.dataset.status || 'all';
    renderEvents();
  });
}

function configureAutoRefresh(seconds) {
  if (refreshTimer) clearInterval(refreshTimer);
  refreshTimer = null;
  if (seconds > 0) {
    refreshTimer = setInterval(() => loadPage(Number(state.recent_events_page || 1)), seconds * 1000);
  }
}

const autoRefresh = document.getElementById('autoRefresh');
autoRefresh.onchange = event => configureAutoRefresh(Number(event.target.value));
configureAutoRefresh(Number(autoRefresh.value));
render();
loadPage(Number(state.recent_events_page || 1));
