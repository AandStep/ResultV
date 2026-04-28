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

package main

import (
	"embed"
	"log"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"resultproxy-wails/internal/system"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var appIcon []byte

func main() {
	if runtime.GOOS == "windows" {
		system.SetProcessAppUserModelID()
	}
	app := NewApp()
	if system.ArgsStartInTray(os.Args) {
		app.SetStartInTray(true)
	}
	app.SetTrayIcon(appIcon)

	if link := system.ExtractDeepLinkArg(os.Args); link != "" {
		app.QueueDeepLink(link)
	}

	cleanupMessenger := system.InitSingletonMessenger(func(payload string) {
		app.restoreMainWindow()
		if payload != "" {
			app.HandleDeepLink(payload)
		}
	})
	defer cleanupMessenger()

	opts := &options.App{
		Title:     " ",
		Width:     1080,
		Height:    720,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 27, A: 1},
		OnStartup:        app.startup,
		OnBeforeClose:    app.BeforeClose,
		OnShutdown:       app.shutdown,
		Windows: &windows.Options{
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			DisableWindowIcon:                 true,
			DisableFramelessWindowDecorations: false,
			WebviewUserDataPath:               system.WebviewUserDataPath(),
			WindowClassName:                   system.WailsWindowClassResultV,
			Theme:                             windows.Dark,
			CustomTheme: &windows.ThemeSettings{
				DarkModeTitleBar:         windows.RGB(24, 24, 27),
				DarkModeTitleBarInactive: windows.RGB(24, 24, 27),
				DarkModeBorder:           windows.RGB(24, 24, 27),
				DarkModeBorderInactive:   windows.RGB(24, 24, 27),
				
				DarkModeTitleText:         windows.RGB(24, 24, 27),
				DarkModeTitleTextInactive: windows.RGB(24, 24, 27),
			},
		},
		Bind: []interface{}{
			app,
		},
	}
	if runtime.GOOS != "windows" {
		opts.SingleInstanceLock = &options.SingleInstanceLock{
			UniqueId: "resultv-desktop",
			OnSecondInstanceLaunch: func(_ options.SecondInstanceData) {
				app.restoreMainWindow()
			},
		}
	}

	err := wails.Run(opts)

	if err != nil {
		log.Fatalf("wails.Run: %v", err)
	}
}
