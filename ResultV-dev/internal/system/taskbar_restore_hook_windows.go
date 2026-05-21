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
	"context"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const WailsWindowClassResultV = "ResultVMainWClass"

const (
	wmActivate   = 0x0006
	wmSysCommand = 0x0112
	scRestore    = 0xF120
	waInactive   = 0

	taskbarSubclassID = 0x52504741
)

var (
	dllComctl32Hook    = windows.NewLazyDLL("comctl32.dll")
	procSetSubclass    = dllComctl32Hook.NewProc("SetWindowSubclass")
	procRemoveSubclass = dllComctl32Hook.NewProc("RemoveWindowSubclass")
	procDefSubclass    = dllComctl32Hook.NewProc("DefSubclassProc")

	dllUser32Hook     = windows.NewLazyDLL("user32.dll")
	procEnumWindows   = dllUser32Hook.NewProc("EnumWindows")
	procGetClassNameW = dllUser32Hook.NewProc("GetClassNameW")
	procGetParent     = dllUser32Hook.NewProc("GetParent")

	enumSearchMu    sync.Mutex
	enumSearchClass string
	enumSearchPID   uint32
	enumSearchFound windows.HWND
	enumWindowsCB   = syscall.NewCallback(enumWindowsProc)
)

type TaskbarRestoreConfig struct {
	ClassName      string
	IsHiddenToTray func() bool
	OnRestore      func()
}

var taskbarHookCfg atomicTaskbarCfg

type atomicTaskbarCfg struct {
	mu sync.RWMutex
	p  *TaskbarRestoreConfig
}

func (a *atomicTaskbarCfg) store(c *TaskbarRestoreConfig) {
	a.mu.Lock()
	a.p = c
	a.mu.Unlock()
}

func (a *atomicTaskbarCfg) load() *TaskbarRestoreConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.p
}

func subclassProc(hWnd uintptr, msg uint32, wParam, lParam, uIdSubclass, dwRefData uintptr) uintptr {
	cfg := taskbarHookCfg.load()
	if cfg != nil && cfg.IsHiddenToTray != nil && cfg.IsHiddenToTray() && cfg.OnRestore != nil {
		switch msg {
		case wmActivate:
			if uint32(wParam)&0xFFFF != waInactive {
				go cfg.OnRestore()
			}
		case wmSysCommand:
			if wParam&0xFFF0 == scRestore&0xFFF0 {
				go cfg.OnRestore()
			}
		}
	}
	r, _, _ := procDefSubclass.Call(hWnd, uintptr(msg), wParam, lParam)
	return r
}

func findTopLevelMainHWND(className string) windows.HWND {
	if className == "" {
		return 0
	}
	enumSearchMu.Lock()
	enumSearchClass = className
	enumSearchPID = windows.GetCurrentProcessId()
	enumSearchFound = 0
	_, _, _ = procEnumWindows.Call(enumWindowsCB, 0)
	found := enumSearchFound
	enumSearchMu.Unlock()
	return found
}

func enumWindowsProc(hwnd, _ uintptr) uintptr {
	par, _, _ := procGetParent.Call(hwnd)
	if par != 0 {
		return 1
	}
	var pid uint32
	_, _ = windows.GetWindowThreadProcessId(windows.HWND(hwnd), &pid)
	if pid != enumSearchPID {
		return 1
	}
	var buf [256]uint16
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return 1
	}
	cls := windows.UTF16ToString(buf[:n])
	if cls == enumSearchClass {
		enumSearchFound = windows.HWND(hwnd)
		return 0
	}
	return 1
}

func StartTaskbarRestoreHook(ctx context.Context, cfg TaskbarRestoreConfig) (cleanup func()) {
	if cfg.ClassName == "" || cfg.OnRestore == nil || cfg.IsHiddenToTray == nil {
		return func() {}
	}
	cfgCopy := cfg
	taskbarHookCfg.store(&cfgCopy)

	scanCtx, cancelScan := context.WithCancel(ctx)
	done := make(chan struct{})
	var mu sync.Mutex
	var hwnd windows.HWND
	var subFn uintptr
	installed := false

	go func() {
		defer close(done)
		t := time.NewTicker(120 * time.Millisecond)
		defer t.Stop()
		sub := syscall.NewCallback(subclassProc)
		for {
			select {
			case <-scanCtx.Done():
				return
			case <-t.C:
				h := findTopLevelMainHWND(cfg.ClassName)
				if h == 0 {
					continue
				}
				ok, _, _ := procSetSubclass.Call(uintptr(h), sub, uintptr(taskbarSubclassID), 0)
				if ok == 0 {
					continue
				}
				mu.Lock()
				hwnd = h
				subFn = sub
				installed = true
				mu.Unlock()
				return
			}
		}
	}()

	return func() {
		cancelScan()
		<-done
		mu.Lock()
		h := hwnd
		sf := subFn
		ok := installed
		mu.Unlock()
		if ok && h != 0 && sf != 0 {
			_, _, _ = procRemoveSubclass.Call(uintptr(h), sf, uintptr(taskbarSubclassID))
		}
		taskbarHookCfg.store(nil)
	}
}
