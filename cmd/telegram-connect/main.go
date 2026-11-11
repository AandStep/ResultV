// Command telegram-connect serves the browser sign-in bridge for ResultV Desktop
// and the subscription gateway that adds temporary Telegram access to expired subscriptions.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"resultproxy-wails/internal/telegramconnect"
)

func readSecret(env string) string {
	path := os.Getenv(env)
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func main() {
	clientID, err := strconv.ParseInt(os.Getenv("TELEGRAM_CLIENT_ID"), 10, 64)
	baseURL := strings.TrimRight(os.Getenv("REMNAWAVE_URL"), "/")
	token := readSecret("REMNAWAVE_READ_TOKEN_FILE")
	if err != nil || clientID <= 0 || baseURL == "" || token == "" {
		log.Fatal("Telegram Connect configuration is incomplete")
	}
	subscriptionURL := os.Getenv("REMNAWAVE_SUBSCRIPTION_URL")
	if subscriptionURL == "" {
		subscriptionURL = "http://remnawave:3000"
	}

	port, _ := strconv.Atoi(os.Getenv("ACCESS_PORT"))
	access := telegramconnect.NewAccessIssuer(telegramconnect.AccessConfig{
		RemnawaveURL: baseURL,
		Token:        readSecret("ACCESS_TOKEN_FILE"),
		Squad:        os.Getenv("ACCESS_SQUAD"),
		Address:      os.Getenv("ACCESS_ADDRESS"),
		Port:         port,
		ServerName:   os.Getenv("ACCESS_SERVER_NAME"),
		PublicKey:    os.Getenv("ACCESS_PUBLIC_KEY"),
		ShortID:      os.Getenv("ACCESS_SHORT_ID"),
	})
	if access != nil {
		go access.RunSweeper(context.Background(), func(err error) { log.Printf("access sweep: %v", err) })
		log.Print("temporary Telegram access is enabled")
	} else {
		log.Print("temporary Telegram access is disabled")
	}
	gateway, err := telegramconnect.NewSubscriptionGateway(subscriptionURL, access)
	if err != nil {
		log.Fatal("Subscription gateway configuration is invalid")
	}

	mux := http.NewServeMux()
	mux.Handle("/api/sub/", gateway)
	mux.Handle("/", telegramconnect.NewHandler(telegramconnect.Origin, clientID,
		telegramconnect.TelegramVerifier(clientID), telegramconnect.RemnawaveLookup(baseURL, token), access))

	server := &http.Server{
		Addr:              ":8090",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      40 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	log.Fatal(server.ListenAndServe())
}
