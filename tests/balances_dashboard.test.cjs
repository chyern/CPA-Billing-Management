const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');
const { webcrypto, createHash } = require('node:crypto');

const script = fs.readFileSync(path.join(__dirname, '../internal/dashboard/assets/balances.js'), 'utf8')
  .replace(/loadBalances\(\)\.catch\([^\n]+\);?\s*$/, '');
const key = 'sk-test-key';
const otherKey = 'sk-other-key';
const keyID = value => createHash('sha256').update(value).digest('hex').slice(0, 16);
async function loadDashboard(initialBalances = [], options = {}) {
  const balances = initialBalances.map(x => ({...x}));
  const hostKeys = options.hostKeys || [key];
  const elements = new Map();
  const requests = [];
  const listeners = {};
  let nextRevision = 1;
  const gates = [];
  const element = id => {
    if (!elements.has(id)) {
      const item = {textContent: '', innerHTML: '', value: '', style: {}, className: '',
        addEventListener(type, fn) { listeners[id + ':' + type] = fn; }, appendChild() {}};
      elements.set(id, item);
    }
    return elements.get(id);
  };
  const makeResponse = (payload, status = 200) => ({ok: status >= 200 && status < 300, status, statusText: 'status', text: async () => JSON.stringify(payload), json: async () => payload});
  const fetchImpl = async (url, fetchOptions = {}) => {
    requests.push({url, options: fetchOptions});
    const method = fetchOptions.method || 'GET';
    let payload;
    let status = 200;
    if (url === '/v0/management/api-keys') {
      if (method === 'PUT') hostKeys.splice(0, hostKeys.length, ...JSON.parse(fetchOptions.body));
      payload = {'api-keys': [...hostKeys]};
    }
    else if (url.startsWith('/v0/management/api-keys?') && method === 'DELETE') {
      const removed = new URL(url, 'http://localhost').searchParams.get('value');
      const index = hostKeys.indexOf(removed);
      if (index >= 0) hostKeys.splice(index, 1);
      payload = {};
    }
    else if (url === '/v0/management/config') payload = {'api-keys': hostKeys};
    else if (url === '/v0/management/cpa-billing-management/key-balances' && method === 'GET') payload = {currency: 'USD', balances: balances.map(x => ({...x}))};
    else if (url.includes('/v0/management/cpa-billing-management/key-balances') && method === 'PATCH') {
      const body = JSON.parse(fetchOptions.body || '{}');
      for (const update of body.updates || []) {
        const row = balances.find(x => x.api_key_id === update.api_key_id);
        if (update.expected_balance_version !== undefined && ((row?.balance_version || '') !== update.expected_balance_version)) { status = 409; continue; }
        if (update.delete || update.configured === false) { if (row) row.configured = false; continue; }
        if (!row) { balances.push({api_key_id: update.api_key_id, api_key: update.api_key, caller_scope: update.caller_scope, configured: update.balance !== undefined, balance: update.balance || 0, note: update.note || '', balance_version: String(nextRevision++)}); }
        else { if (update.balance !== undefined) { row.balance = update.balance; row.configured = true; row.balance_version = String(nextRevision++); } if (update.note !== undefined) row.note = update.note; }
      }
      payload = {currency: 'USD', balances: balances.map(x => ({...x}))};
    } else payload = {};
    const gate = gates.shift();
    if (gate) { gate.started(); await gate.promise; }
    return makeResponse(payload, status);
  };
  const context = vm.createContext({window: {}, document: {getElementById: id => id === 'toastContainer' ? null : element(id), querySelector: () => null, querySelectorAll: () => [], addEventListener(type, fn) { listeners['document:' + type] = fn; }, createElement: () => ({style: {}, setAttribute() {}, remove() {}}), body: {appendChild() {}, removeChild() {}}}, crypto: webcrypto, TextEncoder, fetch: fetchImpl, requireManagementKey: () => true, authHeaders: () => ({}), redirectToManagementLogin() {}, setTimeout() {}});
  vm.runInContext(script, context);
  const run = code => vm.runInContext(code, context);
  const dispatchInput = (id, cls, value) => {
    const item = run(`balances.find(x => x.api_key_id === ${JSON.stringify(id)})`);
    if (!item) throw new Error('row missing');
    const input = {value, dataset: {id}, classList: {contains(name) { return name === cls.slice(1); }}, closest() { return this; }};
    const fn = listeners['balances:input'];
    fn({target: input});
  };
  const deferNextRequest = () => {
    let release, started;
    const promise = new Promise(resolve => { release = resolve; });
    const ready = new Promise(resolve => { started = resolve; });
    gates.push({promise, started});
    return {release, ready};
  };
  return {context, elements, requests, balances, hostKeys, listeners, run, dispatchInput, deferNextRequest};
}
const id = keyID(key);
const row = (extra = {}) => ({api_key_id: id, api_key: 'sk-t••••••-key', api_key_value: key, caller_scope: 'scope', configured: true, balance: 10, balance_version: 'v1', requests: 0, cost: 0, note: '', ...extra});

test('saving a filtered row sends only that row and keeps other keys', async () => {
  const otherID = keyID(otherKey);
  const other = row({api_key_id: otherID, api_key: 'sk-o••••••-key', balance: 4, balance_version: 'o1', note: 'keep'});
  const d = await loadDashboard([row(), other], {hostKeys: [key, otherKey]});
  await d.run('loadBalances()');
  d.elements.get('balanceSearch').value = 'test';
  d.listeners['balanceSearch:input']({target: d.elements.get('balanceSearch')});
  assert.doesNotMatch(d.elements.get('balances').innerHTML, /value="keep"/);
  d.dispatchInput(id, '.note-input', 'changed');
  await d.run(`saveBalances(${JSON.stringify(id)})`);
  const patch = d.requests.find(r => r.options.method === 'PATCH');
  assert.deepEqual(JSON.parse(patch.options.body).updates.map(x => x.api_key_id), [id]);
  assert.equal(d.balances.find(x => x.api_key_id === otherID).balance, 4);
  assert.equal(d.balances.find(x => x.api_key_id === otherID).note, 'keep');
});

test('editing an exhausted key note does not submit a negative balance', async () => {
  const d = await loadDashboard([row({balance: -3, balance_version: 'v2'})]);
  await d.run('loadBalances()');
  d.dispatchInput(id, '.note-input', 'exhausted');
  await d.run(`saveBalances(${JSON.stringify(id)})`);
  const update = JSON.parse(d.requests.find(r => r.options.method === 'PATCH').options.body).updates[0];
  assert.equal(update.note, 'exhausted');
  assert.equal('balance' in update, false);
});

test('stale balance edits receive conflict and cannot restore a consumed balance', async () => {
  const d = await loadDashboard([row()]);
  await d.run('loadBalances()');
  // Simulate usage consumption after the page snapshot.
  d.balances[0].balance = 7; d.balances[0].balance_version = 'v2';
  d.dispatchInput(id, '.balance-input', '12');
  await d.run(`saveBalances(${JSON.stringify(id)})`);
  assert.match(d.elements.get('status').textContent, /保存失败/);
});

test('copy fallback reports failure when execCommand returns false', async () => {
  const d = await loadDashboard([]);
  let toast = '';
  d.context.document.getElementById = id => id === 'toastContainer' ? {appendChild(node) { toast = node.innerHTML || node.textContent; }} : d.elements.get(id) || {textContent: '', innerHTML: '', addEventListener(){}};
  d.context.document.createElement = () => ({style: {}, select() {}, value: '', remove() {}});
  d.context.document.body = {appendChild() {}, removeChild() {}};
  d.context.document.execCommand = () => false;
  await d.run(`fallbackCopy('secret')`);
  assert.match(toast, /复制失败/);
});

test('searching and adding a pending key preserve unsaved row inputs', async () => {
  const d = await loadDashboard([row()]);
  await d.run('loadBalances()');
  d.dispatchInput(id, '.note-input', 'draft note');
  d.dispatchInput(id, '.balance-input', '12.5');
  d.listeners['balanceSearch:input']({target: {value: 'no match'}});
  d.listeners['balanceSearch:input']({target: {value: ''}});
  assert.match(d.elements.get('balances').innerHTML, /value="draft note"/);
  assert.match(d.elements.get('balances').innerHTML, /value="12.5"/);
  d.listeners['addAPIKey:click']();
  assert.match(d.elements.get('balances').innerHTML, /value="draft note"/);
  assert.match(d.elements.get('balances').innerHTML, /value="12.5"/);
  assert.equal(d.run('balances.filter(item => item.pending).length'), 1);
});

test('typing while a load or save awaits a response preserves the newer draft', async () => {
  const d = await loadDashboard([row()]);
  await d.run('loadBalances()');
  const loading = d.deferNextRequest();
  const load = d.run('loadBalances()');
  await loading.ready;
  d.dispatchInput(id, '.note-input', 'typed while loading');
  loading.release();
  await load;
  assert.match(d.elements.get('balances').innerHTML, /value="typed while loading"/);

  const saving = d.deferNextRequest();
  const save = d.run(`saveBalances('${id}')`);
  await saving.ready;
  d.dispatchInput(id, '.note-input', 'newer than saved');
  d.dispatchInput(id, '.balance-input', '19');
  saving.release();
  await save;
  assert.equal(d.balances[0].note, 'typed while loading');
  assert.match(d.elements.get('balances').innerHTML, /value="newer than saved"/);
  assert.match(d.elements.get('balances').innerHTML, /value="19"/);
});

test('adding and deleting another key never rewrite an exhausted key balance', async () => {
  const d = await loadDashboard([row({balance: -3})]);
  await d.run('loadBalances()');
  d.listeners['addAPIKey:click']();
  const pending = d.run('balances.find(item => item.pending)');
  const added = 'sk-new-key-test';
  d.context.document.querySelector = selector => selector.startsWith('.new-key-input') ? {value: added}
    : selector.startsWith('.note-input') ? {value: 'new note'}
    : selector.startsWith('.balance-input') ? {value: '20'} : null;
  d.context.pendingItem = pending;
  await d.run('savePendingAPIKey(pendingItem)');
  assert.equal(d.balances.find(item => item.api_key_id === id).balance, -3);
  assert.equal(d.balances.find(item => item.api_key_id === id).configured, true);
  assert.ok(d.hostKeys.includes(added));
  const addedID = keyID(added);
  d.context.window.showConfirmDialog = async () => true;
  await d.run(`deleteAPIKey(balances.find(item => item.api_key_id === '${addedID}'))`);
  assert.equal(d.balances.find(item => item.api_key_id === id).balance, -3);
  assert.equal(d.balances.find(item => item.api_key_id === id).configured, true);
  assert.ok(!d.hostKeys.includes(added));
  const patches = d.requests.filter(request => request.options.method === 'PATCH');
  assert.equal(patches.length, 2);
  for (const request of patches) {
    assert.deepEqual(JSON.parse(request.options.body).updates.map(item => item.api_key_id), [addedID]);
  }
});

test('successful copying only shows a confirmation without exposing the key', async () => {
  const d = await loadDashboard();
  const toasts = [];
  d.context.document.getElementById = () => ({appendChild(node) { toasts.push(node.innerHTML); }});
  d.context.document.createElement = () => ({style: {}, select() {}, value: ''});
  d.context.document.execCommand = () => true;
  let copied;
  d.context.navigator = {clipboard: {writeText: async value => { copied = value; }}};
  await d.run(`copyToClipboard('${key}')`);
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(copied, key);
  await d.run(`fallbackCopy('${key}')`);
  assert.equal(toasts.length, 2);
  for (const toast of toasts) {
    assert.match(toast, /已复制/);
    assert.ok(!toast.includes(key));
  }
});
