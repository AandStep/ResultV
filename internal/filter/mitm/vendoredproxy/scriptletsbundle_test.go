package proxy

import (
	"bytes"
	"testing"
)

func TestScriptletsBundle_EmbeddedAndPlausible(t *testing.T) {
	if len(scriptletsBundle) < 50_000 {
		t.Fatalf("scriptlets bundle suspiciously small: %d bytes", len(scriptletsBundle))
	}
	if !bytes.Contains(scriptletsBundle, []byte("invoke")) {
		t.Fatal("scriptlets bundle does not export invoke")
	}
	if !bytes.Contains(scriptletsBundle, []byte("window.scriptlets = scriptlets")) {
		t.Fatal("scriptlets bundle does not assign to window.scriptlets")
	}
	if bytes.Contains(scriptletsBundle, []byte("\nexport ")) {
		t.Fatal("scriptlets bundle still contains ESM export statement")
	}
}
