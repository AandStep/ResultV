// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// AdBlockCoordinator starts/stops the HTTPS MITM layer used with sing-box adblock routing.
type AdBlockCoordinator interface {
	// StartMITM launches filtering proxy; upstreamPort is sing-box mixed inbound.
	StartMITM(upstreamPort int) (mitmPort int, err error)
	StopMITM()
}
