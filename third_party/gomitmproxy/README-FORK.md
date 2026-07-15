# gomitmproxy (local fork)

Vendored copy of `github.com/AdguardTeam/gomitmproxy` **v0.2.1** (GPL-3.0, see
LICENSE), wired in via a `replace` directive in the root `go.mod`.

## Why the fork exists

The Android browser ad-block feature (`internal/filter/mitm`) must send its
upstream traffic **through the sing-box tunnel**, not straight to the
internet: the app's own package is excluded from the VPN
(`BoxPlatform.applyAppRouting`), so any plain `net.Dial` from this process
bypasses the tunnel entirely — in RF that kills every RKN-blocked site in the
browser while the VPN is on.

Upstream gomitmproxy has **two** dial paths and only one of them is
configurable:

- `Proxy.connect()` — honours `Config.OnConnect` (CONNECT tunnels,
  passthrough, WebSocket);
- the private `http.Transport` used by `RoundTrip` for MITM'd HTTPS and plain
  HTTP requests — **no hook at all** in v0.2.1.

## The patches

1. `Config.Dial func(network, addr string) (net.Conn, error)` — when set, it
   replaces BOTH the internal dialer (`proxy.dial`) and the transport's
   `DialContext`, so every upstream connection goes through the caller-supplied
   dialer (we pass a SOCKS5 client pointed at the engine's loopback inbound).

2. `Proxy.Close()` force-closes live sockets. Upstream's `Close()` only waits
   on `conns.Wait()`, but passthrough CONNECT tunnels sit in `io.Copy` pairs
   that never observe the `closing` channel — their only exit is the absolute
   read deadline (up to 5 minutes). On Android that pinned the whole VPN
   disconnect: `StopMITM` blocked, `BoxModule.stop()` queued behind it never
   ran, the TUN stayed up pointing at a dead proxy, and reconnect hung.
   The fork keeps a registry of client + tunnel-upstream sockets
   (`trackConn`/`untrackConn`) and closes them all in `Close()`. Regression
   test: `internal/filter/mitm/close_hang_test.go`.

Everything else is byte-identical to upstream v0.2.1 (tests/examples/CI
files dropped).
