// Command smartsrs compiles the Smart blocklist for a country into a binary
// sing-box rule-set, for bundling as an APK asset.
//
// The bundled seed is what makes a fresh install's FIRST Smart connect correct
// and fast: without it the app would fall back to Global routing (every app in
// the tunnel, including banks) for the seconds it takes to download the list.
//
// Regenerate before a release:
//
//	go run ./cmd/smartsrs -country ru -out android/app/src/main/assets/smart-ru.srs
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"resultproxy-wails/internal/proxy"
)

func main() {
	country := flag.String("country", "ru", "ISO alpha-2 country code")
	out := flag.String("out", "android/app/src/main/assets/smart-ru.srs", "output SRS path")
	flag.Parse()

	provider := proxy.NewHTTPBlockedListProvider()
	domains, err := provider.FetchBlockedDomains(context.Background(), *country)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetching %s blocklist: %v\n", *country, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "creating output dir: %v\n", err)
		os.Exit(1)
	}
	if err := proxy.CompileSmartSRS(domains, *out); err != nil {
		fmt.Fprintf(os.Stderr, "compiling SRS: %v\n", err)
		os.Exit(1)
	}
	st, err := os.Stat(*out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stat: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s: %d domains, %.1f KB\n", *out, len(domains), float64(st.Size())/1024)
}
