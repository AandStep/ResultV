// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const (
	certFile = "ca.crt"
	keyFile  = "ca.key"
	caCN     = "ResultV AdBlock Root CA"
)

// Fixed validity window for deterministically generated roots. Using
// time.Now() would make the certificate bytes non-reproducible, defeating
// the whole point of regenerating the same CA after reinstall.
var (
	deterministicNotBefore = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	deterministicNotAfter  = time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
)

// Root holds paths and key material for the filtering root CA.
type Root struct {
	CertificatePath string
	PrivateKeyPath  string
	Certificate     *x509.Certificate
	PrivateKey      *rsa.PrivateKey
}

// EnsureRoot creates or loads the root CA under filterDir. When the CA does
// not yet exist and seed is non-empty, generation is deterministic: the same
// seed reproduces byte-identical cert and key, so a reinstalled app recreates
// the exact CA already trusted by the system. An empty seed falls back to
// random generation.
func EnsureRoot(filterDir, seed string) (*Root, error) {
	if err := os.MkdirAll(filterDir, 0o700); err != nil {
		return nil, err
	}
	certPath := filepath.Join(filterDir, certFile)
	keyPath := filepath.Join(filterDir, keyFile)
	if _, err := os.Stat(certPath); err == nil {
		return loadRoot(certPath, keyPath)
	}
	if seed != "" {
		return generateDeterministic(certPath, keyPath, seed)
	}
	return generateRoot(certPath, keyPath)
}

func generateRoot(certPath, keyPath string) (*Root, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   caCN,
			Organization: []string{"ResultV"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return writeRoot(certPath, keyPath, der, key)
}

// writeRoot persists cert (DER) and key to disk and returns the parsed Root.
func writeRoot(certPath, keyPath string, der []byte, key *rsa.PrivateKey) (*Root, error) {
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER := x509.MarshalPKCS1PrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &Root{
		CertificatePath: certPath,
		PrivateKeyPath:  keyPath,
		Certificate:     cert,
		PrivateKey:      key,
	}, nil
}

func generateDeterministic(certPath, keyPath, seed string) (*Root, error) {
	drbg := newDRBG(seed)
	key, err := generateDeterministicKey(drbg, 2048)
	if err != nil {
		return nil, err
	}
	// Serial from the same stream, consumed AFTER the key. This order is a
	// fixed contract — changing it breaks reproduction of existing CAs.
	serialBytes := make([]byte, 16)
	if _, err := io.ReadFull(drbg, serialBytes); err != nil {
		return nil, err
	}
	serial := new(big.Int).SetBytes(serialBytes)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   caCN,
			Organization: []string{"ResultV"},
		},
		NotBefore:             deterministicNotBefore,
		NotAfter:              deterministicNotAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	// RSA PKCS#1 v1.5 signing ignores the rand argument, so passing the DRBG
	// keeps the call deterministic without consuming more stream bytes.
	der, err := x509.CreateCertificate(drbg, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return writeRoot(certPath, keyPath, der, key)
}

// generateDeterministicKey builds a bits-sized RSA key using only bytes read
// from src. It cannot call rsa.GenerateKey directly: as of Go 1.26,
// crypto/rsa silently substitutes the OS's secure RNG for any non-default
// io.Reader (crypto/internal/rand.CustomReader) unless the process sets
// GODEBUG=cryptocustomrand=1 — and even under that documented-temporary
// escape hatch, crypto/internal/randutil.MaybeReadByte still probabilistically
// consumes one extra byte from the reader first, a coin flip seeded from an
// unseedable global source rather than our seed. Verified empirically: with
// the GODEBUG flag set, two rsa.GenerateKey(sameDRBG(seed), bits) calls
// produced matching keys only ~50% of the time, matching that coin flip.
// Either path defeats reproducibility, so prime search here runs directly
// against math/big, which has no such interception, following the same
// candidate-construction algorithm crypto/rand.Prime used before Go 1.26.
func generateDeterministicKey(src io.Reader, bits int) (*rsa.PrivateKey, error) {
	primeBits := bits / 2
	one := big.NewInt(1)
	e := big.NewInt(65537)
	for {
		p, err := randPrime(src, primeBits)
		if err != nil {
			return nil, err
		}
		q, err := randPrime(src, primeBits)
		if err != nil {
			return nil, err
		}
		if p.Cmp(q) == 0 {
			continue
		}
		n := new(big.Int).Mul(p, q)
		if n.BitLen() != bits {
			continue
		}
		phi := new(big.Int).Mul(new(big.Int).Sub(p, one), new(big.Int).Sub(q, one))
		if new(big.Int).GCD(nil, nil, e, phi).Cmp(one) != 0 {
			continue
		}
		d := new(big.Int).ModInverse(e, phi)
		if d == nil {
			continue
		}
		key := &rsa.PrivateKey{
			PublicKey: rsa.PublicKey{N: n, E: int(e.Int64())},
			D:         d,
			Primes:    []*big.Int{p, q},
		}
		key.Precompute()
		if err := key.Validate(); err != nil {
			continue
		}
		return key, nil
	}
}

// randPrime draws a bits-sized probable prime from src. Adapted from the
// candidate-construction algorithm crypto/rand.Prime used before Go 1.26:
// set the top two bits so a product of two such primes never comes up a bit
// short, and the bottom bit so the candidate is odd.
func randPrime(src io.Reader, bits int) (*big.Int, error) {
	b := uint(bits % 8)
	if b == 0 {
		b = 8
	}
	buf := make([]byte, (bits+7)/8)
	p := new(big.Int)
	for {
		if _, err := io.ReadFull(src, buf); err != nil {
			return nil, err
		}
		buf[0] &= uint8(int(1<<b) - 1)
		if b >= 2 {
			buf[0] |= 3 << (b - 2)
		} else {
			buf[0] |= 1
			if len(buf) > 1 {
				buf[1] |= 0x80
			}
		}
		buf[len(buf)-1] |= 1
		p.SetBytes(buf)
		if p.ProbablyPrime(20) {
			return p, nil
		}
	}
}

func loadRoot(certPath, keyPath string) (*Root, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("invalid key PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}
	return &Root{
		CertificatePath: certPath,
		PrivateKeyPath:  keyPath,
		Certificate:     cert,
		PrivateKey:      key,
	}, nil
}
