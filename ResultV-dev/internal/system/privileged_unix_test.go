//go:build darwin || linux

package system

import "testing"

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"":              "''",
		"abc":           "abc",
		"/usr/bin/x":    "/usr/bin/x",
		"a b":           "'a b'",
		"o'reilly":      `'o'\''reilly'`,
		`back\slash`:    `'back\slash'`,
		"semi;colon":    "'semi;colon'",
		"pipe|cmd":      "'pipe|cmd'",
		"ampersand&bg":  "'ampersand&bg'",
		"dollar$var":    "'dollar$var'",
		"backtick`cmd`": "'backtick`cmd`'",
		"glob*":         "'glob*'",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
