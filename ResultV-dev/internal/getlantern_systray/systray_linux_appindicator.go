// Originally from https://github.com/getlantern/systray (Apache-2.0).
// Modified by ResultV. Modifications are GPL-3.0 — see top-level LICENSE.

// +build linux,legacy_appindicator
//go:build linux && legacy_appindicator

package systray

/*
#cgo pkg-config: gtk+-3.0 appindicator3-0.1
#include "systray.h"
*/
import "C"
