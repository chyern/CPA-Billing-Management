const SUMMARY_API = '/v0/management/cpa-billing-management/summary';
const PAGE_SIZE = 20;

let refreshTimer = null;
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
  const cards = [
    ['总费用', formatMoney(totals.cost)],
    ['请求数', formatNumber(totals.requests)],
    ['总 token', formatNumber(totals.total_tokens)],
    ['失败请求', formatNumber(totals.failed_requests)],
  ];
  document.getElementById('cards').innerHTML = cards
    .map(card => '<div class="card"><div class="label">' + card[0] + '</div><div class="value">' + card[1] + '</div></div>')
    .join('');
}

function renderModels() {
  const models = state.models || [];
  document.getElementById('models').innerHTML = models.length
    ? '<table><thead><tr><th>Provider</th><th>Model</th><th class="num">请求</th><th class="num">输入</th><th class="num">缓存</th><th class="num">输出</th><th class="num">总 token</th><th class="num">费用</th></tr></thead><tbody>'
      + models.map(model => '<tr><td>' + escapeHTML(model.provider) + '</td><td>' + escapeHTML(model.model) + ' ' + (model.priced ? '' : '<span class="pill">未配置模型费用</span>') + '</td><td class="num">' + formatNumber(model.requests) + '</td><td class="num">' + formatNumber(model.input_tokens) + '</td><td class="num">' + formatNumber(model.cached_tokens) + '</td><td class="num">' + formatNumber(model.output_tokens) + '</td><td class="num">' + formatNumber(model.total_tokens) + '</td><td class="num">' + formatMoney(model.cost) + '</td></tr>').join('')
      + '</tbody></table>'
    : '<div class="empty">暂无 usage 事件</div>';
}

function renderAPIKeys() {
  const apiKeys = state.api_keys || [];
  document.getElementById('apiKeys').innerHTML = apiKeys.length
    ? '<table><thead><tr><th>API Key</th><th class="num">请求</th><th class="num">失败</th><th class="num">输入</th><th class="num">缓存</th><th class="num">输出</th><th class="num">总 token</th><th class="num">费用</th></tr></thead><tbody>'
      + apiKeys.map(key => '<tr><td>' + escapeHTML(key.api_key || '未提供') + '</td><td class="num">' + formatNumber(key.requests) + '</td><td class="num">' + formatNumber(key.failed_requests) + '</td><td class="num">' + formatNumber(key.input_tokens) + '</td><td class="num">' + formatNumber(key.cached_tokens) + '</td><td class="num">' + formatNumber(key.output_tokens) + '</td><td class="num">' + formatNumber(key.total_tokens) + '</td><td class="num">' + formatMoney(key.cost) + '</td></tr>').join('')
      + '</tbody></table>'
    : '<div class="empty">暂无 API Key 数据</div>';
}

function renderEvents() {
  const events = state.recent_events || [];
  const eventTable = events.length
    ? '<table><thead><tr><th>时间</th><th>模型</th><th>API Key</th><th class="num">耗时/首字</th><th class="num">输入/缓存</th><th class="num">输出</th><th class="num">费用</th><th>状态</th></tr></thead><tbody>'
      + events.slice().reverse().map(event => '<tr><td>' + escapeHTML(new Date(event.requested_at).toLocaleString()) + '</td><td>' + escapeHTML(event.model || '-') + '</td><td>' + escapeHTML(event.api_key || '-') + '</td><td class="num">' + formatDuration(event.latency_ns) + ' / ' + formatDuration(event.ttft_ns) + '</td><td class="num">' + formatNumber(event.input_tokens) + ' / ' + formatNumber(event.cached_tokens) + '</td><td class="num">' + formatNumber(event.output_tokens) + '</td><td class="num">' + formatMoney(event.cost) + '</td><td>' + (event.failed ? '<span class="pill">失败</span>' : '成功') + '</td></tr>').join('')
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
  element.textContent = message;
  element.className = 'muted status' + (error ? ' error' : '');
}

async function loadPage(page) {
  if (!requireManagementKey()) return;
  const pageNumber = Math.max(1, page);
  try {
    const response = await fetch(SUMMARY_API + '?page=' + pageNumber + '&page_size=' + PAGE_SIZE, {
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
