package mobile

import (
	"context"
	"fmt"
	"sync/atomic"

	"resultproxy-wails/internal/proxy"
)

type tlsCertificateSession struct {
	verified atomic.Pointer[verifiedTLSCertificate]
}

type verifiedTLSCertificate struct {
	host  string
	port  int
	extra string
	key   []byte
}

var certificateSession atomic.Pointer[tlsCertificateSession]

func setTLSCertificateSessionActive(active bool) {
	if active {
		certificateSession.CompareAndSwap(nil, &tlsCertificateSession{})
	} else {
		certificateSession.Store(nil)
	}
}

func prepareSessionCertificatePin(node proxy.ProxyConfig, cfg *proxy.SingBoxConfig) error {
	session := certificateSession.Load()
	var key []byte
	var previous *verifiedTLSCertificate
	if session != nil {
		previous = session.verified.Load()
		if previous != nil && previous.host == node.IP && previous.port == node.Port && previous.extra == string(node.Extra) {
			key = previous.key
		}
	}
	verified, err := proxy.PrepareXrayCertificatePin(context.Background(), node, cfg, key)
	if err != nil {
		return fmt.Errorf("%s: %w", proxy.TLSCertificatePinFailureCode, err)
	}
	if session == nil || key != nil {
		return nil
	}
	// Route and kill-switch reloads can build while the proxy is down. Reuse
	// its verified key within the current session for the recovery probe.
	var pin *verifiedTLSCertificate
	if verified != nil {
		pin = &verifiedTLSCertificate{host: node.IP, port: node.Port, extra: string(node.Extra), key: verified}
	}
	session.verified.CompareAndSwap(previous, pin)
	return nil
}
