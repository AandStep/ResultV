package telegramconnect

import (
	"context"
	"crypto/subtle"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Connect opens the login page and waits for this browser's loopback callback.
// The server gives the subscription only to the caller holding the verifier.
func Connect(ctx context.Context, origin, language string, openBrowser func(string)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, LoginTimeout)
	defer cancel()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", ErrUnavailable
	}
	defer listener.Close()

	verifier := newCode()
	state := challenge(verifier)
	type callback struct {
		ticket string
		err    error
	}
	result := make(chan callback, 1)
	host := listener.Addr().String()
	handler := http.NewServeMux()
	handler.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.Host != host || subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(state)) != 1 {
			http.Error(w, "Invalid callback.", http.StatusBadRequest)
			return
		}
		answer := callback{ticket: query.Get("ticket")}
		switch query.Get("error") {
		case "cancelled":
			answer.err = ErrCancelled
		case "auth_failed":
			answer.err = ErrAuth
		case "unavailable":
			answer.err = ErrUnavailable
		default:
			if !validCode(answer.ticket) {
				http.Error(w, "Invalid callback.", http.StatusBadRequest)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		select {
		case result <- answer:
			_, _ = io.WriteString(w, "Вернитесь в ResultV. / Return to ResultV.")
		default:
			http.Error(w, "Callback already received.", http.StatusConflict)
		}
	})
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout: 5 * time.Second, MaxHeaderBytes: 4096,
	}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()

	_, port, _ := net.SplitHostPort(host)
	query := url.Values{"port": {port}, "challenge": {state}, "lang": {language}}
	loginURL := origin + Prefix + "/login?" + query.Encode()
	if err := checkLoginPage(ctx, loginURL); err != nil {
		return "", err
	}
	if ctx.Err() != nil {
		return "", ErrCancelled
	}
	openBrowser(loginURL)
	select {
	case <-ctx.Done():
		if ctx.Err() == context.Canceled {
			return "", ErrCancelled
		}
		return "", ErrTimeout
	case answer := <-result:
		if answer.err != nil {
			return "", answer.err
		}
		return exchange(ctx, origin, answer.ticket, verifier)
	}
}

func checkLoginPage(ctx context.Context, loginURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, loginURL, nil)
	if err != nil {
		return ErrUnavailable
	}
	client := http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return ErrCancelled
		}
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	return nil
}
