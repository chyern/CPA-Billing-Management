// Mirrors the CLIProxyAPI management center browser credential contract.
const AUTH_STORAGE_KEY = 'cli-proxy-auth';
const AUTH_LOGIN_MARKER = 'isLoggedIn';
const HOST_THEME_STORAGE_KEY = 'cli-proxy-theme';

function systemTheme() {
  return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function storedHostTheme() {
  try {
    const raw = localStorage.getItem(HOST_THEME_STORAGE_KEY);
    if (!raw) return systemTheme();
    const payload = JSON.parse(raw);
    if (!payload || typeof payload !== 'object' || !payload.state || typeof payload.state !== 'object') {
      return systemTheme();
    }
    const theme = typeof payload.state.theme === 'string' ? payload.state.theme : '';
    if (theme === 'dark' || theme === 'white' || theme === 'light') return theme;
    return systemTheme();
  } catch (_) {
    return systemTheme();
  }
}

function currentHostTheme() {
  try {
    if (window.top && window.top !== window) {
      const value = window.top.document.documentElement.getAttribute('data-theme');
      if (value === 'dark' || value === 'white') return value;
      return 'light';
    }
  } catch (_) {}
  return storedHostTheme();
}

function applyHostTheme() {
  document.documentElement.setAttribute('data-theme', currentHostTheme());
}

function initializeHostThemeSync() {
  applyHostTheme();
  try {
    if (window.top && window.top !== window) {
      const hostRoot = window.top.document.documentElement;
      new MutationObserver(applyHostTheme).observe(hostRoot, {attributes: true, attributeFilter: ['data-theme']});
    }
  } catch (_) {}
  window.addEventListener('storage', event => {
    if (!event.key || event.key === HOST_THEME_STORAGE_KEY) applyHostTheme();
  });
  if (window.matchMedia) {
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', applyHostTheme);
  }
}

initializeHostThemeSync();

function readManagementKey() {
  try {
    // The management center keeps unremembered credentials only in its own
    // React state. Do not assume it writes a sessionStorage credential.
    if (localStorage.getItem(AUTH_LOGIN_MARKER) !== 'true') return '';
    const raw = localStorage.getItem(AUTH_STORAGE_KEY);
    if (!raw) return '';

    let json = raw;
    if (raw.startsWith('enc::v1::')) {
      const binary = atob(raw.slice('enc::v1::'.length));
      const encrypted = new Uint8Array(binary.length);
      for (let index = 0; index < binary.length; index++) encrypted[index] = binary.charCodeAt(index);
      const key = new TextEncoder().encode('cli-proxy-api-webui::secure-storage|'
        + window.location.host + '|' + navigator.userAgent);
      const plain = new Uint8Array(encrypted.length);
      for (let index = 0; index < encrypted.length; index++) plain[index] = encrypted[index] ^ key[index % key.length];
      json = new TextDecoder().decode(plain);
    }
    const payload = JSON.parse(json);
    const state = payload && typeof payload === 'object' && payload.state && typeof payload.state === 'object'
      ? payload.state : payload;
    return typeof state.managementKey === 'string' ? state.managementKey.trim() : '';
  } catch (_) {
    return '';
  }
}

// Plugin iframes are recreated on menu switches, but the same-origin management
// window survives. Its memory is the session boundary; no password is persisted.
function getManagementSession() {
  let owner = window;
  try {
    if (window.top !== window && window.top.location.origin === window.location.origin
        && window.top.location.pathname === '/management.html') owner = window.top;
  } catch (_) {}
  const name = '__cpaBillingManagementAuthSession';
  if (!owner[name]) {
    const session = {managementKey: '', shared: owner !== window};
    Object.defineProperty(owner, name, {value: session, configurable: true});
    const clear = () => { session.managementKey = ''; };
    owner.addEventListener('unauthorized', clear);
    owner.addEventListener('hashchange', () => {
      if (/^#\/login(?:[/?]|$)/.test(owner.location.hash || '')) clear();
    });
  }
  return owner[name];
}

const ENFORCE_MANAGEMENT_AUTH = window.location.pathname.startsWith('/v0/resource/plugins/');
const MANAGEMENT_SESSION = getManagementSession();
let MANAGEMENT_KEY = MANAGEMENT_SESSION.managementKey || readManagementKey();
MANAGEMENT_SESSION.managementKey = MANAGEMENT_KEY;
const authHeaders = () => {
  MANAGEMENT_KEY = MANAGEMENT_SESSION.managementKey;
  return MANAGEMENT_KEY ? {Authorization:'Bearer '+MANAGEMENT_KEY} : {};
};

let managementLoginPromise = null;
const authText = value => typeof window.cpaTranslate === 'function' ? window.cpaTranslate(value) : value;

function managementLogin() {
  if (managementLoginPromise) return managementLoginPromise;
  managementLoginPromise = new Promise(resolve => {
    const dialog = document.createElement('dialog');
    dialog.className = 'cpa-auth-dialog';
    dialog.setAttribute('aria-labelledby', 'cpaLoginTitle');
    dialog.setAttribute('aria-describedby', 'cpaLoginDescription');
    dialog.innerHTML = `
      <div class="cpa-auth-brand"><span class="cpa-auth-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><rect x="5" y="10" width="14" height="11" rx="3"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/><path d="M12 14v3"/></svg></span><span>CPA Billing</span></div>
      <h2 id="cpaLoginTitle">登录账单管理</h2>
      <p id="cpaLoginDescription" class="cpa-auth-description">输入 CLIProxyAPI 管理密码，继续查看和管理账单。</p>
      <form class="cpa-auth-form" novalidate>
        <label for="cpaLoginPassword">管理密码</label>
        <div class="cpa-auth-password">
          <input id="cpaLoginPassword" type="password" autocomplete="current-password" placeholder="请输入管理密码" aria-describedby="cpaLoginError" required autofocus>
          <button class="cpa-auth-visibility" type="button" aria-label="显示密码" aria-pressed="false"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/></svg></button>
        </div>
        <p id="cpaLoginError" class="cpa-auth-error" role="alert"></p>
        <button class="btn primary cpa-auth-submit" type="submit"><span class="cpa-auth-spinner" aria-hidden="true"></span><span class="cpa-auth-submit-label">登录并继续</span><span class="cpa-auth-arrow" aria-hidden="true">→</span></button>
      </form>
      <p class="cpa-auth-note">切换菜单无需重复登录，刷新管理中心后需重新输入。</p>`;
    document.body.appendChild(dialog);
    if (!MANAGEMENT_SESSION.shared) dialog.querySelector('.cpa-auth-note').textContent = authText('密码仅在当前页面有效，刷新后需重新输入。');
    const form = dialog.querySelector('.cpa-auth-form');
    const input = dialog.querySelector('input');
    const error = dialog.querySelector('.cpa-auth-error');
    const submit = dialog.querySelector('.cpa-auth-submit');
    const submitLabel = dialog.querySelector('.cpa-auth-submit-label');
    const visibility = dialog.querySelector('.cpa-auth-visibility');
    let submitting = false;
    dialog.addEventListener('cancel', event => event.preventDefault());
    visibility.addEventListener('click', () => {
      const visible = input.type === 'password';
      input.type = visible ? 'text' : 'password';
      visibility.setAttribute('aria-pressed', String(visible));
      visibility.setAttribute('aria-label', authText(visible ? '隐藏密码' : '显示密码'));
    });
    const showError = message => {
      error.textContent = authText(message);
      input.setAttribute('aria-invalid', 'true');
    };
    input.addEventListener('input', () => {
      error.textContent = '';
      input.removeAttribute('aria-invalid');
    });
    form.addEventListener('submit', async event => {
      event.preventDefault();
      if (submitting) return;
      const key = input.value.trim();
      if (!key) {
        showError('请输入管理密码');
        input.focus();
        return;
      }
      submitting = true;
      submit.disabled = true;
      input.readOnly = true;
      visibility.disabled = true;
      form.setAttribute('aria-busy', 'true');
      submitLabel.textContent = authText('正在验证…');
      error.textContent = '';
      input.removeAttribute('aria-invalid');
      try {
        const response = await fetch('/v0/management/debug', {
          method: 'GET', credentials: 'same-origin', cache: 'no-store',
          signal: AbortSignal.timeout(15000),
          headers: {Authorization: 'Bearer ' + key},
        });
        if (!response.ok) {
          showError(response.status === 401 ? '管理密码错误，请重试'
            : response.status === 403 ? '访问受限，请检查管理权限或稍后重试' : '登录失败，请稍后重试');
          return;
        }
        MANAGEMENT_KEY = key;
        MANAGEMENT_SESSION.managementKey = key;
        input.value = '';
        dialog.close();
        dialog.remove();
        managementLoginPromise = null;
        resolve(true);
      } catch (_) {
        showError('无法连接 CLIProxyAPI，请检查服务状态');
      } finally {
        submitting = false;
        submit.disabled = false;
        input.readOnly = false;
        visibility.disabled = false;
        form.setAttribute('aria-busy', 'false');
        submitLabel.textContent = authText('登录并继续');
        if (dialog.open) input.focus();
      }
    });
    dialog.showModal();
  });
  return managementLoginPromise;
}

async function requireManagementKey(forcePrompt = false) {
  MANAGEMENT_KEY = MANAGEMENT_SESSION.managementKey;
  if (MANAGEMENT_KEY) return true;
  if (!ENFORCE_MANAGEMENT_AUTH && !forcePrompt) return true;
  return managementLogin();
}

// Retry only rejected authentication requests. A late 401 from an old key must
// not invalidate a newer login shared by another request or plugin iframe.
async function managementFetch(url, options = {}, shouldContinue = () => true) {
  await requireManagementKey();
  if (!shouldContinue()) return null;
  const request = () => fetch(url, Object.assign({credentials: 'same-origin'}, options, {
    headers: Object.assign({}, options.headers || {}, authHeaders()),
  }));
  const rejectedKey = MANAGEMENT_SESSION.managementKey;
  let response = await request();
  if (response.status !== 401 || !shouldContinue()) return response;
  if (MANAGEMENT_SESSION.managementKey === rejectedKey) MANAGEMENT_SESSION.managementKey = '';
  await requireManagementKey(true);
  if (!shouldContinue()) return null;
  const retryKey = MANAGEMENT_SESSION.managementKey;
  response = await request();
  if (response.status === 401 && MANAGEMENT_SESSION.managementKey === retryKey) {
    MANAGEMENT_SESSION.managementKey = '';
  }
  return response;
}

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

window.showConfirmDialog = function showConfirmDialog(options) {
  if (typeof options === 'string') {
    options = { message: options };
  }
  const opts = Object.assign({
    title: '确认删除',
    message: '确定要执行此操作吗？',
    target: '',
    detail: '',
    confirmText: '删除',
    cancelText: '取消',
    danger: true,
  }, options || {});

  const tr = (typeof window.cpaTranslate === 'function') ? window.cpaTranslate : (v => v);
  const title = tr(opts.title);
  const message = tr(opts.message);
  const detail = opts.detail ? tr(opts.detail) : '';
  const confirmText = tr(opts.confirmText);
  const cancelText = tr(opts.cancelText);
  const target = opts.target || '';
  const isDanger = opts.danger !== false;

  return new Promise(resolve => {
    const existing = document.getElementById('cpaConfirmModal');
    if (existing) existing.remove();

    const backdrop = document.createElement('div');
    backdrop.id = 'cpaConfirmModal';
    backdrop.className = 'cpa-modal-backdrop';
    backdrop.setAttribute('role', 'dialog');
    backdrop.setAttribute('aria-modal', 'true');
    backdrop.setAttribute('aria-labelledby', 'cpaConfirmTitle');

    const escape = value => String(value ?? '').replace(
      /[&<>"']/g,
      c => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[c])
    );

    const iconHtml = isDanger
      ? '<div class="cpa-modal-icon-wrap danger"><svg class="cpa-modal-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg></div>'
      : '<div class="cpa-modal-icon-wrap info"><svg class="cpa-modal-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="12" x2="12" y2="16"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg></div>';

    const targetHtml = target ? ('<div class="cpa-modal-target">' + escape(target) + '</div>') : '';
    const detailHtml = detail ? ('<div class="cpa-modal-detail">' + escape(detail) + '</div>') : '';

    backdrop.innerHTML = `
      <div class="cpa-modal-card">
        <div class="cpa-modal-header">
          ${iconHtml}
          <div class="cpa-modal-title-wrap">
            <h3 class="cpa-modal-title" id="cpaConfirmTitle">${escape(title)}</h3>
            <p class="cpa-modal-desc">${escape(message)}</p>
          </div>
          <button type="button" class="cpa-modal-close" aria-label="关闭">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="18" y1="6" x2="6" y2="18"/>
              <line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </button>
        </div>
        ${targetHtml || detailHtml ? `<div class="cpa-modal-body">${targetHtml}${detailHtml}</div>` : ''}
        <div class="cpa-modal-actions">
          <button type="button" class="btn secondary cpa-modal-btn-cancel">${escape(cancelText)}</button>
          <button type="button" class="btn ${isDanger ? 'danger' : 'primary'} cpa-modal-btn-confirm">${escape(confirmText)}</button>
        </div>
      </div>
    `;

    document.body.appendChild(backdrop);
    requestAnimationFrame(() => {
      backdrop.classList.add('active');
    });

    const previousActive = document.activeElement;
    const cancelBtn = backdrop.querySelector('.cpa-modal-btn-cancel');
    const confirmBtn = backdrop.querySelector('.cpa-modal-btn-confirm');
    const closeBtn = backdrop.querySelector('.cpa-modal-close');

    const originalOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';

    if (cancelBtn) cancelBtn.focus();

    let resolved = false;
    const finish = result => {
      if (resolved) return;
      resolved = true;
      backdrop.classList.remove('active');
      document.removeEventListener('keydown', onKeyDown);
      setTimeout(() => {
        backdrop.remove();
        document.body.style.overflow = originalOverflow;
        if (previousActive && typeof previousActive.focus === 'function') {
          previousActive.focus();
        }
        resolve(result);
      }, 180);
    };

    const onKeyDown = e => {
      if (e.key === 'Escape') {
        e.preventDefault();
        finish(false);
      } else if (e.key === 'Tab') {
        const focusable = [cancelBtn, confirmBtn, closeBtn].filter(Boolean);
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    };

    document.addEventListener('keydown', onKeyDown);
    cancelBtn.addEventListener('click', () => finish(false));
    closeBtn.addEventListener('click', () => finish(false));
    confirmBtn.addEventListener('click', () => finish(true));
    backdrop.addEventListener('click', e => {
      if (e.target === backdrop) finish(false);
    });
  });
};
