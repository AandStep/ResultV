package telegramconnect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	AccessPath     = Prefix + "/access/"
	AccessLifetime = 10 * time.Minute
	AccessName     = "🇩🇪 Временный доступ к Telegram"
	AccessTitle    = "Временный доступ к Telegram"

	accessTag          = "RV_TG_ACCESS"
	accessUserPrefix   = "rv_tg_"
	accessTrafficLimit = 100 << 20
	accessPerMinute    = 20
	accessMaxLeases    = 500
)

var accessUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// AccessConfig describes the Telegram-only inbound and the squad that grants it.
type AccessConfig struct {
	RemnawaveURL string
	Token        string
	Squad        string
	Address      string
	Port         int
	ServerName   string
	PublicKey    string
	ShortID      string
}

func (c AccessConfig) complete() bool {
	return c.RemnawaveURL != "" && c.Token != "" && accessUUID.MatchString(c.Squad) &&
		c.Address != "" && c.Port > 0 && c.Port < 65536 && c.ServerName != "" &&
		c.PublicKey != "" && c.ShortID != ""
}

type accessLease struct {
	id      int64
	uuid    string
	expires time.Time
}

type AccessIssuer struct {
	cfg    AccessConfig
	client *http.Client
	now    func() time.Time

	mu     sync.Mutex
	leases map[string]accessLease
	recent []time.Time
}

// NewAccessIssuer returns nil when the configuration is incomplete, which
// disables temporary access everywhere it is offered.
func NewAccessIssuer(cfg AccessConfig) *AccessIssuer {
	if !cfg.complete() {
		return nil
	}
	cfg.RemnawaveURL = strings.TrimRight(cfg.RemnawaveURL, "/")
	return &AccessIssuer{
		cfg:    cfg,
		now:    time.Now,
		leases: make(map[string]accessLease),
		client: &http.Client{
			Timeout:       8 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (b *AccessIssuer) issue(ctx context.Context, key string) (accessLease, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	for k, lease := range b.leases {
		if !lease.expires.After(now) {
			delete(b.leases, k)
		}
	}
	if lease, ok := b.leases[key]; ok {
		return lease, nil
	}
	for len(b.recent) > 0 && now.Sub(b.recent[0]) >= time.Minute {
		b.recent = b.recent[1:]
	}
	if len(b.recent) >= accessPerMinute || len(b.leases) >= accessMaxLeases {
		return accessLease{}, ErrUnavailable
	}
	b.recent = append(b.recent, now)

	expires := now.Add(AccessLifetime)
	body, _ := json.Marshal(map[string]any{
		"username":             accessUserPrefix + newCode()[:24],
		"expireAt":             expires.UTC().Format(time.RFC3339),
		"activeInternalSquads": []string{b.cfg.Squad},
		"trafficLimitBytes":    accessTrafficLimit,
		"trafficLimitStrategy": "NO_RESET",
		"tag":                  accessTag,
		"description":          "Temporary Telegram access",
	})
	var created struct {
		Response struct {
			ID        int64  `json:"id"`
			VLESSUUID string `json:"vlessUuid"`
		} `json:"response"`
	}
	status, err := b.call(ctx, http.MethodPost, "/api/users", body, &created)
	if err != nil || (status != http.StatusOK && status != http.StatusCreated) ||
		!accessUUID.MatchString(created.Response.VLESSUUID) {
		return accessLease{}, ErrUnavailable
	}
	lease := accessLease{id: created.Response.ID, uuid: created.Response.VLESSUUID, expires: expires}
	b.leases[key] = lease
	return lease, nil
}

func (b *AccessIssuer) call(ctx context.Context, method, path string, body []byte, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, b.cfg.RemnawaveURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+b.cfg.Token)
	resp, err := b.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode < 300 {
		if err := decodeJSON(resp.Body, out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

// Sweep deletes temporary users whose lease is over. Only users carrying both
// the access tag and the generated name prefix are ever touched.
func (b *AccessIssuer) Sweep(ctx context.Context) error {
	query := url.Values{
		"start": {"0"}, "size": {"200"},
		"filters":     {`[{"id":"tag","value":"` + accessTag + `"}]`},
		"filterModes": {`{"tag":"equals"}`},
	}
	var list struct {
		Response struct {
			Users []struct {
				ID       int64     `json:"id"`
				Username string    `json:"username"`
				Tag      string    `json:"tag"`
				ExpireAt time.Time `json:"expireAt"`
			} `json:"users"`
		} `json:"response"`
	}
	status, err := b.call(ctx, http.MethodGet, "/api/users?"+query.Encode(), nil, &list)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("listing temporary users: HTTP %d", status)
	}
	now := b.now()
	var failed error
	for _, user := range list.Response.Users {
		if user.Tag != accessTag || !strings.HasPrefix(user.Username, accessUserPrefix) ||
			user.ID <= 0 || user.ExpireAt.After(now) {
			continue
		}
		status, err := b.call(ctx, http.MethodDelete, "/api/users/"+strconv.FormatInt(user.ID, 10), nil, nil)
		if err != nil || (status != http.StatusOK && status != http.StatusNoContent) {
			failed = errors.Join(failed, fmt.Errorf("deleting temporary user %d: HTTP %d %v", user.ID, status, err))
		}
	}
	return failed
}

// RunSweeper removes expired temporary users once a minute until ctx ends.
func (b *AccessIssuer) RunSweeper(ctx context.Context, report func(error)) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := b.Sweep(ctx); err != nil && report != nil {
			report(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (b *AccessIssuer) link(lease accessLease) string {
	query := url.Values{
		"type": {"tcp"}, "security": {"reality"}, "encryption": {"none"},
		"flow": {"xtls-rprx-vision"}, "fp": {"chrome"}, "sni": {b.cfg.ServerName},
		"pbk": {b.cfg.PublicKey}, "sid": {b.cfg.ShortID},
	}
	u := url.URL{
		Scheme: "vless", User: url.User(lease.uuid),
		Host:     net.JoinHostPort(b.cfg.Address, strconv.Itoa(b.cfg.Port)),
		RawQuery: query.Encode(), Fragment: AccessName,
	}
	return u.String()
}

func (b *AccessIssuer) outbound(lease accessLease) map[string]any {
	return map[string]any{
		"tag":      "proxy",
		"protocol": "vless",
		"settings": map[string]any{"vnext": []any{map[string]any{
			"address": b.cfg.Address,
			"port":    b.cfg.Port,
			"users": []any{map[string]any{
				"id": lease.uuid, "encryption": "none", "flow": "xtls-rprx-vision",
			}},
		}}},
		"streamSettings": map[string]any{
			"network":     "tcp",
			"tcpSettings": map[string]any{},
			"security":    "reality",
			"realitySettings": map[string]any{
				"serverName":  b.cfg.ServerName,
				"publicKey":   b.cfg.PublicKey,
				"shortId":     b.cfg.ShortID,
				"fingerprint": "chrome",
			},
		},
	}
}

func (b *AccessIssuer) userinfo(lease accessLease) string {
	return fmt.Sprintf("upload=0; download=0; total=%d; expire=%d", accessTrafficLimit, lease.expires.Unix())
}

// IsAccessSubscription reports whether raw points at a temporary Telegram access list.
func IsAccessSubscription(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && strings.EqualFold(u.Host, "result-service.ru") &&
		u.User == nil && u.RawQuery == "" && u.Fragment == "" &&
		strings.HasPrefix(u.EscapedPath(), AccessPath) && validCode(strings.TrimPrefix(u.EscapedPath(), AccessPath))
}
