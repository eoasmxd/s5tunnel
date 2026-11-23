//go:build windows

package main

import (
	_ "embed"
	"encoding/binary"
	"runtime"
	"syscall"
	"unsafe"
)

//go:embed icon.ico
var embeddedIconICO []byte

const (
	wmNull          = 0x0000
	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmEndSession    = 0x0016
	wmCommand       = 0x0111
	wmUser          = 0x0400
	wmTrayIcon      = wmUser + 1
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	mfString    = 0x00000000
	mfChecked   = 0x00000008
	mfSeparator = 0x00000800

	tpmLeftAlign   = 0x0000
	tpmBottomAlign = 0x0020
	tpmRightButton = 0x0002

	smCxSmIcon     = 49
	smCySmIcon     = 50
	idiApplication = 32512

	menuIDGlobal   = 1001
	menuIDPAC      = 1002
	menuIDDisabled = 1003
	menuIDQuit     = 1004
)

var (
	trayUser32DLL   = syscall.NewLazyDLL("user32.dll")
	trayShell32DLL  = syscall.NewLazyDLL("shell32.dll")
	trayKernel32DLL = syscall.NewLazyDLL("kernel32.dll")

	procGetModuleHandleW        = trayKernel32DLL.NewProc("GetModuleHandleW")
	procRegisterClassExW        = trayUser32DLL.NewProc("RegisterClassExW")
	procCreateWindowExW         = trayUser32DLL.NewProc("CreateWindowExW")
	procDestroyWindow           = trayUser32DLL.NewProc("DestroyWindow")
	procDefWindowProcW          = trayUser32DLL.NewProc("DefWindowProcW")
	procGetMessageW             = trayUser32DLL.NewProc("GetMessageW")
	procTranslateMessage        = trayUser32DLL.NewProc("TranslateMessage")
	procDispatchMessageW        = trayUser32DLL.NewProc("DispatchMessageW")
	procPostQuitMessage         = trayUser32DLL.NewProc("PostQuitMessage")
	procPostMessageW            = trayUser32DLL.NewProc("PostMessageW")
	procSetForegroundWindow     = trayUser32DLL.NewProc("SetForegroundWindow")
	procCreatePopupMenu         = trayUser32DLL.NewProc("CreatePopupMenu")
	procAppendMenuW             = trayUser32DLL.NewProc("AppendMenuW")
	procTrackPopupMenu          = trayUser32DLL.NewProc("TrackPopupMenu")
	procDestroyMenu             = trayUser32DLL.NewProc("DestroyMenu")
	procGetCursorPos            = trayUser32DLL.NewProc("GetCursorPos")
	procGetSystemMetrics        = trayUser32DLL.NewProc("GetSystemMetrics")
	procCreateIconFromResource  = trayUser32DLL.NewProc("CreateIconFromResourceEx")
	procLoadIconW               = trayUser32DLL.NewProc("LoadIconW")
	procDestroyIcon             = trayUser32DLL.NewProc("DestroyIcon")
	procRegisterWindowMessageW  = trayUser32DLL.NewProc("RegisterWindowMessageW")
	procShellNotifyIconW        = trayShell32DLL.NewProc("Shell_NotifyIconW")
	procMessageBoxW             = trayUser32DLL.NewProc("MessageBoxW")
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  uintptr
	lpszClassName uintptr
	hIconSm       uintptr
}

type winPoint struct {
	x int32
	y int32
}

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      winPoint
}

// notifyIconDataW 对齐 Windows Shell v5 (V2) 的 NOTIFYICONDATAW 结构。
type notifyIconDataW struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
}

var activeTray *trayApp

type trayApp struct {
	hwnd              uintptr
	hIcon             uintptr
	customIcon        bool
	taskbarCreatedMsg uint32
	mode              proxyMode
	tip               string
	hasPAC            func() bool
	onModeChange      func(proxyMode) proxyMode
	onExit            func()
	cleanedUp         bool
}

func newTrayApp(initialMode proxyMode, tip string, hasPAC func() bool, onModeChange func(proxyMode) proxyMode, onExit func()) (*trayApp, error) {
	t := &trayApp{
		mode:         initialMode,
		tip:          tip,
		hasPAC:       hasPAC,
		onModeChange: onModeChange,
		onExit:       onExit,
	}
	activeTray = t

	hInst, _, _ := procGetModuleHandleW.Call(0)
	t.hIcon, t.customIcon = loadEmbeddedIcon()

	className, err := syscall.UTF16PtrFromString("S5TunnelTrayWindow")
	if err != nil {
		return nil, err
	}
	wndProcCb := syscall.NewCallback(trayWndProc)

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   wndProcCb,
		hInstance:     hInst,
		hIcon:         t.hIcon,
		lpszClassName: uintptr(unsafe.Pointer(className)),
	}
	if ret, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); ret == 0 {
		return nil, callErr
	}

	windowTitle, _ := syscall.UTF16PtrFromString("S5Tunnel")
	hwnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		0,
		0, 0, 0, 0,
		0, 0, hInst, 0,
	)
	runtime.KeepAlive(className)
	runtime.KeepAlive(windowTitle)
	if hwnd == 0 {
		return nil, callErr
	}
	t.hwnd = hwnd

	taskbarMsgName, _ := syscall.UTF16PtrFromString("TaskbarCreated")
	msgID, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(taskbarMsgName)))
	runtime.KeepAlive(taskbarMsgName)
	t.taskbarCreatedMsg = uint32(msgID)

	t.notifyIcon(nimAdd)
	return t, nil
}

func (t *trayApp) run() {
	var m winMsg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (t *trayApp) updateState(mode proxyMode, tip string) {
	t.mode = mode
	t.tip = tip
	t.notifyIcon(nimModify)
}

func (t *trayApp) notifyIcon(action uint32) {
	nid := notifyIconDataW{
		cbSize:           uint32(unsafe.Sizeof(notifyIconDataW{})),
		hWnd:             t.hwnd,
		uID:              1,
		uFlags:           nifMessage | nifIcon | nifTip,
		uCallbackMessage: wmTrayIcon,
		hIcon:            t.hIcon,
	}
	setUTF16Fixed(&nid.szTip, t.tip)
	procShellNotifyIconW.Call(uintptr(action), uintptr(unsafe.Pointer(&nid)))
	runtime.KeepAlive(&nid)
}

func (t *trayApp) pacEnabled() bool {
	return t.hasPAC != nil && t.hasPAC()
}

func (t *trayApp) showContextMenu() {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	appendMenuItem(hMenu, mfString|checkedFlag(t.mode == modeGlobal), menuIDGlobal, "全局模式")
	if t.pacEnabled() {
		appendMenuItem(hMenu, mfString|checkedFlag(t.mode == modePAC), menuIDPAC, "PAC 模式")
	}
	appendMenuItem(hMenu, mfString|checkedFlag(t.mode == modeDisabled), menuIDDisabled, "禁用")
	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
	appendMenuItem(hMenu, mfString, menuIDQuit, "退出")

	var pt winPoint
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(t.hwnd)
	procTrackPopupMenu.Call(
		hMenu,
		tpmLeftAlign|tpmBottomAlign|tpmRightButton,
		uintptr(pt.x),
		uintptr(pt.y),
		0,
		t.hwnd,
		0,
	)
	procPostMessageW.Call(t.hwnd, wmNull, 0, 0)
}

func (t *trayApp) cleanup() {
	if t.cleanedUp {
		return
	}
	t.cleanedUp = true

	if t.onExit != nil {
		t.onExit()
	}
	nid := notifyIconDataW{
		cbSize: uint32(unsafe.Sizeof(notifyIconDataW{})),
		hWnd:   t.hwnd,
		uID:    1,
	}
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	runtime.KeepAlive(&nid)

	if t.customIcon && t.hIcon != 0 {
		procDestroyIcon.Call(t.hIcon)
		t.hIcon = 0
	}
}

func trayWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	t := activeTray
	if t != nil {
		if t.taskbarCreatedMsg != 0 && msg == t.taskbarCreatedMsg {
			t.notifyIcon(nimAdd)
			return 0
		}

		switch msg {
		case wmTrayIcon:
			event := uint32(lParam)
			if event == wmRButtonUp {
				t.showContextMenu()
				return 0
			}
			if event == wmLButtonDblClk && t.onModeChange != nil {
				next := modeGlobal
				if t.pacEnabled() {
					next = modePAC
				}
				if t.mode != modeDisabled {
					next = modeDisabled
				}
				t.mode = t.onModeChange(next)
				return 0
			}
		case wmCommand:
			cmdID := uint32(wParam) & 0xFFFF
			switch cmdID {
			case menuIDGlobal:
				if t.onModeChange != nil {
					t.mode = t.onModeChange(modeGlobal)
				}
			case menuIDPAC:
				if t.onModeChange != nil {
					t.mode = t.onModeChange(modePAC)
				}
			case menuIDDisabled:
				if t.onModeChange != nil {
					t.mode = t.onModeChange(modeDisabled)
				}
			case menuIDQuit:
				t.cleanup()
				procDestroyWindow.Call(hwnd)
			}
			return 0
		case wmClose:
			t.cleanup()
			procDestroyWindow.Call(hwnd)
			return 0
		case wmEndSession:
			if wParam != 0 {
				t.cleanup()
			}
			return 0
		case wmDestroy:
			procPostQuitMessage.Call(0)
			return 0
		}
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func loadEmbeddedIcon() (uintptr, bool) {
	cx, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
	cy, _, _ := procGetSystemMetrics.Call(smCySmIcon)
	if cx == 0 {
		cx = 16
	}
	if cy == 0 {
		cy = 16
	}

	if len(embeddedIconICO) >= 6 &&
		binary.LittleEndian.Uint16(embeddedIconICO[0:2]) == 0 &&
		binary.LittleEndian.Uint16(embeddedIconICO[2:4]) == 1 {
		count := int(binary.LittleEndian.Uint16(embeddedIconICO[4:6]))
		bestIdx := -1
		bestSize := 0
		bestBits := 0
		targetSize := int(cx)

		for i := 0; i < count; i++ {
			off := 6 + i*16
			if off+16 > len(embeddedIconICO) {
				break
			}
			w := int(embeddedIconICO[off])
			if w == 0 {
				w = 256
			}
			bits := int(binary.LittleEndian.Uint16(embeddedIconICO[off+6 : off+8]))
			bytesInRes := int(binary.LittleEndian.Uint32(embeddedIconICO[off+8 : off+12]))
			imgOff := int(binary.LittleEndian.Uint32(embeddedIconICO[off+12 : off+16]))
			if bytesInRes <= 0 || imgOff < 0 || imgOff+bytesInRes > len(embeddedIconICO) {
				continue
			}

			if bestIdx < 0 {
				bestIdx, bestSize, bestBits = i, w, bits
				continue
			}

			curMatch := w >= targetSize
			bestMatch := bestSize >= targetSize
			if (curMatch && !bestMatch) ||
				(curMatch && bestMatch && (w < bestSize || (w == bestSize && bits > bestBits))) ||
				(!curMatch && !bestMatch && (w > bestSize || (w == bestSize && bits > bestBits))) {
				bestIdx, bestSize, bestBits = i, w, bits
			}
		}

		if bestIdx >= 0 {
			off := 6 + bestIdx*16
			bytesInRes := int(binary.LittleEndian.Uint32(embeddedIconICO[off+8 : off+12]))
			imgOff := int(binary.LittleEndian.Uint32(embeddedIconICO[off+12 : off+16]))
			imgData := embeddedIconICO[imgOff : imgOff+bytesInRes]
			hIcon, _, _ := procCreateIconFromResource.Call(
				uintptr(unsafe.Pointer(&imgData[0])),
				uintptr(len(imgData)),
				1,
				0x00030000,
				cx,
				cy,
				0,
			)
			runtime.KeepAlive(imgData)
			if hIcon != 0 {
				return hIcon, true
			}
		}
	}

	hDefault, _, _ := procLoadIconW.Call(0, idiApplication)
	return hDefault, false
}

func appendMenuItem(hMenu uintptr, flags uint32, id uint32, label string) {
	ptr, err := syscall.UTF16PtrFromString(label)
	if err != nil {
		return
	}
	procAppendMenuW.Call(hMenu, uintptr(flags), uintptr(id), uintptr(unsafe.Pointer(ptr)))
	runtime.KeepAlive(ptr)
}

func checkedFlag(checked bool) uint32 {
	if checked {
		return mfChecked
	}
	return 0
}

func setUTF16Fixed(dst *[128]uint16, s string) {
	u16, err := syscall.UTF16FromString(s)
	if err != nil {
		return
	}
	n := len(u16)
	if n > len(dst) {
		n = len(dst)
	}
	copy(dst[:], u16[:n])
	dst[len(dst)-1] = 0
}

func showErrorBox(title, msg string) {
	tPtr, _ := syscall.UTF16PtrFromString(title)
	mPtr, _ := syscall.UTF16PtrFromString(msg)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(mPtr)), uintptr(unsafe.Pointer(tPtr)), 0x00000010)
	runtime.KeepAlive(tPtr)
	runtime.KeepAlive(mPtr)
}

