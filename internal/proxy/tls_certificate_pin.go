package proxy

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	sbtls "github.com/sagernet/sing-box/common/tls"
	sblog "github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
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
	var key [sha256.Size]byte
	ctx, cancel := context.WithTimeout(ctx, xrayCertificatePinTimeout)
	defer cancel()
	addresses := serverPinnedIPs(proxy)
	if len(addresses) == 0 {
		addresses = []string{out.Server}
	}
	singleAddress := len(addresses) == 1
	if singleAddress {
		addresses = append(addresses, addresses[0])
	}
	tlsOptions := option.OutboundTLSOptions{
		Enabled:      true,
		ServerName:   out.TLS.ServerName,
		ALPN:         out.TLS.ALPN,
		MinVersion:   out.TLS.MinVersion,
		MaxVersion:   out.TLS.MaxVersion,
		CipherSuites: out.TLS.CipherSuites,
		Insecure:     true, // Only fetch the peer certificate. Its SHA-256 must match below.
	}
	if utls := out.TLS.UTLS; utls != nil {
		tlsOptions.UTLS = &option.OutboundUTLSOptions{Enabled: utls.Enabled, Fingerprint: utls.Fingerprint}
	}
	tlsClient, err := sbtls.NewClient(ctx, sblog.NewNOPFactory().Logger(), out.Server, tlsOptions)
	if err != nil {
		return key, fmt.Errorf("%w: invalid proxy TLS options", errXrayCertificatePinCheck)
	}
	var lastErr error
	for _, ip := range addresses {
		key, err = probeXrayCertificateKey(ctx, ip, out, tlsClient.Clone(), pins)
		if err == nil {
			return key, nil
		}
		lastErr = err
		// Never repeat a known rejection on the same endpoint.
		if singleAddress && errors.Is(err, errXrayCertificateMismatch) {
			break
		}
	}
	// Raw dial and DNS errors may include a subscription server's address.
	var timeout net.Error
	switch {
	case errors.Is(lastErr, errXrayCertificateMismatch):
		return key, fmt.Errorf("%w: %w", errXrayCertificatePinCheck, errXrayCertificateMismatch)
	case errors.Is(ctx.Err(), context.Canceled):
		return key, fmt.Errorf("%w: %w", errXrayCertificatePinCheck, context.Canceled)
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return key, fmt.Errorf("%w: timed out reaching the proxy", errXrayCertificatePinCheck)
	case errors.As(lastErr, &timeout) && timeout.Timeout():
		return key, fmt.Errorf("%w: timed out reaching the proxy", errXrayCertificatePinCheck)
	}
	return key, fmt.Errorf("%w: could not reach the proxy over TLS", errXrayCertificatePinCheck)
}

func probeXrayCertificateKey(ctx context.Context, ip string, out *SBOutbound, tlsClient sbtls.Config, pins map[[sha256.Size]byte]bool) ([sha256.Size]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	dialer := &net.Dialer{}
	if bind := net.ParseIP(out.Inet4BindAddress); bind != nil && !strings.Contains(ip, ":") && !isLoopbackProbeHost(ip) {
		dialer.LocalAddr = &net.TCPAddr{IP: bind}
	}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(out.ServerPort)))
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer conn.Close()
	tlsConn, err := sbtls.ClientHandshake(ctx, conn, tlsClient)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 || !pins[sha256.Sum256(certs[0].Raw)] {
		return [sha256.Size]byte{}, errXrayCertificateMismatch
	}
	publicKey, err := x509.MarshalPKIXPublicKey(certs[0].PublicKey)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(publicKey), nil
}
