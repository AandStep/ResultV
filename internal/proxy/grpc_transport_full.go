// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build with_grpc

package proxy

// grpcFullTransport reports that this binary was built with sing-box's full
// gRPC transport (google.golang.org/grpc) rather than the default lite
// implementation. The build tag must match the one passed to sing-box, because
// the two implementations disagree on how a service_name becomes an HTTP/2
// path: the full one forwards the string verbatim, the lite one routes it
// through url.URL.Path and re-escapes any percent sign. See
// xrayGRPCServiceName.
const grpcFullTransport = true
