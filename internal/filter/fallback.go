// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

// embeddedFallbackRules is used when all remote downloads fail (offline / blocked CDN).
const embeddedFallbackRules = `! ResultV minimal fallback
||doubleclick.net^
||googlesyndication.com^
||googleadservices.com^
||google-analytics.com^
||googletagmanager.com^
||adservice.google.com^
||yandexadexchange.net^
||an.yandex.ru^
||mc.yandex.ru^
||ads.vk.com^
||adfox.ru^
||adriver.ru^
||betweendigital.com^
`
