package tunnel

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"sync"
	"time"
)

const (
	defaultDialTimeout = 10 * time.Second
	defaultKeepAlive   = 30 * time.Second
)

// bufConn 将预读的 bufio.Reader 与底层 net.Conn 绑定，避免读取握手头时丢失后续字节。
type bufConn struct {
	net.Conn
	r *bufio.Reader
}

func NewBufConn(c net.Conn, r *bufio.Reader) net.Conn {
	return &bufConn{Conn: c, r: r}
}

func (c *bufConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

func ConnectServerWithTLS(addr string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   defaultDialTimeout,
		KeepAlive: defaultKeepAlive,
	}
	rawConn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	host, _, _ := net.SplitHostPort(addr)
	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
	}
	if net.ParseIP(host) == nil && host != "" {
		tlsCfg.ServerName = host
	}

	tlsConn := tls.Client(rawConn, tlsCfg)
	_ = tlsConn.SetDeadline(time.Now().Add(defaultDialTimeout))
	if err := tlsConn.Handshake(); err != nil {
		tlsConn.Close()
		return nil, err
	}
	_ = tlsConn.SetDeadline(time.Time{})
	return tlsConn, nil
}

// NewServerTLSConfig 加载指定的 TLS 证书，未指定时在内存中自动生成自签名证书。
func NewServerTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}, nil
}

func WriteHTTP404(w io.Writer) {
	const resp404 = "HTTP/1.1 404 Not Found\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: 13\r\nConnection: close\r\n\r\n404 Not Found"
	_, _ = io.WriteString(w, resp404)
}

func Relay(left, right net.Conn) error {
	var leftErr, rightErr error
	var wg sync.WaitGroup
	const wait = 5 * time.Second

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, leftErr = io.Copy(right, left)
		_ = right.SetReadDeadline(time.Now().Add(wait))
	}()

	_, rightErr = io.Copy(left, right)
	_ = left.SetReadDeadline(time.Now().Add(wait))
	wg.Wait()

	if leftErr != nil && !errors.Is(leftErr, os.ErrDeadlineExceeded) {
		return leftErr
	}
	if rightErr != nil && !errors.Is(rightErr, os.ErrDeadlineExceeded) {
		return rightErr
	}
	return nil
}

