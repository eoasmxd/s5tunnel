//go:build linux && !386

package main

import (
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"syscall"
	"unsafe"
)

const (
	soOriginalDst     = 80
	ip6tSoOriginalDst = 80
)

func getOrigDst(conn net.Conn) (string, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return "", errors.New("not a TCP connection")
	}
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return "", err
	}

	isIPv4 := tcpConn.LocalAddr().(*net.TCPAddr).IP.To4() != nil
	var target string
	var sockErr error

	err = rawConn.Control(func(fd uintptr) {
		if isIPv4 {
			var addr syscall.RawSockaddrInet4
			size := uint32(unsafe.Sizeof(addr))
			_, _, errno := syscall.Syscall6(
				syscall.SYS_GETSOCKOPT,
				fd,
				syscall.SOL_IP,
				soOriginalDst,
				uintptr(unsafe.Pointer(&addr)),
				uintptr(unsafe.Pointer(&size)),
				0,
			)
			if errno != 0 {
				sockErr = errno
				return
			}
			port := binary.BigEndian.Uint16((*[2]byte)(unsafe.Pointer(&addr.Port))[:])
			target = net.JoinHostPort(net.IP(addr.Addr[:]).String(), strconv.Itoa(int(port)))
			return
		}

		var addr syscall.RawSockaddrInet6
		size := uint32(unsafe.Sizeof(addr))
		_, _, errno := syscall.Syscall6(
			syscall.SYS_GETSOCKOPT,
			fd,
			syscall.SOL_IPV6,
			ip6tSoOriginalDst,
			uintptr(unsafe.Pointer(&addr)),
			uintptr(unsafe.Pointer(&size)),
			0,
		)
		if errno != 0 {
			sockErr = errno
			return
		}
		port := binary.BigEndian.Uint16((*[2]byte)(unsafe.Pointer(&addr.Port))[:])
		target = net.JoinHostPort(net.IP(addr.Addr[:]).String(), strconv.Itoa(int(port)))
	})
	if err != nil {
		return "", err
	}
	if sockErr != nil {
		return "", sockErr
	}
	return target, nil
}

