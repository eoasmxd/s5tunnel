package tunnel

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	cfg := ClientConfig{
		LocalAddr:  sec["local_addr"],
		RedirAddr:  sec["redir_addr"],
		ServerAddr: sec["server_addr"],
		Token:      sec["token"],
		PACFile:    filepath.Join(filepath.Dir(path), "pac.txt"),
	}
	if cfg.LocalAddr == "" && cfg.RedirAddr == "" {
		cfg.LocalAddr = "127.0.0.1:1080"
	}
	if cfg.ServerAddr == "" {
		return ClientConfig{}, errors.New("server_addr cannot be empty")
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
	cfg := ServerConfig{
		BindAddr:    ":3017",
		Token:       sec["token"],
		TLSCertFile: sec["tls_cert_file"],
		TLSKeyFile:  sec["tls_key_file"],
	}
	if v := sec["bind_addr"]; v != "" {
		cfg.BindAddr = v
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return ServerConfig{}, errors.New("tls_cert_file and tls_key_file must be set together")
	}
	return cfg, nil
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
