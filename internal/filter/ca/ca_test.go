package ca

import (
	"bytes"
	"os"
	"testing"
)

func TestEnsureRoot_DeterministicSameSeed(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	seed := "device-android-id-abc123"

	r1, err := EnsureRoot(dir1, seed)
	if err != nil {
		t.Fatalf("EnsureRoot dir1: %v", err)
	}
	r2, err := EnsureRoot(dir2, seed)
	if err != nil {
		t.Fatalf("EnsureRoot dir2: %v", err)
	}

	cert1, _ := os.ReadFile(r1.CertificatePath)
	cert2, _ := os.ReadFile(r2.CertificatePath)
	if !bytes.Equal(cert1, cert2) {
		t.Fatal("same seed produced different certificate bytes")
	}
	key1, _ := os.ReadFile(r1.PrivateKeyPath)
	key2, _ := os.ReadFile(r2.PrivateKeyPath)
	if !bytes.Equal(key1, key2) {
		t.Fatal("same seed produced different key bytes")
	}
}

func TestEnsureRoot_DifferentSeedDiffers(t *testing.T) {
	a, err := EnsureRoot(t.TempDir(), "seed-A")
	if err != nil {
		t.Fatalf("EnsureRoot A: %v", err)
	}
	b, err := EnsureRoot(t.TempDir(), "seed-B")
	if err != nil {
		t.Fatalf("EnsureRoot B: %v", err)
	}
	ca, _ := os.ReadFile(a.CertificatePath)
	cb, _ := os.ReadFile(b.CertificatePath)
	if bytes.Equal(ca, cb) {
		t.Fatal("different seeds produced identical certificates")
	}
}

func TestEnsureRoot_EmptySeedStillValid(t *testing.T) {
	r, err := EnsureRoot(t.TempDir(), "")
	if err != nil {
		t.Fatalf("EnsureRoot empty seed: %v", err)
	}
	if r.Certificate == nil || r.PrivateKey == nil {
		t.Fatal("empty seed must still yield a usable CA")
	}
	if r.Certificate.Subject.CommonName != caCN {
		t.Fatalf("unexpected CN %q", r.Certificate.Subject.CommonName)
	}
}

func TestEnsureRoot_ExistingFilesReloadedIgnoringSeed(t *testing.T) {
	dir := t.TempDir()
	first, err := EnsureRoot(dir, "seed-one")
	if err != nil {
		t.Fatalf("first EnsureRoot: %v", err)
	}
	firstCert, _ := os.ReadFile(first.CertificatePath)
	// Different seed, but files already exist -> must reload, not regenerate.
	second, err := EnsureRoot(dir, "seed-two")
	if err != nil {
		t.Fatalf("second EnsureRoot: %v", err)
	}
	secondCert, _ := os.ReadFile(second.CertificatePath)
	if !bytes.Equal(firstCert, secondCert) {
		t.Fatal("existing CA was regenerated instead of reloaded")
	}
}
