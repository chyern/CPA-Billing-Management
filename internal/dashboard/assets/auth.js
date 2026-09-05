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
