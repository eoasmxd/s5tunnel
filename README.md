# S5Tunnel (Secure Private Network Access Tunnel)

English | [简体中文](README.zh.md)

S5Tunnel is a lightweight encrypted proxy tunnel implemented purely using the Go standard library (zero third-party dependencies), designed for **secure remote access to private networks without exposing internal IPs or service ports to the public internet**.

Instead of forwarding ports one by one for Home Assistant, Vaultwarden, NAS web panels, or router admin pages, you only need to expose a single TLS-encrypted and Token-authenticated `S5Tunnel Server` entry point. On your personal computer, run `S5Tunnel Client` (or the single-file Windows tray program `S5Tunnel GUI`) and enable the built-in `PROXY` / `PAC` configuration to access internal services directly by their private IP or hostname.

---

## Use Cases

1. **Home Lab & Smart Home Access**: Securely access Vaultwarden, Home Assistant (`8123`), NAS web UI (`5000`/`443`), router admin pages, or internal cameras without exposing sensitive login interfaces directly on the internet.
2. **Office & Lab Intranet Access**: Reach internal git servers, CI dashboards, databases, or test environments behind a single encrypted gateway.
3. **Unified Single Entry Point**: Replace fragile multi-port forwarding rules with one authenticated tunnel entry, reducing your public attack surface to a single port.

---

## Key Features

- **Zero Internal Port Exposure**: Expose only a single encrypted tunnel entry publicly while keeping internal IPs and service ports completely unreachable from the internet.
- **Standard TLS & Access Control**: End-to-end standard TLS encryption with dynamic Token authentication, rejecting unauthorized access and invalid probes.
- **Single-Port Mixed Proxy & PAC Routing**: Serve SOCKS5, HTTP/HTTPS proxy, and PAC auto-configuration on one local port for precise subnet and domain routing.
- **Lightweight Cross-Platform & Windows Tray**: Pure Go standard library implementation (zero third-party dependencies) for Linux, Windows, macOS, and Docker, including the single-file Windows tray program `S5Tunnel GUI`.

---

## Quick Start

S5Tunnel provides three standalone binaries: **S5Tunnel Server** (`s5ts`), **S5Tunnel Client** (`s5tc`), and the Windows system tray program **S5Tunnel GUI** (`s5tc-gui.exe`).

### 1. S5Tunnel Server (`s5ts`)

`s5ts.ini`:
```ini
[common]
bind_addr = 0.0.0.0:3017
token = your_secure_token
# tls_cert_file = ./server.crt
# tls_key_file = ./server.key
```

Run:
```bash
./s5ts -c ./s5ts.ini
```

### 2. S5Tunnel Client (`s5tc` / `s5tc-gui`)

`s5tc.ini`:
```ini
[common]
local_addr = 127.0.0.1:1080
server_addr = your.domain.or.ip:3017
token = your_secure_token
```

- **S5Tunnel Client (CLI)**:
  ```bash
  ./s5tc -c ./s5tc.ini
  ```
  Provides SOCKS5, HTTP/HTTPS proxy, and the PAC endpoint (`http://127.0.0.1:1080/pac`, see [conf/pac.txt](conf/pac.txt)) on the same port.

- **S5Tunnel GUI (`s5tc-gui.exe`)**:
  Place `s5tc-gui.exe` alongside `s5tc.ini` (and optional `pac.txt`), double-click to launch, and right-click the tray icon to switch proxy modes.

### 3. Docker Deployment

Separate lightweight images are provided for `S5Tunnel Server` and `S5Tunnel Client` (published to `ghcr.io` and Docker Hub). The default configuration directory inside containers is `/etc/s5tunnel/`:

#### S5Tunnel Server (`s5ts`)
```bash
# Mount host config directory (containing s5ts.ini) to container /etc/s5tunnel
docker run -d \
  --name s5ts \
  --restart always \
  --network host \
  -v /path/to/conf:/etc/s5tunnel \
  ghcr.io/eoasmxd/s5ts:latest
```

#### S5Tunnel Client (`s5tc`)
```bash
# Mount host config directory (containing s5tc.ini and optional pac.txt) to container /etc/s5tunnel
docker run -d \
  --name s5tc \
  --restart always \
  --network host \
  -v /path/to/conf:/etc/s5tunnel \
  ghcr.io/eoasmxd/s5tc:latest
```

---

## Best Practices

### 1. With a Public IP (IPv4/IPv6 + DDNS) — Single Entry Point for Home HA / NAS

When your home broadband has a public IP (paired with DDNS if dynamic), **only open/forward the single `S5Tunnel Server` port (e.g., `3017`) on your router or host**. Keep Home Assistant (`8123`), NAS (`5000`), and router admin interfaces completely closed to the internet.

1. **Home `S5Tunnel Server` Config (`s5ts.ini`)** (running on your home NAS, router, or Docker host):
   ```ini
   [common]
   bind_addr = 0.0.0.0:3017
   token = your_strong_s5t_token
   ```
2. **Laptop `S5Tunnel Client` Config (`s5tc.ini`)**:
   ```ini
   [common]
   local_addr = 127.0.0.1:1080
   server_addr = home.yourdomain.com:3017
   token = your_strong_s5t_token
   ```
3. **Laptop PAC Rules (`pac.txt`)** (keep `defaultAction = direct` so regular web traffic goes direct without consuming home upload bandwidth, and only your home subnet/domains enter the tunnel):
   ```javascript
   var proxy = typeof __PROXY__ !== 'undefined' ? __PROXY__ : 'PROXY 127.0.0.1:1080; SOCKS5 127.0.0.1:1080; DIRECT';
   var direct = 'DIRECT';

   // Default policy: regular traffic goes direct; only listed home networks/domains use S5Tunnel
   var defaultAction = direct;

   var proxyNetworks = [
       ['192.168.1.0', '255.255.255.0']
   ];

   var proxyDomains = [
       'home.arpa',
       'lan'
   ];

   // Optional: exclude conflicting local IPs when you are on another LAN using the same subnet
   var directNetworks = [
       // ['192.168.1.1', '255.255.255.255']
   ];

   var directDomains = [];
   ```
   *(Use the full script template from [conf/pac.txt](conf/pac.txt) and adjust `proxyNetworks` / `proxyDomains` / `directNetworks` to match your home LAN.)*

4. **Enable Built-in System Proxy on Your Computer**:
   - **Windows**: Double-click `S5Tunnel GUI` (`s5tc-gui.exe` with `s5tc.ini` and `pac.txt` beside it). It automatically enters **PAC Mode**.
   - **macOS / Linux Desktop**: Start `S5Tunnel Client` (`./s5tc -c ./s5tc.ini`), then set your OS Network Settings automatic proxy configuration URL to `http://127.0.0.1:1080/pac`.
   - **Why PAC Mode instead of Global Mode?** In addition to saving home upload bandwidth on everyday internet traffic, Windows **Global Mode** bypasses `192.168.*` / `10.*` / `172.16.*`–`172.31.*` and `<local>` by default, which prevents private home IPs from entering the tunnel. **PAC Mode** handles internal subnets and hostnames cleanly.

5. **Usage**: Open your browser directly to `http://192.168.1.100:8123` (Home Assistant) or `http://192.168.1.10:5000` (NAS) as if you were at home.

### 2. Without a Public IP (CGNAT / Double NAT) — Expose S5Tunnel via [L4Tunnel](https://github.com/eoasmxd/l4tunnel)

When your home broadband is behind CGNAT with no public IP, you do not need to create separate port-forwarding rules for Home Assistant, Vaultwarden, NAS, router UI, and every internal host—nor expose their web login panels directly on a public VPS port. Instead, **keep `S5Tunnel Server` bound to loopback at home and use [L4Tunnel](https://github.com/eoasmxd/l4tunnel) to expose only that single encrypted `S5Tunnel Server` port through your VPS**:

1. **Home `S5Tunnel Server` Config (`s5ts.ini`)** (only listens on loopback; never exposed to LAN/WAN directly):
   ```ini
   [common]
   bind_addr = 127.0.0.1:3017
   token = your_strong_s5t_token
   ```
2. **Public VPS `L4Tunnel Server` Config (`l4ts.ini`)** (receives the reverse tunnel control channel from home):
   ```ini
   [common]
   bind_addr = 0.0.0.0
   bind_port = 3007
   token = your_strong_l4t_token
   ```
3. **Home `L4Tunnel Client` Config (`l4tc.ini`)** (forwards the local `S5Tunnel Server` port to port `3017` on the VPS):
   ```ini
   [common]
   server_addr = your.vps.ip
   server_port = 3007
   token = your_strong_l4t_token

   [s5tunnel]
   local_ip = 127.0.0.1
   local_port = 3017
   remote_port = 3017
   ```
4. **Laptop `S5Tunnel Client` Config (`s5tc.ini`)** (connects to the `S5Tunnel Server` entry exposed on the VPS, paired with the same `pac.txt` as Practice 1):
   ```ini
   [common]
   local_addr = 127.0.0.1:1080
   server_addr = your.vps.ip:3017
   token = your_strong_s5t_token
   ```

The VPS port `3017` exposes only a single TLS-encrypted and Token-authenticated `S5Tunnel Server` entry—no internal IP or application UI is ever exposed publicly, and a single rule unlocks access to your entire home subnet.

### 3. Single-Port Multiplexing — Share Public Port 443 Across L4Tunnel, S5Tunnel, and Websites via Nginx `stream` SNI

Because both `S5Tunnel Server` and `L4Tunnel Server` run over standard TLS and their clients automatically send TLS SNI when configured with domain names, **you can multiplex the `L4Tunnel` control channel, the `S5Tunnel` entry point, and your regular HTTPS websites onto the single public port `443` using Nginx's Layer-4 `stream` module and `ssl_preread`**—no need to open `3007` or `3017` on your public firewall:

1. **Server Setup** (loopback-only listeners; firewall only exposes port `443`):
   - **Home `S5Tunnel Server` (`s5ts.ini`)**: `bind_addr = 127.0.0.1:3017`
   - **Public VPS `L4Tunnel Server` (`l4ts.ini`)** (when paired with L4Tunnel behind CGNAT): `bind_addr = 127.0.0.1`, `bind_port = 3007` (with home `L4Tunnel Client` forwarding local `3017` to port `3017` on the VPS)
2. **Public Nginx Configuration (top-level `stream` block in `nginx.conf`)**:
   ```nginx
   stream {
       map $ssl_preread_server_name $backend_name {
           # Dedicated domain for L4Tunnel Server (only needed when using L4Tunnel)
           l4t.yourdomain.com    l4tunnel_server;
           # Dedicated domain for S5Tunnel Server
           s5t.yourdomain.com    s5tunnel_server;
           # All other domains -> route to local Nginx Layer-7 HTTPS
           default               web_backend;
       }

       upstream l4tunnel_server {
           server 127.0.0.1:3007;
       }

       upstream s5tunnel_server {
           server 127.0.0.1:3017;
       }

       upstream web_backend {
           server 127.0.0.1:8443; # Local Nginx https server
       }

       server {
           listen 443;
           ssl_preread on; # Inspect SNI without terminating TLS
           proxy_pass $backend_name;
       }
   }
   ```
   *(If your home network has a public IP running `S5Tunnel Server` directly, simply omit `l4t.yourdomain.com` and `l4tunnel_server`.)*
3. **Client Configurations (both connect via port `443` using dedicated domain names)**:
   - **Home `L4Tunnel Client` (`l4tc.ini`)**:
     ```ini
     [common]
     server_addr = l4t.yourdomain.com
     server_port = 443
     token = your_strong_l4t_token

     [s5tunnel]
     local_ip = 127.0.0.1
     local_port = 3017
     remote_port = 3017
     ```
   - **Laptop `S5Tunnel Client` (`s5tc.ini`)**:
     ```ini
     [common]
     local_addr = 127.0.0.1:1080
     server_addr = s5t.yourdomain.com:443
     token = your_strong_s5t_token
     ```

---

## Configuration

### S5Tunnel Server ([conf/s5ts.ini](conf/s5ts.ini))

```ini
[common]
bind_addr = 0.0.0.0:3017
token = 12345678
# tls_cert_file = ./server.crt
# tls_key_file = ./server.key
```

### S5Tunnel Client ([conf/s5tc.ini](conf/s5tc.ini))

```ini
[common]
local_addr = 127.0.0.1:1080
# Linux transparent proxy (iptables/nftables REDIRECT, Linux only)
# redir_addr = 0.0.0.0:1081
server_addr = 127.0.0.1:3017
token = 12345678
```

### PAC Rules ([conf/pac.txt](conf/pac.txt))

`pac.txt` resides in the same directory as `s5tc.ini` and exposes three main groups of settings:
- `defaultAction`: `direct` (default, only listed internal targets enter the tunnel) or `proxy` (all traffic enters the tunnel except direct lists).
- `proxyNetworks` / `proxyDomains`: IPv4 CIDR/mask pairs and domain/hostname suffixes routed through S5Tunnel.
- `directNetworks` / `directDomains`: Higher-priority direct exceptions (useful for excluding a conflicting local router IP such as `192.168.1.1`).

---

## Build

Pure Go standard library implementation with zero third-party dependencies:

### 1. Direct Go Build (Recommended, Cross-Platform)

```bash
go build -o s5ts ./cmd/s5ts
go build -o s5tc ./cmd/s5tc
# Windows windowless system tray client
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -H=windowsgui" -o s5tc-gui.exe ./cmd/s5tc-gui
```

### 2. Using Makefile

```bash
# Local build
make

# Cross-compile and package for all platforms
make release
```

### 3. Local Docker Build

```bash
docker build -f docker/Dockerfile.server -t s5ts .
docker build -f docker/Dockerfile.client -t s5tc .
```

---

## License

S5Tunnel is licensed under the MIT License. See [LICENSE](LICENSE) for details.
