// Mirrors the CLIProxyAPI management center browser credential contract.
const AUTH_STORAGE_KEY = 'cli-proxy-auth';
const AUTH_LOGIN_MARKER = 'isLoggedIn';
const AUTH_PREFIX = 'enc::v1::';
const AUTH_SALT = 'cli-proxy-api-webui::secure-storage';

function readManagementKey() {
  try {
    if (localStorage.getItem(AUTH_LOGIN_MARKER) !== 'true') return '';
    const raw = localStorage.getItem(AUTH_STORAGE_KEY);
    if (!raw) return '';

    let json = raw;
    if (raw.startsWith(AUTH_PREFIX)) {
      const binary = atob(raw.slice(AUTH_PREFIX.length));
      const encrypted = new Uint8Array(binary.length);
      for (let index = 0; index < binary.length; index++) {
        encrypted[index] = binary.charCodeAt(index);
      }
      const key = new TextEncoder().encode(
        AUTH_SALT + '|' + window.location.host + '|' + navigator.userAgent,
      );
      const plain = new Uint8Array(encrypted.length);
      for (let index = 0; index < encrypted.length; index++) {
        plain[index] = encrypted[index] ^ key[index % key.length];
      }
      json = new TextDecoder().decode(plain);
    }

    const payload = JSON.parse(json);
    const state = payload && typeof payload === 'object' && payload.state && typeof payload.state === 'object'
      ? payload.state
      : payload;
    return typeof state.managementKey === 'string' ? state.managementKey.trim() : '';
  } catch (_) {
    return '';
  }
}

const ENFORCE_MANAGEMENT_AUTH = window.location.pathname.startsWith('/v0/resource/plugins/');
const MANAGEMENT_KEY = readManagementKey();
const authHeaders = () => MANAGEMENT_KEY ? {Authorization:'Bearer '+MANAGEMENT_KEY} : {};

function redirectToManagementLogin() {
  const target = new URL('/management.html#/login', window.location.origin).href;
  try {
    if (window.top && window.top !== window) {
      window.top.location.href = target;
      return;
    }
  } catch (_) {}
  window.location.replace(target);
}

function requireManagementKey() {
  if (ENFORCE_MANAGEMENT_AUTH && !MANAGEMENT_KEY) {
    redirectToManagementLogin();
    return false;
  }
  return true;
}
