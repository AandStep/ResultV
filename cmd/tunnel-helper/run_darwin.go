// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build darwin

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	sblog "github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"

	"resultproxy-wails/internal/tunnelipc"
)

// run is the helper entrypoint, called from main.go. It blocks until the
// helper has nothing left to do, then returns (the caller's main exits).
//
// Exit reasons (any one of them stops the helper):
//   - "shutdown" command from main.
//   - Main process disappeared (PID probe).
//   - Connection from main closed cleanly (EOF).
//   - SIGTERM/SIGINT.
//
// On exit, sing-box is stopped if running and the socket file is removed.
func run() {
	var (
		socketPath = flag.String("socket", "", "unix socket path to listen on (required)")
		ownerUID   = flag.Int("owner-uid", -1, "chown the socket to this UID so the unprivileged main process can connect (required)")
		mainPID    = flag.Int("main-pid", 0, "main process PID; helper exits if this process disappears (required)")
	)
	flag.Parse()

	if *socketPath == "" || *ownerUID < 0 || *mainPID <= 0 {
		fmt.Fprintln(os.Stderr, "usage: resultv-tunnel-helper --socket PATH --owner-uid UID --main-pid PID")
		os.Exit(2)
	}

	// Log to stderr; main captures it via the cmd.CombinedOutput we use for the
	// initial osascript launch. Once the helper is running, the user only sees
	// these logs if they tail Console.app.
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetPrefix("[resultv-tunnel-helper] ")
	log.Printf("starting (pid=%d, uid=%d, owner-uid=%d, main-pid=%d)",
		os.Getpid(), os.Geteuid(), *ownerUID, *mainPID)

	if os.Geteuid() != 0 {
		log.Fatalf("must run as root (effective uid=%d)", os.Geteuid())
	}

	if err := os.MkdirAll(filepath.Dir(*socketPath), 0o755); err != nil {
		log.Fatalf("mkdir socket dir: %v", err)
	}
	// Remove any leftover socket from a previous run before binding.
	_ = os.Remove(*socketPath)

	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("listen %s: %v", *socketPath, err)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	// chown so the unprivileged main process (running under the real user)
	// can connect to our socket. Without this, connect() from the user-side
	// returns EACCES because the socket is owned by root.
	if err := os.Chown(*socketPath, *ownerUID, -1); err != nil {
		log.Fatalf("chown socket: %v", err)
	}
	if err := os.Chmod(*socketPath, 0o660); err != nil {
		log.Fatalf("chmod socket: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := &server{}
	defer srv.shutdownEngine() // last-chance TUN cleanup on any exit path.

	// Wire up signal handling and the main-PID watchdog. Either of them
	// triggers context cancellation, which closes the listener and unblocks
	// any pending Accept.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
		select {
		case sig := <-sigCh:
			log.Printf("received signal %v, exiting", sig)
			cancel()
		case <-ctx.Done():
		}
	}()

	go watchMainPID(ctx, *mainPID, cancel)

	// Close the listener when ctx is cancelled so accept() returns and the
	// outer loop exits.
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	// Single-connection server: we expect exactly one client (the main app).
	// If it disconnects, we loop and accept the next one — useful when main
	// reconnects after a brief network blip.
	for ctx.Err() == nil {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Printf("accept: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		log.Printf("client connected: %s", conn.RemoteAddr())
		srv.handleConnection(ctx, conn, cancel)
		log.Printf("client disconnected")
	}

	log.Printf("exiting")
}

// server holds the singleton sing-box instance and serialises start/stop calls
// so we never end up with two sing-box instances racing for the same TUN.
type server struct {
	mu      sync.Mutex
	engine  *box.Box
	cancel  context.CancelFunc
	running bool
}

// handleConnection drains commands from one main-app connection. Returns when
// the connection closes or the helper context is cancelled.
func (s *server) handleConnection(ctx context.Context, conn net.Conn, cancelHelper context.CancelFunc) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	// Allow large config payloads — sing-box configs with whitelists can be
	// well over the default 64 KiB scanner buffer.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	enc := json.NewEncoder(conn)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		var req tunnelipc.Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = enc.Encode(tunnelipc.Response{OK: false, Error: fmt.Sprintf("bad request: %v", err)})
			continue
		}
		resp := s.handleRequest(ctx, &req)
		if err := enc.Encode(resp); err != nil {
			log.Printf("write response: %v", err)
			return
		}
		if req.Cmd == tunnelipc.CmdShutdown {
			cancelHelper()
			return
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("read: %v", err)
	}
}

func (s *server) handleRequest(ctx context.Context, req *tunnelipc.Request) tunnelipc.Response {
	switch req.Cmd {
	case tunnelipc.CmdStart:
		if err := s.startEngine(ctx, req.Config); err != nil {
			return tunnelipc.Response{OK: false, Error: err.Error()}
		}
		return tunnelipc.Response{OK: true, Running: true}
	case tunnelipc.CmdStop:
		s.shutdownEngine()
		return tunnelipc.Response{OK: true, Running: false}
	case tunnelipc.CmdStatus:
		s.mu.Lock()
		running := s.running
		s.mu.Unlock()
		return tunnelipc.Response{OK: true, Running: running}
	case tunnelipc.CmdShutdown:
		s.shutdownEngine()
		return tunnelipc.Response{OK: true}
	default:
		return tunnelipc.Response{OK: false, Error: fmt.Sprintf("unknown cmd %q", req.Cmd)}
	}
}

// startEngine boots sing-box from the supplied raw JSON. If an engine is
// already running, it's shut down first — the new config wins.
//
// Mirrors the engine bootstrap used by the main app's SingBoxEngine
// (internal/proxy/singbox.go) so the helper and in-process engine behave
// identically. The only difference is where the engine runs: this process is
// privileged and creates the TUN device; the in-process engine is used for
// proxy mode where no TUN is needed.
func (s *server) startEngine(ctx context.Context, configJSON json.RawMessage) error {
	if len(configJSON) == 0 {
		return fmt.Errorf("start requires a non-empty config")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		log.Printf("startEngine: replacing existing instance")
		s.shutdownEngineLocked()
	}

	engineCtx, cancel := context.WithCancel(ctx)
	engineCtx = include.Context(engineCtx)

	var opts option.Options
	if err := singjson.UnmarshalContext(engineCtx, configJSON, &opts); err != nil {
		cancel()
		return fmt.Errorf("parse sing-box config: %w", err)
	}

	instance, err := box.New(box.Options{
		Context:           engineCtx,
		Options:           opts,
		PlatformLogWriter: stderrLogWriter{},
	})
	if err != nil {
		cancel()
		return fmt.Errorf("sing-box new: %w", err)
	}
	if err := instance.Start(); err != nil {
		_ = instance.Close()
		cancel()
		return fmt.Errorf("sing-box start: %w", err)
	}

	s.engine = instance
	s.cancel = cancel
	s.running = true
	log.Printf("sing-box started")
	return nil
}

// shutdownEngine stops sing-box if it's running. Safe to call multiple times
// and from any goroutine (including signal handlers via deferred run() exit).
func (s *server) shutdownEngine() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shutdownEngineLocked()
}

func (s *server) shutdownEngineLocked() {
	if !s.running {
		return
	}
	log.Printf("stopping sing-box")
	if s.engine != nil {
		_ = s.engine.Close()
		s.engine = nil
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.running = false
	log.Printf("sing-box stopped")
}

// watchMainPID polls kill(pid, 0) every 500ms. As soon as the main process is
// gone (kill returns ESRCH), we cancel the helper context so the outer run
// loop tears everything down — no orphan helpers, no leftover TUN.
func watchMainPID(ctx context.Context, pid int, cancel context.CancelFunc) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// kill(pid, 0) with signal 0 only probes existence/permissions.
			// As root we can probe any pid, so ESRCH definitively means
			// "process exited".
			if err := syscall.Kill(pid, 0); err != nil {
				log.Printf("main process pid=%d disappeared (%v), exiting", pid, err)
				cancel()
				return
			}
		}
	}
}

// stderrLogWriter is the sing-box PlatformLogWriter for this helper. We
// forward Info-and-worse messages to stderr (Console.app) so engine errors
// (TUN creation failures, route conflicts, etc.) end up in one place with
// the rest of our log output. Debug/Trace are dropped — too noisy.
type stderrLogWriter struct{}

func (stderrLogWriter) WriteMessage(level sblog.Level, msg string) {
	if level > sblog.LevelInfo {
		return
	}
	log.Printf("sing-box[%d] %s", int(level), msg)
}
