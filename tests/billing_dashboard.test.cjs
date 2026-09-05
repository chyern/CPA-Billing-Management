const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const script = fs.readFileSync(path.join(__dirname, '../internal/dashboard/assets/billing.js'), 'utf8');
const nextTurn = () => new Promise(resolve => setImmediate(resolve));

function deferred() {
  let resolve, reject;
  const promise = new Promise((done, fail) => { resolve = done; reject = fail; });
  return {promise, resolve, reject};
}

function summary({page = 1, total = 0, pages = 1, cost = 0, events = []} = {}) {
  return {
    currency: 'USD', totals: {cost}, models: [], api_keys: [],
    recent_events: events, recent_events_page: page,
    recent_events_total: total, recent_events_pages: pages,
  };
}

function loadDashboard() {
  const elements = new Map();
  const requests = [];
  const documentListeners = {};
  let refresh;
  let redirects = 0;
  const element = id => {
    if (id === 'toastContainer') return null;
    if (!elements.has(id)) elements.set(id, {
      textContent: id === 'initial' ? '{}' : '',
      innerHTML: '', value: id === 'autoRefresh' ? '5' : '', style: {},
      listeners: {},
      addEventListener(type, listener) { this.listeners[type] = listener; },
      querySelectorAll: () => [], querySelector: () => null,
      classList: {add() {}, remove() {}},
    });
    return elements.get(id);
  };
  const context = vm.createContext({
    window: {}, URLSearchParams,
    document: {
      getElementById: element, querySelectorAll: () => [],
      addEventListener(type, listener) { documentListeners[type] = listener; },
    },
    requireManagementKey: () => true,
    authHeaders: () => ({}),
    redirectToManagementLogin: () => { redirects++; },
    setInterval: callback => { refresh = callback; return 1; }, clearInterval() {},
    fetch: (url, options) => {
      const pending = deferred();
      requests.push({url, options, ...pending});
      return pending.promise;
    },
  });
  vm.runInContext(script, context);
  return {context, elements, requests, documentListeners, refresh: () => refresh(), redirects: () => redirects};
}

async function respond(request, payload) {
  request.resolve({ok: true, status: 200, json: async () => payload});
  await nextTurn();
}

test('a delayed old response cannot replace a newer date/page query', async () => {
  const {context, elements, requests} = loadDashboard();
  elements.get('startDate').value = '2026-08-01';
  elements.get('endDate').value = '2026-08-31';
  const query = vm.runInContext('loadPage(3)', context);
  const params = new URL(requests[1].url, 'http://localhost').searchParams;
  assert.equal(params.get('start'), '2026-08-01');
  assert.equal(params.get('end'), '2026-08-31');
  assert.equal(params.get('page'), '3');

  await respond(requests[1], summary({page: 3, cost: 8}));
  await query;
  await respond(requests[0], summary({page: 1, cost: 999}));
  assert.equal(vm.runInContext('state.recent_events_page', context), 3);
  assert.equal(vm.runInContext('state.totals.cost', context), 8);
  assert.equal(elements.get('status').textContent, '已更新');
  assert.doesNotMatch(elements.get('cards').innerHTML, /999\.000000/);
});

test('an old response still decoding JSON cannot replace the latest query', async () => {
  const {context, requests} = loadDashboard();
  const oldJSON = deferred();
  requests[0].resolve({ok: true, status: 200, json: () => oldJSON.promise});
  await nextTurn();
  const query = vm.runInContext('loadPage(2)', context);
  await respond(requests[1], summary({page: 2, cost: 5}));
  await query;
  oldJSON.resolve(summary({page: 1, cost: 100}));
  await nextTurn();
  assert.equal(vm.runInContext('state.totals.cost', context), 5);
});

test('superseded errors and unauthorized responses do not change current status or redirect', async () => {
  for (const oldResult of ['error', 'unauthorized']) {
    const {context, elements, requests, redirects} = loadDashboard();
    const query = vm.runInContext('loadPage(2)', context);
    await respond(requests[1], summary({page: 2}));
    await query;
    if (oldResult === 'error') requests[0].reject(new Error('old network failure'));
    else requests[0].resolve({ok: false, status: 401});
    await nextTurn();
    assert.equal(elements.get('status').textContent, '已更新');
    assert.equal(redirects(), 0);
  }
});

test('automatic refresh waits for pending manual queries and then refreshes the resulting page', async () => {
  const {context, requests, refresh} = loadDashboard();
  await refresh();
  assert.equal(requests.length, 1);
  const query = vm.runInContext('loadPage(3)', context);
  await refresh();
  assert.equal(requests.length, 2);
  await respond(requests[0], summary({page: 1}));
  await refresh();
  assert.equal(requests.length, 2, 'completion of a stale request must not clear the pending flag');
  await respond(requests[1], summary({page: 3}));
  await query;

  const automatic = refresh();
  assert.equal(requests.length, 3);
  assert.equal(new URL(requests[2].url, 'http://localhost').searchParams.get('page'), '3');
  await respond(requests[2], summary({page: 3}));
  await automatic;
});

test('event status changes request a filtered first page and retain server pagination totals', async () => {
  const {context, elements, requests} = loadDashboard();
  await respond(requests[0], summary({page: 3, total: 90, pages: 5}));
  const clickStatus = status => {
    const button = {dataset: {status}, classList: {add() {}}};
    elements.get('eventStatusFilter').listeners.click({target: {closest: () => button}});
  };
  clickStatus('failed');
  const params = new URL(requests[1].url, 'http://localhost').searchParams;
  assert.equal(params.get('event_status'), 'failed');
  assert.equal(params.get('page'), '1');
  await respond(requests[1], summary({total: 25, pages: 2, events: [{
    model: 'older-failed-model', failed: true, requested_at: '2026-09-05T08:00:00Z',
  }]}));
  assert.equal(elements.get('eventsCount').textContent, '25');
  assert.match(elements.get('events').innerHTML, /older-failed-model/);
  assert.match(elements.get('events').innerHTML, /第 1 \/ 2 页 · 共 25 条/);

  elements.get('nextPage').onclick();
  const nextParams = new URL(requests[2].url, 'http://localhost').searchParams;
  assert.equal(nextParams.get('event_status'), 'failed');
  assert.equal(nextParams.get('page'), '2');
  await respond(requests[2], summary({page: 2, total: 25, pages: 2}));

  clickStatus('success');
  const successParams = new URL(requests[3].url, 'http://localhost').searchParams;
  assert.equal(successParams.get('event_status'), 'success');
  assert.equal(successParams.get('page'), '1');
  await respond(requests[3], summary());
  assert.equal(vm.runInContext('pageRequestPending', context), false);
});

test('cost help opens and closes inside sortable headers without sorting their tables', () => {
  const {context, documentListeners} = loadDashboard();
  for (const table of ['model', 'key']) {
    const tooltip = {hidden: true};
    const help = {querySelector: () => tooltip};
    const button = {
      expanded: 'false', parentElement: help,
      getAttribute() { return this.expanded; },
      setAttribute(name, value) { this.expanded = value; },
    };
    const header = {dataset: table === 'model' ? {sortModel: 'cost'} : {sortKey: 'cost'}};
    context.document.querySelectorAll = () => [button];
    let stopped = 0;
    const event = {
      target: {closest: selector => ({
        '[data-action="cost-help"]': button,
        '.cost-help': help,
        ['[data-sort-' + table + ']']: header,
      })[selector] || null},
      stopPropagation: () => { stopped++; },
    };

    documentListeners.click(event);
    assert.equal(button.expanded, 'true');
    assert.equal(tooltip.hidden, false);
    assert.equal(vm.runInContext(table + 'SortField', context), '');
    documentListeners.click(event);
    assert.equal(button.expanded, 'false');
    assert.equal(tooltip.hidden, true);
    assert.equal(vm.runInContext(table + 'SortField', context), '');
    assert.equal(stopped, 2);

    const tooltipEvent = {
      target: {closest: selector => selector === '.cost-help' ? help
        : selector === '[data-sort-' + table + ']' ? header : null},
    };
    documentListeners.click(tooltipEvent);
    assert.equal(vm.runInContext(table + 'SortField', context), '');

    // The actual header remains sortable in both directions.
    const headerEvent = {
      target: {closest: selector => selector === '[data-sort-' + table + ']' ? header : null},
    };
    documentListeners.click(headerEvent);
    assert.equal(vm.runInContext(table + 'SortField', context), 'cost');
    assert.equal(vm.runInContext(table + 'SortAsc', context), false);
    documentListeners.click(headerEvent);
    assert.equal(vm.runInContext(table + 'SortAsc', context), true);
  }
});

test('paid wildcard and zero-token priced events do not show an unpriced badge', async () => {
  const {elements, requests} = loadDashboard();
  await respond(requests[0], summary({events: [
    {model: 'paid wildcard', priced_by: '*', priced: true, cost: 2},
    {model: 'zero tokens', priced_by: '*', priced: true, cost: 0},
    {model: 'missing rule', priced_by: '*', priced: false, cost: 0},
  ]}));
  const html = elements.get('events').innerHTML;
  assert.equal((html.match(/未配置模型费用/g) || []).length, 1);
  assert.match(html, /missing rule <span class="pill">未配置模型费用/);
});
