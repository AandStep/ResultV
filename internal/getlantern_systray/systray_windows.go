// Originally from https://github.com/getlantern/systray (Apache-2.0).
// Modified by ResultV. Modifications are GPL-3.0 — see top-level LICENSE.

//go:build windows
// +build windows

package systray

import (
	"crypto/md5"
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	g32                     = windows.NewLazySystemDLL("Gdi32.dll")
	pCreateCompatibleBitmap = g32.NewProc("CreateCompatibleBitmap")
	pCreateCompatibleDC     = g32.NewProc("CreateCompatibleDC")
	pDeleteDC               = g32.NewProc("DeleteDC")
	pSelectObject           = g32.NewProc("SelectObject")

	k32              = windows.NewLazySystemDLL("Kernel32.dll")
	pGetModuleHandle = k32.NewProc("GetModuleHandleW")

	s32              = windows.NewLazySystemDLL("Shell32.dll")
	pShellNotifyIcon = s32.NewProc("Shell_NotifyIconW")

	u32                    = windows.NewLazySystemDLL("User32.dll")
	pCreateMenu            = u32.NewProc("CreateMenu")
	pCreatePopupMenu       = u32.NewProc("CreatePopupMenu")
	pCreateWindowEx        = u32.NewProc("CreateWindowExW")
	pDefWindowProc         = u32.NewProc("DefWindowProcW")
	pRemoveMenu            = u32.NewProc("RemoveMenu")
	pDestroyWindow         = u32.NewProc("DestroyWindow")
	pDispatchMessage       = u32.NewProc("DispatchMessageW")
	pDrawIconEx            = u32.NewProc("DrawIconEx")
	pGetCursorPos          = u32.NewProc("GetCursorPos")
	pGetDC                 = u32.NewProc("GetDC")
	pGetMessage            = u32.NewProc("GetMessageW")
	pGetSystemMetrics      = u32.NewProc("GetSystemMetrics")
	pInsertMenuItem        = u32.NewProc("InsertMenuItemW")
	pLoadCursor            = u32.NewProc("LoadCursorW")
	pLoadIcon              = u32.NewProc("LoadIconW")
	pLoadImage             = u32.NewProc("LoadImageW")
	pPostMessage           = u32.NewProc("PostMessageW")
	pPostQuitMessage       = u32.NewProc("PostQuitMessage")
	pRegisterClass         = u32.NewProc("RegisterClassExW")
	pRegisterWindowMessage = u32.NewProc("RegisterWindowMessageW")
	pReleaseDC             = u32.NewProc("ReleaseDC")
	pSetForegroundWindow   = u32.NewProc("SetForegroundWindow")
	pSetMenuInfo           = u32.NewProc("SetMenuInfo")
	pSetMenuItemInfo       = u32.NewProc("SetMenuItemInfoW")
	pShowWindow            = u32.NewProc("ShowWindow")
	pTrackPopupMenu        = u32.NewProc("TrackPopupMenu")
	pTranslateMessage      = u32.NewProc("TranslateMessage")
	pUnregisterClass       = u32.NewProc("UnregisterClassW")
	pUpdateWindow          = u32.NewProc("UpdateWindow")
)

type wndClassEx struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background windows.Handle
	MenuName, ClassName                *uint16
	IconSm                             windows.Handle
}

func (w *wndClassEx) register() error {
	w.Size = uint32(unsafe.Sizeof(*w))
	res, _, err := pRegisterClass.Call(uintptr(unsafe.Pointer(w)))
	if res == 0 {
		return err
	}
	return nil
}

func (w *wndClassEx) unregister() error {
	res, _, err := pUnregisterClass.Call(
		uintptr(unsafe.Pointer(w.ClassName)),
		uintptr(w.Instance),
	)
	if res == 0 {
		return err
	}
	return nil
}

type notifyIconData struct {
	Size                       uint32
	Wnd                        windows.Handle
	ID, Flags, CallbackMessage uint32
	Icon                       windows.Handle
	Tip                        [128]uint16
	State, StateMask           uint32
	Info                       [256]uint16
	Timeout, Version           uint32
	InfoTitle                  [64]uint16
	InfoFlags                  uint32
	GuidItem                   windows.GUID
	BalloonIcon                windows.Handle
}

func (nid *notifyIconData) add() error {
	const NIM_ADD = 0x00000000
	res, _, err := pShellNotifyIcon.Call(
		uintptr(NIM_ADD),
		uintptr(unsafe.Pointer(nid)),
	)
	if res == 0 {
		return err
	}
	return nil
}

func (nid *notifyIconData) modify() error {
	const NIM_MODIFY = 0x00000001
	res, _, err := pShellNotifyIcon.Call(
		uintptr(NIM_MODIFY),
		uintptr(unsafe.Pointer(nid)),
	)
	if res == 0 {
		return err
	}
	return nil
}

func (nid *notifyIconData) delete() error {
	const NIM_DELETE = 0x00000002
	res, _, err := pShellNotifyIcon.Call(
		uintptr(NIM_DELETE),
		uintptr(unsafe.Pointer(nid)),
	)
	if res == 0 {
		return err
	}
	return nil
}

type menuItemInfo struct {
	Size, Mask, Type, State     uint32
	ID                          uint32
	SubMenu, Checked, Unchecked windows.Handle
	ItemData                    uintptr
	TypeData                    *uint16
	Cch                         uint32
	BMPItem                     windows.Handle
}

type point struct {
	X, Y int32
}

type winTray struct {
	instance,
	icon,
	cursor,
	window windows.Handle

	loadedImages   map[string]windows.Handle
	muLoadedImages sync.RWMutex

	menus   map[uint32]windows.Handle
	muMenus sync.RWMutex

	menuOf   map[uint32]windows.Handle
	muMenuOf sync.RWMutex

	menuItemIcons   map[uint32]windows.Handle
	muMenuItemIcons sync.RWMutex
	visibleItems    map[uint32][]uint32
	muVisibleItems  sync.RWMutex

	nid   *notifyIconData
	muNID sync.RWMutex
	wcex  *wndClassEx

	wmSystrayMessage,
	wmTaskbarCreated uint32
}

func (t *winTray) setIcon(src string) error {
	const NIF_ICON = 0x00000002

	h, err := t.loadIconFrom(src)
	if err != nil {
		return err
	}

	t.muNID.Lock()
	defer t.muNID.Unlock()
	t.nid.Icon = h
	t.nid.Flags |= NIF_ICON
	t.nid.Size = uint32(unsafe.Sizeof(*t.nid))

	return t.nid.modify()
}

func (t *winTray) setTooltip(src string) error {
	const NIF_TIP = 0x00000004
	b, err := windows.UTF16FromString(src)
	if err != nil {
		return err
	}

	t.muNID.Lock()
	defer t.muNID.Unlock()
	copy(t.nid.Tip[:], b[:])
	t.nid.Flags |= NIF_TIP
	t.nid.Size = uint32(unsafe.Sizeof(*t.nid))

	return t.nid.modify()
}

var wt winTray

func (t *winTray) wndProc(hWnd windows.Handle, message uint32, wParam, lParam uintptr) (lResult uintptr) {
	const (
		WM_RBUTTONUP  = 0x0205
		WM_LBUTTONUP  = 0x0202
		WM_COMMAND    = 0x0111
		WM_ENDSESSION = 0x0016
		WM_CLOSE      = 0x0010
		WM_DESTROY    = 0x0002
	)
	switch message {
	case WM_COMMAND:
		menuItemId := int32(wParam)

		if menuItemId != -1 {
			systrayMenuItemSelected(uint32(wParam))
		}
	case WM_CLOSE:
		pDestroyWindow.Call(uintptr(t.window))
		t.wcex.unregister()
	case WM_DESTROY:

		defer pPostQuitMessage.Call(uintptr(int32(0)))
		fallthrough
	case WM_ENDSESSION:
		t.muNID.Lock()
		if t.nid != nil {
			t.nid.delete()
		}
		t.muNID.Unlock()
		systrayExit()
	case t.wmSystrayMessage:
		switch lParam {
		case WM_LBUTTONUP:
			windowsTrayLeftMu.Lock()
			fn := windowsTrayLeftFn
			windowsTrayLeftMu.Unlock()
			if fn != nil {
				go fn()
			}
		case WM_RBUTTONUP:
			t.showMenu()
		}
	case t.wmTaskbarCreated:
		t.muNID.Lock()
		t.nid.add()
		t.muNID.Unlock()
	default:

		lResult, _, _ = pDefWindowProc.Call(
			uintptr(hWnd),
			uintptr(message),
			uintptr(wParam),
			uintptr(lParam),
		)
	}
	return
}

func (t *winTray) initInstance() error {
	const IDI_APPLICATION = 32512
	const IDC_ARROW = 32512

	const SW_HIDE = 0
	const CW_USEDEFAULT = 0x80000000

	const (
		WS_CAPTION     = 0x00C00000
		WS_MAXIMIZEBOX = 0x00010000
		WS_MINIMIZEBOX = 0x00020000
		WS_OVERLAPPED  = 0x00000000
		WS_SYSMENU     = 0x00080000
		WS_THICKFRAME  = 0x00040000

		WS_OVERLAPPEDWINDOW = WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_THICKFRAME | WS_MINIMIZEBOX | WS_MAXIMIZEBOX
	)

	const (
		CS_HREDRAW = 0x0002
		CS_VREDRAW = 0x0001
	)
	const NIF_MESSAGE = 0x00000001

	const WM_USER = 0x0400

	const (
		className  = "SystrayClass"
		windowName = ""
	)

	t.wmSystrayMessage = WM_USER + 1
	t.visibleItems = make(map[uint32][]uint32)
	t.menus = make(map[uint32]windows.Handle)
	t.menuOf = make(map[uint32]windows.Handle)
	t.menuItemIcons = make(map[uint32]windows.Handle)

	taskbarEventNamePtr, _ := windows.UTF16PtrFromString("TaskbarCreated")

	res, _, err := pRegisterWindowMessage.Call(
		uintptr(unsafe.Pointer(taskbarEventNamePtr)),
	)
	t.wmTaskbarCreated = uint32(res)

	t.loadedImages = make(map[string]windows.Handle)

	instanceHandle, _, err := pGetModuleHandle.Call(0)
	if instanceHandle == 0 {
		return err
	}
	t.instance = windows.Handle(instanceHandle)

	iconHandle, _, err := pLoadIcon.Call(0, uintptr(IDI_APPLICATION))
	if iconHandle == 0 {
		return err
	}
	t.icon = windows.Handle(iconHandle)

	cursorHandle, _, err := pLoadCursor.Call(0, uintptr(IDC_ARROW))
	if cursorHandle == 0 {
		return err
	}
	t.cursor = windows.Handle(cursorHandle)

	classNamePtr, err := windows.UTF16PtrFromString(className)
	if err != nil {
		return err
	}

	windowNamePtr, err := windows.UTF16PtrFromString(windowName)
	if err != nil {
		return err
	}

	t.wcex = &wndClassEx{
		Style:      CS_HREDRAW | CS_VREDRAW,
		WndProc:    windows.NewCallback(t.wndProc),
		Instance:   t.instance,
		Icon:       t.icon,
		Cursor:     t.cursor,
		Background: windows.Handle(6),
		ClassName:  classNamePtr,
		IconSm:     t.icon,
	}
	if err := t.wcex.register(); err != nil {
		return err
	}

	windowHandle, _, err := pCreateWindowEx.Call(
		uintptr(0),
		uintptr(unsafe.Pointer(classNamePtr)),
		uintptr(unsafe.Pointer(windowNamePtr)),
		uintptr(WS_OVERLAPPEDWINDOW),
		uintptr(CW_USEDEFAULT),
		uintptr(CW_USEDEFAULT),
		uintptr(CW_USEDEFAULT),
		uintptr(CW_USEDEFAULT),
		uintptr(0),
		uintptr(0),
		uintptr(t.instance),
		uintptr(0),
	)
	if windowHandle == 0 {
		return err
	}
	t.window = windows.Handle(windowHandle)

	pShowWindow.Call(
		uintptr(t.window),
		uintptr(SW_HIDE),
	)

	pUpdateWindow.Call(
		uintptr(t.window),
	)

	t.muNID.Lock()
	defer t.muNID.Unlock()
	t.nid = &notifyIconData{
		Wnd:             windows.Handle(t.window),
		ID:              100,
		Flags:           NIF_MESSAGE,
		CallbackMessage: t.wmSystrayMessage,
	}
	t.nid.Size = uint32(unsafe.Sizeof(*t.nid))

	return t.nid.add()
}

func (t *winTray) createMenu() error {
	const MIM_APPLYTOSUBMENUS = 0x80000000

	menuHandle, _, err := pCreatePopupMenu.Call()
	if menuHandle == 0 {
		return err
	}
	t.menus[0] = windows.Handle(menuHandle)

	mi := struct {
		Size, Mask, Style, Max uint32
		Background             windows.Handle
		ContextHelpID          uint32
		MenuData               uintptr
	}{
		Mask: MIM_APPLYTOSUBMENUS,
	}
	mi.Size = uint32(unsafe.Sizeof(mi))

	res, _, err := pSetMenuInfo.Call(
		uintptr(t.menus[0]),
		uintptr(unsafe.Pointer(&mi)),
	)
	if res == 0 {
		return err
	}
	return nil
}

func (t *winTray) convertToSubMenu(menuItemId uint32) (windows.Handle, error) {
	const MIIM_SUBMENU = 0x00000004

	res, _, err := pCreateMenu.Call()
	if res == 0 {
		return 0, err
	}
	menu := windows.Handle(res)

	mi := menuItemInfo{Mask: MIIM_SUBMENU, SubMenu: menu}
	mi.Size = uint32(unsafe.Sizeof(mi))
	t.muMenuOf.RLock()
	hMenu := t.menuOf[menuItemId]
	t.muMenuOf.RUnlock()
	res, _, err = pSetMenuItemInfo.Call(
		uintptr(hMenu),
		uintptr(menuItemId),
		0,
		uintptr(unsafe.Pointer(&mi)),
	)
	if res == 0 {
		return 0, err
	}
	t.muMenus.Lock()
	t.menus[menuItemId] = menu
	t.muMenus.Unlock()
	return menu, nil
}

func (t *winTray) addOrUpdateMenuItem(menuItemId uint32, parentId uint32, title string, disabled, checked bool) error {

	const (
		MIIM_FTYPE   = 0x00000100
		MIIM_BITMAP  = 0x00000080
		MIIM_STRING  = 0x00000040
		MIIM_SUBMENU = 0x00000004
		MIIM_ID      = 0x00000002
		MIIM_STATE   = 0x00000001
	)
	const MFT_STRING = 0x00000000
	const (
		MFS_CHECKED  = 0x00000008
		MFS_DISABLED = 0x00000003
	)
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return err
	}

	mi := menuItemInfo{
		Mask:     MIIM_FTYPE | MIIM_STRING | MIIM_ID | MIIM_STATE,
		Type:     MFT_STRING,
		ID:       uint32(menuItemId),
		TypeData: titlePtr,
		Cch:      uint32(len(title)),
	}
	mi.Size = uint32(unsafe.Sizeof(mi))
	if disabled {
		mi.State |= MFS_DISABLED
	}
	if checked {
		mi.State |= MFS_CHECKED
	}
	t.muMenuItemIcons.RLock()
	hIcon := t.menuItemIcons[menuItemId]
	t.muMenuItemIcons.RUnlock()
	if hIcon > 0 {
		mi.Mask |= MIIM_BITMAP
		mi.BMPItem = hIcon
	}

	var res uintptr
	t.muMenus.RLock()
	menu, exists := t.menus[parentId]
	t.muMenus.RUnlock()
	if !exists {
		menu, err = t.convertToSubMenu(parentId)
		if err != nil {
			return err
		}
		t.muMenus.Lock()
		t.menus[parentId] = menu
		t.muMenus.Unlock()
	} else if t.getVisibleItemIndex(parentId, menuItemId) != -1 {

		res, _, err = pSetMenuItemInfo.Call(
			uintptr(menu),
			uintptr(menuItemId),
			0,
			uintptr(unsafe.Pointer(&mi)),
		)
	}

	if res == 0 {

		t.muMenus.RLock()
		submenu, exists := t.menus[menuItemId]
		t.muMenus.RUnlock()
		if exists {
			mi.Mask |= MIIM_SUBMENU
			mi.SubMenu = submenu
		}
		t.addToVisibleItems(parentId, menuItemId)
		position := t.getVisibleItemIndex(parentId, menuItemId)
		res, _, err = pInsertMenuItem.Call(
			uintptr(menu),
			uintptr(position),
			1,
			uintptr(unsafe.Pointer(&mi)),
		)
		if res == 0 {
			t.delFromVisibleItems(parentId, menuItemId)
			return err
		}
		t.muMenuOf.Lock()
		t.menuOf[menuItemId] = menu
		t.muMenuOf.Unlock()
	}

	return nil
}

func (t *winTray) addSeparatorMenuItem(menuItemId, parentId uint32) error {

	const (
		MIIM_FTYPE = 0x00000100
		MIIM_ID    = 0x00000002
		MIIM_STATE = 0x00000001
	)
	const MFT_SEPARATOR = 0x00000800

	mi := menuItemInfo{
		Mask: MIIM_FTYPE | MIIM_ID | MIIM_STATE,
		Type: MFT_SEPARATOR,
		ID:   uint32(menuItemId),
	}

	mi.Size = uint32(unsafe.Sizeof(mi))

	t.addToVisibleItems(parentId, menuItemId)
	position := t.getVisibleItemIndex(parentId, menuItemId)
	t.muMenus.RLock()
	menu := uintptr(t.menus[parentId])
	t.muMenus.RUnlock()
	res, _, err := pInsertMenuItem.Call(
		menu,
		uintptr(position),
		1,
		uintptr(unsafe.Pointer(&mi)),
	)
	if res == 0 {
		return err
	}

	return nil
}

func (t *winTray) hideMenuItem(menuItemId, parentId uint32) error {

	const MF_BYCOMMAND = 0x00000000
	const ERROR_SUCCESS syscall.Errno = 0

	t.muMenus.RLock()
	menu := uintptr(t.menus[parentId])
	t.muMenus.RUnlock()
	res, _, err := pRemoveMenu.Call(
		menu,
		uintptr(menuItemId),
		MF_BYCOMMAND,
	)
	if res == 0 && err.(syscall.Errno) != ERROR_SUCCESS {
		return err
	}
	t.delFromVisibleItems(parentId, menuItemId)

	return nil
}

func (t *winTray) showMenu() error {
	const (
		TPM_BOTTOMALIGN = 0x0020
		TPM_LEFTALIGN   = 0x0000
	)
	p := point{}
	res, _, err := pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if res == 0 {
		return err
	}
	pSetForegroundWindow.Call(uintptr(t.window))

	res, _, err = pTrackPopupMenu.Call(
		uintptr(t.menus[0]),
		TPM_BOTTOMALIGN|TPM_LEFTALIGN,
		uintptr(p.X),
		uintptr(p.Y),
		0,
		uintptr(t.window),
		0,
	)
	if res == 0 {
		return err
	}

	return nil
}

func (t *winTray) delFromVisibleItems(parent, val uint32) {
	t.muVisibleItems.Lock()
	defer t.muVisibleItems.Unlock()
	visibleItems := t.visibleItems[parent]
	for i, itemval := range visibleItems {
		if val == itemval {
			t.visibleItems[parent] = append(visibleItems[:i], visibleItems[i+1:]...)
			break
		}
	}
}

func (t *winTray) addToVisibleItems(parent, val uint32) {
	t.muVisibleItems.Lock()
	defer t.muVisibleItems.Unlock()
	if visibleItems, exists := t.visibleItems[parent]; !exists {
		t.visibleItems[parent] = []uint32{val}
	} else {
		newvisible := append(visibleItems, val)
		sort.Slice(newvisible, func(i, j int) bool { return newvisible[i] < newvisible[j] })
		t.visibleItems[parent] = newvisible
	}
}

func (t *winTray) getVisibleItemIndex(parent, val uint32) int {
	t.muVisibleItems.RLock()
	defer t.muVisibleItems.RUnlock()
	for i, itemval := range t.visibleItems[parent] {
		if val == itemval {
			return i
		}
	}
	return -1
}

func (t *winTray) loadIconFrom(src string) (windows.Handle, error) {
	const IMAGE_ICON = 1
	const LR_LOADFROMFILE = 0x00000010
	const LR_DEFAULTSIZE = 0x00000040

	t.muLoadedImages.RLock()
	h, ok := t.loadedImages[src]
	t.muLoadedImages.RUnlock()
	if !ok {
		srcPtr, err := windows.UTF16PtrFromString(src)
		if err != nil {
			return 0, err
		}
		res, _, err := pLoadImage.Call(
			0,
			uintptr(unsafe.Pointer(srcPtr)),
			IMAGE_ICON,
			0,
			0,
			LR_LOADFROMFILE|LR_DEFAULTSIZE,
		)
		if res == 0 {
			return 0, err
		}
		h = windows.Handle(res)
		t.muLoadedImages.Lock()
		t.loadedImages[src] = h
		t.muLoadedImages.Unlock()
	}
	return h, nil
}

func (t *winTray) iconToBitmap(hIcon windows.Handle) (windows.Handle, error) {
	const SM_CXSMICON = 49
	const SM_CYSMICON = 50
	const DI_NORMAL = 0x3
	hDC, _, err := pGetDC.Call(uintptr(0))
	if hDC == 0 {
		return 0, err
	}
	defer pReleaseDC.Call(uintptr(0), hDC)
	hMemDC, _, err := pCreateCompatibleDC.Call(hDC)
	if hMemDC == 0 {
		return 0, err
	}
	defer pDeleteDC.Call(hMemDC)
	cx, _, _ := pGetSystemMetrics.Call(SM_CXSMICON)
	cy, _, _ := pGetSystemMetrics.Call(SM_CYSMICON)
	hMemBmp, _, err := pCreateCompatibleBitmap.Call(hDC, cx, cy)
	if hMemBmp == 0 {
		return 0, err
	}
	hOriginalBmp, _, _ := pSelectObject.Call(hMemDC, hMemBmp)
	defer pSelectObject.Call(hMemDC, hOriginalBmp)
	res, _, err := pDrawIconEx.Call(hMemDC, 0, 0, uintptr(hIcon), cx, cy, 0, uintptr(0), DI_NORMAL)
	if res == 0 {
		return 0, err
	}
	return windows.Handle(hMemBmp), nil
}

func registerSystray() {
	if err := wt.initInstance(); err != nil {
		log.Errorf("Unable to init instance: %v", err)
		return
	}

	if err := wt.createMenu(); err != nil {
		log.Errorf("Unable to create menu: %v", err)
		return
	}

	systrayReady()
}

func nativeLoop() {

	m := &struct {
		WindowHandle windows.Handle
		Message      uint32
		Wparam       uintptr
		Lparam       uintptr
		Time         uint32
		Pt           point
	}{}
	for {
		ret, _, err := pGetMessage.Call(uintptr(unsafe.Pointer(m)), 0, 0, 0)

		switch int32(ret) {
		case -1:
			log.Errorf("Error at message loop: %v", err)
			return
		case 0:
			return
		default:
			pTranslateMessage.Call(uintptr(unsafe.Pointer(m)))
			pDispatchMessage.Call(uintptr(unsafe.Pointer(m)))
		}
	}
}

func quit() {
	const WM_CLOSE = 0x0010

	pPostMessage.Call(
		uintptr(wt.window),
		WM_CLOSE,
		0,
		0,
	)
}

func iconBytesToFilePath(iconBytes []byte) (string, error) {
	bh := md5.Sum(iconBytes)
	dataHash := hex.EncodeToString(bh[:])
	iconFilePath := filepath.Join(os.TempDir(), "systray_temp_icon_"+dataHash)

	if _, err := os.Stat(iconFilePath); os.IsNotExist(err) {
		if err := ioutil.WriteFile(iconFilePath, iconBytes, 0644); err != nil {
			return "", err
		}
	}
	return iconFilePath, nil
}

func SetIcon(iconBytes []byte) {
	iconFilePath, err := iconBytesToFilePath(iconBytes)
	if err != nil {
		log.Errorf("Unable to write icon data to temp file: %v", err)
		return
	}
	if err := wt.setIcon(iconFilePath); err != nil {
		log.Errorf("Unable to set icon: %v", err)
		return
	}
}

func SetTemplateIcon(templateIconBytes []byte, regularIconBytes []byte) {
	SetIcon(regularIconBytes)
}

func SetTitle(title string) {

}

func (item *MenuItem) parentId() uint32 {
	if item.parent != nil {
		return uint32(item.parent.id)
	}
	return 0
}

func (item *MenuItem) SetIcon(iconBytes []byte) {
	iconFilePath, err := iconBytesToFilePath(iconBytes)
	if err != nil {
		log.Errorf("Unable to write icon data to temp file: %v", err)
		return
	}

	h, err := wt.loadIconFrom(iconFilePath)
	if err != nil {
		log.Errorf("Unable to load icon from temp file: %v", err)
		return
	}

	h, err = wt.iconToBitmap(h)
	if err != nil {
		log.Errorf("Unable to convert icon to bitmap: %v", err)
		return
	}
	wt.muMenuItemIcons.Lock()
	wt.menuItemIcons[uint32(item.id)] = h
	wt.muMenuItemIcons.Unlock()

	err = wt.addOrUpdateMenuItem(uint32(item.id), item.parentId(), item.title, item.disabled, item.checked)
	if err != nil {
		log.Errorf("Unable to addOrUpdateMenuItem: %v", err)
		return
	}
}

func SetTooltip(tooltip string) {
	if err := wt.setTooltip(tooltip); err != nil {
		log.Errorf("Unable to set tooltip: %v", err)
		return
	}
}

func addOrUpdateMenuItem(item *MenuItem) {
	err := wt.addOrUpdateMenuItem(uint32(item.id), item.parentId(), item.title, item.disabled, item.checked)
	if err != nil {
		log.Errorf("Unable to addOrUpdateMenuItem: %v", err)
		return
	}
}

func (item *MenuItem) SetTemplateIcon(templateIconBytes []byte, regularIconBytes []byte) {
	item.SetIcon(regularIconBytes)
}

func addSeparator(id uint32) {
	err := wt.addSeparatorMenuItem(id, 0)
	if err != nil {
		log.Errorf("Unable to addSeparator: %v", err)
		return
	}
}

func hideMenuItem(item *MenuItem) {
	err := wt.hideMenuItem(uint32(item.id), item.parentId())
	if err != nil {
		log.Errorf("Unable to hideMenuItem: %v", err)
		return
	}
}

func showMenuItem(item *MenuItem) {
	addOrUpdateMenuItem(item)
}
