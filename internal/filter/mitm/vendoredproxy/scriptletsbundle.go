// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import _ "embed"

// scriptletsBundle is the vendored @adguard/scriptlets UMD build (GPL-3),
// served from the injection host at /scriptlets.js so the content script
// can turn `//scriptlet(...)` rule text into executable code. See NOTICE.md
// for the pinned version and re-vendoring instructions.
//
//go:embed scriptlets.umd.min.js
var scriptletsBundle []byte
