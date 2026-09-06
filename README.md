# maek

> ngrok-like lightweight HTTP reverse proxy tunnel via WebSocket & Yamux.

`maek` allows you to expose local private HTTP(S) services (behind NAT or firewalls) to the public internet using a single VPS instance with a public IP address—**without requiring custom domains, wildcard DNS, or complicated certificate setup**.

---

## Key Features

* **Single Binary**: Run both the VPS gateway (`maek server`) and the local tunnel client (`maek agent`) with a single binary.
* **WebSocket + Yamux Multiplexing**: Multiple concurrent HTTP requests, assets, and streams are multiplexed efficiently over a single WebSocket connection.
* **Zero-Config Web UI**: Browsing to `maek` root displays an active service catalog with one-click connection buttons and live polling.
* **Cookie-Based Routing**: Selecting a service issues a session cookie (`maek_service=<id>`), routing all subsequent browser requests directly to that agent.
* **Smart Floating Widget (`float.js`)**:
  * Injected automatically into HTML responses (`</body>`).
  * Isolated with **Shadow DOM** to avoid CSS style pollution.
  * Displays the connected service status and a one-click **Exit ✕** button.
* **One-Click Disconnect (`/_maek`)**: Navigating to `/_maek` (or clicking Exit) instantly removes the routing cookie and returns you to the main catalog.
* **Header-Based Routing for APIs/CLI**: Supports `X-Maek-Service: <id|name>` for `curl` and automated script requests.
* **Transparent Gzip & CSP Handling**: Automatically decompresses gzipped HTML for script injection and strips strict CSP headers so the widget is never blocked.

---

## Architecture

```
                                      +------------------------------------+
                                      |            maek server             |
                                      |            (Public VPS)            |
                                      +------------------------------------+
                                        |            |                  |
                       GET / (No Cookie)|            | GET / (Cookie)   | WebSocket (/_maek/ws)
                                        v            v                  |
               [Web UI: Service Catalog]   [Reverse Proxy]              |
                                                     |                  |
                                               Yamux Streams            v
                                        +------------------------------------+
                                        |             maek-agent             |
                                        |         (Private Network)          |
                                        +------------------------------------+
                                                         |
                                                     HTTP / TCP
                                                         v
                                        +------------------------------------+
                                        |        Target Local Server         |
                                        |       (e.g. localhost:3000)        |
                                        +------------------------------------+
```

---

## Installation & Build

```bash
git clone https://github.com/gosuda/maek.git
cd maek
go build -o maek ./cmd/maek
```

---

## Quick Start

### 1. Start Server on your VPS
```bash
./maek server -p 8080
```

### 2. Run Local Service
For testing, run any local HTTP server (e.g. port 3000):
```bash
python3 -m http.server 3000
```

### 3. Connect Local Agent
```bash
./maek agent -s ws://<VPS-IP>:8080 -n my-app -t http://localhost:3000
```
Upon connection, the server assigns a random 6-character service ID (e.g. `k8x2p9`):
```text
[maek-agent] Connected to server! Assigned Service ID: [k8x2p9], Name: 'my-app', Target: http://localhost:3000
```

### 4. Access via Browser
1. Open `http://<VPS-IP>:8080/` in your browser.
2. You will see the **maek Tunnel Gateway** catalog with `my-app` listed.
3. Click **Open Service &rarr;**.
4. The local application opens! A small floating badge `maek: my-app [k8x2p9]` appears at the bottom-right corner.
5. Click **Exit ✕** on the badge to return to the catalog anytime.

---

## CLI / API Access (`curl`)

To access services via API or command-line tools without cookies, pass the `X-Maek-Service` header with either the service ID or service name:

```bash
curl -H "X-Maek-Service: my-app" http://<VPS-IP>:8080/api/endpoint
# or
curl -H "X-Maek-Service: k8x2p9" http://<VPS-IP>:8080/api/endpoint
```

---

## CLI Reference

### `maek server`
```
Usage: maek server [flags]

Flags:
  -addr, -p string   Address or port to listen on (default ":8080")
```

### `maek agent`
```
Usage: maek agent [flags]

Flags:
  -server, -s string   Central maek server URL (e.g. "ws://vps-ip:8080")
  -name, -n string     Service identifier name (default "app")
  -target, -t string   Local target HTTP URL (default "http://localhost:3000")
```

---

## License

MIT
