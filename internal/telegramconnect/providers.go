package telegramconnect

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

const telegramIssuer = "https://oauth.telegram.org"

func TelegramVerifier(clientID int64) func(context.Context, string, string) (int64, error) {
	ctx := oidc.ClientContext(context.Background(), &http.Client{Timeout: 8 * time.Second})
	keys := oidc.NewRemoteKeySet(ctx, telegramIssuer+"/.well-known/jwks.json")
	verifier := oidc.NewVerifier(telegramIssuer, keys, &oidc.Config{
		ClientID: strconv.FormatInt(clientID, 10), SupportedSigningAlgs: []string{oidc.RS256},
	})
	return func(ctx context.Context, rawToken, nonce string) (int64, error) {
		token, err := verifier.Verify(ctx, rawToken)
		if err != nil || subtle.ConstantTimeCompare([]byte(token.Nonce), []byte(nonce)) != 1 {
			return 0, ErrAuth
		}
		var claims struct {
			ID json.Number `json:"id"`
		}
		if err := token.Claims(&claims); err != nil {
			return 0, ErrAuth
		}
		id, err := claims.ID.Int64()
		if err != nil || id <= 0 {
			return 0, ErrAuth
		}
		return id, nil
	}
}

// RemnawaveLookup reads only users matching the verified numeric Telegram ID.
func RemnawaveLookup(baseURL, token string) func(context.Context, int64) (string, error) {
	client := &http.Client{
		Timeout:       8 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return func(ctx context.Context, id int64) (string, error) {
		query := url.Values{
			"start": {"0"}, "size": {"2"},
			"filters":     {fmt.Sprintf(`[{"id":"telegramId","value":%d}]`, id)},
			"filterModes": {`{"telegramId":"equals"}`},
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/users?"+query.Encode(), nil)
		if err != nil {
			return "", ErrUnavailable
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			return "", ErrUnavailable
		}
		defer resp.Body.Close()
		var payload struct {
			Response struct {
				Total int
				Users []struct {
					TelegramID      int64
					Status          string
					ExpireAt        time.Time
					SubscriptionURL string
				}
			}
		}
		if resp.StatusCode != http.StatusOK || decodeJSON(resp.Body, &payload) != nil {
			return "", ErrUnavailable
		}
		if payload.Response.Total > 1 {
			return "", ErrAmbiguous
		}
		for _, user := range payload.Response.Users {
			if user.TelegramID != id {
				continue
			}
			if user.Status != "ACTIVE" || !user.ExpireAt.After(time.Now()) {
				return "", ErrInactive
			}
			return user.SubscriptionURL, nil
		}
		return "", ErrNotFound
	}
}
