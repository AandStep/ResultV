// Copyright (C) 2026 ResultV
//
// Licensed under the terms of the GNU General Public License v3 or later.

package system

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"

	"resultproxy-wails/internal/config"
)

// The tray draws the flag as an icon, so a flag emoji left in the title
// would show up a second time — as letters, since Windows menus cannot
// render flag emoji.
func TestTrayServerTitle(t *testing.T) {
	cases := []struct {
		name  string
		entry config.ProxyEntry
		want  string
	}{
		{"flag emoji prefix goes", config.ProxyEntry{Name: "🇫🇮 VLESS TCP | Финляндия", Country: "FI"}, "VLESS TCP | Финляндия"},
		{"emoji then repeated code goes", config.ProxyEntry{Name: "🇳🇱 NL Нидерланды", Country: "nl"}, "Нидерланды"},
		{"code alone is kept when nothing follows", config.ProxyEntry{Name: "🇳🇱 NL", Country: "NL"}, "NL"},
		{"other code prefix is kept", config.ProxyEntry{Name: "DE Берлин", Country: "AT"}, "DE Берлин"},
		{"plain name is untouched", config.ProxyEntry{Name: "amnezia_for_awg", Country: "FI"}, "amnezia_for_awg"},
		{"emoji only falls back to the name", config.ProxyEntry{Name: "🇩🇪"}, "🇩🇪"},
		{"AUTO name keeps its own emoji", config.ProxyEntry{Name: "⚡ Авто | Лучший", Type: "AUTO"}, "⚡ Авто | Лучший"},
		{"no name falls back to host:port", config.ProxyEntry{IP: "5.6.7.8", Port: 1080}, "5.6.7.8:1080"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := trayServerTitle(tc.entry); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTrayServerIcon(t *testing.T) {
	cases := []struct {
		name  string
		entry config.ProxyEntry
		want  string
	}{
		{"country code", config.ProxyEntry{Country: "AT"}, "at"},
		{"country stored as emoji", config.ProxyEntry{Country: "🇯🇵"}, "jp"},
		{"no country, flag in name", config.ProxyEntry{Name: "🇸🇪 Швеция"}, "se"},
		{"country beats the name", config.ProxyEntry{Name: "🇸🇪 Швеция", Country: "DE"}, "de"},
		{"AUTO never shows a flag", config.ProxyEntry{Type: "auto", Country: "AT"}, trayIconAuto},
		{"unknown country", config.ProxyEntry{Name: "Сервер"}, trayIconGlobe},
		{"code missing from the atlas", config.ProxyEntry{Country: "XX"}, trayIconGlobe},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := trayServerIcon(tc.entry); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTrayUpdateServerListRows(t *testing.T) {
	tr := NewTray(nil, TrayCallbacks{})
	tr.UpdateServerList(TrayServerList{
		Groups: []TrayServerGroup{
			{Label: "impVPN", Proxies: []config.ProxyEntry{
				{ID: "auto", Name: "Авто", Type: "AUTO"},
				{ID: "fi", Name: "🇫🇮 Финляндия", Country: "FI"},
			}},
			{Label: "Мои сервера", Proxies: []config.ProxyEntry{{ID: "own", Name: "Свой"}}},
		},
		AutoHeadOf: map[string]string{"m1": "auto"},
	}, "fi")

	want := []trayServerRow{
		{title: "impVPN"},
		{proxyID: "auto", title: "Авто", icon: trayIconAuto},
		{proxyID: "fi", title: "Финляндия", icon: "fi"},
		{title: "Мои сервера"},
		{proxyID: "own", title: "Свой", icon: trayIconGlobe},
	}
	if len(tr.rows) != len(want) {
		t.Fatalf("rows = %+v", tr.rows)
	}
	for i := range want {
		if tr.rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, tr.rows[i], want[i])
		}
	}

	// An AUTO connection reports the member it dialled; the tray stands it
	// in for the head the user picked.
	tr.SetConnectedProxy("m1", "1.2.3.4:443")
	tr.mu.Lock()
	label := tr.connectedLabelLocked()
	listed := tr.listedIDLocked("m1")
	tr.mu.Unlock()
	if listed != "auto" || label != "Авто" {
		t.Fatalf("member m1 → %q / %q, want auto / Авто", listed, label)
	}
}

func TestTrayIconImage(t *testing.T) {
	img, err := trayIconImage("fi", 16)
	if err != nil {
		t.Fatal(err)
	}
	// 4:3 flag centred in a square cell: transparent above, opaque inside.
	if a := img.NRGBAAt(8, 0).A; a != 0 {
		t.Fatalf("row 0 alpha = %d, want 0", a)
	}
	if a := img.NRGBAAt(8, 8).A; a != 255 {
		t.Fatalf("centre alpha = %d, want 255", a)
	}

	// A size between atlases takes the smaller cell, centred.
	odd, err := trayIconImage("fi", 18)
	if err != nil {
		t.Fatal(err)
	}
	if odd.Rect.Dx() != 18 || odd.NRGBAAt(0, 9).A != 0 || odd.NRGBAAt(1, 9).A == 0 {
		t.Fatal("18px icon should hold the 16px cell with a 1px margin")
	}

	if _, err := trayIconImage("nope", 16); err == nil {
		t.Fatal("unknown icon must fail")
	}
}

func TestTrayMarkDrawsInCorner(t *testing.T) {
	img, err := trayIconImage(trayIconBlank, 16)
	if err != nil {
		t.Fatal(err)
	}
	drawTrayMark(img, trayMarkConnected)
	if img.NRGBAAt(0, 0).A != 0 {
		t.Fatal("mark leaked into the top-left corner")
	}
	c := img.NRGBAAt(11, 11)
	if c.A != 255 || c.G < 0x90 || c.R > 0x20 {
		t.Fatalf("dot centre = %+v, want opaque green", c)
	}
}

// energye shrinks the icon from SM_CXICON to SM_CXSMICON by point sampling;
// pixel doubling must hand back the exact small image.
func TestPixelUpscaleSurvivesPointSampling(t *testing.T) {
	small, err := trayIconImage("gb", 16)
	if err != nil {
		t.Fatal(err)
	}
	big := pixelUpscale(small, 32)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			for _, d := range [][2]int{{0, 0}, {1, 1}} {
				if small.NRGBAAt(x, y) != big.NRGBAAt(2*x+d[0], 2*y+d[1]) {
					t.Fatalf("pixel %d,%d lost", x, y)
				}
			}
		}
	}
}

func TestEncodeICO(t *testing.T) {
	img, err := trayIconImage("de", 32)
	if err != nil {
		t.Fatal(err)
	}
	ico, err := encodeICO(img)
	if err != nil {
		t.Fatal(err)
	}
	var hdr struct {
		Reserved, Type, Count uint16
		W, H, Colors, _       uint8
		Planes, Bits          uint16
		Size, Offset          uint32
	}
	if err := binary.Read(bytes.NewReader(ico), binary.LittleEndian, &hdr); err != nil {
		t.Fatal(err)
	}
	if hdr.Type != 1 || hdr.Count != 1 || hdr.W != 32 || hdr.H != 32 || hdr.Bits != 32 || int(hdr.Offset+hdr.Size) != len(ico) {
		t.Fatalf("bad ico header %+v (len %d)", hdr, len(ico))
	}
	if _, err := png.Decode(bytes.NewReader(ico[hdr.Offset:])); err != nil {
		t.Fatalf("embedded png: %v", err)
	}
}
