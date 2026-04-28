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

package system

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/getlantern/systray"
	"resultproxy-wails/internal/config"
)


type TrayCallbacks struct {
	OnShowWindow      func()
	OnDisconnect      func()
	OnQuit            func()
	OnSelectProxy     func(proxyID string)
	OnConnectSelected func(proxyID string)
	// OnUnexpectedExit is called when the systray dies without Stop() being
	// invoked — typically a Windows message-loop error or session event.
	// The app should exit if the window is also hidden (zombie state).
	OnUnexpectedExit func()
}




type Tray struct {
	mu             sync.Mutex
	icon           []byte
	callbacks      TrayCallbacks
	running        bool
	exited         chan struct{}
	stopRequested  bool

	
	mStatus     *systray.MenuItem
	mShow       *systray.MenuItem
	mDisconnect *systray.MenuItem
	mServers    *systray.MenuItem
	mQuit       *systray.MenuItem

	
	proxyLookup        map[string]config.ProxyEntry
	proxyPings         map[string]int64
	serverItems        map[string]*systray.MenuItem
	dynamicItems       []*systray.MenuItem
	selectedProxyID    string
	connectedProxyID   string
	perCountryLimit    int
	emptyItem          *systray.MenuItem
	countryIcons       map[string][]byte
	fallbackIcon       []byte
	httpClient         *http.Client
	lastMenuSignature  string
	lastSelectedID     string
	serverTitleCache   map[string]string
	statusTitleCache   string
	statusTooltipCache string
	clickDispatcher   *trayClickDispatcher
	pendingProxies    []config.ProxyEntry 
	pendingSelectedID string
}



func NewTray(icon []byte, cb TrayCallbacks) *Tray {
	fallbackIcon := buildFallbackIcon()
	return &Tray{
		icon:            icon,
		callbacks:       cb,
		exited:          make(chan struct{}),
		proxyLookup:     make(map[string]config.ProxyEntry),
		proxyPings:      make(map[string]int64),
		serverItems:     make(map[string]*systray.MenuItem),
		perCountryLimit: 20,
		countryIcons:    make(map[string][]byte),
		fallbackIcon:    fallbackIcon,
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
			Transport: &http.Transport{
				Proxy:               nil, 
				MaxIdleConns:        8,
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		serverTitleCache: make(map[string]string),
		clickDispatcher:  newTrayClickDispatcher(),
	}
}



func (t *Tray) Start() {
	go systray.Run(t.onReady, t.onExit)
}


func (t *Tray) Stop() {
	t.mu.Lock()
	running := t.running
	t.stopRequested = true
	t.mu.Unlock()

	if running {
		systray.Quit()
		select {
		case <-t.exited:
		case <-time.After(2 * time.Second):
		}
	}
}

func (t *Tray) onReady() {
	t.mu.Lock()
	t.running = true
	t.mu.Unlock()

	
	if len(t.icon) > 0 {
		systray.SetIcon(t.icon)
	}
	systray.SetTitle("ResultV")
	systray.SetTooltip("ResultV — Отключено")

	systray.SetWindowsTrayLeftClick(func() {
		t.safeCall(func() {
			if t.callbacks.OnShowWindow != nil {
				t.callbacks.OnShowWindow()
			}
		})
	})

	
	t.mStatus = systray.AddMenuItem("⚪ Отключено", "Статус подключения")
	t.mStatus.Disable() 

	systray.AddSeparator()

	t.mShow = systray.AddMenuItem("Показать окно", "Открыть окно ResultV")
	t.mDisconnect = systray.AddMenuItem("Отключить", "Отключить прокси")
	t.mDisconnect.Disable() 

	systray.AddSeparator()
	t.mServers = systray.AddMenuItem("Серверы", "Список серверов для быстрого подключения")
	t.emptyItem = t.mServers.AddSubMenuItem("(список пуст)", "")
	t.emptyItem.Disable()

	t.mQuit = systray.AddMenuItem("Выход", "Закрыть ResultV")

	
	go t.eventLoop()
	t.clickDispatcher.start(t.handleServerClick)

	
	t.mu.Lock()
	pendingProxies := t.pendingProxies
	pendingSelectedID := t.pendingSelectedID
	t.pendingProxies = nil
	t.pendingSelectedID = ""
	t.mu.Unlock()

	if len(pendingProxies) > 0 {
		
		
		t.prefetchCountryIcons(pendingProxies)

		t.mu.Lock()
		t.proxyLookup = make(map[string]config.ProxyEntry, len(pendingProxies))
		for _, p := range pendingProxies {
			t.proxyLookup[p.ID] = p
		}
		if pendingSelectedID != "" {
			t.selectedProxyID = pendingSelectedID
		}
		t.lastMenuSignature = buildProxyListSignature(pendingProxies)
		t.lastSelectedID = t.selectedProxyID
		t.rebuildServersMenuLocked(pendingProxies)
		t.mu.Unlock()
	}
}

func (t *Tray) eventLoop() {
	for {
		select {
		case <-t.mShow.ClickedCh:
			t.safeCall(func() {
				if t.callbacks.OnShowWindow != nil {
					t.callbacks.OnShowWindow()
				}
			})
		case <-t.mDisconnect.ClickedCh:
			t.safeCall(func() {
				if t.callbacks.OnDisconnect != nil {
					t.callbacks.OnDisconnect()
				}
			})
		case <-t.mQuit.ClickedCh:
			t.safeCall(func() {
				if t.callbacks.OnQuit != nil {
					t.callbacks.OnQuit()
				}
			})
			return
		}
	}
}

func (t *Tray) onExit() {
	t.clickDispatcher.stop()
	t.mu.Lock()
	t.running = false
	wasExpected := t.stopRequested
	cb := t.callbacks.OnUnexpectedExit
	t.mu.Unlock()
	close(t.exited)
	if !wasExpected && cb != nil {
		go cb()
	}
}


func (t *Tray) SetConnected(serverName string) {
	t.SetConnectedProxy("", serverName)
}


func (t *Tray) SetConnectedProxy(proxyID, serverName string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running || t.mStatus == nil {
		return
	}

	title := "🟢 " + serverName
	tooltip := "ResultV — " + serverName
	if t.statusTitleCache != title {
		t.mStatus.SetTitle(title)
		t.statusTitleCache = title
	}
	if t.statusTooltipCache != tooltip {
		systray.SetTooltip(tooltip)
		t.statusTooltipCache = tooltip
	}
	t.connectedProxyID = proxyID
	if proxyID != "" {
		t.selectedProxyID = proxyID
	}
	t.mDisconnect.Enable()
}


func (t *Tray) SetDisconnected() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running || t.mStatus == nil {
		return
	}

	title := "⚪ Отключено"
	tooltip := "ResultV — Отключено"
	if t.statusTitleCache != title {
		t.mStatus.SetTitle(title)
		t.statusTitleCache = title
	}
	if t.statusTooltipCache != tooltip {
		systray.SetTooltip(tooltip)
		t.statusTooltipCache = tooltip
	}
	t.connectedProxyID = ""
	t.mDisconnect.Disable()
}


func (t *Tray) SetKillSwitchActive() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running || t.mStatus == nil {
		return
	}

	title := "🔴 Kill Switch — интернет заблокирован"
	tooltip := "ResultV — Kill Switch активен"
	if t.statusTitleCache != title {
		t.mStatus.SetTitle(title)
		t.statusTitleCache = title
	}
	if t.statusTooltipCache != tooltip {
		systray.SetTooltip(tooltip)
		t.statusTooltipCache = tooltip
	}
}


func (t *Tray) UpdateProxyList(proxies []config.ProxyEntry, selectedProxyID string) {
	t.mu.Lock()

	t.proxyLookup = make(map[string]config.ProxyEntry, len(proxies))
	for _, p := range proxies {
		t.proxyLookup[p.ID] = p
	}
	if selectedProxyID != "" {
		t.selectedProxyID = selectedProxyID
	}

	signature := buildProxyListSignature(proxies)
	selection := t.selectedProxyID

	if !t.running || t.mServers == nil {
		
		t.pendingProxies = make([]config.ProxyEntry, len(proxies))
		copy(t.pendingProxies, proxies)
		t.pendingSelectedID = t.selectedProxyID
		t.mu.Unlock()
		return
	}

	
	if signature == t.lastMenuSignature {
		if selection != t.lastSelectedID {
			t.lastSelectedID = selection
			t.refreshServerTitlesLocked()
		}
		t.mu.Unlock()
		return
	}

	t.lastMenuSignature = signature
	t.lastSelectedID = selection

	
	t.mu.Unlock()
	t.prefetchCountryIcons(proxies)
	t.mu.Lock()

	if !t.running || t.mServers == nil {
		t.mu.Unlock()
		return
	}
	t.rebuildServersMenuLocked(proxies)
	t.mu.Unlock()
}


func (t *Tray) UpdateProxyPings(pings map[string]int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, ping := range pings {
		t.proxyPings[id] = ping
	}
	
	
	
	
}

func (t *Tray) rebuildServersMenuLocked(proxies []config.ProxyEntry) {
	for _, item := range t.dynamicItems {
		item.Hide()
	}
	t.dynamicItems = t.dynamicItems[:0]
	t.serverItems = make(map[string]*systray.MenuItem)
	t.serverTitleCache = make(map[string]string) 
	if t.emptyItem != nil {
		t.emptyItem.Hide()
	}

	groups := BuildTrayMenuGroups(proxies, t.perCountryLimit)
	if len(groups) == 0 {
		t.clickDispatcher.update(nil)
		if t.emptyItem != nil {
			t.emptyItem.Show()
		}
		return
	}

	
	
	
	
	
	multiProvider := len(groups) > 1

	for i, provider := range groups {
		if multiProvider {
			
			if i > 0 {
				
				sep := t.mServers.AddSubMenuItem("", "")
				sep.Disable()
				t.dynamicItems = append(t.dynamicItems, sep)
			}
			header := t.mServers.AddSubMenuItem("── "+provider.Provider+" ──", "")
			header.Disable()
			t.dynamicItems = append(t.dynamicItems, header)
		}

		for _, country := range provider.Countries {
			cTitle := formatCountryTitle(country.Country)
			cItem := t.mServers.AddSubMenuItem(cTitle, "")
			if icon := t.getCountryIconLocked(country.Country); len(icon) > 0 {
				cItem.SetIcon(icon)
			}
			t.dynamicItems = append(t.dynamicItems, cItem)

			for _, server := range country.Servers {
				if ping, ok := t.proxyPings[server.ID]; ok {
					server.PingMs = ping
				}
				title := formatServerTitle(server, server.ID == t.connectedProxyID)
				srvItem := cItem.AddSubMenuItem(
					title,
					fmt.Sprintf("%s:%d", server.IP, server.Port),
				)
				if icon := t.getCountryIconLocked(server.Country); len(icon) > 0 {
					srvItem.SetIcon(icon)
				}
				t.dynamicItems = append(t.dynamicItems, srvItem)
				t.serverItems[server.ID] = srvItem
				t.serverTitleCache[server.ID] = title
			}
			if country.HiddenCount > 0 {
				more := cItem.AddSubMenuItem(fmt.Sprintf("... еще %d серверов (полный список в окне)", country.HiddenCount), "")
				more.Disable()
				t.dynamicItems = append(t.dynamicItems, more)
			}
		}
	}
	t.clickDispatcher.update(buildServerClickBindings(t.serverItems))
}

func (t *Tray) handleServerClick(proxyID string) {
	t.safeCall(func() {
		t.mu.Lock()
		t.selectedProxyID = proxyID
		t.refreshServerTitlesLocked()
		t.mu.Unlock()
		if t.callbacks.OnSelectProxy != nil {
			t.callbacks.OnSelectProxy(proxyID)
		}
		if t.callbacks.OnConnectSelected != nil {
			t.callbacks.OnConnectSelected(proxyID)
		}
	})
}

func (t *Tray) refreshServerTitlesLocked() {
	for proxyID, item := range t.serverItems {
		entry, ok := t.proxyLookup[proxyID]
		if !ok {
			continue
		}
		server := TrayServer{
			ID:      entry.ID,
			Name:    entry.Name,
			Country: entry.Country,
			IP:      entry.IP,
			Port:    entry.Port,
			PingMs:  -1,
		}
		if ping, ok := t.proxyPings[proxyID]; ok {
			server.PingMs = ping
		}
		if server.Name == "" {
			server.Name = fmt.Sprintf("%s:%d", server.IP, server.Port)
		}
		title := formatServerTitle(server, proxyID == t.connectedProxyID)
		prev := t.serverTitleCache[proxyID]
		if prev == title {
			continue
		}
		item.SetTitle(title)
		t.serverTitleCache[proxyID] = title
	}
}

func (t *Tray) safeCall(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[tray] recovered panic: %v", r)
		}
	}()
	fn()
}

func (t *Tray) getCountryIconLocked(country string) []byte {
	isoCode := countryISOCode(country)
	if isoCode == "" {
		return t.fallbackIcon
	}
	if cached, ok := t.countryIcons[isoCode]; ok {
		return cached
	}
	return t.fallbackIcon
}

func (t *Tray) downloadCountryIcon(isoCode string) []byte {
	urls := []string{
		fmt.Sprintf("https://flagcdn.com/w20/%s.png", isoCode),
		fmt.Sprintf("https://flagpedia.net/data/flags/w20/%s.png", isoCode),
	}
	for attempt := 1; attempt <= 1; attempt++ {
		for _, url := range urls {
			resp, err := t.httpClient.Get(url)
			if err != nil {
				continue
			}
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				continue
			}
			if readErr != nil || len(body) == 0 {
				continue
			}
			icon, convErr := pngToICO(body, 16)
			if convErr != nil {
				continue
			}
			return icon
		}
	}
	return nil
}

func (t *Tray) prefetchCountryIcons(proxies []config.ProxyEntry) {
	unique := make(map[string]struct{})
	for _, p := range proxies {
		if iso := countryISOCode(p.Country); iso != "" {
			unique[iso] = struct{}{}
		}
	}
	if len(unique) == 0 {
		return
	}

	missing := make([]string, 0, len(unique))
	t.mu.Lock()
	for iso := range unique {
		if _, ok := t.countryIcons[iso]; !ok {
			missing = append(missing, iso)
		}
	}
	t.mu.Unlock()
	if len(missing) == 0 {
		return
	}


	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) 
	for _, iso := range missing {
		iso := iso
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			icon := t.downloadCountryIcon(iso)
			if len(icon) == 0 {
				return
			}
			t.mu.Lock()
			t.countryIcons[iso] = icon
			t.mu.Unlock()
		}()
	}
	wg.Wait()
}

func buildProxyListSignature(proxies []config.ProxyEntry) string {
	if len(proxies) == 0 {
		return "empty"
	}
	var b bytes.Buffer
	for _, p := range proxies {
		b.WriteString(p.ID)
		b.WriteByte('|')
		b.WriteString(p.Country)
		b.WriteByte('|')
		b.WriteString(p.Name)
		b.WriteByte(';')
	}
	return b.String()
}

func buildFallbackIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	bg := color.NRGBA{R: 52, G: 58, B: 64, A: 255}
	fg := color.NRGBA{R: 173, G: 181, B: 189, A: 255}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, bg)
		}
	}
	for y := 5; y <= 10; y++ {
		for x := 5; x <= 10; x++ {
			img.Set(x, y, fg)
		}
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return nil
	}
	icon, err := pngToICO(pngBuf.Bytes(), 16)
	if err != nil {
		return nil
	}
	return icon
}

func pngToICO(pngData []byte, size int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, err
	}
	if size <= 0 || size > 256 {
		return nil, fmt.Errorf("invalid target size: %d", size)
	}

	dstPixels := make([]byte, size*size*4)
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil, fmt.Errorf("invalid source image bounds")
	}

	for y := 0; y < size; y++ {
		sy := srcBounds.Min.Y + (y*srcH)/size
		for x := 0; x < size; x++ {
			sx := srcBounds.Min.X + (x*srcW)/size
			c := color.NRGBAModel.Convert(src.At(sx, sy)).(color.NRGBA)
			
			row := size - 1 - y
			i := (row*size + x) * 4
			dstPixels[i+0] = c.B
			dstPixels[i+1] = c.G
			dstPixels[i+2] = c.R
			dstPixels[i+3] = c.A
		}
	}

	maskRowSize := ((size + 31) / 32) * 4
	maskBytes := make([]byte, maskRowSize*size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			i := (y*size + x) * 4
			alpha := dstPixels[i+3]
			if alpha == 0 {
				byteIndex := y*maskRowSize + x/8
				bit := uint(7 - (x % 8))
				maskBytes[byteIndex] |= 1 << bit
			}
		}
	}

	var dib bytes.Buffer
	if err := binary.Write(&dib, binary.LittleEndian, uint32(40)); err != nil { 
		return nil, err
	}
	if err := binary.Write(&dib, binary.LittleEndian, int32(size)); err != nil {
		return nil, err
	}
	if err := binary.Write(&dib, binary.LittleEndian, int32(size*2)); err != nil { 
		return nil, err
	}
	if err := binary.Write(&dib, binary.LittleEndian, uint16(1)); err != nil { 
		return nil, err
	}
	if err := binary.Write(&dib, binary.LittleEndian, uint16(32)); err != nil { 
		return nil, err
	}
	if err := binary.Write(&dib, binary.LittleEndian, uint32(0)); err != nil { 
		return nil, err
	}
	if err := binary.Write(&dib, binary.LittleEndian, uint32(len(dstPixels)+len(maskBytes))); err != nil {
		return nil, err
	}
	for i := 0; i < 4; i++ { 
		if err := binary.Write(&dib, binary.LittleEndian, int32(0)); err != nil {
			return nil, err
		}
	}
	if _, err := dib.Write(dstPixels); err != nil {
		return nil, err
	}
	if _, err := dib.Write(maskBytes); err != nil {
		return nil, err
	}

	var icon bytes.Buffer
	if err := binary.Write(&icon, binary.LittleEndian, uint16(0)); err != nil {
		return nil, err
	}
	if err := binary.Write(&icon, binary.LittleEndian, uint16(1)); err != nil {
		return nil, err
	}
	if err := binary.Write(&icon, binary.LittleEndian, uint16(1)); err != nil {
		return nil, err
	}
	w := byte(size)
	if size == 256 {
		w = 0
	}
	if err := icon.WriteByte(w); err != nil {
		return nil, err
	}
	if err := icon.WriteByte(w); err != nil {
		return nil, err
	}
	if err := icon.WriteByte(0); err != nil {
		return nil, err
	}
	if err := icon.WriteByte(0); err != nil {
		return nil, err
	}
	if err := binary.Write(&icon, binary.LittleEndian, uint16(1)); err != nil {
		return nil, err
	}
	if err := binary.Write(&icon, binary.LittleEndian, uint16(32)); err != nil {
		return nil, err
	}
	if err := binary.Write(&icon, binary.LittleEndian, uint32(dib.Len())); err != nil {
		return nil, err
	}
	if err := binary.Write(&icon, binary.LittleEndian, uint32(6+16)); err != nil {
		return nil, err
	}
	if _, err := icon.Write(dib.Bytes()); err != nil {
		return nil, err
	}
	return icon.Bytes(), nil
}
