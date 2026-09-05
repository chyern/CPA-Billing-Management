const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const script = fs.readFileSync(path.join(__dirname, '../internal/dashboard/assets/pricing.js'), 'utf8');

async function loadDashboard(savedRules, models, {initialRules = [], waitForModels, waitForPricing} = {}) {
  const elements = new Map();
  const requests = [];
  const element = id => {
    if (!elements.has(id)) elements.set(id, {
      textContent: id === 'initial' ? JSON.stringify({rules: initialRules}) : '',
      innerHTML: '', value: '', style: {},
      listeners: {},
      addEventListener(type, listener) { this.listeners[type] = listener; }, appendChild() {},
    });
    return elements.get(id);
  };
  Object.assign(element('syncSource'), {
    value: 'models.dev', selectedIndex: 0,
    options: [{value: 'models.dev', text: 'Models.dev'}],
  });
  const context = vm.createContext({
    window: {},
    document: {getElementById: element, querySelectorAll: () => [], querySelector: () => null},
    localStorage: {getItem() { return null; }, setItem() {}},
    requireManagementKey: () => true,
    authHeaders: () => ({}),
    fetch: async (url, options = {}) => {
      requests.push({url, options});
      let payload;
      if (url === '/v0/management/config') payload = {'api-keys': ['test-key']};
      else if (url === '/v1/models') {
        if (waitForModels) await waitForModels;
        payload = {data: models};
      }
      else if (options.method === 'POST') payload = {rules: JSON.parse(options.body).rules, changes: []};
      else {
        if (waitForPricing) await waitForPricing;
        payload = {rules: savedRules, currency: 'USD'};
      }
      return {ok: true, status: 200, json: async () => payload};
    },
  });
  // No toast DOM is needed to exercise loading or syncing.
  context.document.getElementById = id => id === 'toastContainer' ? null : element(id);
  vm.runInContext(script, context);
  await new Promise(resolve => setImmediate(resolve));
  return {context, elements, requests};
}

const priceRule = (match, input = 0, output = 0, cacheRead = 0, cacheCreation = 0) => ({
  match, input_per_million: input, output_per_million: output,
  cache_read_per_million: cacheRead, cache_creation_per_million: cacheCreation,
});

const nextTurn = () => new Promise(resolve => setImmediate(resolve));

function deferred() {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return {promise, resolve};
}

function editRule(elements, index, values) {
  const saveButton = {disabled: true};
  const row = {
    dataset: {i: String(index)},
    querySelector: () => saveButton,
    querySelectorAll: () => inputs,
  };
  const inputs = Object.entries(values).map(([key, value]) => ({
    dataset: {k: key}, value: String(value),
    closest: selector => selector === 'input' ? inputs[0] : row,
  }));
  elements.get('rules').listeners.input({target: inputs[0]});
  assert.equal(saveButton.disabled, false);
}

test('an empty database displays served models and includes them in the sync preview', async () => {
  const {context, elements, requests} = await loadDashboard([], [
    {id: 'gpt-5.5', owned_by: 'codex'},
    {id: 'claude-sonnet', owned_by: 'claude'},
  ]);
  const markup = elements.get('rules').innerHTML;
  assert.match(markup, /value="gpt-5\.5" readonly/);
  assert.match(markup, /value="claude-sonnet" readonly/);
  assert.doesNotMatch(markup, /暂无价格规则/);

  await elements.get('sync').onclick();
  const sync = requests.find(request => request.options.method === 'POST');
  assert.match(sync.url, /source=models\.dev&preview=1/);
  const syncPayload = JSON.parse(sync.options.body);
  assert.deepEqual(syncPayload.rules, [priceRule('gpt-5.5'), priceRule('claude-sonnet')]);
  assert.deepEqual(syncPayload.models, [
    {provider: 'codex', model: 'gpt-5.5'},
    {provider: 'claude', model: 'claude-sonnet'},
  ]);
  assert.equal(requests.some(request => request.options.method === 'PUT'), false);
  assert.equal(vm.runInContext('validateRules()', context), '');
});

test('model merging preserves saved prices, normalizes prefixes, and retains wildcard prices', async () => {
  const {context} = await loadDashboard([
    priceRule('codex/GPT-5.5', 2.5, 15, 0.25),
    priceRule('*', 1, 2, 0.1, 0.2),
    priceRule('manual-model', 3, 4),
  ], [
    {id: 'gpt-5.5', owned_by: 'codex'},
    {id: 'openai/gpt-5.5', owned_by: 'openai'},
    {id: 'claude/claude-sonnet', owned_by: 'claude'},
    {id: 'claude-sonnet', owned_by: 'anthropic'},
  ]);
  const expected = [
    priceRule('gpt-5.5', 2.5, 15, 0.25),
    priceRule('*', 1, 2, 0.1, 0.2),
    priceRule('manual-model', 3, 4),
    priceRule('claude-sonnet', 1, 2, 0.1, 0.2),
  ];
  assert.deepEqual(JSON.parse(vm.runInContext('JSON.stringify(rules)', context)), expected);
  await vm.runInContext('loadServedModels()', context);
  assert.deepEqual(JSON.parse(vm.runInContext('JSON.stringify(rules)', context)), expected);
});

test('delayed model discovery preserves unsaved prices and empty fields when it re-renders', async () => {
  const modelsReady = deferred();
  const {context, elements} = await loadDashboard([priceRule('gpt-5.5', 2.5, 15)], [
    {id: 'gpt-5.5', owned_by: 'codex'},
    {id: 'claude-sonnet', owned_by: 'claude'},
  ], {waitForModels: modelsReady.promise});
  editRule(elements, 0, {...priceRule('gpt-5.5', 9.75, ''), cache_read_per_million: ''});
  modelsReady.resolve();
  await nextTurn();

  assert.equal(vm.runInContext('rules[0].input_per_million', context), 9.75);
  assert.equal(vm.runInContext('Number.isNaN(rules[0].output_per_million)', context), true);
  assert.equal(vm.runInContext('rules[0]._dirty', context), true);
  const markup = elements.get('rules').innerHTML;
  assert.match(markup, /data-k="input_per_million"[^>]*value="9.75"/);
  assert.match(markup, /data-k="output_per_million"[^>]*value=""/);
  assert.match(markup, /data-action="save-row" data-index="0">/);
  assert.match(markup, /value="claude-sonnet" readonly/);
});

test('delayed model discovery preserves a partially entered new rule', async () => {
  const modelsReady = deferred();
  const {context, elements} = await loadDashboard([], [{id: 'gpt-5.5', owned_by: 'codex'}], {
    waitForModels: modelsReady.promise,
  });
  elements.get('add').onclick();
  editRule(elements, 0, priceRule('custom-model', 7, '', '', ''));
  modelsReady.resolve();
  await nextTurn();

  assert.equal(vm.runInContext('rules[0].match', context), 'custom-model');
  assert.equal(vm.runInContext('rules[0].input_per_million', context), 7);
  assert.match(elements.get('rules').innerHTML, /value="custom-model"/);
  assert.match(elements.get('rules').innerHTML, /data-k="input_per_million"[^>]*value="7"/);
  assert.match(elements.get('rules').innerHTML, /data-k="output_per_million"[^>]*value=""/);
});

test('initial pricing refresh does not replace edits made before it completes', async () => {
  const pricingReady = deferred();
  const {context, elements} = await loadDashboard([priceRule('gpt-5.5', 2)], [], {
    initialRules: [priceRule('gpt-5.5', 1)], waitForPricing: pricingReady.promise,
  });
  editRule(elements, 0, priceRule('gpt-5.5', 8));
  pricingReady.resolve();
  await nextTurn();

  assert.equal(vm.runInContext('rules[0].input_per_million', context), 8);
  assert.match(elements.get('rules').innerHTML, /data-k="input_per_million"[^>]*value="8"/);
});
