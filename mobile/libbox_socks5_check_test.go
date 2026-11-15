package mobile

import (
	"net"
	"testing"

	"github.com/pion/stun/v3"
)

func TestSocks5ExitTraceRequiresIP(t *testing.T) {
	ip, country, err := parseSocks5ExitTrace([]byte("fl=56f\nip=2001:db8::1\nloc=de\n"))
	if err != nil || ip != "2001:db8::1" || country != "DE" {
		t.Fatalf("ip=%q country=%q err=%v", ip, country, err)
	}
	if _, _, err := parseSocks5ExitTrace([]byte("ip=unknown\nloc=DE")); err == nil {
		t.Fatal("accepted invalid exit address")
	}
}

func TestSocks5ExitRejectsUnrelatedDatagram(t *testing.T) {
	request := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	address := stun.XORMappedAddress{IP: net.ParseIP("203.0.113.1"), Port: 9231}
	other := stun.MustBuild(stun.TransactionID, stun.BindingSuccess, address)
	if socks5ExitSTUNSuccess(other.Raw, request.TransactionID) {
		t.Fatal("an unrelated UDP packet was treated as proof of exit relay support")
	}
	reply := stun.MustBuild(request, stun.BindingSuccess, address)
	if !socks5ExitSTUNSuccess(reply.Raw, request.TransactionID) {
		t.Fatal("matching STUN binding response was rejected")
	}
}
