// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

import "github.com/AdguardTeam/urlfilter/rules"

// ListSource describes a remote filter subscription with fallback mirrors.
type ListSource struct {
	ID   rules.ListID
	Name string
	URLs []string
}

// DefaultSources is the built-in filter set for urlfilter (network + cosmetic).
// GitHub raw is listed first, then a jsDelivr CDN mirror of the same repo
// content (reachable from RU networks when raw.githubusercontent.com is
// blocked/down — a frequent cause of ads leaking through), then the official
// host, which often times out behind Cloudflare from RU networks.
var DefaultSources = []ListSource{
	{
		ID: 1, Name: "adguard-base",
		URLs: []string{
			"https://raw.githubusercontent.com/AdguardTeam/FiltersRegistry/master/filters/filter_2_Base/filter.txt",
			"https://cdn.jsdelivr.net/gh/AdguardTeam/FiltersRegistry@master/filters/filter_2_Base/filter.txt",
			"https://filters.adtidy.org/windows/filters/2.txt",
		},
	},
	{
		ID: 2, Name: "adguard-tracking",
		URLs: []string{
			"https://raw.githubusercontent.com/AdguardTeam/FiltersRegistry/master/filters/filter_17_TrackParam/filter.txt",
			"https://cdn.jsdelivr.net/gh/AdguardTeam/FiltersRegistry@master/filters/filter_17_TrackParam/filter.txt",
			"https://filters.adtidy.org/windows/filters/3.txt",
		},
	},
	{
		ID: 3, Name: "adguard-russian",
		URLs: []string{
			"https://raw.githubusercontent.com/AdguardTeam/FiltersRegistry/master/filters/filter_1_Russian/filter.txt",
			"https://cdn.jsdelivr.net/gh/AdguardTeam/FiltersRegistry@master/filters/filter_1_Russian/filter.txt",
			"https://filters.adtidy.org/windows/filters/1.txt",
		},
	},
	{
		ID: 4, Name: "easylist",
		URLs: []string{
			"https://raw.githubusercontent.com/easylist/easylist/master/easylist/easylist.txt",
			"https://cdn.jsdelivr.net/gh/easylist/easylist@master/easylist/easylist.txt",
			"https://easylist.to/easylist/easylist.txt",
		},
	},
	{
		ID: 5, Name: "easyprivacy",
		URLs: []string{
			"https://raw.githubusercontent.com/easylist/easylist/master/easyprivacy/easyprivacy.txt",
			"https://cdn.jsdelivr.net/gh/easylist/easylist@master/easyprivacy/easyprivacy.txt",
			"https://easylist.to/easylist/easyprivacy.txt",
		},
	},
	{
		ID: 6, Name: "fanboy-annoyance",
		URLs: []string{
			"https://raw.githubusercontent.com/easylist/easylist/master/fanboy-annoyance/fanboy-annoyance.txt",
			"https://cdn.jsdelivr.net/gh/easylist/easylist@master/fanboy-annoyance/fanboy-annoyance.txt",
			"https://easylist.to/easylist/fanboy-annoyance.txt",
		},
	},
	{
		ID: 7, Name: "adguard-annoyances",
		URLs: []string{
			"https://raw.githubusercontent.com/AdguardTeam/FiltersRegistry/master/filters/filter_14_Annoyances/filter.txt",
			"https://cdn.jsdelivr.net/gh/AdguardTeam/FiltersRegistry@master/filters/filter_14_Annoyances/filter.txt",
			"https://filters.adtidy.org/windows/filters/14.txt",
		},
	},
	{
		ID: 8, Name: "adguard-mobile-ads",
		URLs: []string{
			"https://raw.githubusercontent.com/AdguardTeam/FiltersRegistry/master/filters/filter_11_Mobile/filter.txt",
			"https://cdn.jsdelivr.net/gh/AdguardTeam/FiltersRegistry@master/filters/filter_11_Mobile/filter.txt",
			"https://filters.adtidy.org/windows/filters/11.txt",
		},
	},
}
