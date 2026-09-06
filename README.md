<div align="center">

# maek<span style="color:#1d9bf0">.</span>

**Instant tunnels to your localhost. Zero DNS, zero accounts, pure simplicity.**

*The self-hosted, single-binary ngrok alternative with automated link previews and Twitter-style dashboard.*

<br/>

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Docker Image](https://img.shields.io/badge/Docker-~15MB-2496ED?style=flat&logo=docker)](https://hub.docker.com/)
[![UI Accent](https://img.shields.io/badge/UI-Twitter_Blue-1d9bf0?style=flat)](internal/server/static)
[![Zero Config](https://img.shields.io/badge/DNS-Zero_Config-success?style=flat)](#zero-config-routing-no-wildcard-dns)
[![Multiplexing](https://img.shields.io/badge/Tunnel-Yamux_over_WebSocket-purple?style=flat)](#architecture)

</div>

```bash
# 1. On your VPS ($3/mo, no domain needed)
maek server -p 8080

# 2. On your local machine (macOS / Linux / Windows)
curl -fsSL http://<VPS-IP>:8080/_maek/install.sh | sh
maek agent -s ws://<VPS-IP>:8080 -t http://localhost:3000
```

---

## ⚡ Why maek?

Most tunneling tools either lock you into costly monthly subscriptions, require annoying account registrations, or force you into complicated wildcard DNS and TLS certificate setups. 

**`maek` solves this cleanly.** You run one tiny binary on a cheap VPS, and it gives you unlimited, beautiful tunnels with zero external dependencies.

| Feature | `maek` | `ngrok` (Free) | Cloudflare Tunnel | `frp` |
| :--- | :---: | :---: | :---: | :---: |
| **Account / Sign-up** | ❌ **None** | ⚠️ Required | ⚠️ Required | ❌ None |
| **Custom Domain / Wildcard DNS** | ❌ **Not needed** (Works on IP) | ❌ Random subdomains | ⚠️ Custom domain required | ⚠️ Wildcard DNS required |
| **Tunnel Limits & Pricing** | ♾️ **Unlimited (Free & OSS)** | ⚠️ 1 tunnel / rate limits | ♾️ Free | ♾️ Self-hosted |
| **Private / Airgap Network** | ✅ **100% Self-contained** | ❌ Cloud only | ❌ Cloud only | ⚠️ Manual binary copy |
| **Client Installation** | 🚀 **One-line from your VPS** | ⚠️ Download from cloud | ⚠️ Cloudflare package | ⚠️ Manual config |
| **Web Catalog Dashboard** | ✅ **Twitter-style UI** | ⚠️ Web dashboard | ❌ Cloud console only | ⚠️ Basic admin UI |
| **Auto Metadata Scraping** | ✅ **Title, icon, description** | ❌ None | ❌ None | ❌ None |
| **Social / Messenger Previews** | ✅ **Slack, Discord, KakaoTalk** | ❌ None | ❌ None | ❌ None |
| **WebSocket & Vite HMR** | ✅ **Full Yamux multiplexing** | ✅ Yes | ✅ Yes | ✅ Yes |

---

## ✨ Key Features

### 🎯 Zero-Config Routing (No Wildcard DNS)
No need to purchase domains or configure wildcard DNS (`*.domain.com`). 
* **Direct URLs**: Share `http://<VPS-IP>:8080/_maek/my-app` or by ID `http://<VPS-IP>:8080/_maek/@k8x2p9`.
* **Cookie Routing**: Visiting a direct URL or selecting an app in the catalog issues a lightweight session cookie (`maek_service=<id>`), routing all subsequent browser requests transparently.
* **API / CLI Friendly**: Pass `X-Maek-Service: my-app` in `curl` or automated test scripts.

### 🤖 Zero-Touch Auto-Scraping
Just point `maek agent` to your local service:
```bash
maek agent -s ws://vps:8080 -t http://localhost:3000
```
`maek` inspects your local app and automatically extracts:
- App name from `<title>`
- Description from `<meta name="description">` or `og:description`
- Favicon and app icon from `<link rel="icon">` or Apple Touch Icons

### 💬 Rich Social & Messenger Previews
When you paste a `maek` link into **Slack, Discord, KakaoTalk, Twitter/X, or Telegram**, crawler bots are automatically served dynamic OpenGraph metadata with embedded thumbnails (`/_maek/thumb?id=xxx`). Your team sees a rich preview card instead of a blank or redirected link!

### 🪟 Isolated Floating Widget (`float.js`)
When browsing your tunneled app, a sleek floating indicator is injected seamlessly before `</body>`:
* **Zero CSS conflicts**: Rendered inside a **Shadow DOM**.
* **Magnetic drag-and-drop**: Snaps to the nearest screen corner and remembers its position in `localStorage`.
* **Compact / Full mode**: Toggle between a minimal status dot and full service details.
* **One-Click Disconnect**: Click **Exit ✕** anytime to clear your routing cookie and return to the catalog.

### 📦 Self-Hosted Embedded Installers
The `maek server` binary **embeds multi-platform client archives** (`go:embed`).
* Client machines download install scripts directly from your server:
  * **macOS / Linux**: `curl -fsSL http://<VPS-IP>:8080/_maek/install.sh | sh`
  * **Windows**: `irm http://<VPS-IP>:8080/_maek/install.ps1 | iex`
* **Zero dependency on `api.github.com`**: Works flawlessly in private VPCs, company intranets, and restricted networks.

### 🎨 Twitter-Style Web Dashboard
Open `http://<VPS-IP>:8080/` in your browser to experience:
* Clean Twitter/X typography with **Twitter Blue** (`#1d9bf0`) accents.
* **2-Step Onboarding**: Auto-detects your OS (macOS, Linux, Windows, Go) with one-click copy buttons and dynamic agent command generator.
* Live real-time service feed with status indicators and direct `curl` snippets.

### ⚡ Yamux Multiplexing over WebSocket
All traffic—concurrent HTTP requests, high-volume static assets, and full-duplex WebSockets (such as **Next.js HMR**, **Vite**, live chat, or web terminals)—is multiplexed over a single persistent outbound WebSocket connection.

---

## 🏗️ Architecture

```
                                  +------------------------------------+
                                  |            maek server             |
                                  |          (Public VPS / IP)         |
                                  +------------------------------------+
                                    |            |                  |
                   GET / (No Cookie)|            | GET / (Cookie)   | WebSocket (/_maek/ws)
                                    v            v                  |
           [Web UI: Service Catalog]    [Reverse Proxy Router]      |
                                                 |                  |
                                           Yamux Streams            v
                                    +------------------------------------+
                                    |             maek-agent             |
                                    |         (Private Network)          |
                                    +------------------------------------+
                                                     |
                                            HTTP / TCP / WebSockets
                                                     v
                                    +------------------------------------+
                                    |        Target Local Service        |
                                    |     (Next.js, Vite, FastAPI, ...)  |
                                    +------------------------------------+
```

---

## 🚀 Quick Start

### 1. Start Server on your VPS
Run the single binary on any machine with a public IP:
```bash
./maek server -p 8080
```
Or via Docker:
```bash
docker run -d --name maek -p 8080:8080 --restart always ghcr.io/gosuda/maek:latest
```

### 2. Install Agent on your Local Machine
On your development machine, install the CLI with a single command straight from your server:

**macOS / Linux**:
```bash
curl -fsSL http://<VPS-IP>:8080/_maek/install.sh | sh
```

**Windows (PowerShell)**:
```powershell
irm http://<VPS-IP>:8080/_maek/install.ps1 | iex
```

### 3. Connect your Local Service
Expose any local port (e.g. Next.js, Django, FastAPI, Spring Boot on port 3000):
```bash
# Zero-config (metadata auto-detected from target)
maek agent -s ws://<VPS-IP>:8080 -t http://localhost:3000

# Or with custom name and ID
maek agent -s ws://<VPS-IP>:8080 -n dev-app -i my-app -t http://localhost:3000
```

### 4. Access & Share
* **Web Catalog**: Open `http://<VPS-IP>:8080/` in your browser.
* **Direct URL**: Share `http://<VPS-IP>:8080/_maek/dev-app` with your team!
* **cURL**:
  ```bash
  curl -H "X-Maek-Service: dev-app" http://<VPS-IP>:8080/api/health
  ```

---

## 📦 Installation Options

### Option A: One-Liner Script (Recommended)
Download directly from your running `maek server`:
```bash
# macOS & Linux
curl -fsSL http://<VPS-IP>:8080/_maek/install.sh | sh

# Windows (PowerShell 5.1+)
irm http://<VPS-IP>:8080/_maek/install.ps1 | iex
```

### Option B: Go Install
```bash
go install github.com/gosuda/maek/cmd/maek@latest
```

### Option C: Build from Source
```bash
git clone https://github.com/gosuda/maek.git
cd maek
make build
```

### Option D: Docker
```bash
# Build image
make docker-build

# Run container
make docker-run
```

---

## 💻 CLI Reference

### `maek server`
```text
Usage: maek server [flags]

Flags:
  -addr, -p string   Address or port to listen on (default ":8080")
```

### `maek agent`
```text
Usage: maek agent [flags]

Flags:
  -server, -s string   Central maek server URL (e.g. "ws://vps-ip:8080")
  -name, -n string     Service name (optional, auto-detected from target)
  -id, -i string       Preferred custom service ID (optional, max 32 chars)
  -desc, -d string     Short description of service (optional, auto-detected)
  -thumb string        Thumbnail URL or image avatar (optional, auto-detected)
  -target, -t string   Local target HTTP URL (default "http://localhost:3000")
```

---

## 🛠️ Endpoints Reference

| Endpoint | Method | Description |
| :--- | :---: | :--- |
| `/` | `GET` | Catalog Web UI (or proxy to service if routing cookie/header present) |
| `/_maek` | `GET` | Forces catalog UI view & clears active routing cookie |
| `/_maek/<name\|id>` | `GET` | Direct service URL (sets cookie and 302 redirects, or serves OG tags to bots) |
| `/_maek/ws` | `GET` | WebSocket endpoint for agent Yamux tunnel connection |
| `/_maek/thumb` | `GET` | Serves binary image thumbnails (`image/png`) for social cards |
| `/_maek/install.sh` | `GET` | macOS / Linux installation shell script with self-hosted server template |
| `/_maek/install.ps1`| `GET` | Windows PowerShell installation script |
| `/_maek/download` | `GET` | Direct binary download endpoint (`?os=Darwin\|Linux\|Windows&arch=arm64\|x86_64`) |
| `/_maek/version` | `GET` | Server version information (JSON or plain text) |

---

## 📄 License

Distributed under the MIT License. See [LICENSE](LICENSE) for more information.
