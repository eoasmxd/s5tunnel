package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"github.com/eoasmxd/s5tunnel/tunnel"
)

func main() {
	confPath := flag.String("c", "./s5tc.ini", "config file path")
	showVersion := flag.Bool("v", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Println(tunnel.Version())
		os.Exit(0)
	}

	cfg, err := tunnel.LoadClientConfig(*confPath)
	if err != nil {
		log.Fatalf("load config failed %s: %v", *confPath, err)
	}

	var wg sync.WaitGroup
	if cfg.LocalAddr != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var pacLog string
			if tunnel.HasPACFile(cfg) {
				pacLog = fmt.Sprintf("pac available at http://%s/pac", tunnel.NormalizeLocalAddr(cfg.LocalAddr))
			}
			serveTCP(cfg.LocalAddr, "mixed", pacLog, func(c net.Conn) {
				tunnel.HandleMixedProxy(c, cfg)
			})
		}()
	}
	if cfg.RedirAddr != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			serveTCP(cfg.RedirAddr, "redir", "", func(c net.Conn) {
				handleRedir(c, cfg)
			})
		}()
	}
	wg.Wait()
}

func serveTCP(listenAddr, mode, extraLog string, handler func(net.Conn)) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("%s listen failed %s: %v", mode, listenAddr, err)
	}
	log.Printf("%s listening on %s", mode, listenAddr)
	if extraLog != "" {
		log.Print(extraLog)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("%s accept error: %v", mode, err)
			continue
		}
		go handler(conn)
	}
}

func handleRedir(conn net.Conn, cfg tunnel.ClientConfig) {
	defer conn.Close()

	target, err := getOrigDst(conn)
	if err != nil {
		log.Printf("redir get original dst failed: %v", err)
		return
	}
	tunnel.ForwardTarget(conn, target, cfg)
}
