// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import _ "embed"

// extendedCSSBundle is the vendored @adguard/extended-css UMD build (GPL-3),
// served from the injection host at /extended-css.js so the content script
// can apply ExtendedCSS cosmetic rules (:has, :contains, :matches-css,
// :upward, :remove, …) that a plain <style> tag cannot express. See NOTICE.md
// for the pinned version and re-vendoring instructions.
//
//go:embed extendedcss.umd.min.js
var extendedCSSBundle []byte
