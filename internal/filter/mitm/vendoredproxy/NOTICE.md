# Vendored from AdguardTeam/urlfilter

This package is vendored from `github.com/AdguardTeam/urlfilter@v0.23.2`'s
`proxy` subpackage (https://github.com/AdguardTeam/urlfilter), licensed
GPL-3.0 (same license as this project).

Vendored on: 2026-07-02.

## Why vendored instead of imported

`urlfilter/proxy.NewServer` unconditionally overwrites the underlying
`gomitmproxy.Config`'s `OnRequest`/`OnResponse`/`OnConnect` fields during
construction and exposes no extension point — there is no way to layer
additional response post-processing on top of the package's own `*Server`
from outside the package. See
`docs/superpowers/specs/2026-07-02-browser-adblock-hardening-design.md`.

## Modifications from upstream

- `htmlfilter.go`: `filterHTML` now also strips
  `<meta http-equiv="Content-Security-Policy">` tags from HTML response
  bodies before computing the cosmetic-injection point. Upstream only
  strips CSP delivered via HTTP response headers (its own
  `// TODO(ameshkov): HANDLE CSP PROPERLY!` comment, left unmodified,
  documents this gap) — meta-tag CSP silently blocked the injected
  content-script on any page using it.

All other files (`proxy.go`, `handlers.go`, `contentscript.go`,
`contentscripttmpl.go`, `doc.go`, `httpcache.go`, `pages.go`,
`pagestmpl.go`, `session.go`, `util.go`) are byte-for-byte identical to
upstream v0.23.2 — verify with a `diff` against
`$(go env GOMODCACHE)/github.com/!adguard!team/urlfilter@v0.23.2/proxy/`.

## scriptlets.umd.min.js

Vendored from the `@adguard/scriptlets` npm package, version 2.4.3, file
`dist/scriptlets/index.js` (via cdn.jsdelivr.net). Copyright (C) AdGuard
Software Ltd. Licensed GPL-3.0 — same as this project.

To update: download the new `dist/scriptlets/index.js`, replace the file,
update the version here, and re-run the vendoredproxy tests.
