//go:build !linux || 386

package main

import (
	"errors"
	"net"
)

func getOrigDst(conn net.Conn) (string, error) {
	return "", errors.New("redir is only supported on linux (non-386)")
}

