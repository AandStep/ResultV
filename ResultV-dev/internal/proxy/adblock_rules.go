// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"os"
	"path/filepath"
)

const (
	adBlockRuleSetTag      = "ads"
	adBlockRuleSetTagRU    = "ads-ru"
	adBlockMITMOutbound    = "adblock-mitm"
	adBlockMITMUpdateEvery = "24h"
	adBlockRuleSetsSubdir  = "filter/rulesets"
	minLocalSRSBytes       = 512
)

// Default remote adblock rule-sets (sing-box binary SRS).
var defaultAdBlockRuleSetURLs = []struct {
	tag      string
	fileName string
	url      string
}{
	{
		tag: adBlockRuleSetTag, fileName: "ads.srs",
		url: "https://raw.githubusercontent.com/REIJI007/AdBlock_Rule_For_Sing-box/main/adblock_reject.srs",
	},
	{
		tag: adBlockRuleSetTagRU, fileName: "ads-ru.srs",
		url: "https://raw.githubusercontent.com/runetfreedom/russia-v2ray-rules-dat/release/sing-box/rule-set-geosite/geosite-category-ads-all.srs",
	},
}

func buildAdBlockRuleSets(dataDir string) []SBRuleSet {
	out := make([]SBRuleSet, 0, len(defaultAdBlockRuleSetURLs))
	for _, src := range defaultAdBlockRuleSetURLs {
		localPath := filepath.Join(dataDir, adBlockRuleSetsSubdir, src.fileName)
		if st, err := os.Stat(localPath); err == nil && st.Size() >= minLocalSRSBytes {
			out = append(out, SBRuleSet{
				Type:   "local",
				Tag:    src.tag,
				Format: "binary",
				LocalOptions: SBLocalRuleSet{
					Path: localPath,
				},
			})
			continue
		}
		out = append(out, SBRuleSet{
			Type:   "remote",
			Tag:    src.tag,
			Format: "binary",
			RemoteOptions: SBRemoteRuleSet{
				URL:            src.url,
				UpdateInterval: adBlockMITMUpdateEvery,
			},
		})
	}
	return out
}

func adBlockRuleSetTags() []string {
	tags := make([]string, len(defaultAdBlockRuleSetURLs))
	for i, src := range defaultAdBlockRuleSetURLs {
		tags[i] = src.tag
	}
	return tags
}

// browserProcessPathRegexes matches common desktop browsers for MITM routing.
func browserProcessPathRegexes() []string {
	names := []string{
		"chrome.exe", "chromium.exe", "msedge.exe", "brave.exe",
		"firefox.exe", "opera.exe", "vivaldi.exe", "browser.exe",
		"waterfox.exe", "librewolf.exe", "yandex.exe", "yandexbrowser.exe",
	}
	return appWhitelistPathRegexes(names)
}

// adBlockPinningBypassDomains are never MITM'd (banks, updates, pinning-heavy).
var adBlockPinningBypassDomains = []string{
	"apple.com", "icloud.com", "mzstatic.com",
	"microsoft.com", "windows.com", "windowsupdate.com", "live.com",
	"google.com", "googleapis.com", "gstatic.com",
	"gosuslugi.ru", "nalog.ru", "sberbank.ru", "vtb.ru", "tinkoff.ru",
	"alfabank.ru", "raiffeisen.ru", "openbank.ru",
}
