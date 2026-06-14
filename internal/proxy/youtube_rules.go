// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// YouTube geo-split.
//
// Premise: Google suspended ad monetization in Russia in 2022, so it serves
// NO ads to Russian IPs — while RKN throttles the heavy video CDN
// (*.googlevideo.com). So we split YouTube traffic by destination:
//
//   - video bytes (*.googlevideo.com) → proxy: the foreign exit dodges RKN
//     throttling, so playback is fast.
//   - player / API / ad-decision (youtubei.googleapis.com, *.youtube.com) →
//     direct: the request leaves from the user's real Russian IP, so Google's
//     InnerTube player response carries no `adPlacements` and the client shows
//     no ads.
//
// This is pure routing — no MITM, no TLS interception — so it cannot break a
// connection the way an interception layer would. It is only meaningful when
// the device's `direct` egress is itself a Russian IP (the user is physically
// in Russia); elsewhere the player call leaves from a non-RU IP and ads return.
// You cannot spoof the source IP: Google decides geo from the IP that actually
// terminates the TCP connection, which is why the request must genuinely
// originate from Russia (here: the user's own ISP, via `direct`).

// youTubeVideoSuffixes carry the actual media; route through the proxy so RKN
// throttling of the video CDN is bypassed.
var youTubeVideoSuffixes = []string{
	".googlevideo.com",
}

// youTubeDirectDomains are exact apex hosts that carry the player / ad
// decision; route direct so Google sees the user's Russian IP.
var youTubeDirectDomains = []string{
	"youtube.com",
	"youtubei.googleapis.com",
	"youtube-nocookie.com",
}

// youTubeDirectSuffixes match the subdomains of the above (www., m., s., …).
// Leading dot keeps the match to whole-label suffixes only (".youtube.com"
// matches "www.youtube.com" but not "notyoutube.com").
var youTubeDirectSuffixes = []string{
	".youtube.com",
	".youtube-nocookie.com",
}
