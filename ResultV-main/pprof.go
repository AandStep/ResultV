// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"
	"time"
)

// startPprofIfEnabled starts a local pprof HTTP server when the user opts in
// via the RESULTPROXY_PPROF environment variable. Set RESULTPROXY_PPROF=1 to
// listen on the default 127.0.0.1:6060, or set it to "host:port" for a custom
// bind address.
//
// Pprof is only useful for diagnosing long-session performance problems
// (CPU, goroutine, heap profiles). It is NEVER exposed unless explicitly
// requested, and it only binds to loopback.
func startPprofIfEnabled() {
	v := strings.TrimSpace(os.Getenv("RESULTPROXY_PPROF"))
	if v == "" || v == "0" || strings.EqualFold(v, "false") {
		return
	}

	addr := "127.0.0.1:6060"
	if v != "1" && !strings.EqualFold(v, "true") {
		addr = v
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") && !strings.HasPrefix(addr, "localhost:") {
		// Refuse non-loopback binds: exposing pprof on any other interface
		// would leak internal state of an authenticated session.
		fmt.Fprintf(os.Stderr, "pprof: refusing non-loopback bind %q\n", addr)
		return
	}

	srv := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		fmt.Fprintf(os.Stderr, "pprof: listening on http://%s/debug/pprof/\n", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "pprof: %v\n", err)
		}
	}()
}
