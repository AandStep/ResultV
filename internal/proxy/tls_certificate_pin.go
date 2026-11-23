package proxy

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const xrayCertificatePinTimeout = 5 * time.Second
const connectErrorTLSCertificatePin = "tls_certificate_pin_failed"

var (
	errXrayCertificatePinCheck = errors.New("TLS certificate pin check failed")
	errXrayCertificateMismatch = errors.New("TLS leaf certificate does not match the configured fingerprint")
)

// prepareXrayCertificatePin verifies the exact leaf DER fingerprint before
// starting the core, then pins its public key in the core's actual TLS dials.
// This core version supports key pinning only. A mismatched or unreachable
// peer leaves the core unstarted. No application data is sent by this check.
func prepareXrayCertificatePin(ctx context.Context, proxy ProxyConfig, cfg *SingBoxConfig, verifiedKey []byte) ([]byte, error) {
	raw := getStringField(parseExtra(proxy), "pinnedPeerCertSha256", "")
	if raw == "" {
		return nil, nil
	}
	pins := make(map[[sha256.Size]byte]bool)
	for _, field := range strings.Split(raw, ",") {
		digest, err := hex.DecodeString(strings.ReplaceAll(strings.TrimSpace(field), ":", ""))
		if err != nil || len(digest) != sha256.Size {
			return nil, errors.New("invalid TLS certificate SHA-256 fingerprint")
		}
		pins[[sha256.Size]byte(digest)] = true
	}

	for i := range cfg.Outbounds {
		out := &cfg.Outbounds[i]
		if out.Tag != "proxy" {
			continue
		}
		if out.TLS == nil || !out.TLS.Enabled || out.TLS.Reality != nil {
			return nil, errors.New("certificate pin requires a TLS proxy outbound")
		}
		if verifiedKey == nil {
			key, err := resolveXrayCertificateKey(ctx, proxy, out, pins)
			if err != nil {
				return nil, err
			}
			verifiedKey = key[:]
		}
		out.TLS.Insecure = false
		out.TLS.CertificatePublicKeySHA256 = [][]byte{verifiedKey}
		return verifiedKey, nil
	}
	return nil, errors.New("certificate pin requires a TLS proxy outbound")
}

func resolveXrayCertificateKey(ctx context.Context, proxy ProxyConfig, out *SBOutbound, pins map[[sha256.Size]byte]bool) ([sha256.Size]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, xrayCertificatePinTimeout)
	defer cancel()
	addresses := serverPinnedIPs(proxy)
	if len(addresses) == 0 {
		addresses = []string{out.Server}
	}
	var key [sha256.Size]byte
	verify := func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 || !pins[sha256.Sum256(state.PeerCertificates[0].Raw)] {
			return errXrayCertificateMismatch
		}
		publicKey, err := x509.MarshalPKIXPublicKey(state.PeerCertificates[0].PublicKey)
		if err != nil {
			return err
		}
		key = sha256.Sum256(publicKey)
		return nil
	}
	tlsOptions := &tls.Config{
		ServerName:         firstNonEmpty(out.TLS.ServerName, out.Server),
		NextProtos:         out.TLS.ALPN,
		InsecureSkipVerify: true, // VerifyConnection requires an exact certificate pin.
		VerifyConnection:   verify,
	}
	var lastErr error
	for _, ip := range addresses {
		dialer := &net.Dialer{Timeout: 2 * time.Second}
		if bind := net.ParseIP(out.Inet4BindAddress); bind != nil && !strings.Contains(ip, ":") && !isLoopbackProbeHost(ip) {
			dialer.LocalAddr = &net.TCPAddr{IP: bind}
		}
		tlsDialer := tls.Dialer{NetDialer: dialer, Config: tlsOptions}
		conn, err := tlsDialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(out.ServerPort)))
		if err == nil {
			_ = conn.Close()
			return key, nil
		}
		lastErr = err
	}
	// Raw dial and DNS errors may include a subscription server's address.
	var timeout net.Error
	switch {
	case errors.Is(lastErr, errXrayCertificateMismatch):
		return key, fmt.Errorf("%w: %w", errXrayCertificatePinCheck, errXrayCertificateMismatch)
	case errors.Is(ctx.Err(), context.Canceled):
		return key, fmt.Errorf("%w: %w", errXrayCertificatePinCheck, context.Canceled)
	case errors.As(lastErr, &timeout) && timeout.Timeout():
		return key, fmt.Errorf("%w: timed out reaching the proxy", errXrayCertificatePinCheck)
	}
	return key, fmt.Errorf("%w: could not reach the proxy over TLS", errXrayCertificatePinCheck)
}
