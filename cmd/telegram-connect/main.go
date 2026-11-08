// Command telegram-connect serves the browser sign-in bridge for ResultV Desktop.
package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"resultproxy-wails/internal/telegramconnect"
)

func main() {
	clientID, err := strconv.ParseInt(os.Getenv("TELEGRAM_CLIENT_ID"), 10, 64)
	baseURL := strings.TrimRight(os.Getenv("REMNAWAVE_URL"), "/")
	tokenFile, tokenErr := os.ReadFile(os.Getenv("REMNAWAVE_READ_TOKEN_FILE"))
	token := strings.TrimSpace(string(tokenFile))
	if err != nil || clientID <= 0 || baseURL == "" || tokenErr != nil || token == "" {
		log.Fatal("Telegram Connect configuration is incomplete")
	}
	server := &http.Server{
		Addr: ":8090",
		Handler: telegramconnect.NewHandler(telegramconnect.Origin, clientID,
			telegramconnect.TelegramVerifier(clientID), telegramconnect.RemnawaveLookup(baseURL, token)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	log.Fatal(server.ListenAndServe())
}
