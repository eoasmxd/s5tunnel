# S5Tunnel（安全访问内网的轻量加密代理隧道）

[English](README.md) | 简体中文

S5Tunnel 是基于 Go 标准库实现（零第三方依赖）的轻量级加密代理隧道工具，专注于**在不暴露内网 IP 与业务端口到公网的前提下，安全访问家庭或办公私有内网资源**。

无需为家里的 Home Assistant、Vaultwarden、NAS 或路由器后台逐一做端口映射，只需对外保留唯一一个经过 TLS 加密与 Token 校验的 `S5Tunnel Server` 入口。在自己电脑上运行 `S5Tunnel Client`（或单文件 Windows 托盘程序 `S5Tunnel GUI`）并配置内置 `PROXY` / `PAC`，即可直接按原生内网 IP 或内网域名访问家中全部服务。

---

## 适用场景

1. **家庭内网安全访问**：远程安全访问家中的 Vaultwarden、Home Assistant（`8123`）、NAS 管理后台（`5000`/`443`）、软路由/路由器后台或内网摄像头，无需把敏感登录后台直接挂到公网。
2. **办公/实验室私网接入**：通过单一加密入口访问内网代码仓库、构建面板、数据库或测试环境。
3. **替代逐端口映射的统一入口**：用一条加密隧道代替零散的端口转发规则，将公网暴露面收敛到单个高安全端口。

---

## 核心特性

- **零内网端口暴露**：公网仅开放唯一加密隧道入口，不暴露内网 IP 与业务端口，即可安全访问内网各类服务。
- **标准 TLS 与访问控制**：全链路采用标准 TLS 加密与 Token 动态认证，拒绝未授权访问与非法探测。
- **单端口混合代理与 PAC 分流**：同一本地端口同时提供 SOCKS5、HTTP/HTTPS 代理及 PAC 自动分流服务，支持按内网网段与域名精准路由。
- **轻量跨平台与 Windows 托盘**：纯 Go 标准库零第三方依赖，支持 Linux / Windows / macOS 与 Docker 部署，另提供单文件 Windows 托盘程序 `S5Tunnel GUI`。

---

## 快速使用

S5Tunnel 包含三个独立程序：**S5Tunnel Server**（`s5ts`）、**S5Tunnel Client**（`s5tc`）以及 Windows 托盘程序 **S5Tunnel GUI**（`s5tc-gui.exe`）。

### 1. S5Tunnel Server（`s5ts`）

编写 `s5ts.ini`：

```ini
[common]
bind_addr = 0.0.0.0:3017
token = your_secure_token
# tls_cert_file = ./server.crt
# tls_key_file = ./server.key
```

启动服务端：

```bash
./s5ts -c ./s5ts.ini
```

### 2. S5Tunnel Client（`s5tc` / `s5tc-gui`）

编写 `s5tc.ini`：

```ini
[common]
local_addr = 127.0.0.1:1080
server_addr = your.domain.or.ip:3017
token = your_secure_token
```

- **S5Tunnel Client（命令行）**：
  ```bash
  ./s5tc -c ./s5tc.ini
  ```
  同端口同时提供 SOCKS5、HTTP/HTTPS 代理与 PAC 地址（`http://127.0.0.1:1080/pac`，规则文件见 [conf/pac.txt](conf/pac.txt)）。

- **S5Tunnel GUI（`s5tc-gui.exe`）**：
  将 `s5tc-gui.exe` 与 `s5tc.ini`（以及可选的 `pac.txt`）放在同一目录下双击运行，右键托盘图标即可切换代理模式。

### 3. Docker 部署

`S5Tunnel Server` 与 `S5Tunnel Client` 均提供了独立的轻量容器镜像（发布至 `ghcr.io` 与 Docker Hub），容器内统一约定配置目录为 `/etc/s5tunnel/`：

#### S5Tunnel Server（`s5ts`）
```bash
# 将宿主机配置目录（包含 s5ts.ini）挂载至容器 /etc/s5tunnel
docker run -d \
  --name s5ts \
  --restart always \
  --network host \
  -v /path/to/conf:/etc/s5tunnel \
  ghcr.io/eoasmxd/s5ts:latest
```

#### S5Tunnel Client（`s5tc`）
```bash
# 将宿主机配置目录（包含 s5tc.ini 及可选的 pac.txt）挂载至容器 /etc/s5tunnel
docker run -d \
  --name s5tc \
  --restart always \
  --network host \
  -v /path/to/conf:/etc/s5tunnel \
  ghcr.io/eoasmxd/s5tc:latest
```

---

## 最佳实践

### 1. 有公网 IP（IPv4/IPv6 + DDNS）— 单入口安全访问家中 HA / NAS

当家庭宽带拥有公网 IP 时（动态 IP 可搭配 DDNS 域名），**在路由器或宿主机上只开放 `S5Tunnel Server` 这一个端口（如 `3017`）**，Home Assistant（`8123`）、NAS（`5000`）、路由器后台等一律不对外做端口映射。

1. **家中 `S5Tunnel Server` 配置（`s5ts.ini`）**（部署在家里 NAS、软路由或 Docker 容器中）：
   ```ini
   [common]
   bind_addr = 0.0.0.0:3017
   token = your_strong_s5t_token
   ```
2. **随身电脑 `S5Tunnel Client` 配置（`s5tc.ini`）**：
   ```ini
   [common]
   local_addr = 127.0.0.1:1080
   server_addr = home.yourdomain.com:3017
   token = your_strong_s5t_token
   ```
3. **随身电脑 PAC 规则（`pac.txt`）**（保持 `defaultAction = direct`，日常上网直连不绕家里上行带宽，只让命中家中内网的流量进入隧道）：
   ```javascript
   var proxy = typeof __PROXY__ !== 'undefined' ? __PROXY__ : 'PROXY 127.0.0.1:1080; SOCKS5 127.0.0.1:1080; DIRECT';
   var direct = 'DIRECT';

   // 默认策略：仅代理列表走隧道，其余公网流量直连
   var defaultAction = direct;

   var proxyNetworks = [
       ['192.168.1.0', '255.255.255.0']
   ];

   var proxyDomains = [
       'home.arpa',
       'lan'
   ];

   // 可选：当外部所在局域网与家中网段冲突时，排除本地路由器等冲突 IP
   var directNetworks = [
       // ['192.168.1.1', '255.255.255.255']
   ];

   var directDomains = [];
   ```
   *（完整规则脚本请直接使用 [conf/pac.txt](conf/pac.txt)，按自家实际网段与内网域名调整即可。）*

4. **电脑端配置内置 PROXY / PAC**：
   - **Windows**：将 `S5Tunnel GUI`（`s5tc-gui.exe`）与 `s5tc.ini`、`pac.txt` 放在同一目录双击运行，自动启用 **PAC 模式**。
   - **macOS / Linux 桌面**：启动 `S5Tunnel Client`（`./s5tc -c ./s5tc.ini`）后，在系统网络设置中将“自动代理配置（PAC）”地址设为 `http://127.0.0.1:1080/pac`。
   - **为什么推荐 PAC 模式而不是全局模式？** 除了避免日常上网占用家中宽带上行带宽外，Windows 的 **全局模式** 默认会绕过 `192.168.*`、`10.*`、`172.16.*`～`172.31.*` 与 `<local>`，导致内网 IP 无法进入隧道；而 **PAC 模式** 可以精准分流家中内网网段与内网短域名。

5. **访问效果**：在外网电脑浏览器中直接输入 `http://192.168.1.100:8123`（Home Assistant）或 `http://192.168.1.10:5000`（NAS），即可像在家中局域网一样直接访问。

### 2. 无公网 IP（宽带大内网 / CGNAT）— 配合 [L4Tunnel](https://github.com/eoasmxd/l4tunnel) 暴露 S5Tunnel 进入内网

如果家庭宽带处于运营商大内网（无公网 IP），无需在穿透工具里为 HA、Vaultwarden、NAS、路由器后台等每个服务逐一配置映射规则，更不必把业务登录页直接挂在公网 VPS 端口上。**只需让家里的 `S5Tunnel Server` 监听本地回环，再用 [L4Tunnel](https://github.com/eoasmxd/l4tunnel) 将这唯一一个加密端口暴露到公网 VPS**：

1. **家中 `S5Tunnel Server` 配置（`s5ts.ini`）**（仅监听本机回环，不对外直接暴露）：
   ```ini
   [common]
   bind_addr = 127.0.0.1:3017
   token = your_strong_s5t_token
   ```
2. **公网 VPS 的 `L4Tunnel Server` 配置（`l4ts.ini`）**（接收家中内网的反向控制连接）：
   ```ini
   [common]
   bind_addr = 0.0.0.0
   bind_port = 3007
   token = your_strong_l4t_token
   ```
3. **家中 `L4Tunnel Client` 配置（`l4tc.ini`）**（仅将本地 `S5Tunnel Server` 单端口映射至公网 VPS 的 `3017` 端口）：
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
4. **随身电脑 `S5Tunnel Client` 配置（`s5tc.ini`）**（连接公网 VPS 暴露出来的 `S5Tunnel Server` 入口，配合实践 1 中的 `pac.txt`）：
   ```ini
   [common]
   local_addr = 127.0.0.1:1080
   server_addr = your.vps.ip:3017
   token = your_strong_s5t_token
   ```

公网 VPS 的 `3017` 端口对外只暴露经过标准 TLS 加密与 Token 认证的 `S5Tunnel Server` 单一入口，不裸露任何内网 IP 与业务端口，只需一条穿透配置即可安全访问家中整个局域网。

### 3. 单端口复用 — 基于 Nginx `stream` SNI 将 L4Tunnel、S5Tunnel 与网站全部复用到 443 端口

由于 `S5Tunnel Server` 与 `L4Tunnel Server` 均运行在标准 TLS 之上，且客户端使用域名连接时均会自动携带 TLS SNI，因此**可通过 Nginx 四层 `stream` 模块的 `ssl_preread`，将 `L4Tunnel` 控制通道、`S5Tunnel` 入口以及常规 HTTPS 网站全部复用到公网唯一的 `443` 端口**，公网防火墙无需额外放行 `3007` 或 `3017`：

1. **服务端配置**（仅本地监听，公网防火墙仅放行 `443`）：
   - **家中 `S5Tunnel Server`（`s5ts.ini`）**：`bind_addr = 127.0.0.1:3017`
   - **公网 VPS 的 `L4Tunnel Server`（`l4ts.ini`）**（无公网 IP 联动 L4Tunnel 时配置）：`bind_addr = 127.0.0.1`、`bind_port = 3007`（配合家中 `L4Tunnel Client` 将本地 `3017` 映射至公网 VPS 本机 `3017` 端口）
2. **公网 Nginx 配置（`nginx.conf` 顶层 `stream` 块）**：
   ```nginx
   stream {
       map $ssl_preread_server_name $backend_name {
           # L4Tunnel Server 专用域名（仅在配合 L4Tunnel 时需要）
           l4t.yourdomain.com    l4tunnel_server;
           # S5Tunnel Server 专用域名
           s5t.yourdomain.com    s5tunnel_server;
           # 其它所有域名，分流给本地 Nginx 七层 HTTPS 站点
           default               web_backend;
       }

       upstream l4tunnel_server {
           server 127.0.0.1:3007;
       }

       upstream s5tunnel_server {
           server 127.0.0.1:3017;
       }

       upstream web_backend {
           server 127.0.0.1:8443; # 本地 Nginx https server 监听端口
       }

       server {
           listen 443;
           ssl_preread on; # 仅预读 SNI 而不解密 TLS
           proxy_pass $backend_name;
       }
   }
   ```
   *（若家里有公网 IP 直接运行 `S5Tunnel Server`，可省略 `l4t.yourdomain.com` 与 `l4tunnel_server` 配置。）*
3. **客户端配置（全部通过各自域名连接 `443` 端口）**：
   - **家中 `L4Tunnel Client`（`l4tc.ini`）**：
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
   - **随身电脑 `S5Tunnel Client`（`s5tc.ini`）**：
     ```ini
     [common]
     local_addr = 127.0.0.1:1080
     server_addr = s5t.yourdomain.com:443
     token = your_strong_s5t_token
     ```

---

## 配置文件说明

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
# Linux 透明代理（配合 iptables/nftables REDIRECT，仅 Linux 有效）
# redir_addr = 0.0.0.0:1081
server_addr = 127.0.0.1:3017
token = 12345678
```

### PAC 分流规则 ([conf/pac.txt](conf/pac.txt))

`pac.txt` 放置于 `s5tc.ini` 同目录下，包含 3 组核心配置项：
- `defaultAction`：`direct`（默认，仅命中的内网列表走隧道）或 `proxy`（除直连列表外全走隧道）。
- `proxyNetworks` / `proxyDomains`：走 S5Tunnel 隧道的内网网段（IP/掩码）与域名/主机名列表。
- `directNetworks` / `directDomains`：优先级更高的直连例外列表（可用于排除与家中同网段的当前本地路由器 IP，如 `192.168.1.1`）。

---

## 构建与编译

本项目采用纯 Go 标准库实现，零第三方依赖：

### 1. 原生 Go 编译（推荐，全平台通用）

```bash
go build -o s5ts ./cmd/s5ts
go build -o s5tc ./cmd/s5tc
# Windows 无控制台窗口托盘客户端
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -H=windowsgui" -o s5tc-gui.exe ./cmd/s5tc-gui
```

### 2. 使用 Makefile 编译

```bash
# 本地编译
make

# 全平台交叉编译与打包
make release
```

### 3. 本地手工构建 Docker 镜像

```bash
docker build -f docker/Dockerfile.server -t s5ts .
docker build -f docker/Dockerfile.client -t s5tc .
```

---

## 开源协议

本项目采用 MIT License 协议开源，详情参见 [LICENSE](LICENSE) 文件。
