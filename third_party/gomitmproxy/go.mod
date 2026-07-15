// Local fork of github.com/AdguardTeam/gomitmproxy v0.2.1 — see README-FORK.md.
// Wired in via a `replace` in the repo-root go.mod; its dependency versions are
// resolved by the parent module (this file only declares the module path so the
// replace target is a valid module). Test-only deps from upstream are dropped.
module github.com/AdguardTeam/gomitmproxy

go 1.14

// Versions aligned with the parent module's resolved graph (see repo-root
// go.mod) so tooling that loads this nested module standalone finds matching
// go.sum entries. Upstream v0.2.1 pinned older minimums; MVS in the parent
// already selects these, and the fork compiles cleanly against them.
require (
	github.com/AdguardTeam/golibs v0.35.13
	github.com/pkg/errors v0.9.1
	golang.org/x/text v0.38.0
)
