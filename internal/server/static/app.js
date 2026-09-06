    const AVATAR_COLORS = [
      '#0f1419', '#1d9bf0', '#00ba7c', '#7856ff', '#f91880', '#ff7a00', '#00c7e2'
    ];

    function pickColor(str) {
      let hash = 0;
      for (let i = 0; i < str.length; i++) {
        hash = str.charCodeAt(i) + ((hash << 5) - hash);
      }
      return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length];
    }

    function timeAgo(dateString) {
      const now = new Date();
      const past = new Date(dateString);
      const diffSec = Math.floor((now - past) / 1000);
      if (diffSec < 5) return 'just now';
      if (diffSec < 60) return `${diffSec}s`;
      const diffMin = Math.floor(diffSec / 60);
      if (diffMin < 60) return `${diffMin}m`;
      const diffHour = Math.floor(diffMin / 60);
      if (diffHour < 24) return `${diffHour}h`;
      return `${Math.floor(diffHour / 24)}d`;
    }

    function escapeHtml(str) {
      if (!str) return '';
      return str.replace(/[&<>"']/g, m => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
      })[m]);
    }

    // OS Detection & Installer Tab Handler
    let currentOS = 'macos';

    function detectOS() {
      const ua = (navigator.userAgent || '').toLowerCase();
      if (ua.includes('mac') || ua.includes('darwin')) return 'macos';
      if (ua.includes('win')) return 'windows';
      if (ua.includes('linux') || ua.includes('x11')) return 'linux';
      return 'macos';
    }

    function selectOSTab(os) {
      currentOS = os;
      ['macos', 'linux', 'windows', 'go'].forEach(t => {
        const el = document.getElementById(`tab-${t}`);
        if (el) el.classList.toggle('active', t === os);
      });

      const origin = window.location.origin || `${window.location.protocol}//${window.location.host}`;
      const cmdBox = document.getElementById('install-cmd-output');
      const linksBox = document.getElementById('install-direct-links');

      if (os === 'macos') {
        cmdBox.textContent = `curl -fsSL ${origin}/_maek/install.sh | sh`;
        linksBox.innerHTML = `Direct download: <a href="${origin}/_maek/download?os=Darwin&arch=arm64">Apple Silicon (arm64)</a> · <a href="${origin}/_maek/download?os=Darwin&arch=x86_64">Intel (x86_64)</a>`;
      } else if (os === 'linux') {
        cmdBox.textContent = `curl -fsSL ${origin}/_maek/install.sh | sh`;
        linksBox.innerHTML = `Direct download: <a href="${origin}/_maek/download?os=Linux&arch=x86_64">x86_64</a> · <a href="${origin}/_maek/download?os=Linux&arch=arm64">arm64</a>`;
      } else if (os === 'windows') {
        cmdBox.textContent = `irm ${origin}/_maek/install.ps1 | iex`;
        linksBox.innerHTML = `Direct download: <a href="${origin}/_maek/download?os=Windows&arch=x86_64">Windows x86_64 (.zip)</a>`;
      } else if (os === 'go') {
        cmdBox.textContent = `go install github.com/gosuda/maek/cmd/maek@latest`;
        linksBox.innerHTML = `Requires Go 1.22+ · <a href="https://github.com/gosuda/maek/releases" target="_blank" rel="noopener">GitHub Releases &rarr;</a>`;
      }
    }

    function copyInstallCmd() {
      const text = document.getElementById('install-cmd-output').textContent;
      navigator.clipboard.writeText(text).then(() => {
        const btn = document.getElementById('install-copy-btn');
        btn.textContent = 'Copied!';
        setTimeout(() => { btn.textContent = 'Copy'; }, 2000);
      });
    }

    // Interactive Command Builder
    function updateCommand() {
      const host = window.location.host || '127.0.0.1:8080';
      const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const name = document.getElementById('param-name').value.trim();
      const target = document.getElementById('param-target').value.trim() || 'http://localhost:3000';
      const id = document.getElementById('param-id').value.trim();
      const desc = document.getElementById('param-desc').value.trim();

      let lines = [
        'maek agent \\',
        `  --server ${wsProto}//${host} \\`
      ];

      if (name) {
        lines.push(`  --name ${name} \\`);
      }
      if (id) {
        lines.push(`  --id ${id} \\`);
      }
      if (desc) {
        lines.push(`  --desc "${desc}" \\`);
      }
      lines.push(`  --target ${target}`);

      document.getElementById('cmd-output').textContent = lines.join('\n');
    }

    function copyAgentCmd() {
      const text = document.getElementById('cmd-output').textContent;
      navigator.clipboard.writeText(text).then(() => {
        const btn = document.getElementById('cmd-copy-btn');
        btn.textContent = 'Copied!';
        setTimeout(() => { btn.textContent = 'Copy'; }, 2000);
      });
    }

    function updateLLMsCmd() {
      document.getElementById('llms-cmd-output').textContent = `curl ${window.location.origin}/_maek/llms.txt`;
    }

    function copyLLMsCmd() {
      const text = document.getElementById('llms-cmd-output').textContent;
      navigator.clipboard.writeText(text).then(() => {
        const btn = document.getElementById('llms-copy-btn');
        btn.textContent = 'Copied!';
        setTimeout(() => { btn.textContent = 'Copy'; }, 2000);
      });
    }

    function copySnippet(btn, text) {
      navigator.clipboard.writeText(text).then(() => {
        const orig = btn.textContent;
        btn.textContent = 'Copied!';
        setTimeout(() => { btn.textContent = orig; }, 2000);
      });
    }

    // Per-service URL/cURL snippet tab state (survives periodic re-renders)
    const snippetTabs = {};

    function setSnippetTab(serviceId, tab) {
      snippetTabs[serviceId] = tab;
      const isUrl = tab === 'url';
      const urlBtn = document.getElementById(`stab-url-${serviceId}`);
      const curlBtn = document.getElementById(`stab-curl-${serviceId}`);
      const urlBox = document.getElementById(`sbox-url-${serviceId}`);
      const curlBox = document.getElementById(`sbox-curl-${serviceId}`);
      if (urlBtn) urlBtn.classList.toggle('active', isUrl);
      if (curlBtn) curlBtn.classList.toggle('active', !isUrl);
      if (urlBox) urlBox.style.display = isUrl ? 'flex' : 'none';
      if (curlBox) curlBox.style.display = isUrl ? 'none' : 'flex';
    }

    async function fetchServices() {
      try {
        const res = await fetch('/_maek/api/services');
        if (!res.ok) return;
        const services = await res.json();
        renderServices(services);
      } catch (e) {
        console.error('Failed to fetch services:', e);
      }
    }

    function renderServices(services) {
      const feed = document.getElementById('feed');
      const countLabel = document.getElementById('count-label');
      const host = window.location.host;

      const count = services ? services.length : 0;
      countLabel.textContent = count === 1 ? '1 tunnel online' : `${count} tunnels online`;

      if (!services || services.length === 0) {
        feed.innerHTML = `
          <div class="empty-feed">
            No active tunnels connected yet.<br>
            Run the command in the quick start guide below to connect a service.
          </div>
        `;
        return;
      }

      feed.innerHTML = services.map(svc => {
        const initial = (svc.name || 'A').charAt(0).toUpperCase();
        const color = pickColor(svc.name || 'A');
        const hasThumb = Boolean(svc.thumbnail && svc.thumbnail.trim());
        const hasDesc = Boolean(svc.description && svc.description.trim());
        const directUrl = `${window.location.protocol}//${host}/_maek/${encodeURIComponent(svc.name || svc.id)}`;
        const curlCmd = `curl -H "X-Maek-Service: ${svc.id}" ${window.location.protocol}//${host}/`;
        const tab = snippetTabs[svc.id] || 'url';
        const isUrl = tab === 'url';

        const avatarHtml = hasThumb 
          ? `<img src="${escapeHtml(svc.thumbnail)}" alt="${escapeHtml(svc.name)}" onerror="this.parentElement.innerHTML='${initial}'" />`
          : initial;

        const mediaBoxHtml = hasThumb ? `
          <div class="media-card">
            <img src="${escapeHtml(svc.thumbnail)}" alt="Thumbnail" />
          </div>
        ` : '';

        return `
          <div class="service-item">
            <div class="avatar" style="background-color: ${color}">
              ${avatarHtml}
            </div>
            <div class="service-body">
              <div class="service-top">
                <div class="service-identity">
                  <div class="name-line">
                    <span class="service-name">${escapeHtml(svc.name || svc.id)}</span>
                    <span class="dot">&middot;</span>
                    <span class="service-time">${timeAgo(svc.connected_at)}</span>
                  </div>
                </div>
                <a class="connect-button" href="/_maek/${encodeURIComponent(svc.name || svc.id)}">Connect &rarr;</a>
              </div>

              ${hasDesc ? `<div class="service-desc">${escapeHtml(svc.description)}</div>` : ''}
              ${mediaBoxHtml}

              <!-- URL / cURL access snippets with tabs -->
              <div class="snippet-tabs">
                <button id="stab-url-${svc.id}" class="snippet-tab ${isUrl ? 'active' : ''}" onclick="setSnippetTab('${svc.id}', 'url')">URL</button>
                <button id="stab-curl-${svc.id}" class="snippet-tab ${isUrl ? '' : 'active'}" onclick="setSnippetTab('${svc.id}', 'curl')">cURL</button>
              </div>

              <div class="snippets">
                <div id="sbox-url-${svc.id}" class="curl-snippet" style="display:${isUrl ? 'flex' : 'none'};">
                  <code class="curl-code"><a href="${escapeHtml(directUrl)}" target="_blank" rel="noopener">${escapeHtml(directUrl)}</a></code>
                  <button class="curl-copy-btn" onclick="copySnippet(this, '${escapeHtml(directUrl)}')">Copy</button>
                </div>
                <div id="sbox-curl-${svc.id}" class="curl-snippet" style="display:${isUrl ? 'none' : 'flex'};">
                  <code class="curl-code">${escapeHtml(curlCmd)}</code>
                  <button class="curl-copy-btn" onclick="copySnippet(this, '${escapeHtml(curlCmd)}')">Copy</button>
                </div>
              </div>
            </div>
          </div>
        `;
      }).join('');
    }

    async function fetchVersion() {
      try {
        const res = await fetch('/_maek/version');
        if (!res.ok) return;
        const v = await res.json();
        const tag = document.getElementById('version-tag');
        const footer = document.getElementById('footer-version');
        if (tag && v.version) {
          tag.textContent = `v${v.version}`;
          tag.title = `Version: ${v.version}\nCommit: ${v.commit}\nBuild Date: ${v.date}`;
        }
        if (footer && v.version) {
          const shortCommit = v.commit && v.commit !== 'none' ? ` (${v.commit.substring(0, 7)})` : '';
          footer.textContent = `maek v${v.version}${shortCommit} · ${v.date}`;
        }
      } catch (e) {
        console.error('Failed to fetch version:', e);
      }
    }

    // Initialize OS tab, command, version, and polling
    selectOSTab(detectOS());
    updateCommand();
    updateLLMsCmd();
    fetchVersion();
    fetchServices();
    setInterval(fetchServices, 2500);
