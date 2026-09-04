// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build !no_adblock

package mobile

// adBlockCompiledIn mirrors proxy.adBlockSupported for the tests. The `play`
// distribution builds with no_adblock and emits no rule_set rules at all, so
// assertions anchored on one have no premise there — the ordering invariants
// that survive the cut stay under test in both configurations.
const adBlockCompiledIn = true
