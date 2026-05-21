// Originally from https://github.com/getlantern/systray (Apache-2.0).
// Modified by ResultV. Modifications are GPL-3.0 — see top-level LICENSE.

/*
Package systray is a cross-platform Go library to place an icon and menu in the notification area.
*/
package systray

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/getlantern/golog"
)

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
		return
	}
	select {
	case item.ClickedCh <- struct{}{}:
	
	default:
	}
}
