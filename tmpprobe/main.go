// Временная программа для разбора подписки: что она отдаёт в заголовках и
// в теле. Не входит в сборку приложения, удаляется после разбора.
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"resultproxy-wails/internal/proxy"
)

func redact(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparsable>"
	}
	path := u.Path
	if len(path) > 12 {
		path = path[:12] + "…(" + fmt.Sprint(len(path)) + " chars)"
	}
	return u.Scheme + "://" + u.Host + path
}

func main() {
	link := os.Args[1]
	plain, err := proxy.DecodeDeepLink(link)
	if err != nil {
		fmt.Println("decode error:", err)
		return
	}
	plain = strings.TrimSpace(plain)
	fmt.Println("== decoded payload ==")
	fmt.Println("lines:", len(strings.Split(plain, "\n")))
	fmt.Println("url:", redact(plain))

	if !strings.HasPrefix(strings.ToLower(plain), "http") {
		fmt.Println("payload is not a URL; first 400 chars:")
		if len(plain) > 400 {
			plain = plain[:400]
		}
		fmt.Println(plain)
		return
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 20 * time.Second, Jar: jar}

	for _, ua := range []string{"ResultV/3.3.2", "Happ/1.0", "v2rayNG/1.8.5"} {
		req, _ := http.NewRequest(http.MethodGet, plain, nil)
		req.Header.Set("User-Agent", ua)
		req.Header.Set("x-device-os", "windows")
		req.Header.Set("x-ver-os", "10.0.26200")
		req.Header.Set("x-device-model", "PC")
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("fetch error:", err)
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		fmt.Printf("\n== UA %q -> HTTP %d, %d bytes ==\n", ua, resp.StatusCode, len(body))
		keys := make([]string, 0, len(resp.Header))
		for k := range resp.Header {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := strings.Join(resp.Header[k], " | ")
			if len(v) > 300 {
				v = v[:300] + "…"
			}
			fmt.Printf("  %-32s %s\n", k, v)
		}

		trimmed := strings.TrimSpace(string(body))
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			fmt.Println("  --- body (JSON, first 1200 chars) ---")
			if len(trimmed) > 1200 {
				trimmed = trimmed[:1200]
			}
			fmt.Println(trimmed)
		} else {
			fmt.Println("  --- body: not JSON, first 200 chars ---")
			if len(trimmed) > 200 {
				trimmed = trimmed[:200]
			}
			fmt.Println(trimmed)
		}
	}
}
