// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Ad-blocking is implemented as pure sing-box routing: DNS + route `reject`
// of ad/tracker domains via binary `rule_set` lists. No MITM / TLS
// interception (that breaks tunnelled connections — see the desktop's
// abandoned filter layer), so it is safe to leave on alongside any proxy.

const (
	adBlockRuleSetTag     = "ads"    // global ad/tracker list
	adBlockRuleSetTagRU   = "ads-ru" // Russia-tuned ads list
	adBlockUpdateInterval = "24h"
	adBlockRuleSetsSubdir = "adblock"
	// minLocalSRSBytes guards against referencing a truncated / half-written
	// SRS as a `local` rule_set — sing-box fails startup on a corrupt rule_set,
	// which would break the connection. Below this size we fall back to remote.
	minLocalSRSBytes = 512
)

// adBlockRuleSetSource is one downloadable sing-box binary (SRS) rule-set.
// urls are tried in order until one yields a valid SRS — the jsDelivr CDN
// mirror is a fallback for when raw.githubusercontent.com is blocked/down in
// Russia (a frequent cause of "ads appear intermittently").
type adBlockRuleSetSource struct {
	tag      string
	fileName string
	urls     []string
}

// defaultAdBlockRuleSets are the pre-compiled SRS lists we reject on: a
// global ad/tracker list plus a Russia-tuned ads list (matches the desktop
// build's sources). Each source lists the github raw URL first, then a jsDelivr
// mirror of the same content for github-outage resilience.
var defaultAdBlockRuleSets = []adBlockRuleSetSource{
	{
		tag:      adBlockRuleSetTag,
		fileName: "ads.srs",
		urls: []string{
			"https://raw.githubusercontent.com/REIJI007/AdBlock_Rule_For_Sing-box/main/adblock_reject.srs",
			"https://cdn.jsdelivr.net/gh/REIJI007/AdBlock_Rule_For_Sing-box@main/adblock_reject.srs",
		},
	},
	{
		tag:      adBlockRuleSetTagRU,
		fileName: "ads-ru.srs",
		urls: []string{
			"https://raw.githubusercontent.com/runetfreedom/russia-v2ray-rules-dat/release/sing-box/rule-set-geosite/geosite-category-ads-all.srs",
			"https://cdn.jsdelivr.net/gh/runetfreedom/russia-v2ray-rules-dat@release/sing-box/rule-set-geosite/geosite-category-ads-all.srs",
		},
	},
}

// adBlockRuleSetsDir is where DownloadAdBlockRuleSets caches the SRS files and
// where buildAdBlockRuleSets looks for the local copy.
func adBlockRuleSetsDir(dataDir string) string {
	return filepath.Join(dataDir, adBlockRuleSetsSubdir)
}

// buildAdBlockRuleSets returns the sing-box rule_set definitions for the ad
// lists. A locally cached SRS (written by DownloadAdBlockRuleSets) is
// preferred — instant and offline; when it is absent or truncated we emit a
// remote rule_set that sing-box downloads itself via the `direct` outbound and
// caches in cache_file.
//
// download_detour MUST be "direct": the download has to bypass the proxy (the
// tunnel may not be up yet, and routing it through the proxy would add a
// startup dependency). A failed remote fetch is non-fatal in sing-box — the
// rule simply starts empty and updates in the background — so a dead URL
// degrades to "no blocking", never a broken connection.
func buildAdBlockRuleSets(dataDir string) []SBRouteRuleSet {
	out := make([]SBRouteRuleSet, 0, len(defaultAdBlockRuleSets))
	for _, src := range defaultAdBlockRuleSets {
		localPath := filepath.Join(adBlockRuleSetsDir(dataDir), src.fileName)
		if st, err := os.Stat(localPath); err == nil && st.Size() >= minLocalSRSBytes {
			out = append(out, SBRouteRuleSet{
				Type:   "local",
				Tag:    src.tag,
				Format: "binary",
				Path:   localPath,
			})
			continue
		}
		out = append(out, SBRouteRuleSet{
			Type:           "remote",
			Tag:            src.tag,
			Format:         "binary",
			URL:            src.urls[0],
			DownloadDetour: "direct",
			UpdateInterval: adBlockUpdateInterval,
		})
	}
	return out
}

// adBlockRuleSetTags returns the rule_set tags the reject rules reference.
func adBlockRuleSetTags() []string {
	tags := make([]string, len(defaultAdBlockRuleSets))
	for i, src := range defaultAdBlockRuleSets {
		tags[i] = src.tag
	}
	return tags
}

// adBlockConnectivityBypassDomains are hosts Android uses for captive-portal
// checks, Private DNS (DoT) validation and push delivery. Ad/tracker SRS lists
// often include *.gstatic.com — blocking them makes the OS report "no internet"
// / "Private DNS server unavailable" even when the tunnel is fine.
//
// The mtalk / FCM hosts are here for the same reason, found on a device: the
// ad rule_set was rejecting com.google.android.gms' connection to
// mtalk.google.com:5228, which kills push notifications in EVERY app while
// ad-block is on. Google rotates GCM across alt1..alt8-mtalk, so all of them
// must be listed — these are exact-match entries, not suffixes.
var adBlockConnectivityBypassDomains = []string{
	"connectivitycheck.gstatic.com",
	"www.gstatic.com",
	"clients3.google.com",
	"play.googleapis.com",
	"www.google.com",
	"dns.google",
	"dns.quad9.net",
	"one.one.one.one",
	"dns.cloudflare.com",
	"cloudflare-dns.com",
	"dnsotls-ds.metric.gstatic.com",
	// FCM / GCM push (ports 5228-5230).
	"mtalk.google.com",
	"mtalk4.google.com",
	"alt1-mtalk.google.com",
	"alt2-mtalk.google.com",
	"alt3-mtalk.google.com",
	"alt4-mtalk.google.com",
	"alt5-mtalk.google.com",
	"alt6-mtalk.google.com",
	"alt7-mtalk.google.com",
	"alt8-mtalk.google.com",
	"android.googleapis.com",
	"fcm.googleapis.com",
}

// AdBlockDownloadResult reports how many ad lists were cached locally.
type AdBlockDownloadResult struct {
	Ready int
	Total int
	Err   error
}

// DownloadAdBlockRuleSets fetches each ad rule-set SRS into dataDir/adblock so
// buildAdBlockRuleSets can reference them locally on the next connect. Partial
// success is fine: any list that fails to download stays remote, and any
// previously cached copy is left untouched (writes are atomic).
func DownloadAdBlockRuleSets(ctx context.Context, dataDir string) AdBlockDownloadResult {
	res := AdBlockDownloadResult{Total: len(defaultAdBlockRuleSets)}
	dir := adBlockRuleSetsDir(dataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		res.Err = fmt.Errorf("creating adblock dir: %w", err)
		return res
	}
	client := &http.Client{Timeout: 60 * time.Second}
	var lastErr error
	for _, src := range defaultAdBlockRuleSets {
		if err := downloadAdBlockSRS(ctx, client, src, dir); err != nil {
			lastErr = err
			continue
		}
		res.Ready++
	}
	if res.Ready == 0 && lastErr != nil {
		res.Err = lastErr
	}
	return res
}

func downloadAdBlockSRS(ctx context.Context, client *http.Client, src adBlockRuleSetSource, dir string) error {
	var lastErr error
	for _, u := range src.urls {
		data, err := fetchAdBlockSRS(ctx, client, u)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", src.tag, err)
			continue // try the next mirror (github raw is often blocked in RU)
		}
		// Atomic write: a half-written SRS referenced as a local rule_set would
		// fail sing-box startup and break the connection. Write to a temp file
		// and rename so the live path always points at a complete file — and a
		// failed download never clobbers a previously cached copy.
		dst := filepath.Join(dir, src.fileName)
		tmp := dst + ".tmp"
		if err := os.WriteFile(tmp, data, 0o600); err != nil {
			return fmt.Errorf("%s: writing temp: %w", src.tag, err)
		}
		if err := os.Rename(tmp, dst); err != nil {
			os.Remove(tmp)
			return fmt.Errorf("%s: renaming: %w", src.tag, err)
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%s: no URLs configured", src.tag)
	}
	return lastErr
}

// fetchAdBlockSRS GETs one SRS URL and returns its bytes, validating the
// response is a plausible (non-truncated) rule-set.
func fetchAdBlockSRS(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	if len(data) < minLocalSRSBytes {
		return nil, fmt.Errorf("response too small (%d bytes)", len(data))
	}
	return data, nil
}
