// Copyright (C) 2026 ResultProxy
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build !windows

package proxy

import "fmt"

// newSystemProxy creates its platform-specific SystemProxy implementation.
func newSystemProxy(router *Router) SystemProxy {
	return NewStubSystemProxy()
}

// StubSystemProxy is a no-op implementation for non-Windows platforms.
// TODO: implement macOS (networksetup) and Linux (gsettings) variants.
type StubSystemProxy struct{}

func NewStubSystemProxy() *StubSystemProxy { return &StubSystemProxy{} }

func (s *StubSystemProxy) Set(addr string, bypass []string) error {
	return fmt.Errorf("system proxy not implemented on this platform")
}

func (s *StubSystemProxy) Disable() error {
	return nil
}

func (s *StubSystemProxy) DisableSync() {}

func (s *StubSystemProxy) ApplyKillSwitch() error {
	return fmt.Errorf("kill switch not implemented on this platform")
}
