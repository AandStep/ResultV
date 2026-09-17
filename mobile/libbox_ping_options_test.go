// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package mobile

import (
	"testing"
	"time"

	"resultproxy-wails/internal/config"
)

// Пустая строка настроек — обычный случай, а не ошибка: так выглядит вызов из
// сборки, где настройки ещё не заведены.
func TestDecodePingOptionsDefaults(t *testing.T) {
	for _, raw := range []string{"", "   ", "не json", "{}"} {
		got := decodePingOptions(raw)
		if got.Type != config.PingTypeAuto {
			t.Errorf("%q: тип = %q", raw, got.Type)
		}
		if got.TestURL != config.DefaultPingTestURL {
			t.Errorf("%q: адрес = %q", raw, got.TestURL)
		}
		if got.Timeout != 3*time.Second {
			t.Errorf("%q: таймаут = %v", raw, got.Timeout)
		}
	}
}

// Ключи JSON совпадают с именами в общем конфиге — иначе один и тот же
// параметр назывался бы в двух местах по-разному.
func TestDecodePingOptionsReadsSettings(t *testing.T) {
	got := decodePingOptions(`{"pingType":"http_head","pingTestUrl":"https://example.org/ping","pingTimeoutSec":7}`)
	if got.Type != config.PingTypeHTTPHead {
		t.Errorf("тип = %q", got.Type)
	}
	if got.TestURL != "https://example.org/ping" {
		t.Errorf("адрес = %q", got.TestURL)
	}
	if got.Timeout != 7*time.Second {
		t.Errorf("таймаут = %v", got.Timeout)
	}
}

// Негодные значения не должны доезжать до движка: адрес по plain-HTTP
// превратил бы мёртвый узел в живой, а таймаут вне границ — в зависший обход.
func TestDecodePingOptionsSanitises(t *testing.T) {
	got := decodePingOptions(`{"pingType":"нечто","pingTestUrl":"http://insecure","pingTimeoutSec":99}`)
	if got.Type != config.PingTypeAuto {
		t.Errorf("тип = %q", got.Type)
	}
	if got.TestURL != config.DefaultPingTestURL {
		t.Errorf("адрес = %q", got.TestURL)
	}
	if got.Timeout != 10*time.Second {
		t.Errorf("таймаут = %v", got.Timeout)
	}
}
