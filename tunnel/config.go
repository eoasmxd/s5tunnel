package tunnel

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ClientConfig struct {
	LocalAddr  string
	RedirAddr  string
	ServerAddr string
	Token      string
	PACFile    string
}

type ServerConfig struct {
	BindAddr    string
	Token       string
	TLSCertFile string
	TLSKeyFile  string
}

func LoadClientConfig(path string) (ClientConfig, error) {
	sec, err := parseCommonIni(path)
	if err != nil {
		return ClientConfig{}, err
	}

	serverHost, err := validateHostOnly("server_addr", "server_port", sec["server_addr"])
	if err != nil {
		return ClientConfig{}, err
	}
	if serverHost == "" {
		return ClientConfig{}, errors.New("server_addr cannot be empty")
	}
	serverPort, err := parseRequiredPort("server_port", sec["server_port"], 3017)
	if err != nil {
		return ClientConfig{}, err
	}

	localAddr, err := parseOptionalListenAddr("local_addr", "local_port", sec["local_addr"], sec["local_port"], "127.0.0.1")
	if err != nil {
		return ClientConfig{}, err
	}
	redirAddr, err := parseOptionalListenAddr("redir_addr", "redir_port", sec["redir_addr"], sec["redir_port"], "0.0.0.0")
	if err != nil {
		return ClientConfig{}, err
	}

	cfg := ClientConfig{
		LocalAddr:  localAddr,
		RedirAddr:  redirAddr,
		ServerAddr: net.JoinHostPort(serverHost, strconv.Itoa(serverPort)),
		Token:      sec["token"],
		PACFile:    filepath.Join(filepath.Dir(path), "pac.txt"),
	}
	if cfg.LocalAddr == "" && cfg.RedirAddr == "" {
		cfg.LocalAddr = "127.0.0.1:1080"
	}
	return cfg, nil
}

// NormalizeLocalAddr 将通配监听地址转换为本机可连接地址。
func NormalizeLocalAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "127.0.0.1:1080"
	}
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	if strings.HasPrefix(addr, "0.0.0.0:") {
		return "127.0.0.1" + addr[len("0.0.0.0"):]
	}
	if strings.HasPrefix(addr, "[::]:") {
		return "[::1]" + addr[len("[::]"):]
	}
	return addr
}

func LoadServerConfig(path string) (ServerConfig, error) {
	sec, err := parseCommonIni(path)
	if err != nil {
		return ServerConfig{}, err
	}

	bindHost, err := validateHostOnly("bind_addr", "bind_port", sec["bind_addr"])
	if err != nil {
		return ServerConfig{}, err
	}
	if bindHost == "" {
		bindHost = "0.0.0.0"
	}
	bindPort, err := parseRequiredPort("bind_port", sec["bind_port"], 3017)
	if err != nil {
		return ServerConfig{}, err
	}

	cfg := ServerConfig{
		BindAddr:    net.JoinHostPort(bindHost, strconv.Itoa(bindPort)),
		Token:       sec["token"],
		TLSCertFile: sec["tls_cert_file"],
		TLSKeyFile:  sec["tls_key_file"],
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return ServerConfig{}, errors.New("tls_cert_file and tls_key_file must be set together")
	}
	return cfg, nil
}

func validateHostOnly(addrKey, portKey, raw string) (string, error) {
	host := strings.TrimSpace(raw)
	if host == "" {
		return "", nil
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		inner := host[1 : len(host)-1]
		if net.ParseIP(inner) != nil {
			return inner, nil
		}
	}
	if net.ParseIP(host) != nil {
		return host, nil
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return "", fmt.Errorf("%s should not include port; use %s instead", addrKey, portKey)
	}
	return host, nil
}

func parseRequiredPort(key, raw string, defaultPort int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultPort, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid %s: %s", key, raw)
	}
	return port, nil
}

func parseOptionalListenAddr(addrKey, portKey, rawAddr, rawPort, defaultHost string) (string, error) {
	host, err := validateHostOnly(addrKey, portKey, rawAddr)
	if err != nil {
		return "", err
	}
	rawPort = strings.TrimSpace(rawPort)
	if rawPort == "" {
		if host != "" {
			return "", fmt.Errorf("%s must be set when %s is specified", portKey, addrKey)
		}
		return "", nil
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 0 || port > 65535 {
		return "", fmt.Errorf("invalid %s: %s", portKey, rawPort)
	}
	if port == 0 {
		return "", nil
	}
	if host == "" {
		host = defaultHost
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func parseCommonIni(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sections := make(map[string]map[string]string)
	current := ""

	scanner := bufio.NewScanner(strings.NewReader(string(b)))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSpace(line[1 : len(line)-1])
			if current == "" {
				return nil, fmt.Errorf("empty section name at line %d", lineNum)
			}
			if _, ok := sections[current]; !ok {
				sections[current] = make(map[string]string)
			}
			continue
		}
		if current == "" {
			return nil, fmt.Errorf("key outside section at line %d", lineNum)
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			return nil, fmt.Errorf("invalid ini syntax at line %d", lineNum)
		}
		key := strings.TrimSpace(line[:idx])
		val := stripInlineComment(strings.TrimSpace(line[idx+1:]))
		sections[current][key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	common, ok := sections["common"]
	if !ok {
		return nil, errors.New("missing [common] section")
	}
	return common, nil
}

func stripInlineComment(s string) string {
	for i := 0; i < len(s); i++ {
		if (s[i] == '#' || s[i] == ';') && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
			return strings.TrimSpace(s[:i])
		}
	}
	return s
}
