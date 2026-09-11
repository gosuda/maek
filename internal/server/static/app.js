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
      } else {
        cmdBox.textContent = `go install github.com/gosuda/maek/cmd/maek@latest`;
        linksBox.innerHTML = `Requires Go 1.22+ · <a href="https://github.com/gosuda/maek/releases" target="_blank" rel="noopener">GitHub Releases &rarr;</a>`;
      }
      updateCommand();
    }

    function copyText(id, buttonId) {
      navigator.clipboard.writeText(document.getElementById(id).textContent).then(() => {
        const btn = document.getElementById(buttonId);
        btn.textContent = 'Copied!';
        setTimeout(() => { btn.textContent = 'Copy'; }, 2000);
      });
    }

    function copyInstallCmd() { copyText('install-cmd-output', 'install-copy-btn'); }
    function copyAgentCmd() { copyText('cmd-output', 'cmd-copy-btn'); }
    function copyLLMsCmd() { copyText('llms-cmd-output', 'llms-copy-btn'); }

    function shellQuotePosix(s) { return "'" + s.replace(/'/g, "'\\''") + "'"; }
    function shellQuotePs(s) { return "'" + s.replace(/'/g, "''") + "'"; }

    function updateCommand() {
      const host = window.location.host || '127.0.0.1:8080';
      const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const alias = document.getElementById('param-alias').value.trim();
      const target = document.getElementById('param-target').value.trim() || 'http://localhost:3000';
      const desc = document.getElementById('param-desc').value.trim();
      const effectiveOS = currentOS === 'go' ? detectOS() : currentOS;
      const isPowerShell = effectiveOS === 'windows';
      const q = isPowerShell ? shellQuotePs : shellQuotePosix;
      const cont = isPowerShell ? '`' : '\\';
      const lines = [`maek agent ${cont}`, `  --server ${wsProto}//${host} ${cont}`];
      if (alias) lines.push(`  --alias ${q(alias)} ${cont}`);
      if (desc) lines.push(`  --desc ${q(desc)} ${cont}`);
      lines.push(`  --target ${q(target)}`);
      document.getElementById('cmd-output').textContent = lines.join('\n');
    }

    function updateLLMsCmd() {
      document.getElementById('llms-cmd-output').textContent = `curl ${window.location.origin}/_maek/llms.txt`;
    }

    function copySnippet(btn, text) {
      navigator.clipboard.writeText(text).then(() => {
        const orig = btn.textContent;
        btn.textContent = 'Copied!';
        setTimeout(() => { btn.textContent = orig; }, 2000);
      });
    }

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
        renderServices(await res.json());
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
        feed.innerHTML = `<div class="empty-feed">No active tunnels connected yet.<br>Run the command in the quick start guide below to connect a service.</div>`;
        return;
      }

      const sorted = [...services].sort((a, b) => new Date(a.connected_at) - new Date(b.connected_at));
      feed.innerHTML = sorted.map(svc => {
        const alias = svc.alias || svc.id;
        const hasThumb = Boolean(svc.thumbnail && svc.thumbnail.trim());
        const hasDesc = Boolean(svc.description && svc.description.trim());
        const directUrl = `${window.location.protocol}//${host}/_maek/${encodeURIComponent(svc.id)}`;
        const curlCmd = `curl -H "X-Maek-Service: ${svc.id}" ${window.location.protocol}//${host}/`;
        const tab = snippetTabs[svc.id] || 'url';
        const isUrl = tab === 'url';
        const mediaBoxHtml = hasThumb ? `<div class="media-card"><img src="${escapeHtml(svc.thumbnail)}" alt="Thumbnail" /></div>` : '';
        return `
          <div class="service-item">
            <div class="service-body">
              <div class="service-top">
                <div class="service-identity"><div class="name-line"><span class="service-name">${escapeHtml(alias)}</span><span class="dot">&middot;</span><span class="service-time">${timeAgo(svc.connected_at)}</span></div></div>
                <a class="connect-button" href="/_maek/${encodeURIComponent(svc.id)}">Connect &rarr;</a>
              </div>
              ${hasDesc ? `<div class="service-desc">${escapeHtml(svc.description)}</div>` : ''}
              ${mediaBoxHtml}
              <div class="snippet-tabs">
                <button id="stab-url-${svc.id}" class="snippet-tab ${isUrl ? 'active' : ''}" onclick="setSnippetTab('${svc.id}', 'url')">URL</button>
                <button id="stab-curl-${svc.id}" class="snippet-tab ${isUrl ? '' : 'active'}" onclick="setSnippetTab('${svc.id}', 'curl')">cURL</button>
              </div>
              <div class="snippets">
                <div id="sbox-url-${svc.id}" class="curl-snippet" style="display:${isUrl ? 'flex' : 'none'};"><code class="curl-code"><a href="${escapeHtml(directUrl)}" target="_blank" rel="noopener">${escapeHtml(directUrl)}</a></code><button class="curl-copy-btn" onclick="copySnippet(this, '${escapeHtml(directUrl)}')">Copy</button></div>
                <div id="sbox-curl-${svc.id}" class="curl-snippet" style="display:${isUrl ? 'none' : 'flex'};"><code class="curl-code">${escapeHtml(curlCmd)}</code><button class="curl-copy-btn" onclick="copySnippet(this, '${escapeHtml(curlCmd)}')">Copy</button></div>
              </div>
            </div>
          </div>`;
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

    selectOSTab(detectOS());
    updateCommand();
    updateLLMsCmd();
    fetchVersion();
    fetchServices();
    setInterval(fetchServices, 2500);
