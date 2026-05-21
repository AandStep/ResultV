// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

//go:build windows

package system

import (
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	bridgeClass = "ResultVSingletonBridgeCls"
	bridgeTitle = "ResultVSingletonBridgeWnd"
	mutexName   = `Global\ResultVAppSingletonMutex_v2`
	wmCopydata  = 0x004A
	wmQuit      = 0x0012
	copyMagic   = 0x52505349

	WS_POPUP         = 0x80000000
	WS_EX_TOOLWINDOW = 0x00000080
	WS_EX_NOACTIVATE = 0x08000000
	swHide           = 0
	CS_VREDRAW       = 0x0001
	CS_HREDRAW       = 0x0002
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type copyDataStruct struct {
	dwData uintptr
	cbData uint32
	lpData uintptr
}

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

var (
	dllUser32   = windows.NewLazyDLL("user32.dll")
	dllKernel32 = windows.NewLazyDLL("kernel32.dll")

	procRegisterClassExW   = dllUser32.NewProc("RegisterClassExW")
	procCreateWindowExW    = dllUser32.NewProc("CreateWindowExW")
	procDefWindowProcW     = dllUser32.NewProc("DefWindowProcW")
	procGetMessageW        = dllUser32.NewProc("GetMessageW")
	procTranslateMessage   = dllUser32.NewProc("TranslateMessage")
	procDispatchMessageW   = dllUser32.NewProc("DispatchMessageW")
	procDestroyWindow      = dllUser32.NewProc("DestroyWindow")
	procPostThreadMessageW = dllUser32.NewProc("PostThreadMessageW")
	procFindWindowW        = dllUser32.NewProc("FindWindowW")
	procSendMessageW       = dllUser32.NewProc("SendMessageW")
	procShowWindow         = dllUser32.NewProc("ShowWindow")
	procUnregisterClassW   = dllUser32.NewProc("UnregisterClassW")
	procGetModuleHandleW   = dllKernel32.NewProc("GetModuleHandleW")
)

func InitSingletonMessenger(onActivate func(payload string)) (cleanup func()) {
	mxName, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return func() {}
	}
	_, err = windows.CreateMutex(nil, false, mxName)
	secondInstance := err == windows.ERROR_ALREADY_EXISTS || err == windows.ERROR_ACCESS_DENIED
	if secondInstance {
		notifyRunningInstance(extractDeepLinkArg(os.Args))
		os.Exit(0)
	}

	var bridgeThreadID atomic.Uint32
	ready := make(chan struct{})
	exited := make(chan struct{})

	classPtr, _ := windows.UTF16PtrFromString(bridgeClass)
	titlePtr, _ := windows.UTF16PtrFromString(bridgeTitle)

	go func() {
		defer close(exited)

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		bridgeThreadID.Store(windows.GetCurrentThreadId())

		mod, _, _ := procGetModuleHandleW.Call(0)

		cb := syscall.NewCallback(func(hwnd, uMsg, wParam, lParam uintptr) uintptr {
			if uint32(uMsg) == wmCopydata {
				cd := (*copyDataStruct)(unsafe.Pointer(lParam))
				if cd != nil && cd.dwData == copyMagic && onActivate != nil {
					payload := ""
					if cd.cbData > 0 && cd.lpData != 0 {
						buf := make([]byte, cd.cbData)
						src := unsafe.Slice((*byte)(unsafe.Pointer(cd.lpData)), cd.cbData)
						copy(buf, src)
						payload = string(buf)
					}
					go onActivate(payload)
				}
				return 0
			}
			r, _, _ := procDefWindowProcW.Call(hwnd, uMsg, wParam, lParam)
			return r
		})

		wcx := wndClassEx{
			Size:       uint32(unsafe.Sizeof(wndClassEx{})),
			Style:      CS_HREDRAW | CS_VREDRAW,
			WndProc:    cb,
			Instance:   mod,
			Background: 6,
			ClassName:  classPtr,
		}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wcx)))

		hwnd, _, _ := procCreateWindowExW.Call(
			uintptr(WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE),
			uintptr(unsafe.Pointer(classPtr)),
			uintptr(unsafe.Pointer(titlePtr)),
			uintptr(WS_POPUP),
			0, 0, 0, 0,
			0,
			0,
			mod,
			0,
		)
		if hwnd != 0 {
			procShowWindow.Call(hwnd, uintptr(swHide))
		}

		close(ready)

		var m winMsg
		for {
			gm, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(gm) == 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}

		if hwnd != 0 {
			procDestroyWindow.Call(hwnd)
		}
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(classPtr)), mod)
	}()

	<-ready

	return func() {
		if tid := bridgeThreadID.Load(); tid != 0 {
			procPostThreadMessageW.Call(uintptr(tid), uintptr(wmQuit), 0, 0)
		}
		<-exited
	}
}

func notifyRunningInstance(payload string) {
	classPtr, err := windows.UTF16PtrFromString(bridgeClass)
	if err != nil {
		return
	}
	titlePtr, err := windows.UTF16PtrFromString(bridgeTitle)
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(classPtr)), uintptr(unsafe.Pointer(titlePtr)))
	if hwnd == 0 {
		return
	}
	var cds copyDataStruct
	cds.dwData = copyMagic
	if payload != "" {
		buf := []byte(payload)
		cds.cbData = uint32(len(buf))
		cds.lpData = uintptr(unsafe.Pointer(&buf[0]))
		procSendMessageW.Call(hwnd, uintptr(wmCopydata), 0, uintptr(unsafe.Pointer(&cds)))
		runtime.KeepAlive(buf)
		return
	}
	cds.cbData = 0
	cds.lpData = 0
	procSendMessageW.Call(hwnd, uintptr(wmCopydata), 0, uintptr(unsafe.Pointer(&cds)))
}

func extractDeepLinkArg(args []string) string {
	return ExtractDeepLinkArg(args)
}

// ExtractDeepLinkArg scans process args for the first resultv: URL.
func ExtractDeepLinkArg(args []string) string {
	const scheme = "resultv:"
	if len(args) <= 1 {
		return ""
	}
	for _, a := range args[1:] {
		s := strings.TrimSpace(a)
		if len(s) > len(scheme) && strings.EqualFold(s[:len(scheme)], scheme) {
			return s
		}
	}
	return ""
}
