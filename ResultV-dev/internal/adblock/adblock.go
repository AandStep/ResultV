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

package adblock

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)


var fallbackAdDomains = []string{
	"doubleclick.net",
	"googlesyndication.com",
	"googleadservices.com",
	"googletagmanager.com",
	"google-analytics.com",
	"yandexadexchange.net",
	"mc.yandex.ru",
	"ads.vk.com",
	"adcash.com",
	"adsterra.com",
	"popads.net",
}


var listURLs = []string{
	"https://pgl.yoyo.org/adservers/serverlist.php?hostformat=nohtml&showintro=0",
}




type Blocker struct {
	mu      sync.RWMutex
	domains map[string]struct{} 
	loaded  bool
}


func New() *Blocker {
	b := &Blocker{
		domains: make(map[string]struct{}, len(fallbackAdDomains)),
	}
	for _, d := range fallbackAdDomains {
		b.domains[d] = struct{}{}
	}
	return b
}


func (b *Blocker) LoadFromCache(cacheDir string) error {
	cachePath := filepath.Join(cacheDir, "adblock_domains.txt")

	f, err := os.Open(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil 
		}
		return fmt.Errorf("opening adblock cache: %w", err)
	}
	defer f.Close()

	b.mu.Lock()
	defer b.mu.Unlock()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			b.domains[strings.ToLower(line)] = struct{}{}
			count++
		}
	}
	b.loaded = true
	return nil
}


func (b *Blocker) UpdateLists(cacheDir string) error {
	var allDomains []string

	client := &http.Client{Timeout: 30 * time.Second}

	for _, url := range listURLs {
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			
			parts := strings.Fields(line)
			var domain string
			if len(parts) >= 2 && (parts[0] == "127.0.0.1" || parts[0] == "0.0.0.0") {
				domain = parts[1]
			} else if len(parts) == 1 {
				domain = parts[0]
			}
			if domain != "" && domain != "localhost" {
				allDomains = append(allDomains, strings.ToLower(domain))
			}
		}
	}

	if len(allDomains) == 0 {
		return fmt.Errorf("no domains fetched from any list")
	}

	b.mu.Lock()
	for _, d := range allDomains {
		b.domains[d] = struct{}{}
	}
	b.loaded = true
	b.mu.Unlock()

	
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	cachePath := filepath.Join(cacheDir, "adblock_domains.txt")
	f, err := os.Create(cachePath)
	if err != nil {
		return fmt.Errorf("creating cache file: %w", err)
	}
	defer f.Close()

	b.mu.RLock()
	for d := range b.domains {
		fmt.Fprintln(f, d)
	}
	b.mu.RUnlock()

	return nil
}


func (b *Blocker) IsAdDomain(hostname string) bool {
	if hostname == "" {
		return false
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	h := strings.ToLower(hostname)

	
	if _, ok := b.domains[h]; ok {
		return true
	}

	
	for d := range b.domains {
		if strings.HasSuffix(h, "."+d) {
			return true
		}
	}

	return false
}


func (b *Blocker) GetDomainCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.domains)
}



func (b *Blocker) GetDomains() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]string, 0, len(b.domains))
	for d := range b.domains {
		result = append(result, d)
	}
	return result
}
