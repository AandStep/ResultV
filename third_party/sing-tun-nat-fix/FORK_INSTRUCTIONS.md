# Carrying the sing-tun NAT-wrap fix via an `AandStep/sing-tun` fork

This repo currently carries the fix by **vendoring** the patched module at
`third_party/sing-tun/` (a `replace` in `go.mod` points at it). That is
self-contained and needs no GitHub fork. If you'd rather keep the ResultV repo
small (the vendored copy is ~145 files), move the fix into a GitHub fork and
replace the local path with a module version. Steps below.

## 1. Create the fork

```bash
# Clone upstream at the exact version sing-box 1.13.x uses
git clone https://github.com/SagerNet/sing-tun.git
cd sing-tun
git checkout v0.8.10            # latest release; identical NAT code to v0.8.9
git checkout -b nat-port-wrap-fix
```

## 2. Apply the patch + regression test

```bash
git apply /path/to/third_party/sing-tun-nat-fix/stack_system_nat.patch
cp /path/to/third_party/sing-tun-nat-fix/stack_system_nat_natwrap_test.go ./stack_system_nat_test.go
go test -run TestTCPNatDoesNotReuseLivePorts ./.   # must PASS
git add -A && git commit -m "fix(system): scan for free NAT port instead of monotonic uint16 wrap"
```

## 3. Push to your org and tag

```bash
git remote set-url origin https://github.com/AandStep/sing-tun.git   # create this repo first
git push -u origin nat-port-wrap-fix
git tag v0.8.10-extended-1.0.0
git push origin v0.8.10-extended-1.0.0
```

## 4. Point ResultV at the fork (replaces the vendored copy)

In `go.mod`, swap the vendored replace:

```diff
-replace github.com/sagernet/sing-tun => ./third_party/sing-tun
+replace github.com/sagernet/sing-tun => github.com/AandStep/sing-tun v0.8.10-extended-1.0.0
```

Then:

```bash
go mod tidy
git rm -r third_party/sing-tun            # drop the vendored copy
wails build                                # confirm it still builds
```

This matches the pattern of your other `replace` lines (shtorm-7/* forks).

## Maintenance on a future sing-tun bump

`Lookup` has been byte-identical from v0.7.x through `main`, so re-applying is
trivial: `git checkout <new-tag> -b nat-fix-<ver>`, `git apply` the patch (or
cherry-pick the commit), re-tag. Better: land the upstream PR (see
`UPSTREAM_PR.md`) — once merged, drop the fork/vendor entirely and use the
stock version.
