// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build !darwin

package system

func RegisterDockQuitHandler(_, _ func()) {}
func InstallDockQuitMenu() bool { return false }
