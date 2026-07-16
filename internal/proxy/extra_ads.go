// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

// Curated supplement of ad-serving hosts confirmed to leak through every
// connected public source (AdGuard Base/Russian/Mobile/Annoyances, EasyList,
// EasyPrivacy, Fanboy, RU AdList, and both reject SRS sets — verified
// 2026-07). Caught live via the engine connection log. Rejected at DNS +
// route level for every app; the browser MITM additionally blocks them via
// filter's embedded extra rules.

// extraAdDeliveryDomains are exact apex hosts to reject.
var extraAdDeliveryDomains = []string{
	"pubserv.pro",       // psb-dsp.pubserv.pro — video pre-roll DSP on RU streaming sites
	"foxstreetcore.com", // cs11.foxstreetcore.com — native banner creatives + click tracker (etarg/adultmasters network)
	"ofjvnvjf.win",      // rotating popup-slider iframe host (caught 2026-07-16, hot.noodlemagazine.com)
	"betamountwo.com",   // ad infra loaded alongside the above; empty 200 at apex, in no public list
	"adultmasters.pro",  // banner-network landing/branding host on the same creatives
	"nmsrv.run",         // player ad-config host, contacted at player init before pre-rolls; not referenced by page HTML
	"kintg.site",        // paired with nmsrv.run at every player init; empty 201 at apex, in no public list
}

// extraAdDeliverySuffixes mirror extraAdDeliveryDomains for subdomain matches.
var extraAdDeliverySuffixes = []string{
	".pubserv.pro",
	".foxstreetcore.com",
	".ofjvnvjf.win",
	".betamountwo.com",
	".adultmasters.pro",
	".nmsrv.run",
	".kintg.site",
}

// adDeliveryRejectSuffixes is the full DomainSuffix payload for the plain
// (non-RuleSet) ad reject rules in buildDNS/buildRoute: the curated YouTube
// set plus the supplement above.
func adDeliveryRejectSuffixes() []string {
	out := make([]string, 0, len(youTubeAdDeliverySuffixes)+len(extraAdDeliverySuffixes))
	out = append(out, youTubeAdDeliverySuffixes...)
	out = append(out, extraAdDeliverySuffixes...)
	return out
}
