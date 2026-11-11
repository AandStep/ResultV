package main

import (
	"context"
	"errors"
	"fmt"
	"slices"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"resultproxy-wails/internal/config"
	"resultproxy-wails/internal/proxy"
	"resultproxy-wails/internal/telegramconnect"
)

// ConnectTelegramSubscription completes browser login and installs the verified user's subscription.
func (a *App) ConnectTelegramSubscription(language string) (string, error) {
	if a.ctx == nil || a.config == nil {
		return "", telegramconnect.ErrUnavailable
	}
	a.telegramConnectMu.Lock()
	if a.telegramConnectCancel != nil {
		a.telegramConnectMu.Unlock()
		return "", errors.New("telegram_connect_busy")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.telegramConnectCancel = cancel
	a.telegramConnectMu.Unlock()
	defer func() {
		cancel()
		a.telegramConnectMu.Lock()
		a.telegramConnectCancel = nil
		a.telegramConnectMu.Unlock()
	}()

	link, err := telegramconnect.Connect(ctx, telegramconnect.Origin, language, func(loginURL string) {
		wailsRuntime.BrowserOpenURL(a.ctx, loginURL)
	})
	if ctx.Err() != nil {
		return "", telegramconnect.ErrCancelled
	}
	if err != nil {
		return "", err
	}
	a.restoreMainWindow()
	key := telegramconnect.SubscriptionKey(link)
	cfg := a.config.GetConfig()
	for _, sub := range cfg.Subscriptions {
		if telegramconnect.SubscriptionKey(sub.URL) == key && hasUsableSubscriptionEntries(cfg.Proxies, sub.URL) {
			a.dropTelegramAccess()
			return sub.URL, nil
		}
	}
	if _, err := a.addSubscription("ResultV", link, false, "telegram", nil, true); err != nil {
		if err.Error() == "telegram_subscription_unusable" {
			return "", err
		}
		return "", errors.New("telegram_import_failed")
	}
	a.dropTelegramAccess()
	a.refreshTrayProxyList()
	wailsRuntime.EventsEmit(a.ctx, "config:updated", appConfigForUI(a.config.GetConfig()))
	return link, nil
}

// dropTelegramAccess removes the temporary Telegram-only server once the real
// subscription is installed, disconnecting first if the tunnel runs through it.
func (a *App) dropTelegramAccess() {
	cfg := a.config.GetConfig()
	urls := map[string]bool{}
	var ids []string
	for _, sub := range cfg.Subscriptions {
		if telegramconnect.IsAccessSubscription(sub.URL) {
			urls[sub.URL] = true
			ids = append(ids, sub.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	if a.proxy != nil {
		if current := a.proxy.GetStatus().CurrentProxy; current != nil && usesAccessServer(cfg.Proxies, urls, *current) {
			if err := a.Disconnect(); err != nil {
				a.log.Warning(fmt.Sprintf("Не удалось отключить временный сервер Telegram: %v", err))
			}
		}
	}
	for _, id := range ids {
		if err := a.DeleteSubscription(id); err != nil {
			a.log.Warning(fmt.Sprintf("Не удалось удалить временный сервер Telegram: %v", err))
		}
	}
}

func usesAccessServer(entries []config.ProxyEntry, urls map[string]bool, current proxy.ProxyConfig) bool {
	return slices.ContainsFunc(entries, func(entry config.ProxyEntry) bool {
		if !urls[entry.SubscriptionURL] {
			return false
		}
		if current.ID != "" {
			return entry.ID == current.ID
		}
		return entry.IP == current.IP && entry.Port == current.Port
	})
}

// CancelTelegramSubscription cancels the current browser login.
func (a *App) CancelTelegramSubscription() {
	a.telegramConnectMu.Lock()
	defer a.telegramConnectMu.Unlock()
	if a.telegramConnectCancel != nil {
		a.telegramConnectCancel()
	}
}

func hasUsableSubscriptionEntries(entries []config.ProxyEntry, url string) bool {
	return slices.ContainsFunc(entries, func(entry config.ProxyEntry) bool {
		return entry.SubscriptionURL == url && entry.IP != "" && entry.Type != "SECTION"
	})
}
