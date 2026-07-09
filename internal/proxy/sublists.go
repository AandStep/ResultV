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
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"

	"resultproxy-wails/internal/config"
)

// MaxSubscriptionRoutingLists caps how many routing lists a subscription may
// declare — garbage protection; extras are dropped.
const MaxSubscriptionRoutingLists = 10

// subRoutingListDecl is one provider-declared routing list in the payload.
type subRoutingListDecl struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Action string `json:"action"`
}

// ExtractSubscriptionRoutingLists parses provider-delivered routing lists from
// a subscription response. Two channels, same payload shape (JSON array of
// {name,url,action}): the Routing-Lists header carries base64(JSON) and works
// with any body format; a JSON body may carry a top-level "routingLists" key.
// The header wins only when it yields at least one valid entry after validation.
// Returned entries have only Name/URL/Action set; invalid entries are silently dropped.
func ExtractSubscriptionRoutingLists(headerVal, body string) []config.RoutingList {
	if fromHeader := validateSubRoutingLists(declsFromHeader(headerVal)); len(fromHeader) > 0 {
		return fromHeader
	}
	return validateSubRoutingLists(declsFromJSONBody(body))
}

func declsFromHeader(headerVal string) []subRoutingListDecl {
	v := strings.TrimSpace(headerVal)
	if v == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(v)
		if err != nil {
			return nil
		}
	}
	var decls []subRoutingListDecl
	if err := json.Unmarshal(raw, &decls); err != nil {
		return nil
	}
	return decls
}

func declsFromJSONBody(body string) []subRoutingListDecl {
	t := strings.TrimSpace(body)
	if !strings.HasPrefix(t, "{") {
		return nil
	}
	var f struct {
		RoutingLists []subRoutingListDecl `json:"routingLists"`
	}
	if err := json.Unmarshal([]byte(t), &f); err != nil {
		return nil
	}
	return f.RoutingLists
}

func validateSubRoutingLists(decls []subRoutingListDecl) []config.RoutingList {
	out := make([]config.RoutingList, 0, len(decls))
	for _, d := range decls {
		if len(out) >= MaxSubscriptionRoutingLists {
			break
		}
		u := NormalizeRoutingListURL(strings.TrimSpace(d.URL))
		lower := strings.ToLower(u)
		if u == "" || (!strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://")) {
			continue
		}
		switch d.Action {
		case "proxy", "direct", "block":
		default:
			continue
		}
		name := strings.TrimSpace(d.Name)
		if name == "" {
			if parsed, err := url.Parse(u); err == nil {
				name = parsed.Hostname()
			}
		}
		if r := []rune(name); len(r) > 64 {
			name = string(r[:64])
		}
		out = append(out, config.RoutingList{Name: name, URL: u, Action: d.Action})
	}
	return out
}
