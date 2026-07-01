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

// Root holds paths and key material for the filtering root CA.
type Root struct {
	CertificatePath string
	PrivateKeyPath  string
	Certificate     *x509.Certificate
	PrivateKey      *rsa.PrivateKey
}

// EnsureRoot creates or loads the root CA under filterDir.
func EnsureRoot(filterDir string) (*Root, error) {
	if err := os.MkdirAll(filterDir, 0o700); err != nil {
		return nil, err
	}
	certPath := filepath.Join(filterDir, certFile)
	keyPath := filepath.Join(filterDir, keyFile)
	if _, err := os.Stat(certPath); err == nil {
		return loadRoot(certPath, keyPath)
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
