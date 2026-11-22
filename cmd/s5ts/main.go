package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/eoasmxd/s5tunnel/tunnel"
)

const handshakeTimeout = 10 * time.Second

func main() {
	confPath := flag.String("c", "./s5ts.ini", "config file path")
	showVersion := flag.Bool("v", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Println(tunnel.Version())
		os.Exit(0)
	}

	cfg, err := tunnel.LoadServerConfig(*confPath)
	if err != nil {
		log.Fatalf("load config failed %s: %v", *confPath, err)
	}

	tlsCfg, err := tunnel.NewServerTLSConfig(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		log.Fatalf("init TLS config failed: %v", err)
	}

	ln, err := tls.Listen("tcp", cfg.BindAddr, tlsCfg)
	if err != nil {
		log.Fatalf("listen failed %s: %v", cfg.BindAddr, err)
	}
	log.Printf("server listening on %s", cfg.BindAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		go handleServer(conn, cfg)
	}
}

func handleServer(conn net.Conn, cfg tunnel.ServerConfig) {
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		tunnel.WriteHTTP404(conn)
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	if req.Method != http.MethodConnect || req.Host == "" {
		tunnel.WriteHTTP404(conn)
		return
	}

	if !tunnel.CheckAuth(cfg.Token, req.Header.Get("X-Timestamp"), req.Header.Get("X-Auth")) {
		log.Printf("auth failed: %s", conn.RemoteAddr())
		tunnel.WriteHTTP404(conn)
		return
	}

	targetConn, err := net.DialTimeout("tcp", req.Host, handshakeTimeout)
	if err != nil {
		log.Printf("connect target failed %s: %v", req.Host, err)
		return
	}
	defer targetConn.Close()

	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		log.Printf("send 200 response failed: %v", err)
		return
	}

	log.Printf("proxy %s <-> %s", conn.RemoteAddr(), req.Host)
	if err := tunnel.Relay(tunnel.NewBufConn(conn, br), targetConn); err != nil {
		log.Printf("relay error: %v", err)
	}
}

