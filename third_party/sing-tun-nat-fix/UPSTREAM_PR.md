# Upstream PR draft — SagerNet/sing-tun

Open against https://github.com/SagerNet/sing-tun . Attach `stack_system_nat.patch`
(applies to `main`; the function is identical there) and add the regression test
as `stack_system_nat_test.go`.

---

**Title:** fix(system): don't reuse in-use NAT ports — monotonic uint16 counter wraps and tears down live connections

**Body:**

### Problem

On the `system` stack, `TCPNat.Lookup` (`stack_system_nat.go`) assigns each new
TCP flow a NAT port from a monotonically incrementing `uint16` counter
(`portIndex`, 10000..65535) and writes `portMap[nextPort]` **without checking
whether that port is still held by a live session**:

```go
nextPort := n.portIndex
if nextPort == 0 {
    nextPort = 10000
    n.portIndex = 10001
} else {
    n.portIndex++
}
n.addrMap[source] = nextPort
...
n.portMap[nextPort] = &TCPSession{...} // overwrites any existing session
```

After ~55,536 flows the counter wraps back to 10000. As it sweeps the low ports
again it overwrites the NAT slots of **still-alive long-lived flows** (gRPC/
websocket/push/streaming). Their reverse mapping (`LookupBack`) then resolves to
the wrong session and the OS aborts them — every long-lived connection drops in
a ~1s burst.

This is only reachable under very high connection churn (e.g. DNS-over-TCP
through the tunnel + heavy short-lived traffic), which is why it's rarely seen,
but it is reproducible and protocol-independent. Reported downstream as all
tunnelled connections dropping at once with `WSAECONNABORTED`/`WSAECONNRESET` on
Windows under sustained churn.

### Fix

Scan forward from the rolling cursor for a port that no live session occupies,
instead of blindly overwriting. Closed sessions are reaped within the NAT
timeout, so in practice far fewer than the ~55k ports are held at once and the
scan resolves immediately. Genuine simultaneous-flow exhaustion now returns an
error instead of silently corrupting an established connection. Lock order
(`portAccess` then `addrAccess`) matches `checkTimeout` to avoid a deadlock.

### Test

`TestTCPNatDoesNotReuseLivePorts` holds 50 long-lived sessions, then churns
200,000 short-lived flows (≈3.6× the port range) with each reaped immediately.
Pre-fix it fails at iteration 55,536 (port 10000 reused while live); post-fix it
passes and all 50 long-lived sessions remain intact.
