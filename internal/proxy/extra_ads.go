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
	"pubserv.pro", // psb-dsp.pubserv.pro — video pre-roll DSP on RU streaming sites
}

// extraAdDeliverySuffixes mirror extraAdDeliveryDomains for subdomain matches.
var extraAdDeliverySuffixes = []string{
	".pubserv.pro",
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
