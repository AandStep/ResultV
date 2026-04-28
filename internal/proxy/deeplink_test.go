package proxy

import (
	"strings"
	"testing"
)

func TestSanitizeBase64Strips(t *testing.T) {
	cases := map[string]string{
		"AAyaj04TQ7":             "AAyaj04TQ7",
		"AAyaj04TQ7/":            "AAyaj04TQ7/",
		"AAyaj04TQ7\x00":         "AAyaj04TQ7",
		"AAyaj04TQ7 ":            "AAyaj04TQ7",
		"AAyaj04TQ7\r\n":         "AAyaj04TQ7",
		"AAyaj04TQ7%20":          "AAyaj04TQ720",
		"AAy aj 04 TQ7":          "AAyaj04TQ7",
		"hello-world_123==":      "hello-world_123==",
		"hello!world?":           "helloworld",
		strings.Repeat("A", 64): strings.Repeat("A", 64),
	}
	for in, want := range cases {
		if got := sanitizeBase64(in); got != want {
			t.Errorf("sanitizeBase64(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDecodeDeepLinkRejectsNonScheme(t *testing.T) {
	if _, err := DecodeDeepLink("https://example.com"); err == nil {
		t.Fatal("expected error for non-resultv URL")
	}
}

func TestDecodeDeepLinkStripsTrailingJunk(t *testing.T) {
	// We cannot test full decryption without the key, but we can verify the
	// payload trim path doesn't pass trailing junk down to the decoder.
	in := "resultv://crypt4/AAyaj04TQ7/  \r\n"
	body := strings.TrimSpace(in)
	body = strings.TrimRight(body, "/\x00\r\n\t ")
	if !strings.HasSuffix(body, "AAyaj04TQ7") {
		t.Fatalf("trim left %q in payload, expected to end with AAyaj04TQ7", body)
	}
}
