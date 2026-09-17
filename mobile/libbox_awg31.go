// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mobile

import (
	"fmt"
	"sync"

	"github.com/sagernet/sing-box/experimental/libbox"

	"resultproxy-wails/internal/proxy"
)

// The AmneziaWG 3.1 switches cannot travel in the engine config (see
// internal/proxy/awg31.go), so they are pushed into the running device after
// start. On Android that means reaching the box, and the box belongs to Kotlin:
// libbox.CommandServer is created there and lives for the session. gomobile
// binds both packages in one invocation (scripts/build-android-aar.sh), so a
// *libbox.CommandServer is a legal parameter here — that is the whole reason
// this file can exist.
//
// The node is not passed in. It is remembered at the moment its config is
// built, which is the one funnel every path goes through — subscription entry,
// pasted URI, AUTO member swap, and every reload. Threading the entry through
// the Kotlin call sites instead would mean touching five of them, two of which
// (the browser ad-block attachment) never hold the node at all.
var (
	builtNodeMu sync.Mutex
	builtNodeIn proxy.ProxyConfig
)

// rememberBuiltNode records the node the engine is about to be started with.
func rememberBuiltNode(p proxy.ProxyConfig) {
	builtNodeMu.Lock()
	defer builtNodeMu.Unlock()
	builtNodeIn = p
}

// builtNode returns the node of the most recently built config.
func builtNode() proxy.ProxyConfig {
	builtNodeMu.Lock()
	defer builtNodeMu.Unlock()
	return builtNodeIn
}

// ApplyAWG31 pushes the AmneziaWG 3.1 switches of the running node into its
// live WireGuard device. Returns a description of what was applied ("" when
// the node states nothing), and an error when the device could not be reached
// — which is not fatal: the session keeps running with 3.0 behaviour.
//
// Called from Kotlin right after startOrReloadService, on start and on every
// reload: the device is recreated each time, and random_trailers has to match
// the peer before the first handshake is answered.
func ApplyAWG31(server *libbox.CommandServer) (string, error) {
	if server == nil {
		return "", fmt.Errorf("AWG 3.1: сервер ядра не передан")
	}
	instance := server.Instance()
	if instance == nil || instance.Box() == nil {
		return "", fmt.Errorf("AWG 3.1: ядро не запущено")
	}
	return proxy.ApplyAWG31(instance.Box().Endpoint(), builtNode())
}

// WGDiagLine returns one line of WireGuard device and gVisor stack counters for
// the running session, or an error when there is no WireGuard endpoint to read
// — which is the ordinary case for every other protocol.
//
// Sampling is driven from Kotlin (WgDiagSampler) rather than by a goroutine
// here: the desktop writes these into its core log file, and on Android the
// only sink that reaches the user is the in-app log, which lives in Kotlin.
func WGDiagLine(server *libbox.CommandServer) (string, error) {
	if server == nil {
		return "", fmt.Errorf("счётчики WG: сервер ядра не передан")
	}
	instance := server.Instance()
	if instance == nil || instance.Box() == nil {
		return "", fmt.Errorf("счётчики WG: ядро не запущено")
	}
	return proxy.WGDiagLine(instance.Box().Endpoint())
}
