//go:build windows

package main

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

type proxyMode int

const (
	modeDisabled proxyMode = iota
	modePAC
	modeGlobal
)

const (
	internetOptionRefresh             = 37
	internetOptionSettingsChanged     = 39
	internetOptionPerConnectionOption = 75

	internetPerConnFlags         = 1
	internetPerConnProxyServer   = 2
	internetPerConnProxyBypass   = 3
	internetPerConnAutoConfigURL = 4

	proxyTypeDirect       = 0x00000001
	proxyTypeProxy        = 0x00000002
	proxyTypeAutoProxyURL = 0x00000004

	defaultBypassList = "localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;<local>"
)

var (
	wininetDLL          = syscall.NewLazyDLL("wininet.dll")
	wininetQueryOptionW = wininetDLL.NewProc("InternetQueryOptionW")
	wininetSetOptionW   = wininetDLL.NewProc("InternetSetOptionW")

	wininetKernel32DLL = syscall.NewLazyDLL("kernel32.dll")
	wininetGlobalFree  = wininetKernel32DLL.NewProc("GlobalFree")
	wininetLstrlenW    = wininetKernel32DLL.NewProc("lstrlenW")
	wininetLstrcpynW   = wininetKernel32DLL.NewProc("lstrcpynW")
)

// internetPerConnOption 对齐 Win32/Win64 的 INTERNET_PER_CONN_OPTION 结构体布局。
type internetPerConnOption struct {
	dwOption uint32
	value    uint64
}

type internetPerConnOptionList struct {
	dwSize        uint32
	pszConnection uintptr
	dwOptionCount uint32
	dwOptionError uint32
	pOptions      uintptr
}

// sysProxyManager 通过 WinINet API 管理系统代理设置并维护原始状态备份。
type sysProxyManager struct {
	mu           sync.Mutex
	hasBackup    bool
	origFlags    uint32
	origServer   string
	origBypass   string
	origAutoConf string
}

func (m *sysProxyManager) apply(mode proxyMode, proxyAddr, pacURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if mode == modeDisabled {
		if !m.hasBackup {
			return nil
		}
		if err := setWinINetOptions(m.origFlags, m.origServer, m.origBypass, m.origAutoConf, true); err != nil {
			return err
		}
		m.hasBackup = false
		return nil
	}

	if !m.hasBackup {
		flags, server, bypass, autoConf, ok := queryWinINetSettings()
		if !ok {
			flags = proxyTypeDirect
		}
		m.origFlags = flags
		m.origServer = server
		m.origBypass = bypass
		m.origAutoConf = autoConf
		m.hasBackup = true
	}

	switch mode {
	case modePAC:
		return setWinINetOptions(proxyTypeDirect|proxyTypeAutoProxyURL, "", "", pacURL, false)
	case modeGlobal:
		return setWinINetOptions(proxyTypeDirect|proxyTypeProxy, proxyAddr, defaultBypassList, "", true)
	default:
		return nil
	}
}

func queryWinINetSettings() (flags uint32, proxyServer, bypass, pacURL string, ok bool) {
	options := [4]internetPerConnOption{
		{dwOption: internetPerConnFlags},
		{dwOption: internetPerConnProxyServer},
		{dwOption: internetPerConnProxyBypass},
		{dwOption: internetPerConnAutoConfigURL},
	}

	list := internetPerConnOptionList{
		dwSize:        uint32(unsafe.Sizeof(internetPerConnOptionList{})),
		pszConnection: 0,
		dwOptionCount: 4,
		pOptions:      uintptr(unsafe.Pointer(&options[0])),
	}
	bufSize := list.dwSize

	ret, _, _ := wininetQueryOptionW.Call(
		0,
		internetOptionPerConnectionOption,
		uintptr(unsafe.Pointer(&list)),
		uintptr(unsafe.Pointer(&bufSize)),
	)
	runtime.KeepAlive(&options)
	runtime.KeepAlive(&list)

	if ret == 0 {
		return proxyTypeDirect, "", "", "", false
	}

	flags = uint32(options[0].value)
	proxyServer = readAndFreeOptionString(uintptr(options[1].value))
	bypass = readAndFreeOptionString(uintptr(options[2].value))
	pacURL = readAndFreeOptionString(uintptr(options[3].value))
	return flags, proxyServer, bypass, pacURL, true
}

func readAndFreeOptionString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	defer wininetGlobalFree.Call(ptr)

	n, _, _ := wininetLstrlenW.Call(ptr)
	if n == 0 {
		return ""
	}
	if n > 4096 {
		n = 4096
	}
	buf := make([]uint16, n+1)
	wininetLstrcpynW.Call(uintptr(unsafe.Pointer(&buf[0])), ptr, uintptr(len(buf)))
	runtime.KeepAlive(buf)
	return syscall.UTF16ToString(buf)
}

func setWinINetOptions(flags uint32, proxyServer, bypass, pacURL string, includeProxyFields bool) error {
	var keepAliveStrs [][]uint16
	toUTF16Val := func(s string) (uint64, error) {
		if s == "" {
			return 0, nil
		}
		u16, err := syscall.UTF16FromString(s)
		if err != nil {
			return 0, err
		}
		keepAliveStrs = append(keepAliveStrs, u16)
		return uint64(uintptr(unsafe.Pointer(&u16[0]))), nil
	}

	pacVal, err := toUTF16Val(pacURL)
	if err != nil {
		return err
	}

	var optionsBuf [4]internetPerConnOption
	count := uint32(2)
	optionsBuf[0] = internetPerConnOption{dwOption: internetPerConnFlags, value: uint64(flags)}

	if includeProxyFields {
		count = 4
		serverVal, err := toUTF16Val(proxyServer)
		if err != nil {
			return err
		}
		bypassVal, err := toUTF16Val(bypass)
		if err != nil {
			return err
		}
		optionsBuf[1] = internetPerConnOption{dwOption: internetPerConnProxyServer, value: serverVal}
		optionsBuf[2] = internetPerConnOption{dwOption: internetPerConnProxyBypass, value: bypassVal}
		optionsBuf[3] = internetPerConnOption{dwOption: internetPerConnAutoConfigURL, value: pacVal}
	} else {
		optionsBuf[1] = internetPerConnOption{dwOption: internetPerConnAutoConfigURL, value: pacVal}
	}

	list := internetPerConnOptionList{
		dwSize:        uint32(unsafe.Sizeof(internetPerConnOptionList{})),
		pszConnection: 0,
		dwOptionCount: count,
		pOptions:      uintptr(unsafe.Pointer(&optionsBuf[0])),
	}

	ret, _, callErr := wininetSetOptionW.Call(
		0,
		internetOptionPerConnectionOption,
		uintptr(unsafe.Pointer(&list)),
		uintptr(list.dwSize),
	)
	runtime.KeepAlive(keepAliveStrs)
	runtime.KeepAlive(&optionsBuf)
	runtime.KeepAlive(&list)

	if ret == 0 {
		return callErr
	}

	wininetSetOptionW.Call(0, internetOptionSettingsChanged, 0, 0)
	wininetSetOptionW.Call(0, internetOptionRefresh, 0, 0)
	return nil
}

