package gomitmproxy

import "testing"

// TestIsTLSException_SuffixMatch guards the fix for YouTube breaking under the
// browser ad-block MITM proxy: exceptions must match subdomains, not only the
// exact host. Google enforces Certificate Transparency + system trust anchors
// on youtube.com / googleapis.com and their subdomains, so MITMing any of them
// makes the browser reject the forged cert and the site dies.
func TestIsTLSException_SuffixMatch(t *testing.T) {
	p := &Proxy{invalidTLSHosts: map[string]bool{
		"youtube.com":    true,
		"googleapis.com": true,
	}}

	cases := []struct {
		host string
		want bool
	}{
		{"youtube.com", true},              // exact
		{"www.youtube.com", true},          // subdomain
		{"youtubei.googleapis.com", true},  // subdomain of a different exception
		{"googleapis.com", true},           // exact
		{"example.com", false},             // unrelated
		{"notyoutube.com", false},          // suffix without dot boundary must NOT match
		{"youtube.com.evil.com", false},    // exception as a left label must NOT match
	}
	for _, c := range cases {
		if got := p.isTLSException(c.host); got != c.want {
			t.Errorf("isTLSException(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}
