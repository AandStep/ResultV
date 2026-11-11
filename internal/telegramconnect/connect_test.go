package telegramconnect

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestConnectDoesNotOpenMissingLoginPage(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	_, err := Connect(t.Context(), server.URL, "en", func(string) {
		t.Fatal("browser opened despite a 404")
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("404 returned %v", err)
	}
}

func TestConnectCanCancelAndRetry(t *testing.T) {
	server := httptest.NewServer(NewHandler(Origin, 123, nil, nil))
	defer server.Close()

	for range 2 {
		ctx, cancel := context.WithCancel(t.Context())
		var callbackHost string
		_, err := Connect(ctx, server.URL, "en", func(loginURL string) {
			u, err := url.Parse(loginURL)
			if err != nil {
				t.Fatal(err)
			}
			callbackHost = "127.0.0.1:" + u.Query().Get("port")
			cancel()
		})
		cancel()
		if !errors.Is(err, ErrCancelled) || callbackHost == "" {
			t.Fatalf("expected cancellation after opening the browser, got %v", err)
		}
		client := http.Client{Timeout: time.Second}
		if response, err := client.Get("http://" + callbackHost + "/callback"); err == nil {
			response.Body.Close()
			t.Fatal("cancelled login still accepts browser callbacks")
		}
	}
}

func TestConnectReturnsSubscriptionOnlyToInitiatingClient(t *testing.T) {
	const origin = "https://result-service.ru"
	const subscription = origin + "/s/test-subscription"
	var lookupCount int
	handler := NewHandler(origin, 123,
		func(_ context.Context, token, nonce string) (int64, error) {
			if token != "verified-token" || !validCode(nonce) {
				return 0, ErrAuth
			}
			return 987654321, nil
		},
		func(_ context.Context, id int64) (string, error) {
			lookupCount++
			if id != 987654321 {
				t.Fatal("lookup did not use verified Telegram ID")
			}
			return subscription, nil
		},
	)
	server := httptest.NewServer(handler)
	defer server.Close()

	got, err := Connect(t.Context(), server.URL, "en", func(loginURL string) {
		login, err := http.Get(loginURL)
		if err != nil {
			t.Fatal(err)
		}
		defer login.Body.Close()
		page, _ := io.ReadAll(login.Body)
		nonce := regexp.MustCompile(`name="telegram-nonce" content="([A-Za-z0-9_-]+)"`).FindStringSubmatch(string(page))
		if login.StatusCode != http.StatusOK || len(nonce) != 2 {
			t.Fatal("login page did not expose a fresh nonce")
		}

		post := func(path string, data any) *http.Response {
			body, _ := json.Marshal(data)
			req, _ := http.NewRequest(http.MethodPost, server.URL+Prefix+path, strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", origin)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { res.Body.Close() })
			return res
		}

		if res := post("/verify", map[string]string{"nonce": nonce[1], "idToken": "forged"}); res.StatusCode != http.StatusUnauthorized {
			t.Fatal("unverified token was accepted")
		}
		res := post("/verify", map[string]string{"nonce": nonce[1], "idToken": "verified-token"})
		var response struct{ Ticket string }
		if res.StatusCode != http.StatusOK || json.NewDecoder(res.Body).Decode(&response) != nil || !validCode(response.Ticket) {
			t.Fatal("verified login did not produce a one-time ticket")
		}
		if res := post("/verify", map[string]string{"nonce": nonce[1], "idToken": "verified-token"}); res.StatusCode != http.StatusUnauthorized {
			t.Fatal("login nonce was reusable")
		}
		if res := post("/exchange", map[string]string{"ticket": response.Ticket, "verifier": newCode()}); res.StatusCode != http.StatusUnauthorized {
			t.Fatal("ticket worked from another client")
		}
		if lookupCount != 0 {
			t.Fatal("lookup ran without proof of possession")
		}

		start, _ := url.Parse(loginURL)
		query := start.Query()
		callback := "http://127.0.0.1:" + query.Get("port") + "/callback?" + url.Values{
			"state": {query.Get("challenge")}, "ticket": {response.Ticket},
		}.Encode()
		callbackResponse, err := http.Get(callback)
		if err != nil {
			t.Fatal(err)
		}
		callbackResponse.Body.Close()
		if callbackResponse.StatusCode != http.StatusOK {
			t.Fatal("desktop callback failed")
		}
	})
	if err != nil || got != subscription || lookupCount != 1 {
		t.Fatalf("connect result = %q, %v, lookups = %d", got, err, lookupCount)
	}
}
