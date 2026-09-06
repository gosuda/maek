(function() {
  if (window.__MAEK_FLOAT_INIT__) return;
  window.__MAEK_FLOAT_INIT__ = true;

  const currentScript = document.currentScript || document.querySelector('script[src*="/_maek/float.js"]');
  const serviceId = currentScript ? (currentScript.getAttribute('data-id') || '') : '';
  const serviceName = currentScript ? (currentScript.getAttribute('data-name') || 'app') : 'app';

  const STORAGE_POS_KEY = 'maek_float_pos';
  const STORAGE_MODE_KEY = 'maek_float_mode';

  const POSITIONS = {
    'bottom-right': { bottom: '20px', right: '20px', top: 'auto', left: 'auto' },
    'bottom-left':  { bottom: '20px', left: '20px', top: 'auto', right: 'auto' },
    'top-left':     { top: '20px', left: '20px', bottom: 'auto', right: 'auto' },
    'top-right':    { top: '20px', right: '20px', bottom: 'auto', left: 'auto' }
  };

  function getStoredPosition() {
    try {
      const val = localStorage.getItem(STORAGE_POS_KEY);
      if (val && POSITIONS[val]) return val;
    } catch (e) {}
    return 'bottom-right';
  }

  function getStoredMode() {
    try {
      const val = localStorage.getItem(STORAGE_MODE_KEY);
      if (val === 'compact' || val === 'full') return val;
    } catch (e) {}
    return 'full';
  }

  let currentPos = getStoredPosition();
  let currentMode = getStoredMode();

  const host = document.createElement('div');
  host.id = 'maek-float-root';
  host.style.position = 'fixed';
  host.style.zIndex = '2147483647';
  host.style.pointerEvents = 'auto';
  host.style.fontFamily = '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif';
  host.style.lineHeight = 'normal';

  function applyPosition(pos, animate) {
    if (!POSITIONS[pos]) pos = 'bottom-right';
    currentPos = pos;
    try {
      localStorage.setItem(STORAGE_POS_KEY, pos);
    } catch (e) {}

    host.style.transition = animate
      ? 'top 0.22s cubic-bezier(0.16, 1, 0.3, 1), bottom 0.22s cubic-bezier(0.16, 1, 0.3, 1), left 0.22s cubic-bezier(0.16, 1, 0.3, 1), right 0.22s cubic-bezier(0.16, 1, 0.3, 1)'
      : 'none';

    const p = POSITIONS[pos];
    host.style.top = p.top;
    host.style.bottom = p.bottom;
    host.style.left = p.left;
    host.style.right = p.right;
    host.style.transform = 'none';
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
      padding: 6px 11px;
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
      overflow: hidden;
      transition: background 0.15s ease, border-color 0.15s ease, box-shadow 0.15s ease, padding 0.2s cubic-bezier(0.16, 1, 0.3, 1), gap 0.2s cubic-bezier(0.16, 1, 0.3, 1);
    }
    .badge:hover {
      background: #ffffff;
      border-color: rgba(15, 20, 25, 0.25);
      box-shadow: 0 6px 20px rgba(0, 0, 0, 0.12);
    }
    .badge.dragging {
      cursor: grabbing;
      box-shadow: 0 12px 28px rgba(0, 0, 0, 0.18);
      border-color: #1d9bf0;
    }

    .status-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background-color: #16a34a;
      box-shadow: 0 0 0 2px rgba(22, 163, 74, 0.2);
      flex-shrink: 0;
      cursor: pointer;
      transition: transform 0.15s ease;
    }
    .status-dot:hover {
      transform: scale(1.2);
    }

    .svc-name {
      color: #0f1419;
      font-weight: 700;
      font-size: 13px;
      max-width: 180px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      line-height: 1;
      opacity: 1;
      transition: max-width 0.22s cubic-bezier(0.16, 1, 0.3, 1), opacity 0.15s ease;
    }

    .toggle-btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 20px;
      height: 20px;
      max-width: 20px;
      border-radius: 50%;
      border: 1px solid #cfd9de;
      background: #f7f9f9;
      color: #536471;
      cursor: pointer;
      outline: none;
      padding: 0;
      margin-left: 1px;
      overflow: hidden;
      flex-shrink: 0;
      opacity: 1;
      transition: max-width 0.22s cubic-bezier(0.16, 1, 0.3, 1), opacity 0.15s ease, margin 0.2s ease, background 0.15s ease, border-color 0.15s ease;
    }
    .toggle-btn:hover {
      background: #eff3f4;
      color: #0f1419;
      border-color: #8b98a5;
    }
    .toggle-btn svg {
      width: 11px;
      height: 11px;
      flex-shrink: 0;
    }

    .exit-btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 20px;
      height: 20px;
      background: #fee2e2;
      color: #dc2626;
      border: 1px solid #fecaca;
      border-radius: 50%;
      font-size: 11px;
      font-weight: 700;
      text-decoration: none;
      cursor: pointer;
      line-height: 1;
      margin-left: 1px;
      flex-shrink: 0;
      transition: all 0.15s ease;
    }
    .exit-btn:hover {
      background: #fecaca;
      color: #b91c1c;
      border-color: #f87171;
    }

    /* Compact mode styles */
    .badge.mode-compact {
      padding: 5px 8px;
      gap: 6px;
      cursor: pointer;
    }
    .badge.mode-compact .svc-name {
      max-width: 0;
      opacity: 0;
      pointer-events: none;
    }
    .badge.mode-compact .toggle-btn {
      max-width: 0;
      opacity: 0;
      margin: 0;
      border: none;
      pointer-events: none;
    }
  `;

  const badge = document.createElement('div');
  badge.className = 'badge mode-' + currentMode;

  const dot = document.createElement('span');
  dot.className = 'status-dot';

  const nameSpan = document.createElement('span');
  nameSpan.className = 'svc-name';
  nameSpan.textContent = serviceName;
  nameSpan.title = serviceName + (serviceId ? ` (@${serviceId})` : '');

  const toggleBtn = document.createElement('button');
  toggleBtn.type = 'button';
  toggleBtn.className = 'toggle-btn';
  toggleBtn.title = '컴팩트 모드로 전환';
  toggleBtn.innerHTML = `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
      <path d="M4 14h6v6M20 10h-6V4M14 10l7-7M3 21l7-7"/>
    </svg>
  `;

  const exitLink = document.createElement('a');
  exitLink.className = 'exit-btn';
  exitLink.href = '/_maek';
  exitLink.textContent = '✕';
  exitLink.title = 'Exit and return to maek catalog';

  function updateModeUI(mode) {
    currentMode = mode;
    try {
      localStorage.setItem(STORAGE_MODE_KEY, mode);
    } catch (e) {}

    if (mode === 'compact') {
      badge.classList.remove('mode-full');
      badge.classList.add('mode-compact');
      dot.title = `${serviceName} (클릭하여 펼치기)`;
      badge.title = `${serviceName} - 클릭하여 펼치기 (드래그하여 위치 이동)`;
    } else {
      badge.classList.remove('mode-compact');
      badge.classList.add('mode-full');
      dot.title = `${serviceName} (활성 상태)`;
      badge.title = '드래그하여 모서리로 이동';
    }

    // Re-verify that host is cleanly anchored to its corner
    applyPosition(currentPos, false);
  }

  updateModeUI(currentMode);

  toggleBtn.addEventListener('click', function(e) {
    e.stopPropagation();
    e.preventDefault();
    updateModeUI('compact');
  });

  dot.addEventListener('click', function(e) {
    e.stopPropagation();
    if (currentMode === 'compact') {
      updateModeUI('full');
    }
  });

  badge.appendChild(dot);
  badge.appendChild(nameSpan);
  badge.appendChild(toggleBtn);
  badge.appendChild(exitLink);

  shadow.appendChild(style);
  shadow.appendChild(badge);

  // Drag-and-drop snapping using transform (never breaks corner layout anchors)
  let isPointerDown = false;
  let isDragging = false;
  let didDrag = false;
  let startX = 0;
  let startY = 0;

  function onPointerDown(clientX, clientY, target) {
    if (target.closest('.exit-btn') || target.closest('.toggle-btn')) {
      return;
    }
    isPointerDown = true;
    didDrag = false;
    isDragging = false;
    startX = clientX;
    startY = clientY;
  }

  function onPointerMove(clientX, clientY) {
    if (!isPointerDown) return;
    const dx = clientX - startX;
    const dy = clientY - startY;

    if (!isDragging) {
      if (Math.hypot(dx, dy) > 4) {
        isDragging = true;
        didDrag = true;
        badge.classList.add('dragging');
        host.style.transition = 'none';
      }
    }

    if (isDragging) {
      host.style.transform = `translate3d(${dx}px, ${dy}px, 0)`;
    }
  }

  function onPointerUp() {
    if (!isPointerDown) return;
    isPointerDown = false;

    if (isDragging) {
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
    } else {
      // Clean click on badge in compact mode expands to full
      if (currentMode === 'compact') {
        updateModeUI('full');
      }
    }
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
    if (e.touches.length === 1) {
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
