const SUMMARY_API = '/v0/management/cpa-billing-management/summary';
const PAGE_SIZE = 20;

let refreshTimer = null;
const localDate = date => {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return year + '-' + month + '-' + day;
};
const startDate = document.getElementById('startDate');
const endDate = document.getElementById('endDate');
const queryButton = document.getElementById('query');
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

function formatDuration(nanoseconds) {
  const milliseconds = Number(nanoseconds || 0) / 1e6;
  if (!Number.isFinite(milliseconds) || milliseconds <= 0) return '-';
  if (milliseconds < 1) return '不足 1 ms';
  if (milliseconds < 1000) return Math.round(milliseconds) + ' ms';
  if (milliseconds < 10000) return (milliseconds / 1000).toFixed(2) + ' s';
  return (milliseconds / 1000).toFixed(1) + ' s';
}

function renderCards() {
  const totals = state.totals || {};
  const failed = Number(totals.failed_requests || 0);
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
      icon: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
    },
  ];
  document.getElementById('cards').innerHTML = cards
    .map(card => {
      let cls = 'card';
      if (card.isPrimary) cls += ' primary-kpi';
      if (card.isAlert) cls += ' alert-kpi' + (card.hasFailed ? ' has-failed' : '');
      return '<div class="' + cls + '"><div class="card-header-row"><span class="label">' + card.label + '</span><span class="card-icon">' + card.icon + '</span></div><div class="value">' + card.value + '</div></div>';
    })
    .join('');
}

function renderModels() {
  const models = state.models || [];
  document.getElementById('models').innerHTML = models.length
    ? '<table><thead><tr><th>Provider</th><th>Model</th><th class="num">请求</th><th class="num">输入</th><th class="num">缓存</th><th class="num">输出</th><th class="num">总 token</th><th class="num">费用</th></tr></thead><tbody>'
      + models.map(model => '<tr><td><span class="provider-badge">' + escapeHTML(model.provider) + '</span></td><td>' + escapeHTML(model.model) + ' ' + (model.priced ? '' : '<span class="pill">未配置模型费用</span>') + '</td><td class="num">' + formatNumber(model.requests) + '</td><td class="num">' + formatNumber(model.input_tokens) + '</td><td class="num">' + formatNumber(model.cached_tokens) + '</td><td class="num">' + formatNumber(model.output_tokens) + '</td><td class="num">' + formatNumber(model.total_tokens) + '</td><td class="num">' + formatMoney(model.cost) + '</td></tr>').join('')
      + '</tbody></table>'
    : '<div class="empty">暂无 usage 事件</div>';
}

function renderAPIKeys() {
  const apiKeys = state.api_keys || [];
  document.getElementById('apiKeys').innerHTML = apiKeys.length
    ? '<table><thead><tr><th>API Key</th><th class="num">请求</th><th class="num">失败</th><th class="num">输入</th><th class="num">缓存</th><th class="num">输出</th><th class="num">总 token</th><th class="num">费用</th></tr></thead><tbody>'
      + apiKeys.map(key => '<tr><td><span class="code-tag">' + escapeHTML(key.api_key || '未提供') + '</span></td><td class="num">' + formatNumber(key.requests) + '</td><td class="num">' + formatNumber(key.failed_requests) + '</td><td class="num">' + formatNumber(key.input_tokens) + '</td><td class="num">' + formatNumber(key.cached_tokens) + '</td><td class="num">' + formatNumber(key.output_tokens) + '</td><td class="num">' + formatNumber(key.total_tokens) + '</td><td class="num">' + formatMoney(key.cost) + '</td></tr>').join('')
      + '</tbody></table>'
    : '<div class="empty">暂无 API Key 数据</div>';
}

function renderEvents() {
  const events = state.recent_events || [];
  const eventTable = events.length
    ? '<table><thead><tr><th>时间</th><th>模型</th><th>API Key</th><th class="num">耗时/首字</th><th class="num">输入/缓存</th><th class="num">输出</th><th class="num">费用</th><th>状态</th></tr></thead><tbody>'
      + events.slice().reverse().map(event => '<tr>'
        + '<td>' + escapeHTML(new Date(event.requested_at).toLocaleString()) + '</td>'
        + '<td>' + escapeHTML(event.model || '-') + '</td>'
        + '<td><span class="code-tag">' + escapeHTML(event.api_key || '-') + '</span></td>'
        + '<td class="num"><div class="dual-metric"><span class="dual-metric-primary">' + formatDuration(event.latency_ns) + '</span><span class="dual-metric-secondary">首字 ' + formatDuration(event.ttft_ns) + '</span></div></td>'
        + '<td class="num"><div class="dual-metric"><span class="dual-metric-primary">' + formatNumber(event.input_tokens) + '</span><span class="dual-metric-secondary">缓存 ' + formatNumber(event.cached_tokens) + '</span></div></td>'
        + '<td class="num">' + formatNumber(event.output_tokens) + '</td>'
        + '<td class="num">' + formatMoney(event.cost) + '</td>'
        + '<td>' + (event.failed ? '<span class="pill danger">失败</span>' : '<span class="pill success">成功</span>') + '</td>'
      + '</tr>').join('')
      + '</tbody></table>'
    : '<div class="empty">暂无最近事件</div>';
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
    showStatus('更新失败：' + (error.message || '请求失败'), true);
  }
}

queryButton.onclick = () => {
  if (startDate.value && endDate.value && endDate.value < startDate.value) {
    showStatus('查询失败：结束日期不能早于开始日期', true);
    return;
  }
  if (quickDates) {
    quickDates.querySelectorAll('.pill-btn').forEach(btn => btn.classList.remove('active'));
  }
  loadPage(1);
};

const quickDates = document.getElementById('quickDates');
if (quickDates) {
  quickDates.addEventListener('click', event => {
    const target = event.target.closest('[data-days]');
    if (!target) return;
    quickDates.querySelectorAll('.pill-btn').forEach(btn => btn.classList.remove('active'));
    target.classList.add('active');
    const days = Number(target.dataset.days);
    const end = new Date();
    const start = new Date();
    if (days > 0) {
      start.setDate(start.getDate() - (days - 1));
    }
    startDate.value = localDate(start);
    endDate.value = localDate(end);
    loadPage(1);
  });
}

const navPricing = document.getElementById('navPricing');
if (navPricing) {
  if (window.location.pathname.startsWith('/v0/resource/plugins/')) {
    navPricing.href = '/v0/resource/plugins/cpa-billing-management/pricing';
  } else {
    navPricing.href = '/pricing';
  }
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

