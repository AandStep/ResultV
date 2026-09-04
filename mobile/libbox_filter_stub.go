// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build no_mitm

// Stub browser ad-block bindings for the `play` distribution.
//
// Google Play's VpnService policy forbids VPN apps from interfering with ads,
// and no app in the store ships TLS interception with a user-installed root CA
// — that is exactly why the full AdGuard is distributed outside Play. The Play
// build therefore must not merely leave this feature unreachable: the code must
// not be in the binary at all, because review looks at what the binary
// contains, not at what Kotlin calls.
//
// Signatures are kept identical to the real ones so the shared Kotlin code
// compiles against either build.

package mobile

import "errors"

// ErrMITMUnavailable is returned by every browser ad-block binding here.
var ErrMITMUnavailable = errors.New("browser ad-block is not available in this build")

func FetchFilterLists(dataDir string) (string, error) { return "", ErrMITMUnavailable }

func FilterCARootPath(dataDir string) (string, error) { return "", ErrMITMUnavailable }

func SetFilterCASeed(dataDir, seed string) error { return ErrMITMUnavailable }

func StartFilterProxy(dataDir string, listenPort int) (string, error) {
	return "", ErrMITMUnavailable
}

func StopFilterProxy() {}

func FilterStatus(dataDir string) (string, error) { return "", ErrMITMUnavailable }
