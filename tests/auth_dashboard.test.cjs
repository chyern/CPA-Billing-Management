const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');
const {TextEncoder, TextDecoder} = require('node:util');

const source = fs.readFileSync(path.join(__dirname, '../internal/dashboard/assets/auth.js'), 'utf8');

function readKey(value, loggedIn = true, sessionValue = null) {
  const storage = new Map([
    ['isLoggedIn', loggedIn ? 'true' : 'false'],
    ['cli-proxy-auth', value],
  ]);
  const context = vm.createContext({
    window: {
      matchMedia: () => ({matches: false, addEventListener() {}}),
      top: null,
      addEventListener() {},
      location: {pathname: '/v0/resource/plugins/cpa-billing-management/billing', origin: 'http://localhost', host: 'localhost'},
    },
    document: {documentElement: {setAttribute() {}}, body: {}, addEventListener() {}},
    localStorage: {getItem: key => storage.get(key) || null},
    sessionStorage: {getItem: key => key === 'cli-proxy-auth' ? sessionValue : (sessionValue ? 'true' : null)},
    navigator: {userAgent: 'test'},
    TextEncoder, TextDecoder,
    atob: value => Buffer.from(value, 'base64').toString('binary'),
    MutationObserver: class { observe() {} },
    setTimeout,
    URL,
  });
  context.window.top = context.window;
  vm.runInContext(source + '\n;globalThis.__readManagementKey = readManagementKey;', context);
  return vm.runInContext('__readManagementKey()', context);
}

test('reads the management center credential formats', () => {
  assert.equal(readKey(JSON.stringify({state: {managementKey: '  current-secret  '}})), 'current-secret');
  assert.equal(readKey(JSON.stringify({managementKey: 'direct-secret'})), 'direct-secret');
  const payload = JSON.stringify({state: {managementKey: 'encrypted-secret'}});
  const key = Buffer.from('cli-proxy-api-webui::secure-storage|localhost|test');
  const bytes = Buffer.from(payload);
  for (let index = 0; index < bytes.length; index++) bytes[index] ^= key[index % key.length];
  assert.equal(readKey('enc::v1::' + bytes.toString('base64')), 'encrypted-secret');
  assert.equal(readKey(JSON.stringify({state: {managementKey: 'secret'}}), false), '');
  assert.equal(readKey('', false, JSON.stringify({state: {managementKey: 'session-secret'}})), '');
});

function hostWindow() {
  const listeners = {};
  return {
    location: {origin: 'http://localhost', pathname: '/management.html', hash: '#/plugins/billing'},
    document: {documentElement: {getAttribute: () => 'light'}},
    addEventListener(type, listener) { (listeners[type] ||= []).push(listener); },
    emit(type) { (listeners[type] || []).forEach(listener => listener()); },
  };
}

function pluginPage(host, fetchImpl = async () => ({ok: true, status: 200})) {
  const dialogs = [];
  function element() {
    const listeners = {};
    const attributes = {};
    return {
      value: '', textContent: '', type: 'password', disabled: false,
      addEventListener(type, fn) { listeners[type] = fn; },
      fire(type) { return listeners[type]({preventDefault() {}}); },
      setAttribute(name, value) { attributes[name] = value; },
      removeAttribute(name) { delete attributes[name]; },
      focus() {},
    };
  }
  const context = vm.createContext({
    window: {
      top: host, location: {origin: 'http://localhost', host: 'localhost', pathname: '/v0/resource/plugins/cpa-billing-management/billing'},
      addEventListener() {}, matchMedia: () => ({matches: false, addEventListener() {}}),
    },
    document: {
      documentElement: {setAttribute() {}}, body: {appendChild() {}},
      createElement() {
        const elements = new Map();
        const dialog = {...element(), open: false, removed: false,
          querySelector(selector) {
            if (!elements.has(selector)) elements.set(selector, element());
            return elements.get(selector);
          },
          showModal() { this.open = true; }, close() { this.open = false; }, remove() { this.removed = true; },
        };
        dialogs.push(dialog);
        return dialog;
      },
    },
    localStorage: {getItem: () => null, setItem() { assert.fail('must not persist a password'); }},
    sessionStorage: {getItem: () => null, setItem() { assert.fail('must not persist a password'); }},
    MutationObserver: class { observe() {} },
    fetch: fetchImpl, AbortSignal, TextEncoder, TextDecoder,
  });
  if (!host) context.window.top = context.window;
  vm.runInContext(source, context);
  return {
    dialogs, run: code => vm.runInContext(code, context),
    submit(key) {
      const dialog = dialogs.at(-1);
      dialog.querySelector('input').value = key;
      return dialog.querySelector('.cpa-auth-form').fire('submit');
    },
  };
}

const tick = () => new Promise(resolve => setImmediate(resolve));

test('verified login survives plugin recreation but not host refresh or another tab', async () => {
  const host = hostWindow();
  const page = pluginPage(host);
  const login = page.run('requireManagementKey()');
  await page.submit('temporary-password');
  await login;
  assert.equal(page.dialogs[0].removed, true);
  assert.equal(page.dialogs[0].querySelector('input').value, '');
  for (let index = 0; index < 3; index++) {
    const nextPage = pluginPage(host);
    await nextPage.run('requireManagementKey()');
    assert.equal(nextPage.dialogs.length, 0);
    assert.equal(nextPage.run('authHeaders().Authorization'), 'Bearer temporary-password');
  }
  for (const nextHost of [hostWindow(), hostWindow(), null]) {
    const nextPage = pluginPage(nextHost);
    assert.equal(nextPage.run('authHeaders().Authorization'), undefined);
    nextPage.run('requireManagementKey()');
    assert.equal(nextPage.dialogs.length, 1);
  }
});

test('wrong passwords and network errors remain in the same dialog without saving a credential', async () => {
  let status = 401;
  const requests = [];
  const page = pluginPage(hostWindow(), async (url, options) => {
    requests.push({url, options});
    if (status === 0) throw new Error('offline');
    return {ok: status === 200, status};
  });
  const pending = page.run('managementFetch("/v0/management/config")');
  await page.submit('   ');
  assert.equal(requests.length, 0);
  await page.submit('wrong');
  assert.equal(requests.length, 1);
  assert.match(page.dialogs[0].querySelector('.cpa-auth-error').textContent, /密码错误/);
  assert.equal(page.run('authHeaders().Authorization'), undefined);
  status = 0;
  await page.submit('retry');
  assert.match(page.dialogs[0].querySelector('.cpa-auth-error').textContent, /无法连接/);
  status = 200;
  await page.submit('correct');
  await pending;
  assert.equal(page.dialogs.length, 1);
  assert.equal(requests.at(-1).options.headers.Authorization, 'Bearer correct');
});

test('logout and unauthorized events clear the shared temporary session', async () => {
  for (const event of ['hashchange', 'unauthorized']) {
    const host = hostWindow();
    const page = pluginPage(host);
    const login = page.run('requireManagementKey()');
    await page.submit('temporary-password');
    await login;
    host.location.hash = '#/login';
    host.emit(event);
    assert.equal(page.run('authHeaders().Authorization'), undefined);
    assert.equal(pluginPage(host).run('authHeaders().Authorization'), undefined);
  }
});

test('concurrent expired requests share one prompt, and a late 401 cannot clear the new login', async () => {
  let late;
  const requests = [];
  const page = pluginPage(hostWindow(), async (url, options) => {
    requests.push({url, options});
    if (url === '/late' && options.headers.Authorization === 'Bearer old') return new Promise(resolve => { late = resolve; });
    return {ok: true, status: url !== '/v0/management/debug' && options.headers.Authorization === 'Bearer old' ? 401 : 200};
  });
  const firstLogin = page.run('requireManagementKey()');
  await page.submit('old');
  await firstLogin;
  const a = page.run('managementFetch("/first")');
  const b = page.run('managementFetch("/second")');
  const c = page.run('managementFetch("/late")');
  await tick();
  assert.equal(page.dialogs.length, 2, 'all expired requests share one additional dialog');
  await page.submit('new');
  await Promise.all([a, b]);
  late({ok: false, status: 401});
  await c;
  assert.equal(page.dialogs.length, 2);
  assert.equal(page.run('authHeaders().Authorization'), 'Bearer new');
  assert.equal(requests.at(-1).options.headers.Authorization, 'Bearer new');
});

test('a superseded request does not open a login dialog for its late 401', async () => {
  let finish;
  const page = pluginPage(hostWindow(), async url => url === '/v0/management/debug'
    ? {ok: true, status: 200} : new Promise(resolve => { finish = resolve; }));
  const login = page.run('requireManagementKey()');
  await page.submit('temporary-password');
  await login;
  const request = page.run('globalThis.current = true; managementFetch("/old", {}, () => current)');
  await tick();
  page.run('current = false');
  finish({ok: false, status: 401});
  await request;
  assert.equal(page.dialogs.length, 1);
  assert.equal(page.run('authHeaders().Authorization'), 'Bearer temporary-password');
});
