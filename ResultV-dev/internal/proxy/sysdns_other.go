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

//go:build !windows

package proxy

type noopSystemDNS struct{}

func (noopSystemDNS) Override(servers []string) error              { return nil }
func (noopSystemDNS) OverrideTunnelAdapter(adapterIP, dnsIP string) error { return nil }
func (noopSystemDNS) Restore() error                    { return nil }
func (noopSystemDNS) SnapshotExists() bool              { return false }
func (noopSystemDNS) RestoreCommands() ([]string, error) { return nil, nil }
func (noopSystemDNS) DeleteSnapshot() error             { return nil }

func newSystemDNS() SystemDNS { return noopSystemDNS{} }
