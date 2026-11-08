package main

import (
	"errors"
	"slices"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/telegramconnect"
)

// ConnectTelegramSubscription completes browser login and installs the verified user's subscription.
func (a *App) ConnectTelegramSubscription(language string) (string, error) {
	if a.ctx == nil || a.config == nil {
		return "", telegramconnect.ErrUnavailable
	}
	if !a.telegramConnectMu.TryLock() {
		return "", errors.New("telegram_connect_busy")
	}
	defer a.telegramConnectMu.Unlock()

	link, err := telegramconnect.Connect(a.ctx, telegramconnect.Origin, language, func(loginURL string) {
		wailsRuntime.BrowserOpenURL(a.ctx, loginURL)
	})
	a.restoreMainWindow()
	if err != nil {
		return "", err
	}

	key := telegramconnect.SubscriptionKey(link)
	cfg := a.config.GetConfig()
	for _, sub := range cfg.Subscriptions {
		if telegramconnect.SubscriptionKey(sub.URL) == key && hasUsableSubscriptionEntries(cfg.Proxies, sub.URL) {
			return sub.URL, nil
		}
	}
	if _, err := a.addSubscription("ResultV", link, false, "telegram", nil, true); err != nil {
		if err.Error() == "telegram_subscription_unusable" {
			return "", err
		}
		return "", errors.New("telegram_import_failed")
	}
	a.refreshTrayProxyList()
	wailsRuntime.EventsEmit(a.ctx, "config:updated", appConfigForUI(a.config.GetConfig()))
	return link, nil
}

func hasUsableSubscriptionEntries(entries []config.ProxyEntry, url string) bool {
	return slices.ContainsFunc(entries, func(entry config.ProxyEntry) bool {
		return entry.SubscriptionURL == url && entry.IP != "" && entry.Type != "SECTION"
	})
}
