package ca

import (
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/chacha20"
)

// deterministicReader yields an unbounded, reproducible keystream. Keyed by
// SHA-256(drbgSalt || seed) and run as ChaCha20 over an all-zero plaintext,
// it gives the same bytes every run for a given seed — enough to drive RSA
// key generation, which HKDF's 255*hashlen output cap could not guarantee.
type deterministicReader struct {
	cipher *chacha20.Cipher
}

// drbgSalt namespaces this stream. Bump the version suffix only with a
// deliberate, breaking change to CA reproduction.
const drbgSalt = "resultv-ca-drbg-v1"

func newDRBG(seed string) io.Reader {
	sum := sha256.Sum256(append([]byte(drbgSalt), seed...))
	// ChaCha20 needs a 32-byte key and 12-byte nonce; a zero nonce is fine
	// because each seed produces a distinct key.
	c, err := chacha20.NewUnauthenticatedCipher(sum[:], make([]byte, chacha20.NonceSize))
	if err != nil {
		// NewUnauthenticatedCipher only errors on wrong key/nonce sizes,
		// which are fixed constants here.
		panic(err)
	}
	return &deterministicReader{cipher: c}
}

func (r *deterministicReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	r.cipher.XORKeyStream(p, p)
	return len(p), nil
}
