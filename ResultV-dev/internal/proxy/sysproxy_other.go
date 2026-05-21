// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

//go:build !windows && !darwin && !linux

package proxy

import "fmt"


func newSystemProxy(router *Router) SystemProxy {
	return NewStubSystemProxy()
}



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
