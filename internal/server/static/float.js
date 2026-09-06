(function() {
  if (window.__MAEK_FLOAT_INIT__) return;
  window.__MAEK_FLOAT_INIT__ = true;

  const currentScript = document.currentScript || document.querySelector('script[src*="/_maek/float.js"]');
  const serviceId = currentScript ? (currentScript.getAttribute('data-id') || '') : '';
  const serviceName = currentScript ? (currentScript.getAttribute('data-name') || 'maek') : 'maek';

  const host = document.createElement('div');
  host.id = 'maek-float-root';
  host.style.position = 'fixed';
  host.style.bottom = '20px';
  host.style.right = '20px';
  host.style.zIndex = '2147483647';
  host.style.pointerEvents = 'auto';
  host.style.fontFamily = '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif';

  const shadow = host.attachShadow({ mode: 'open' });

  const style = document.createElement('style');
  style.textContent = `
    * {
      box-sizing: border-box;
      margin: 0;
      padding: 0;
    }
    .badge {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      padding: 7px 12px;
      background: rgba(18, 18, 24, 0.88);
      backdrop-filter: blur(12px);
      -webkit-backdrop-filter: blur(12px);
      border: 1px solid rgba(255, 255, 255, 0.16);
      border-radius: 9999px;
      color: #f1f5f9;
      font-size: 13px;
      font-weight: 500;
      box-shadow: 0 4px 20px rgba(0, 0, 0, 0.35);
      user-select: none;
      transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
    }
    .badge:hover {
      background: rgba(28, 28, 38, 0.95);
      border-color: rgba(255, 255, 255, 0.28);
      box-shadow: 0 6px 24px rgba(0, 0, 0, 0.45);
      transform: translateY(-1px);
    }
    .status-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background-color: #22c55e;
      box-shadow: 0 0 8px #22c55e;
      flex-shrink: 0;
    }
    .brand {
      color: #94a3b8;
      font-weight: 600;
      letter-spacing: 0.5px;
    }
    .svc-name {
      color: #38bdf8;
      font-weight: 600;
      max-width: 140px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .svc-id {
      color: #64748b;
      font-size: 11px;
      font-family: monospace;
    }
    .exit-btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      margin-left: 4px;
      padding: 3px 8px;
      background: rgba(239, 68, 68, 0.15);
      color: #f87171;
      border: 1px solid rgba(239, 68, 68, 0.3);
      border-radius: 6px;
      font-size: 11px;
      font-weight: 600;
      text-decoration: none;
      cursor: pointer;
      transition: all 0.15s ease;
    }
    .exit-btn:hover {
      background: rgba(239, 68, 68, 0.3);
      color: #fca5a5;
      border-color: rgba(239, 68, 68, 0.5);
    }
  `;

  const badge = document.createElement('div');
  badge.className = 'badge';

  const dot = document.createElement('span');
  dot.className = 'status-dot';

  const brand = document.createElement('span');
  brand.className = 'brand';
  brand.textContent = 'maek:';

  const nameSpan = document.createElement('span');
  nameSpan.className = 'svc-name';
  nameSpan.textContent = serviceName;
  nameSpan.title = serviceName + (serviceId ? ' (' + serviceId + ')' : '');

  const idSpan = document.createElement('span');
  idSpan.className = 'svc-id';
  idSpan.textContent = serviceId ? `[${serviceId}]` : '';

  const exitLink = document.createElement('a');
  exitLink.className = 'exit-btn';
  exitLink.href = '/_maek';
  exitLink.textContent = 'Exit ✕';
  exitLink.title = 'Disconnect and return to maek catalog';

  badge.appendChild(dot);
  badge.appendChild(brand);
  badge.appendChild(nameSpan);
  if (serviceId) {
    badge.appendChild(idSpan);
  }
  badge.appendChild(exitLink);

  shadow.appendChild(style);
  shadow.appendChild(badge);

  function mount() {
    if (!document.body) {
      document.addEventListener('DOMContentLoaded', mount);
      return;
    }
    document.body.appendChild(host);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', mount);
  } else {
    mount();
  }
})();
