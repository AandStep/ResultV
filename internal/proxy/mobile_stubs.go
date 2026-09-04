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

//go:build mobile

// Mobile-build stubs for symbols that live in desktop-only files
// (datadir.go, manager.go) and would otherwise leave the proxy package
// undefined under -tags=mobile. These stubs deliberately have no
// platform integration: callers on mobile must supply DataDir via
// EngineConfig and must not rely on post-start HTTP probing.

package proxy

// resultProxyDataDir returns "" on mobile builds. The Android caller
// must always set EngineConfig.DataDir to ctx.filesDir explicitly.
func resultProxyDataDir() string {
	return ""
}

// tunnelProbeDomains is empty on mobile — Android VpnService delivers
// connectivity feedback through the OS itself, so the desktop's
// post-start HTTP probe is unnecessary.
var tunnelProbeDomains = []string{}

// defaultUTLSFingerprint is "chrome" on mobile: there is no Edge WebView2 to
// match against, and chrome is the safest mainstream fingerprint that still
// passes Reality's masquerade check.
func defaultUTLSFingerprint() string { return "chrome" }
