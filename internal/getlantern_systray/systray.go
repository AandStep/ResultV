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

package systray

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/golog"
)

// sdbgLogShim writes a timestamped diagnostic line to the same shutdown-trace
// file used by the main package's sdbg helper. This vendored package can't
// import internal/sdbg (it's a separate Go module via go.mod replace), so we
// duplicate the minimal logic here. Only used for diagnosing the macOS quit
// path; safe to remove once the issue is resolved.
var (
	sdbgShimMu     sync.Mutex
	sdbgShimFile   *os.File
	sdbgShimOpened bool
)

func sdbgLogShim(format string, args ...interface{}) {
	sdbgShimMu.Lock()
	defer sdbgShimMu.Unlock()
	if !sdbgShimOpened {
		sdbgShimOpened = true
		home, _ := os.UserHomeDir()
		dir := filepath.Join(home, "Library", "Logs", "ResultV")
		_ = os.MkdirAll(dir, 0o755)
		path := filepath.Join(dir, "shutdown-trace.log")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			sdbgShimFile = f
		} else {
			fmt.Fprintf(os.Stderr, "[sdbgShim] cannot open trace: %v\n", err)
		}
	}
	msg := fmt.Sprintf(format, args...)
	stamp := time.Now().Format("15:04:05.000000")
	line := fmt.Sprintf("[%s pid=%d] %s\n", stamp, os.Getpid(), msg)
	fmt.Fprint(os.Stderr, line)
	if sdbgShimFile != nil {
		_, _ = sdbgShimFile.WriteString(line)
		_ = sdbgShimFile.Sync()
	}
}

var (
	log = golog.LoggerFor("systray")

	systrayReady  func()
	systrayExit   func()
	menuItems     = make(map[uint32]*MenuItem)
	menuItemsLock sync.RWMutex

	currentID = uint32(0)
	quitOnce  sync.Once

	windowsTrayLeftMu sync.Mutex
	windowsTrayLeftFn func()
)

func init() {
	runtime.LockOSThread()
}



func SetWindowsTrayLeftClick(fn func()) {
	windowsTrayLeftMu.Lock()
	windowsTrayLeftFn = fn
	windowsTrayLeftMu.Unlock()
}



type MenuItem struct {
	
	ClickedCh chan struct{}

	
	id uint32
	
	title string
	
	tooltip string
	
	disabled bool
	
	checked bool
	
	isCheckable bool
	
	parent *MenuItem
}

func (item *MenuItem) String() string {
	if item.parent == nil {
		return fmt.Sprintf("MenuItem[%d, %q]", item.id, item.title)
	}
	return fmt.Sprintf("MenuItem[%d, parent %d, %q]", item.id, item.parent.id, item.title)
}


func newMenuItem(title string, tooltip string, parent *MenuItem) *MenuItem {
	return &MenuItem{
		ClickedCh:   make(chan struct{}),
		id:          atomic.AddUint32(&currentID, 1),
		title:       title,
		tooltip:     tooltip,
		disabled:    false,
		checked:     false,
		isCheckable: false,
		parent:      parent,
	}
}



func Run(onReady func(), onExit func()) {
	Register(onReady, onExit)
	nativeLoop()
}






func Register(onReady func(), onExit func()) {
	if onReady == nil {
		systrayReady = func() {}
	} else {
		
		readyCh := make(chan interface{})
		go func() {
			<-readyCh
			onReady()
		}()
		systrayReady = func() {
			close(readyCh)
		}
	}
	
	
	if onExit == nil {
		onExit = func() {}
	}
	systrayExit = onExit
	registerSystray()
}


func Quit() {
	quitOnce.Do(quit)
}




func AddMenuItem(title string, tooltip string) *MenuItem {
	item := newMenuItem(title, tooltip, nil)
	item.update()
	return item
}




func AddMenuItemCheckbox(title string, tooltip string, checked bool) *MenuItem {
	item := newMenuItem(title, tooltip, nil)
	item.isCheckable = true
	item.checked = checked
	item.update()
	return item
}


func AddSeparator() {
	addSeparator(atomic.AddUint32(&currentID, 1))
}




func (item *MenuItem) AddSubMenuItem(title string, tooltip string) *MenuItem {
	child := newMenuItem(title, tooltip, item)
	child.update()
	return child
}




func (item *MenuItem) AddSubMenuItemCheckbox(title string, tooltip string, checked bool) *MenuItem {
	child := newMenuItem(title, tooltip, item)
	child.isCheckable = true
	child.checked = checked
	child.update()
	return child
}


func (item *MenuItem) SetTitle(title string) {
	item.title = title
	item.update()
}


func (item *MenuItem) SetTooltip(tooltip string) {
	item.tooltip = tooltip
	item.update()
}


func (item *MenuItem) Disabled() bool {
	return item.disabled
}


func (item *MenuItem) Enable() {
	item.disabled = false
	item.update()
}


func (item *MenuItem) Disable() {
	item.disabled = true
	item.update()
}


func (item *MenuItem) Hide() {
	hideMenuItem(item)
}


func (item *MenuItem) Show() {
	showMenuItem(item)
}


func (item *MenuItem) Checked() bool {
	return item.checked
}


func (item *MenuItem) Check() {
	item.checked = true
	item.update()
}


func (item *MenuItem) Uncheck() {
	item.checked = false
	item.update()
}


func (item *MenuItem) update() {
	menuItemsLock.Lock()
	menuItems[item.id] = item
	menuItemsLock.Unlock()
	addOrUpdateMenuItem(item)
}

func systrayMenuItemSelected(id uint32) {
	menuItemsLock.RLock()
	item, ok := menuItems[id]
	menuItemsLock.RUnlock()
	if !ok {
		log.Errorf("No menu item with ID %v", id)
		sdbgLogShim("systrayMenuItemSelected(id=%d): NO MENU ITEM FOUND in map", id)
		return
	}
	sdbgLogShim("systrayMenuItemSelected(id=%d): pushing to ClickedCh (title=%q)", id, item.title)
	select {
	case item.ClickedCh <- struct{}{}:
		sdbgLogShim("systrayMenuItemSelected(id=%d): push succeeded", id)
	default:
		sdbgLogShim("systrayMenuItemSelected(id=%d): channel full, click DROPPED", id)
	}
}
