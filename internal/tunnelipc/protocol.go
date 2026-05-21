// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package tunnelipc holds the wire format shared between the main ResultV app
// and the privileged macOS tunnel helper (cmd/tunnel-helper).
//
// Transport: a unix-domain socket created by the helper, chowned to the main
// process's UID, lock-stepped to one connection. Frames: newline-delimited
// JSON, one Request or Response per line. The helper accepts exactly one
// connection; concurrent connect attempts are dropped.
package tunnelipc

import "encoding/json"

// CmdStart asks the helper to start sing-box with the supplied raw SingBoxConfig
// JSON. The helper does not interpret the inner JSON — it forwards it straight
// to sing-box's option parser. Putting the config on the wire as opaque bytes
// keeps the protocol stable across sing-box upgrades.
const CmdStart = "start"

// CmdStop asks the helper to stop the running sing-box engine. The helper
// stays alive afterwards so the caller can issue another start.
const CmdStop = "stop"

// CmdStatus asks for the helper's current state. Cheap, side-effect free —
// useful for connect-time sanity checks and for the UI to poll if needed.
const CmdStatus = "status"

// CmdShutdown asks the helper to stop sing-box (if running) and exit the
// process. The main app sends this in BeforeClose so the helper doesn't
// outlive the GUI.
const CmdShutdown = "shutdown"

// Request is the wire shape main → helper. Config is only used for CmdStart;
// its contents are the JSON representation of an internal/proxy.SingBoxConfig.
type Request struct {
	Cmd    string          `json:"cmd"`
	Config json.RawMessage `json:"config,omitempty"`
}

// Response is the wire shape helper → main. OK signals whether the command
// completed; Error carries a human-readable failure description (already
// suitable for surfacing in the UI). Running is set by CmdStatus.
type Response struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Running bool   `json:"running,omitempty"`
}

// SocketPathSuffix is the suffix appended to the user data directory to form
// the helper's unix socket path. Centralised here so main and helper agree.
const SocketPathSuffix = "tunnel-helper.sock"
