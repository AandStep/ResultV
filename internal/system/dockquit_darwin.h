// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

#pragma once

// Installs a custom Dock context menu (applicationDockMenu:) on the running
// NSApplication delegate with an explicit "Quit" item that calls back into Go.
// This is separate from the system "Force Quit" entry macOS shows when it
// thinks the app is hung; our item uses the same graceful path as Cmd+Q.
void resultvInstallDockQuitMenu(void);

// Installs on the main thread; returns 1 when applicationDockMenu: was added, 0 otherwise.
int resultvInstallDockQuitMenuSync(void);
