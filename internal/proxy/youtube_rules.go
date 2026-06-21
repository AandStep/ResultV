// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// YouTube geo-split for Russia.
//
// Google suspended ad monetization for Russian IPs in 2022, so InnerTube
// player responses carry no adPlacements when the API call leaves from a
// Russian IP — while RKN throttles the heavy video CDN (*.googlevideo.com).
// We split YouTube traffic by destination:
//
//   - video bytes (*.googlevideo.com) → proxy: foreign exit dodges RKN throttling.
//   - player / API / ad-decision (youtubei.googleapis.com, *.youtube.com) →
//     direct: request leaves from the user's real Russian IP, so the client
//     shows no ads.
//
// Ad-delivery host suffixes are rejected as a secondary layer (banners/trackers).
// Pure routing — no MITM. Meaningful when the device's direct egress is a
// Russian IP; elsewhere the player call leaves from a non-RU IP and ads return.

// youTubeVideoSuffixes carry the actual media; route through the proxy so RKN
// throttling of the video CDN is bypassed.
var youTubeVideoSuffixes = []string{
	".googlevideo.com",
}

// youTubeDirectDomains are exact apex hosts that carry the player / ad decision;
// route direct so Google sees the user's Russian IP.
var youTubeDirectDomains = []string{
	"youtube.com",
	"youtubei.googleapis.com",
	"youtube-nocookie.com",
}

// youTubeDirectSuffixes match subdomains of the above (www., m., s., …).
var youTubeDirectSuffixes = []string{
	".youtube.com",
	".youtube-nocookie.com",
}

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

// appendYouTubeUnblockRouteRules applies geo-split routing and rejects
// ad-delivery domains. Must run after the sniff rule. Order matters:
// googlevideo.com must match before .youtube.com.
func appendYouTubeUnblockRouteRules(rules []SBRouteRule) []SBRouteRule {
	rules = append(rules, SBRouteRule{
		DomainSuffix: youTubeVideoSuffixes,
		Outbound:     "proxy",
		Action:       "route",
	})
	rules = append(rules, SBRouteRule{
		Domain:       youTubeDirectDomains,
		DomainSuffix: youTubeDirectSuffixes,
		Outbound:     "direct",
		Action:       "route",
	})
	rules = append(rules, SBRouteRule{
		DomainSuffix: youTubeAdDeliverySuffixes,
		Action:       "reject",
	})
	return rules
}

// appendYouTubeUnblockDNSRules rejects ad-delivery lookups when YouTube
// unblock is on without global ad-block (otherwise buildDNS handles it).
func appendYouTubeUnblockDNSRules(rules []SBDNSRule) []SBDNSRule {
	return append(rules, SBDNSRule{
		DomainSuffix: youTubeAdDeliverySuffixes,
		Action:       "reject",
	})
}
