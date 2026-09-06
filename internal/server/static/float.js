(function() {
  if (window.__MAEK_FLOAT_INIT__) return;
  window.__MAEK_FLOAT_INIT__ = true;

  const currentScript = document.currentScript || document.querySelector('script[src*="/_maek/float.js"]');
  const serviceId = currentScript ? (currentScript.getAttribute('data-id') || '') : '';
  const serviceName = currentScript ? (currentScript.getAttribute('data-name') || 'maek') : 'maek';

  const STORAGE_KEY = 'maek_float_pos';
  const CORNER_ORDER = ['bottom-right', 'bottom-left', 'top-left', 'top-right'];
  const POSITIONS = {
    'bottom-right': { bottom: '20px', right: '20px', top: 'auto', left: 'auto' },
    'bottom-left':  { bottom: '20px', left: '20px', top: 'auto', right: 'auto' },
    'top-left':     { top: '20px', left: '20px', bottom: 'auto', right: 'auto' },
    'top-right':    { top: '20px', right: '20px', bottom: 'auto', left: 'auto' }
  };

  function getStoredPosition() {
    try {
      const val = localStorage.getItem(STORAGE_KEY);
      if (val && POSITIONS[val]) return val;
    } catch (e) {}
    return 'bottom-right';
  }

  let currentPos = getStoredPosition();

  const host = document.createElement('div');
  host.id = 'maek-float-root';
  host.style.position = 'fixed';
  host.style.zIndex = '2147483647';
  host.style.pointerEvents = 'auto';
  host.style.fontFamily = '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif';

  function applyPosition(pos, animate) {
    if (!POSITIONS[pos]) pos = 'bottom-right';
    currentPos = pos;
    try {
      localStorage.setItem(STORAGE_KEY, pos);
    } catch (e) {}

    if (!animate) {
      host.style.transition = 'none';
    } else {
      host.style.transition = 'top 0.22s cubic-bezier(0.16, 1, 0.3, 1), bottom 0.22s cubic-bezier(0.16, 1, 0.3, 1), left 0.22s cubic-bezier(0.16, 1, 0.3, 1), right 0.22s cubic-bezier(0.16, 1, 0.3, 1)';
    }

    const p = POSITIONS[pos];
    host.style.top = p.top;
    host.style.bottom = p.bottom;
    host.style.left = p.left;
    host.style.right = p.right;
  }

  applyPosition(currentPos, false);

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
      gap: 7px;
      padding: 6px 12px;
      background: rgba(255, 255, 255, 0.94);
      backdrop-filter: blur(16px);
      -webkit-backdrop-filter: blur(16px);
      border: 1px solid rgba(15, 20, 25, 0.15);
      border-radius: 9999px;
      color: #0f1419;
      font-size: 13px;
      font-weight: 500;
      box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08), 0 1px 3px rgba(0, 0, 0, 0.04);
      user-select: none;
      -webkit-user-select: none;
      cursor: grab;
      transition: background 0.15s ease, border-color 0.15s ease, box-shadow 0.15s ease, transform 0.15s ease;
    }
    .badge:hover {
      background: #ffffff;
      border-color: rgba(15, 20, 25, 0.25);
      box-shadow: 0 6px 20px rgba(0, 0, 0, 0.12);
    }
    .badge.dragging {
      cursor: grabbing;
      box-shadow: 0 12px 28px rgba(0, 0, 0, 0.18);
      transform: scale(1.02);
      border-color: #1d9bf0;
    }
    .drag-handle {
      display: flex;
      align-items: center;
      color: #8b98a5;
      cursor: grab;
      margin-right: -2px;
    }
    .badge.dragging .drag-handle {
      cursor: grabbing;
    }
    .status-dot {
      width: 7px;
      height: 7px;
      border-radius: 50%;
      background-color: #16a34a;
      box-shadow: 0 0 0 2px rgba(22, 163, 74, 0.2);
      flex-shrink: 0;
    }
    .brand {
      color: #536471;
      font-weight: 700;
      font-size: 12px;
      letter-spacing: 0.3px;
    }
    .svc-name {
      color: #0f1419;
      font-weight: 700;
      font-size: 13px;
      max-width: 140px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .svc-id {
      color: #536471;
      font-size: 12px;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    }
    .pos-btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 22px;
      height: 22px;
      border-radius: 50%;
      border: 1px solid #cfd9de;
      background: #f7f9f9;
      color: #536471;
      cursor: pointer;
      outline: none;
      padding: 0;
      margin-left: 2px;
      transition: all 0.15s ease;
    }
    .pos-btn:hover {
      background: #eff3f4;
      color: #0f1419;
      border-color: #8b98a5;
    }
    .pos-btn svg {
      width: 12px;
      height: 12px;
    }
    .exit-btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      margin-left: 3px;
      padding: 3px 9px;
      background: #fee2e2;
      color: #dc2626;
      border: 1px solid #fecaca;
      border-radius: 9999px;
      font-size: 11px;
      font-weight: 700;
      text-decoration: none;
      cursor: pointer;
      white-space: nowrap;
      transition: all 0.15s ease;
    }
    .exit-btn:hover {
      background: #fecaca;
      color: #b91c1c;
      border-color: #f87171;
    }
  `;

  const badge = document.createElement('div');
  badge.className = 'badge';

  // Drag handle icon
  const dragHandle = document.createElement('span');
  dragHandle.className = 'drag-handle';
  dragHandle.title = 'Drag to move corner';
  dragHandle.innerHTML = `
    <svg width="10" height="14" viewBox="0 0 10 14" fill="currentColor">
      <circle cx="2" cy="2" r="1.2"/>
      <circle cx="8" cy="2" r="1.2"/>
      <circle cx="2" cy="7" r="1.2"/>
      <circle cx="8" cy="7" r="1.2"/>
      <circle cx="2" cy="12" r="1.2"/>
      <circle cx="8" cy="12" r="1.2"/>
    </svg>
  `;

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
  idSpan.textContent = serviceId ? `@${serviceId}` : '';

  // Position switcher button (cycles corners on click)
  const posBtn = document.createElement('button');
  posBtn.type = 'button';
  posBtn.className = 'pos-btn';
  posBtn.title = 'Cycle position (Bottom-Right -> Bottom-Left -> Top-Left -> Top-Right)';
  posBtn.innerHTML = `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
      <path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/>
    </svg>
  `;

  posBtn.addEventListener('click', function(e) {
    e.stopPropagation();
    e.preventDefault();
    const idx = CORNER_ORDER.indexOf(currentPos);
    const nextPos = CORNER_ORDER[(idx + 1) % CORNER_ORDER.length];
    applyPosition(nextPos, true);
  });

  const exitLink = document.createElement('a');
  exitLink.className = 'exit-btn';
  exitLink.href = '/_maek';
  exitLink.textContent = 'Exit ✕';
  exitLink.title = 'Disconnect and return to maek catalog';

  badge.appendChild(dragHandle);
  badge.appendChild(dot);
  badge.appendChild(brand);
  badge.appendChild(nameSpan);
  if (serviceId) {
    badge.appendChild(idSpan);
  }
  badge.appendChild(posBtn);
  badge.appendChild(exitLink);

  shadow.appendChild(style);
  shadow.appendChild(badge);

  // Drag-and-drop to nearest corner
  let isDragging = false;
  let startX = 0;
  let startY = 0;
  let initialLeft = 0;
  let initialTop = 0;

  function onPointerDown(clientX, clientY, target) {
    if (target.closest('.exit-btn') || target.closest('.pos-btn')) {
      return;
    }
    isDragging = true;
    startX = clientX;
    startY = clientY;

    const rect = host.getBoundingClientRect();
    initialLeft = rect.left;
    initialTop = rect.top;

    host.style.transition = 'none';
    host.style.top = initialTop + 'px';
    host.style.left = initialLeft + 'px';
    host.style.bottom = 'auto';
    host.style.right = 'auto';

    badge.classList.add('dragging');
  }

  function onPointerMove(clientX, clientY) {
    if (!isDragging) return;
    const dx = clientX - startX;
    const dy = clientY - startY;
    host.style.left = (initialLeft + dx) + 'px';
    host.style.top = (initialTop + dy) + 'px';
  }

  function onPointerUp() {
    if (!isDragging) return;
    isDragging = false;
    badge.classList.remove('dragging');

    const rect = host.getBoundingClientRect();
    const centerX = rect.left + rect.width / 2;
    const centerY = rect.top + rect.height / 2;
    const midX = window.innerWidth / 2;
    const midY = window.innerHeight / 2;

    let targetCorner = 'bottom-right';
    if (centerY < midY) {
      targetCorner = (centerX < midX) ? 'top-left' : 'top-right';
    } else {
      targetCorner = (centerX < midX) ? 'bottom-left' : 'bottom-right';
    }

    applyPosition(targetCorner, true);
  }

  badge.addEventListener('mousedown', function(e) {
    onPointerDown(e.clientX, e.clientY, e.target);

    function onMouseMove(e) {
      onPointerMove(e.clientX, e.clientY);
    }

    function onMouseUp() {
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
      onPointerUp();
    }

    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  });

  badge.addEventListener('touchstart', function(e) {
    if (e.touches.length === 1) {
      onPointerDown(e.touches[0].clientX, e.touches[0].clientY, e.target);
    }
  }, { passive: true });

  badge.addEventListener('touchmove', function(e) {
    if (isDragging && e.touches.length === 1) {
      onPointerMove(e.touches[0].clientX, e.touches[0].clientY);
    }
  }, { passive: true });

  badge.addEventListener('touchend', function() {
    onPointerUp();
  });

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
