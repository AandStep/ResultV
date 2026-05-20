// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

#pragma once

// Reply to a pending NSApplication terminate request. Safe from any thread.
void resultvReplyToApplicationShouldTerminate(void);

// Swizzles applicationShouldTerminate: on [NSApp delegate] so we never block the
// main thread on Wails' synchronous processMessage("Q"), reply immediately, and
// route quit through the Go callback. Returns 1 when installed, 0 otherwise.
int resultvInstallTerminateInterceptorSync(void);
