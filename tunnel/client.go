package tunnel

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	socksVersion   = 5
	cmdConnect     = 1
	atypIPv4       = 1
	atypDomainName = 3
	atypIPv6       = 4
)

// HandleMixedProxy 在单端口同时处理 SOCKS5、HTTP/HTTPS 代理与本地 PAC 请求。
func HandleMixedProxy(conn net.Conn, cfg ClientConfig) {
	defer conn.Close()

	br := bufio.NewReader(conn)
	first, err := br.Peek(1)
	if err != nil {
		return
	}

	if first[0] == socksVersion {
		bufConn := NewBufConn(conn, br)
		target, err := socks5Handshake(bufConn)
		if err != nil {
			log.Printf("socks5 handshake failed: %v", err)
			return
		}
		ForwardTarget(bufConn, target, cfg)
		return
	}

	handleHTTPProxy(conn, br, cfg)
}

func ForwardTarget(conn net.Conn, target string, cfg ClientConfig) {
	tlsConn, err := dialTunnel(target, cfg)
	if err != nil {
		log.Printf("%v", err)
		return
	}
	defer tlsConn.Close()

	log.Printf("proxy %s <-> %s <-> %s", conn.RemoteAddr(), cfg.ServerAddr, target)
	if err := Relay(tlsConn, conn); err != nil {
		log.Printf("relay error: %v", err)
	}
}

func handleHTTPProxy(conn net.Conn, br *bufio.Reader, cfg ClientConfig) {
	req, err := http.ReadRequest(br)
	if err != nil {
		log.Printf("read HTTP proxy request failed: %v", err)
		return
	}
	defer req.Body.Close()

	if req.Method == http.MethodConnect {
		target := normalizeHostPort(req.Host, "443")
		if target == "" {
			_, _ = io.WriteString(conn, "HTTP/1.1 400 Bad Request\r\nConnection: close\r\n\r\n")
			return
		}

		tlsConn, err := dialTunnel(target, cfg)
		if err != nil {
			log.Printf("%v", err)
			_, _ = io.WriteString(conn, "HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n")
			return
		}
		defer tlsConn.Close()

		if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}

		log.Printf("proxy %s <-> %s <-> %s", conn.RemoteAddr(), cfg.ServerAddr, target)
		if err := Relay(tlsConn, NewBufConn(conn, br)); err != nil {
			log.Printf("relay error: %v", err)
		}
		return
	}

	if !req.URL.IsAbs() && req.URL.Host == "" {
		handleLocalHTTP(conn, req, cfg)
		return
	}

	host := req.URL.Host
	target := normalizeHostPort(host, "80")
	if target == "" {
		_, _ = io.WriteString(conn, "HTTP/1.1 400 Bad Request\r\nConnection: close\r\n\r\n")
		return
	}

	tlsConn, err := dialTunnel(target, cfg)
	if err != nil {
		log.Printf("%v", err)
		_, _ = io.WriteString(conn, "HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n")
		return
	}
	defer tlsConn.Close()

	req.RequestURI = ""
	req.Host = host
	req.URL.Scheme = ""
	req.URL.Host = ""
	req.Header.Del("Proxy-Connection")
	req.Header.Del("Proxy-Authenticate")
	req.Header.Del("Proxy-Authorization")
	req.Header.Set("Connection", "close")
	req.Close = true

	if err := req.Write(tlsConn); err != nil {
		log.Printf("write HTTP request failed: %v", err)
		return
	}

	log.Printf("proxy %s <-> %s <-> %s", conn.RemoteAddr(), cfg.ServerAddr, target)
	if err := Relay(tlsConn, NewBufConn(conn, br)); err != nil {
		log.Printf("relay error: %v", err)
	}
}

func dialTunnel(target string, cfg ClientConfig) (net.Conn, error) {
	tlsConn, err := ConnectServerWithTLS(cfg.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("connect server failed %s: %w", cfg.ServerAddr, err)
	}

	ts := time.Now().Unix()
	authKey := GetAuthKey(cfg.Token, ts)
	reqStr := fmt.Sprintf(
		"CONNECT %s HTTP/1.1\r\nHost: %s\r\nX-Timestamp: %d\r\nX-Auth: %s\r\n\r\n",
		target, target, ts, authKey,
	)
	if _, err := tlsConn.Write([]byte(reqStr)); err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("send CONNECT request failed: %w", err)
	}

	br := bufio.NewReader(tlsConn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("read server response failed: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tlsConn.Close()
		return nil, fmt.Errorf("server rejected connection %s: HTTP %d", target, resp.StatusCode)
	}

	return NewBufConn(tlsConn, br), nil
}

func normalizeHostPort(host, defaultPort string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if h, p, err := net.SplitHostPort(host); err == nil {
		if p == "" {
			return net.JoinHostPort(h, defaultPort)
		}
		return host
	}
	return net.JoinHostPort(strings.Trim(host, "[]"), defaultPort)
}

func socks5Handshake(rw io.ReadWriter) (string, error) {
	header := make([]byte, 3)
	if _, err := io.ReadFull(rw, header[:2]); err != nil {
		return "", err
	}
	if header[0] != socksVersion {
		return "", errors.New("unsupported SOCKS version")
	}

	methods := make([]byte, header[1])
	if _, err := io.ReadFull(rw, methods); err != nil {
		return "", err
	}
	if _, err := rw.Write([]byte{socksVersion, 0}); err != nil {
		return "", err
	}

	if _, err := io.ReadFull(rw, header[:3]); err != nil {
		return "", err
	}
	cmd := header[1]
	target, err := readSocks5Addr(rw)
	if err != nil {
		return "", err
	}

	if cmd != cmdConnect {
		_, _ = rw.Write([]byte{socksVersion, 7, 0, 1, 0, 0, 0, 0, 0, 0})
		return "", errors.New("SOCKS command not supported")
	}

	if _, err := rw.Write([]byte{socksVersion, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return "", err
	}
	return target, nil
}

func readSocks5Addr(r io.Reader) (string, error) {
	var atyp [1]byte
	if _, err := io.ReadFull(r, atyp[:]); err != nil {
		return "", err
	}

	var host string
	switch atyp[0] {
	case atypIPv4:
		var buf [net.IPv4len]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return "", err
		}
		host = net.IP(buf[:]).String()
	case atypDomainName:
		var lenBuf [1]byte
		if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
			return "", err
		}
		buf := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		host = string(buf)
	case atypIPv6:
		var buf [net.IPv6len]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return "", err
		}
		host = net.IP(buf[:]).String()
	default:
		return "", errors.New("address type not supported")
	}

	var portBuf [2]byte
	if _, err := io.ReadFull(r, portBuf[:]); err != nil {
		return "", err
	}
	port := strconv.Itoa((int(portBuf[0]) << 8) | int(portBuf[1]))
	return net.JoinHostPort(host, port), nil
}

