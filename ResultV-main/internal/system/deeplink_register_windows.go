// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build windows

package system

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// RegisterResultVProtocol registers the resultv:// URL scheme under the
// current user so links opened in a browser launch this executable. Safe to
// call on every startup — only writes when the existing target differs.
//
// Installer-based installs already register the protocol (HKLM or HKCU via
// NSIS), but portable builds rely on this runtime fallback.
func RegisterResultVProtocol() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	command := `"` + exe + `" "%1"`
	icon := exe + ",0"

	root, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\resultv`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer root.Close()

	if existing, _, _ := root.GetStringValue(""); existing != "URL:ResultV Protocol" {
		_ = root.SetStringValue("", "URL:ResultV Protocol")
	}
	if _, _, err := root.GetStringValue("URL Protocol"); err != nil {
		_ = root.SetStringValue("URL Protocol", "")
	}

	iconKey, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\resultv\DefaultIcon`, registry.SET_VALUE)
	if err == nil {
		if existing, _, _ := iconKey.GetStringValue(""); existing != icon {
			_ = iconKey.SetStringValue("", icon)
		}
		iconKey.Close()
	}

	cmdKey, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\resultv\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer cmdKey.Close()
	if existing, _, _ := cmdKey.GetStringValue(""); existing != command {
		return cmdKey.SetStringValue("", command)
	}
	return nil
}
