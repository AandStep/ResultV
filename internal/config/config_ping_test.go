// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package config

import (
	"testing"
	"time"
)

// Пустой тип — это «авто», а не ошибка: так читается любой конфиг, написанный
// до появления поля.
func TestEffectivePingTypeFallsBackToAuto(t *testing.T) {
	for _, raw := range []string{"", "  ", "нечто"} {
		if got := (AppSettings{PingType: raw}).EffectivePingType(); got != PingTypeAuto {
			t.Errorf("EffectivePingType(%q) = %q, ожидалось %q", raw, got, PingTypeAuto)
		}
	}
	if got := (AppSettings{PingType: PingTypeHTTPHead}).EffectivePingType(); got != PingTypeHTTPHead {
		t.Errorf("заданный тип потерялся: %q", got)
	}
}

// HTTPS — требование корректности, а не вкуса: по plain-HTTP запрос идёт через
// наш же петлевой инбаунд, который на мёртвый узел отвечает собственным 502,
// и мёртвый узел прочитался бы как живой.
func TestValidatePingTestURL(t *testing.T) {
	for _, raw := range []string{
		"",
		"   ",
		"http://example.com/generate_204",
		"https://",
		"://",
	} {
		if err := ValidatePingTestURL(raw); err == nil {
			t.Errorf("%q должен быть отклонён", raw)
		}
	}
	if err := ValidatePingTestURL(DefaultPingTestURL); err != nil {
		t.Errorf("дефолтный адрес должен проходить: %v", err)
	}
}

// Границы: ноль значит «дефолт», а не «не ждать вовсе»; потолок не даёт одному
// мёртвому узлу растянуть весь обход списка. Значение вне диапазона зажимается
// к границе, а не сбрасывается в дефолт — 11 у человека означает «подольше», и
// три секунды были бы ответом не на его вопрос.
func TestEffectivePingTimeout(t *testing.T) {
	cases := map[int]time.Duration{
		0:    3 * time.Second,
		-5:   3 * time.Second,
		1:    1 * time.Second,
		3:    3 * time.Second,
		10:   10 * time.Second,
		11:   10 * time.Second,
		1000: 10 * time.Second,
	}
	for in, want := range cases {
		if got := (AppSettings{PingTimeoutSec: in}).EffectivePingTimeout(); got != want {
			t.Errorf("EffectivePingTimeout(%d) = %v, ожидалось %v", in, got, want)
		}
	}
}

// Пустой адрес читается как дефолтный — иначе http-тип у свежего конфига просто
// не работал бы.
func TestEffectivePingTestURL(t *testing.T) {
	if got := (AppSettings{}).EffectivePingTestURL(); got != DefaultPingTestURL {
		t.Errorf("пустой адрес = %q, ожидался дефолт", got)
	}
	const custom = "https://example.org/ping"
	if got := (AppSettings{PingTestURL: custom}).EffectivePingTestURL(); got != custom {
		t.Errorf("заданный адрес потерялся: %q", got)
	}
	// Негодный адрес не должен становиться тихим отказом на стороне движка:
	// читатель возвращает дефолт, а несогласие ловит валидация в UI.
	if got := (AppSettings{PingTestURL: "http://insecure"}).EffectivePingTestURL(); got != DefaultPingTestURL {
		t.Errorf("негодный адрес = %q, ожидался дефолт", got)
	}
}
