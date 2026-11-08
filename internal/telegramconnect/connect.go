// Package telegramconnect exchanges a verified Telegram login for a ResultV subscription.
package telegramconnect

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	Prefix       = "/desktop/telegram"
	Origin       = "https://result-service.ru"
	LoginTimeout = 5 * time.Minute
)

var (
	ErrAuth          = errors.New("telegram_auth_failed")
	ErrCancelled     = errors.New("telegram_cancelled")
	ErrTimeout       = errors.New("telegram_timeout")
	ErrUnavailable   = errors.New("telegram_unavailable")
	ErrNotFound      = errors.New("telegram_subscription_not_found")
	ErrInactive      = errors.New("telegram_subscription_inactive")
	ErrAmbiguous     = errors.New("telegram_subscription_ambiguous")
	subscriptionPath = regexp.MustCompile(`^/s/([a-zA-Z0-9_-]+)/?$`)
)

// SubscriptionKey identifies the same subscription on either first-party domain.
func SubscriptionKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	if !strings.EqualFold(u.Host, "result-service.ru") && !strings.EqualFold(u.Host, "cdn.result-proxy.ru:8443") {
		return ""
	}
	match := subscriptionPath.FindStringSubmatch(u.EscapedPath())
	if match == nil {
		return ""
	}
	return match[1]
}

func newCode() string {
	var raw [32]byte
	rand.Read(raw[:])
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

func validCode(s string) bool {
	if len(s) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(s)
	return err == nil && len(decoded) == 32
}

func challenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func decodeJSON(r io.Reader, value any) error {
	return json.NewDecoder(io.LimitReader(r, 64<<10)).Decode(value)
}

type exchangeResponse struct {
	SubscriptionURL string `json:"subscriptionUrl,omitempty"`
	Error           string `json:"error,omitempty"`
}

func exchange(ctx context.Context, endpoint, ticket, verifier string) (string, error) {
	body, _ := json.Marshal(map[string]string{"ticket": ticket, "verifier": verifier})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+Prefix+"/exchange", strings.NewReader(string(body)))
	if err != nil {
		return "", ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{
		Timeout:       12 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	res, err := client.Do(req)
	if err != nil {
		return "", ErrUnavailable
	}
	defer res.Body.Close()
	var reply exchangeResponse
	if err := decodeJSON(res.Body, &reply); err != nil {
		return "", ErrUnavailable
	}
	if res.StatusCode == http.StatusOK && SubscriptionKey(reply.SubscriptionURL) != "" {
		return reply.SubscriptionURL, nil
	}
	for _, known := range []error{ErrNotFound, ErrInactive, ErrAmbiguous, ErrAuth} {
		if reply.Error == known.Error() {
			return "", known
		}
	}
	return "", ErrUnavailable
}
