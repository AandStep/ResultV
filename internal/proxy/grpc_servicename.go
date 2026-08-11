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

package proxy

import (
	"net/url"
	"strings"
)

// xrayGRPCServiceName rewrites an Xray `serviceName` into the explicit gRPC
// path that sing-box forwards verbatim, so the HTTP/2 `:path` we send matches
// what the Xray server registered.
//
// Xray reads serviceName by its LEADING slash
// (transport/internet/grpc/config.go):
//
//   - no leading slash — "old school" form. The WHOLE name is percent-escaped
//     into the gRPC service name and the stream is Xray's default "Tun". So
//     "www-debian-org/4c4394da" is served at /www-debian-org%2F4c4394da/Tun —
//     the inner slash is data, not a separator.
//   - leading slash — a custom path, already complete: everything up to the last
//     slash is the service, the last segment is the stream name. Nothing to add.
//
// sing-box decides by "contains a slash" instead, which misreads every
// old-school name that happens to hold an inner slash: it drops both the
// escaping and the /Tun suffix, the server finds no such method, and every
// connection dies instantly with "upload handshake: io: read/write on closed
// pipe". Emitting the path ourselves keeps us independent of that heuristic —
// a leading-slash value is passed through untouched by any version.
//
// Only meaningful with the full gRPC transport (see grpcFullTransport): the
// lite implementation routes the value through url.URL.Path, which re-escapes
// our "%2F" into "%252F".
func xrayGRPCServiceName(serviceName string) string {
	if serviceName == "" {
		// Nothing to translate; let the core apply its own default.
		return ""
	}
	if strings.HasPrefix(serviceName, "/") {
		return serviceName
	}
	return "/" + url.PathEscape(serviceName) + "/Tun"
}
