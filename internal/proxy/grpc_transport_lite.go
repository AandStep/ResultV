// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

//go:build !with_grpc

package proxy

// grpcFullTransport reports that this binary uses sing-box's default lite gRPC
// transport. There the service_name goes through url.URL.Path, so the explicit
// path from xrayGRPCServiceName would come out with "%2F" turned into "%252F" —
// we leave service_name untouched instead. See grpc_transport_full.go.
const grpcFullTransport = false
