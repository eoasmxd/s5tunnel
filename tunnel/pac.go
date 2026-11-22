package tunnel

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// HasPACFile 检查配置对应的 pac.txt 文件是否存在且为普通文件。
func HasPACFile(cfg ClientConfig) bool {
	if strings.TrimSpace(cfg.PACFile) == "" {
		return false
	}
	info, err := os.Stat(cfg.PACFile)
	return err == nil && !info.IsDir()
}

// PACURL 返回带内容摘要的本地 PAC 地址，避免客户端或系统缓存旧脚本。
func PACURL(cfg ClientConfig) (string, error) {
	proxyAddr := NormalizeLocalAddr(cfg.LocalAddr)
	script, err := buildPACScript(cfg.PACFile, proxyAddr)
	if err != nil {
		return "", err
	}
	sum := md5.Sum([]byte(script))
	return fmt.Sprintf("http://%s/pac?hash=%s", proxyAddr, hex.EncodeToString(sum[:])), nil
}

func buildPACScript(pacFile, proxyAddr string) (string, error) {
	content, err := readPACContent(pacFile)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("var __PROXY__ = 'PROXY %s; SOCKS5 %s; DIRECT';\r\n%s", proxyAddr, proxyAddr, content), nil
}

func readPACContent(pacFile string) (string, error) {
	if strings.TrimSpace(pacFile) == "" {
		return "", os.ErrNotExist
	}
	b, err := os.ReadFile(pacFile)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func handleLocalHTTP(w io.Writer, req *http.Request, cfg ClientConfig) {
	if req.URL.Path != "/pac" {
		WriteHTTP404(w)
		return
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		const resp405 = "HTTP/1.1 405 Method Not Allowed\r\nAllow: GET, HEAD\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: 18\r\nConnection: close\r\n\r\nMethod Not Allowed"
		_, _ = io.WriteString(w, resp405)
		return
	}

	body, err := buildPACScript(cfg.PACFile, NormalizeLocalAddr(cfg.LocalAddr))
	if err != nil {
		log.Printf("read pac file failed %s: %v", cfg.PACFile, err)
		if errors.Is(err, os.ErrNotExist) {
			WriteHTTP404(w)
		} else {
			const resp500 = "HTTP/1.1 500 Internal Server Error\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: 21\r\nConnection: close\r\n\r\nInternal Server Error"
			_, _ = io.WriteString(w, resp500)
		}
		return
	}

	header := fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Type: application/x-ns-proxy-autoconfig; charset=utf-8\r\nCache-Control: no-cache\r\nContent-Length: %d\r\nConnection: close\r\n\r\n",
		len(body),
	)
	if _, err := io.WriteString(w, header); err != nil {
		return
	}
	if req.Method == http.MethodGet {
		_, _ = io.WriteString(w, body)
	}
}
