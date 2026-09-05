const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');
const {TextEncoder, TextDecoder} = require('node:util');

const source = fs.readFileSync(path.join(__dirname, '../internal/dashboard/assets/auth.js'), 'utf8');

function readKey(value, loggedIn = true) {
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
});
