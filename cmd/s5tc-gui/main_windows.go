//go:build windows

package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/eoasmxd/s5tunnel/tunnel"
)

const (
	errorAlreadyExists = syscall.Errno(183)
	configFileName     = "s5tc.ini"
	stateFileName      = "s5tc-gui.json"
)

var (
	mainKernel32DLL = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutex = mainKernel32DLL.NewProc("CreateMutexW")
	procCloseHandle = mainKernel32DLL.NewProc("CloseHandle")
)

func main() {
	runtime.LockOSThread()

	exePath, err := os.Executable()
	if err != nil {
		showErrorBox("S5Tunnel 错误", fmt.Sprintf("获取程序路径失败: %v", err))
		return
	}
	exeDir := filepath.Dir(exePath)
	if err := os.Chdir(exeDir); err != nil {
		showErrorBox("S5Tunnel 错误", fmt.Sprintf("切换工作目录失败: %v", err))
		return
	}

	mutexHandle, alreadyRunning := acquireSingleInstanceMutex(exeDir)
	if alreadyRunning {
		showErrorBox("S5Tunnel", "当前目录下已有一个 S5Tunnel 实例正在运行，请查看系统托盘。")
		return
	}
	if mutexHandle != 0 {
		defer procCloseHandle.Call(mutexHandle)
	}

	cfg, err := tunnel.LoadClientConfig(configFileName)
	if err != nil {
		showErrorBox("S5Tunnel 错误", fmt.Sprintf("读取配置文件 %s 失败: %v", configFileName, err))
		return
	}
	if cfg.LocalAddr == "" {
		cfg.LocalAddr = "127.0.0.1:1080"
	}

	ln, err := net.Listen("tcp", cfg.LocalAddr)
	if err != nil {
		showErrorBox("S5Tunnel 错误", fmt.Sprintf("本地端口监听失败 %s: %v", cfg.LocalAddr, err))
		return
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go tunnel.HandleMixedProxy(conn, cfg)
		}
	}()

	proxyEndpoint := tunnel.NormalizeLocalAddr(cfg.LocalAddr)
	sysProxy := &sysProxyManager{}
	applyMode := func(mode proxyMode) error {
		var pacURL string
		if mode == modePAC {
			var err error
			pacURL, err = tunnel.PACURL(cfg)
			if err != nil {
				return err
			}
		}
		return sysProxy.apply(mode, proxyEndpoint, pacURL)
	}

	currentMode := resolveInitialMode(tunnel.HasPACFile(cfg), stateFileName)
	if err := applyMode(currentMode); err != nil {
		showErrorBox("S5Tunnel 错误", fmt.Sprintf("设置系统代理失败: %v", err))
		currentMode = modeDisabled
	}

	var tray *trayApp
	onModeChange := func(next proxyMode) proxyMode {
		if err := applyMode(next); err != nil {
			showErrorBox("S5Tunnel 错误", fmt.Sprintf("切换系统代理失败: %v", err))
			return currentMode
		}
		currentMode = next
		_ = saveProxyMode(stateFileName, currentMode)
		if tray != nil {
			tray.updateState(currentMode, formatTrayTip(currentMode, cfg))
		}
		return currentMode
	}

	onExit := func() {
		_ = sysProxy.apply(modeDisabled, "", "")
		_ = ln.Close()
	}

	tray, err = newTrayApp(currentMode, formatTrayTip(currentMode, cfg), func() bool {
		return tunnel.HasPACFile(cfg)
	}, onModeChange, onExit)
	if err != nil {
		onExit()
		showErrorBox("S5Tunnel 错误", fmt.Sprintf("初始化系统托盘失败: %v", err))
		return
	}

	tray.run()
}

func acquireSingleInstanceMutex(dir string) (uintptr, bool) {
	sum := md5.Sum([]byte(strings.ToLower(filepath.Clean(dir))))
	name := "Local\\S5TunnelGUI_" + hex.EncodeToString(sum[:])
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, false
	}

	handle, _, callErr := procCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(namePtr)))
	runtime.KeepAlive(namePtr)
	if callErr == errorAlreadyExists {
		if handle != 0 {
			procCloseHandle.Call(handle)
		}
		return 0, true
	}
	return handle, false
}

func formatTrayTip(mode proxyMode, cfg tunnel.ClientConfig) string {
	var modeName string
	switch mode {
	case modeGlobal:
		modeName = "全局"
	case modePAC:
		modeName = "PAC"
	default:
		modeName = "已禁用"
	}
	return fmt.Sprintf("S5Tunnel %s (%s)\n本地: %s\n服务端: %s", tunnel.Version(), modeName, cfg.LocalAddr, cfg.ServerAddr)
}

