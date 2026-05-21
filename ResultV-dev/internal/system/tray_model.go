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

package system

import (
	"fmt"
	"sort"
	"strings"

	"resultproxy-wails/internal/config"
)

const (
	defaultProviderName = "Мои прокси"
	defaultCountryName  = "Unknown"
)

type TrayServer struct {
	ID      string
	Name    string
	Country string
	IP      string
	Port    int
	PingMs  int64
}

type TrayCountryGroup struct {
	Country     string
	Servers     []TrayServer
	HiddenCount int
}

type TrayProviderGroup struct {
	Provider  string
	Countries []TrayCountryGroup
}

func BuildTrayMenuGroups(proxies []config.ProxyEntry, perCountryLimit int) []TrayProviderGroup {
	if perCountryLimit <= 0 {
		perCountryLimit = 20
	}

	grouped := make(map[string]map[string][]TrayServer)
	for _, p := range proxies {
		provider := strings.TrimSpace(p.Provider)
		if provider == "" {
			provider = defaultProviderName
		}
		country := normalizeCountry(p.Country)

		groupedByCountry, ok := grouped[provider]
		if !ok {
			groupedByCountry = make(map[string][]TrayServer)
			grouped[provider] = groupedByCountry
		}

		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = fmt.Sprintf("%s:%d", p.IP, p.Port)
		}
		groupedByCountry[country] = append(groupedByCountry[country], TrayServer{
			ID:      p.ID,
			Name:    name,
			Country: country,
			IP:      p.IP,
			Port:    p.Port,
			PingMs:  -1,
		})
	}

	providers := make([]string, 0, len(grouped))
	for provider := range grouped {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool {
		if providers[i] == defaultProviderName {
			return false
		}
		if providers[j] == defaultProviderName {
			return true
		}
		return strings.ToLower(providers[i]) < strings.ToLower(providers[j])
	})

	result := make([]TrayProviderGroup, 0, len(providers))
	for _, provider := range providers {
		countryMap := grouped[provider]
		countries := make([]string, 0, len(countryMap))
		for c := range countryMap {
			countries = append(countries, c)
		}
		sort.Slice(countries, func(i, j int) bool {
			if countries[i] == defaultCountryName {
				return false
			}
			if countries[j] == defaultCountryName {
				return true
			}
			return countries[i] < countries[j]
		})

		countryGroups := make([]TrayCountryGroup, 0, len(countries))
		for _, country := range countries {
			servers := countryMap[country]
			hidden := 0
			if len(servers) > perCountryLimit {
				hidden = len(servers) - perCountryLimit
				servers = servers[:perCountryLimit]
			}
			countryGroups = append(countryGroups, TrayCountryGroup{
				Country:     country,
				Servers:     servers,
				HiddenCount: hidden,
			})
		}

		result = append(result, TrayProviderGroup{
			Provider:  provider,
			Countries: countryGroups,
		})
	}

	return result
}

func normalizeCountry(country string) string {
	value := strings.TrimSpace(country)
	if value == "" || strings.EqualFold(value, "unknown") {
		return defaultCountryName
	}
	value = strings.ToUpper(value)
	if len([]rune(value)) > 2 {
		return value
	}
	return value
}

func countryToFlag(country string) string {
	code := normalizeCountry(country)
	if code == defaultCountryName {
		return "🌐"
	}
	runes := []rune(code)
	if len(runes) != 2 {
		return "🌐"
	}

	const base = 0x1F1E6
	first := runes[0]
	second := runes[1]
	if first < 'A' || first > 'Z' || second < 'A' || second > 'Z' {
		return "🌐"
	}
	return string([]rune{
		rune(base + (first - 'A')),
		rune(base + (second - 'A')),
	})
}

func countryDisplayName(country string) string {
	code := normalizeCountry(country)
	if code == defaultCountryName {
		return "Неизвестно"
	}
	if name, ok := countryNamesRU[code]; ok {
		return name
	}
	return code
}

func formatCountryTitle(country string) string {
	return countryDisplayName(country)
}

func countryISOCode(country string) string {
	code := normalizeCountry(country)
	runes := []rune(code)
	if len(runes) != 2 {
		return ""
	}
	if !isLatinLetter(runes[0]) || !isLatinLetter(runes[1]) {
		return ""
	}
	return strings.ToLower(code)
}

func formatServerTitle(server TrayServer, connected bool) string {
	statusMark := "  "
	if connected {
		statusMark = "✅"
	}
	ping := "..."
	if server.PingMs > 0 {
		ping = fmt.Sprintf("%dms", server.PingMs)
	} else if server.PingMs == 0 {
		ping = "Online"
	}
	name := sanitizeServerDisplayName(server.Name, server.Country)
	return fmt.Sprintf("%s %s [%s]", statusMark, name, ping)
}

func sanitizeServerDisplayName(name, country string) string {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return clean
	}

	flag := countryToFlag(country)
	if flag != "" && flag != "🌐" && strings.HasPrefix(clean, flag) {
		clean = strings.TrimSpace(strings.TrimPrefix(clean, flag))
	}

	code := normalizeCountry(country)
	if code == defaultCountryName {
		return clean
	}

	lower := strings.ToLower(clean)
	codeLower := strings.ToLower(code)
	if strings.HasPrefix(lower, codeLower) {
		runes := []rune(clean)
		if len(runes) == 2 || !isLatinLetter(runes[2]) {
			clean = strings.TrimSpace(string(runes[2:]))
			clean = strings.TrimLeft(clean, "-_:|[]() ")
		}
	}
	if clean == "" {
		return strings.TrimSpace(name)
	}
	return clean
}

func isLatinLetter(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

var countryNamesRU = map[string]string{
	"US": "США",
	"GB": "Великобритания",
	"DE": "Германия",
	"FR": "Франция",
	"NL": "Нидерланды",
	"FI": "Финляндия",
	"SE": "Швеция",
	"NO": "Норвегия",
	"CH": "Швейцария",
	"IT": "Италия",
	"ES": "Испания",
	"PL": "Польша",
	"CZ": "Чехия",
	"AT": "Австрия",
	"CA": "Канада",
	"JP": "Япония",
	"SG": "Сингапур",
	"HK": "Гонконг",
	"KR": "Южная Корея",
	"TR": "Турция",
	"AE": "ОАЭ",
	"RU": "Россия",
	"UA": "Украина",
	"KZ": "Казахстан",
}
