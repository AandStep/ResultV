// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// Known YouTube / Google-ads domains, used by the AdBlock DNS+route reject
// (see adblock_rules.go, engine.go buildDNS/buildRoute). Pure domain data —
// no MITM, no routing tricks.

// youTubeCoreDomains are exact hosts in the YouTube player stack. Used for DNS
// bypass when global ad-block SRS lists flag googlevideo.com.
var youTubeCoreDomains = []string{
	"youtube.com",
	"youtubei.googleapis.com",
	"youtube-nocookie.com",
	"youtube.googleapis.com",
	"youtu.be",
	"ytimg.com",
	"ggpht.com",
	"googlevideo.com",
	"accounts.google.com",
	"gstatic.com",
}

// youTubeCoreSuffixes match YouTube / CDN subdomains for DNS bypass.
var youTubeCoreSuffixes = []string{
	".youtube.com",
	".youtube-nocookie.com",
	".ytimg.com",
	".ggpht.com",
	".googlevideo.com",
	".gstatic.com",
}

// youTubeAdDeliverySuffixes are Google ad-serving hosts on the YouTube page.
var youTubeAdDeliverySuffixes = []string{
	".doubleclick.net",
	".googleadservices.com",
	".googlesyndication.com",
	".googletagservices.com",
	".google-analytics.com",
	".googletagmanager.com",
	".adservice.google.com",
}

// youTubeDNSBypassSuffixes lists core YouTube hosts that must still resolve
// when global ad-block DNS reject is on (SRS lists often flag googlevideo.com).
func youTubeDNSBypassSuffixes() []string {
	out := append([]string{}, youTubeCoreSuffixes...)
	return out
}
