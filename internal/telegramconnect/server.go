package telegramconnect

import (
	"context"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"sync"
	"time"
)

//go:embed login.html
var loginHTML string

var loginTemplate = template.Must(template.New("login").Parse(loginHTML))

const ticketLifetime = time.Minute

type loginSession struct {
	challenge string
	expires   time.Time
}

type loginTicket struct {
	loginSession
	userID int64
}

type loginServer struct {
	origin   string
	clientID int64
	verify   func(context.Context, string, string) (int64, error)
	lookup   func(context.Context, int64) (string, error)

	mu       sync.Mutex
	sessions map[string]loginSession
	tickets  map[string]loginTicket
}

// NewHandler serves only the desktop login and one-time ticket endpoints.
func NewHandler(origin string, clientID int64,
	verify func(context.Context, string, string) (int64, error),
	lookup func(context.Context, int64) (string, error),
) http.Handler {
	server := &loginServer{
		origin: origin, clientID: clientID, verify: verify, lookup: lookup,
		sessions: make(map[string]loginSession), tickets: make(map[string]loginTicket),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+Prefix+"/login", server.login)
	mux.HandleFunc("POST "+Prefix+"/verify", server.verifyLogin)
	mux.HandleFunc("POST "+Prefix+"/exchange", server.exchangeTicket)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}

func (s *loginServer) login(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	port, err := strconv.Atoi(query.Get("port"))
	state := query.Get("challenge")
	if err != nil || port < 1024 || port > 65535 || !validCode(state) {
		http.Error(w, "Start Telegram login from ResultV.", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	now := time.Now()
	nonce := newCode()
	s.mu.Lock()
	for key, session := range s.sessions {
		if !session.expires.After(now) {
			delete(s.sessions, key)
		}
	}
	for key, ticket := range s.tickets {
		if !ticket.expires.After(now) {
			delete(s.tickets, key)
		}
	}
	if len(s.sessions)+len(s.tickets) >= 2048 {
		s.mu.Unlock()
		http.Error(w, "Try again later.", http.StatusServiceUnavailable)
		return
	}
	s.sessions[nonce] = loginSession{challenge: state, expires: now.Add(LoginTimeout)}
	s.mu.Unlock()

	lang := "en"
	if query.Get("lang") == "ru" {
		lang = "ru"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'nonce-"+nonce+"'; script-src 'nonce-"+nonce+"' https://oauth.telegram.org; connect-src 'self' https://oauth.telegram.org; frame-src https://oauth.telegram.org; frame-ancestors 'none'; base-uri 'none'")
	_ = loginTemplate.Execute(w, struct {
		ClientID int64
		Port     int
		Nonce    string
		State    string
		Language string
	}{s.clientID, port, nonce, state, lang})
}

func (s *loginServer) verifyLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Nonce   string `json:"nonce"`
		IDToken string `json:"idToken"`
	}
	if r.Header.Get("Origin") != s.origin ||
		decodeJSON(r.Body, &input) != nil || !validCode(input.Nonce) || input.IDToken == "" {
		respondError(w, http.StatusUnauthorized, ErrAuth)
		return
	}
	s.mu.Lock()
	session, found := s.sessions[input.Nonce]
	s.mu.Unlock()
	if !found || !session.expires.After(time.Now()) {
		respondError(w, http.StatusUnauthorized, ErrAuth)
		return
	}
	userID, err := s.verify(r.Context(), input.IDToken, input.Nonce)
	if err != nil || userID <= 0 {
		respondError(w, http.StatusUnauthorized, ErrAuth)
		return
	}
	ticket := newCode()
	s.mu.Lock()
	_, found = s.sessions[input.Nonce]
	now := time.Now()
	if !found || !session.expires.After(now) {
		s.mu.Unlock()
		respondError(w, http.StatusUnauthorized, ErrAuth)
		return
	}
	delete(s.sessions, input.Nonce)
	s.tickets[ticket] = loginTicket{loginSession: loginSession{
		challenge: session.challenge, expires: now.Add(ticketLifetime),
	}, userID: userID}
	s.mu.Unlock()
	time.AfterFunc(ticketLifetime, func() {
		s.mu.Lock()
		delete(s.tickets, ticket)
		s.mu.Unlock()
	})
	respondJSON(w, http.StatusOK, map[string]string{"ticket": ticket})
}

func (s *loginServer) exchangeTicket(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Ticket   string `json:"ticket"`
		Verifier string `json:"verifier"`
	}
	if decodeJSON(r.Body, &input) != nil || !validCode(input.Ticket) || !validCode(input.Verifier) {
		respondError(w, http.StatusUnauthorized, ErrAuth)
		return
	}
	s.mu.Lock()
	ticket, found := s.tickets[input.Ticket]
	if !found || !ticket.expires.After(time.Now()) ||
		subtle.ConstantTimeCompare([]byte(ticket.challenge), []byte(challenge(input.Verifier))) != 1 {
		s.mu.Unlock()
		respondError(w, http.StatusUnauthorized, ErrAuth)
		return
	}
	delete(s.tickets, input.Ticket)
	s.mu.Unlock()

	link, err := s.lookup(r.Context(), ticket.userID)
	switch {
	case errors.Is(err, ErrNotFound):
		respondError(w, http.StatusNotFound, ErrNotFound)
	case errors.Is(err, ErrInactive):
		respondError(w, http.StatusForbidden, ErrInactive)
	case errors.Is(err, ErrAmbiguous):
		respondError(w, http.StatusConflict, ErrAmbiguous)
	case err != nil || SubscriptionKey(link) == "":
		respondError(w, http.StatusBadGateway, ErrUnavailable)
	default:
		respondJSON(w, http.StatusOK, exchangeResponse{SubscriptionURL: link})
	}
}

func respondError(w http.ResponseWriter, status int, err error) {
	respondJSON(w, status, exchangeResponse{Error: err.Error()})
}

func respondJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
