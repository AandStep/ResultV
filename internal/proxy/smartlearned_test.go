package proxy

import (
	"context"
	"slices"
	"testing"
	"time"

	"resultproxy-wails/internal/verdict"
)

func TestSmartLearnedRoutesListAndForget(t *testing.T) {
	dir := t.TempDir()
	store := verdict.New([]byte("test-salt"), nil)
	store.Learn("previous.example.org", verdict.Proxy)
	if err := store.Save(SmartVerdictStorePath(dir)); err != nil {
		t.Fatal(err)
	}
	relay, err := StartSmartRelay(SmartRelayOptions{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = relay.Close() })
	relay.store.Learn("z.example.net", verdict.Proxy)
	relay.store.LearnUnverified("a.example.net")
	relay.store.Learn("direct.example.net", verdict.Direct)
	relay.store.Seed("manual.example.net", verdict.Proxy, verdict.SourceUser)

	list, err := ManageSmartLearnedRoutes(dir, relay, "", false)
	if err != nil || !slices.Equal(list.Items, []string{"a.example.net", "z.example.net"}) || list.Hidden != 1 {
		t.Fatalf("learned routes = %+v, err = %v", list, err)
	}
	list, err = ManageSmartLearnedRoutes(dir, relay, " Z.Example.Net. ", false)
	if err != nil || !slices.Equal(list.Items, []string{"a.example.net"}) || list.Hidden != 1 {
		t.Fatalf("after forgetting = %+v, err = %v", list, err)
	}
	saved, err := verdict.Load(SmartVerdictStorePath(dir), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := saved.Lookup("z.example.net"); ok {
		t.Fatal("forgotten verdict is still saved")
	}
	if direct := ReadSmartDirectSet(SmartDirectSetPath(dir)); !slices.Equal(direct, []string{"direct.example.net"}) {
		t.Fatalf("unrelated learned-direct rules changed: %v", direct)
	}
}

func TestSmartLearnedRoutesResetWhileDisconnected(t *testing.T) {
	dir := t.TempDir()
	store := verdict.New([]byte("test-salt"), nil)
	store.Learn("direct.example.org", verdict.Direct)
	store.Learn("proxy.example.org", verdict.Proxy)
	store.Seed("manual.example.org", verdict.Proxy, verdict.SourceUser)
	if err := store.Save(SmartVerdictStorePath(dir)); err != nil {
		t.Fatal(err)
	}
	if err := RenderSmartDirectSet(SmartDirectSetPath(dir), []string{"direct.example.org"}); err != nil {
		t.Fatal(err)
	}
	list, err := ManageSmartLearnedRoutes(dir, nil, "", true)
	if err != nil || len(list.Items) != 0 || list.Hidden != 0 {
		t.Fatalf("after reset = %+v, err = %v", list, err)
	}
	saved, err := verdict.Load(SmartVerdictStorePath(dir), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"direct.example.org", "proxy.example.org"} {
		if _, ok := saved.Lookup(host); ok {
			t.Fatalf("learned verdict still exists: %s", host)
		}
	}
	if rec, ok := saved.Lookup("manual.example.org"); !ok || rec.Source != verdict.SourceUser {
		t.Fatalf("manual rule changed: %+v", rec)
	}
	if direct := ReadSmartDirectSet(SmartDirectSetPath(dir)); len(direct) != 0 {
		t.Fatalf("old learned-direct rules still loaded: %v", direct)
	}
}

func TestSmartLearnedRoutesResetDiscardsAnOngoingProbe(t *testing.T) {
	dir := t.TempDir()
	relay, err := StartSmartRelay(SmartRelayOptions{DataDir: dir, MemoryOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = relay.Close() })
	started, result := make(chan struct{}), make(chan struct{})
	relay.probe = func(context.Context, string, *probeGate) verdict.Decision {
		close(started)
		<-result
		return verdict.Proxy
	}
	host := "inflight.example.org"
	relay.learn(host, true, time.Now())
	<-started
	_, err = ManageSmartLearnedRoutes(dir, relay, "", true)
	close(result)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { _, ok := relay.inflight.Load(host); return !ok }, "probe did not finish")
	if _, ok := relay.store.Lookup(host); ok {
		t.Fatal("probe started before reset restored the old verdict")
	}
}
