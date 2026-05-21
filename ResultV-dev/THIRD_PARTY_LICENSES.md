# Third-Party Licenses

ResultV includes the following third-party components. All other code is
licensed under GPL-3.0 — see [LICENSE](LICENSE).

---

## getlantern/systray

- **Source**: https://github.com/getlantern/systray v1.2.2
- **Location**: `internal/getlantern_systray/`
- **License**: Apache-2.0 (see `internal/getlantern_systray/LICENSE`)

The vendored copy has been modified by ResultV. Original package comment:
> Package systray is a cross-platform Go library to place an icon and menu
> in the notification area.

---

## Go module dependencies

All other dependencies are consumed as normal Go modules and are not
vendored into this repository. Their licenses are reproduced in the module
cache (`go env GOMODCACHE`) and can be audited with:

```
go-licenses report ./...
```

Key dependency licenses:
| Module | License |
|--------|---------|
| github.com/sagernet/sing-box | GPL-3.0 |
| github.com/sagernet/sing | MIT |
| github.com/sagernet/sing-tun | MIT |
| github.com/sagernet/sing-quic | MIT |
| github.com/sagernet/quic-go | MIT |
| github.com/sagernet/gvisor | Apache-2.0 |
| github.com/wailsapp/wails | MIT |
| golang.org/x/sys | BSD-3-Clause |
| github.com/getlantern/golog | Apache-2.0 |
| github.com/AdguardTeam/urlfilter | GPL-3.0 |
| github.com/AdguardTeam/gomitmproxy | MIT |
| github.com/AdguardTeam/golibs | Apache-2.0 |
