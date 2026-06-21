// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build !windows

package ca

import "fmt"

func IsInstalled() bool { return false }

func Install(certPath string) error {
	return fmt.Errorf("automatic CA install is only supported on Windows")
}

func Uninstall() error { return nil }
