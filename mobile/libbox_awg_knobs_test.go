// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mobile

import "testing"

// The binding Kotlin calls on connect. Most user-pasted AWG profiles arrive as
// an awg:// URI rather than a parsed entry, so the URI path has to work too.
func TestUnsupportedAWGKnobs(t *testing.T) {
	cases := []struct {
		name  string
		entry string
		want  string
	}{
		{
			"awg with j-knobs and itime",
			`{"type":"AMNEZIAWG","ip":"h","port":1,"extra":{"amnezia":{"jc":8,"j1":"a","j3":"c","itime":30}}}`,
			"j1,j3,itime",
		},
		{
			"awg with only supported knobs",
			`{"type":"AMNEZIAWG","ip":"h","port":1,"extra":{"amnezia":{"jc":8,"h1":"1-5","i1":"<b 0x1>"}}}`,
			"",
		},
		{
			"plain wireguard is never reported",
			`{"type":"WIREGUARD","ip":"h","port":1,"extra":{"amnezia":{"j1":"a"}}}`,
			"",
		},
		{"garbage in, nothing out", `not json`, ""},
		{"empty", ``, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UnsupportedAWGKnobs(tc.entry); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// End-to-end from the share URI Kotlin actually holds: an awg:// link with
// j-knobs must both report them and produce a config free of them.
func TestUnsupportedAWGKnobsFromParsedURI(t *testing.T) {
	const uri = "awg://kEbboLQ%2FVHN9S7STGzfGPtB70jpbDL5916LzlVaiYHY%3D@example.com:10700" +
		"?public_key=ThmNvESNsK6fT4KwFChveubO9itL2ZdIRdg4IFoXpEo%3D" +
		"&jc=8&h1=1-5&j1=%3Cb+0xf1%3E&itime=30#AWG"

	entry, err := ParseProxyURI(uri)
	if err != nil {
		t.Fatalf("ParseProxyURI: %v", err)
	}
	if got := UnsupportedAWGKnobs(entry); got != "j1,itime" {
		t.Errorf("knobs = %q, want %q", got, "j1,itime")
	}
}
