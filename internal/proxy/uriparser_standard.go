package proxy

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	M "github.com/sagernet/sing/common/metadata"

	"resultproxy-wails/internal/config"
)

// IsSubscriptionURL disambiguates single HTTP(S) links before the app starts
// a download. A path or query denotes a subscription. A bare authority with
// credentials, a port or an IP denotes a proxy, as does an authority entered
// with the HTTP protocol selected.
func IsSubscriptionURL(text string, preferHTTP bool) bool {
	text = normalizeSubscriptionBody(text)
	if strings.ContainsAny(text, "\r\n") {
		return false
	}
	u, err := url.Parse(text)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	if !hasProxyEndpointPath(u) {
		return true
	}
	return !preferHTTP && u.User == nil && !strings.HasSuffix(u.Host, ":") &&
		u.Port() == "" && net.ParseIP(u.Hostname()) == nil
}

func hasProxyEndpointPath(u *url.URL) bool {
	return (u.Path == "" || u.EscapedPath() == "/") && u.RawQuery == "" && !u.ForceQuery
}

func parseStandardProxy(line, defaultScheme string) (config.ProxyEntry, error) {
	scheme, address, hasScheme := strings.Cut(line, "://")
	if !hasScheme {
		scheme, address = defaultScheme, line
	}
	scheme = strings.ToLower(scheme)
	defaultPort := 1080
	switch scheme {
	case "socks", "socks5h":
		scheme = "socks5"
	case "socks4", "socks4a", "socks5":
	case "http":
		defaultPort = 80
	case "https":
		defaultPort = 443
	default:
		return config.ProxyEntry{}, fmt.Errorf("unsupported URI scheme")
	}

	address, ambiguous := normalizeProxyAuthority(address, hasScheme)
	if ambiguous {
		return config.ProxyEntry{}, fmt.Errorf("ambiguous proxy address: use a URL with an explicit scheme")
	}
	u, err := url.Parse(scheme + "://" + address)
	if err != nil || !hasProxyEndpointPath(u) {
		return config.ProxyEntry{}, fmt.Errorf("invalid %s proxy address", scheme)
	}

	host := u.Hostname()
	ip := net.ParseIP(host)
	if ip == nil && (!M.IsDomainName(host) || strings.ContainsAny(u.Host, "[]")) {
		return config.ProxyEntry{}, fmt.Errorf("invalid %s proxy host", scheme)
	}
	username := u.User.Username()
	password, hasPassword := u.User.Password()
	if !hasScheme && ip == nil && u.Port() == "" &&
		(!strings.Contains(host, ".") || (u.User != nil && !hasPassword)) {
		return config.ProxyEntry{}, fmt.Errorf("proxy address needs a port")
	}

	port := defaultPort
	if u.Port() != "" || strings.HasSuffix(u.Host, ":") {
		port, err = strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return config.ProxyEntry{}, fmt.Errorf("invalid %s proxy port", scheme)
		}
	}
	if u.User != nil && username == "" {
		return config.ProxyEntry{}, fmt.Errorf("missing %s proxy username", scheme)
	}
	if password != "" && (scheme == "socks4" || scheme == "socks4a") {
		return config.ProxyEntry{}, fmt.Errorf("%s does not support a password", scheme)
	}
	kind := strings.ToUpper(scheme)
	name := firstNonEmpty(u.Fragment, kind+" "+net.JoinHostPort(host, strconv.Itoa(port)))
	return config.ProxyEntry{
		IP: host, Port: port, Type: kind, Name: name,
		Username: username, Password: password,
		Country: countryFromNameAndHost(name, host),
	}, nil
}

// Normalize bare IPv6 and the host:port:user:password format supplied by
// proxy providers. The URL parser handles the standard user:password@host form.
func normalizeProxyAuthority(address string, hasScheme bool) (string, bool) {
	authority, fragment, _ := strings.Cut(address, "#")
	if ip := net.ParseIP(authority); ip != nil && strings.Contains(authority, ":") {
		return "[" + authority + "]" + strings.TrimPrefix(address, authority), false
	}
	if hasScheme || strings.ContainsAny(authority, "/?") {
		return address, false
	}
	hostEnd := strings.IndexByte(authority, ':')
	if strings.HasPrefix(authority, "[") {
		hostEnd = strings.Index(authority, "]:")
		if hostEnd >= 0 {
			hostEnd++
		}
	}
	if hostEnd < 0 {
		return address, false
	}
	host := authority[:hostEnd]
	parts := strings.SplitN(authority[hostEnd+1:], ":", 3)
	if len(parts) != 3 {
		return address, false
	}
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return address, false
	}
	if u, err := url.Parse("//" + authority); err == nil && u.User != nil &&
		(u.Port() != "" || net.ParseIP(u.Hostname()) != nil || strings.Contains(u.Hostname(), ".")) {
		return "", true
	}
	authority = url.UserPassword(parts[1], parts[2]).String() + "@" + host + ":" + parts[0]
	if fragment != "" {
		authority += "#" + fragment
	}
	return authority, false
}
