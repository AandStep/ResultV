package telegramconnect

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const gatewayBodyLimit = 4 << 20

var (
	gatewayPath   = regexp.MustCompile(`^/api/sub/([A-Za-z0-9_-]{6,64})(/[A-Za-z0-9_-]{1,32})?/?$`)
	userinfoField = regexp.MustCompile(`(?i)(?:^|;)\s*expire\s*=\s*(\d+)`)
	hopHeaders    = []string{
		"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"Proxy-Connection", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
	}
)

// SubscriptionGateway passes Remnawave subscriptions through unchanged and
// appends temporary Telegram access to expired ones, so their owner can renew.
type SubscriptionGateway struct {
	upstream *url.URL
	client   *http.Client
	access   *AccessIssuer
	now      func() time.Time
}

func NewSubscriptionGateway(upstream string, access *AccessIssuer) (*SubscriptionGateway, error) {
	u, err := url.Parse(strings.TrimRight(upstream, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, ErrUnavailable
	}
	return &SubscriptionGateway{
		upstream: u,
		access:   access,
		now:      time.Now,
		client: &http.Client{
			Timeout:       25 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (g *SubscriptionGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	match := gatewayPath.FindStringSubmatch(r.URL.EscapedPath())
	if match == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		http.NotFound(w, r)
		return
	}
	target := *g.upstream
	target.Path = r.URL.Path
	target.RawPath = r.URL.RawPath
	target.RawQuery = r.URL.RawQuery
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), nil)
	if err != nil {
		http.Error(w, "Bad gateway", http.StatusBadGateway)
		return
	}
	req.Header = r.Header.Clone()
	for _, h := range hopHeaders {
		req.Header.Del(h)
	}
	req.Header.Del("Accept-Encoding")
	if req.Header.Get("X-Forwarded-Proto") == "" {
		req.Header.Set("X-Forwarded-Proto", "https")
	}
	if req.Header.Get("X-Forwarded-For") == "" {
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			req.Header.Set("X-Forwarded-For", host)
		}
	}
	req.Host = r.Host

	resp, err := g.client.Do(req)
	if err != nil {
		http.Error(w, "Bad gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	header := w.Header()
	for k, v := range resp.Header {
		header[k] = v
	}
	for _, h := range hopHeaders {
		header.Del(h)
	}

	if g.access == nil || r.Method != http.MethodGet || resp.StatusCode != http.StatusOK || !g.expired(resp.Header) {
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, gatewayBodyLimit+1))
	if err != nil || len(body) > gatewayBodyLimit {
		http.Error(w, "Bad gateway", http.StatusBadGateway)
		return
	}
	if lease, err := g.access.issue(r.Context(), "sub:"+match[1]); err == nil {
		if extended, ok := g.access.appendTo(body, resp.Header.Get("Content-Type"), lease); ok {
			body = extended
		}
	}
	header.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func (g *SubscriptionGateway) expired(h http.Header) bool {
	m := userinfoField.FindStringSubmatch(h.Get("Subscription-Userinfo"))
	if m == nil {
		return false
	}
	expire, err := strconv.ParseInt(m[1], 10, 64)
	return err == nil && expire > 0 && expire <= g.now().Unix()
}

// appendTo adds the access server in the format the subscription already uses:
// an Xray JSON array or a base64 list of links. Anything else is left alone.
func (b *AccessIssuer) appendTo(body []byte, contentType string, lease accessLease) ([]byte, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var configs []json.RawMessage
		if json.Unmarshal(trimmed, &configs) != nil {
			return nil, false
		}
		entry, err := b.jsonEntry(configs, lease)
		if err != nil {
			return nil, false
		}
		out, err := json.Marshal(append(configs, entry))
		return out, err == nil
	}
	if strings.Contains(strings.ToLower(contentType), "json") {
		return nil, false
	}
	text, ok := decodeLinkList(trimmed)
	if !ok {
		return nil, false
	}
	list := strings.TrimRight(text, "\r\n") + "\n" + b.link(lease) + "\n"
	return []byte(base64.StdEncoding.EncodeToString([]byte(list))), true
}

// jsonEntry reuses the first config of the subscription as a template, so the
// access server carries the same DNS, routing and local inbounds as the rest.
func (b *AccessIssuer) jsonEntry(configs []json.RawMessage, lease accessLease) (json.RawMessage, error) {
	entry := map[string]any{}
	if len(configs) > 0 && json.Unmarshal(configs[0], &entry) != nil {
		entry = map[string]any{}
	}
	ours := b.outbound(lease)
	outbounds, _ := entry["outbounds"].([]any)
	replaced := false
	for i, o := range outbounds {
		if m, ok := o.(map[string]any); ok && m["tag"] == "proxy" {
			outbounds[i] = ours
			replaced = true
			break
		}
	}
	if !replaced {
		outbounds = []any{ours,
			map[string]any{"tag": "direct", "protocol": "freedom"},
			map[string]any{"tag": "block", "protocol": "blackhole"},
		}
	}
	entry["outbounds"] = outbounds
	entry["remarks"] = AccessName
	return json.Marshal(entry)
}

func decodeLinkList(raw []byte) (string, bool) {
	s := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, string(raw))
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		decoded, err := enc.DecodeString(s)
		if err == nil && strings.Contains(string(decoded), "://") {
			return string(decoded), true
		}
	}
	return "", false
}
