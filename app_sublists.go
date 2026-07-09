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

package main

import "resultproxy-wails/internal/config"

// SubscriptionPreview is what the import UI gets from a subscription fetch:
// the nodes plus any provider-declared routing lists (Name/URL/Action only —
// IDs and enabled-state are assigned at AddSubscription time).
type SubscriptionPreview struct {
	Proxies      []config.ProxyEntry  `json:"proxies"`
	RoutingLists []config.RoutingList `json:"routingLists"`
}
