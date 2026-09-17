// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"testing"
	"time"
)

// Прежние вызывающие не передают бюджет и должны получить ровно то поведение,
// что было: две секунды.
func TestPingICMPDefaultTimeoutUnchanged(t *testing.T) {
	if pingICMPHostDefaultTimeout != 2*time.Second {
		t.Errorf("дефолтный бюджет ICMP = %v, был 2s", pingICMPHostDefaultTimeout)
	}
}

// Заданный бюджет обязан ограничивать пробу: 192.0.2.1 — TEST-NET-1, он не
// отвечает никогда, поэтому проба должна вернуться по своему сроку, а не по
// чужому.
func TestPingICMPHonoursTimeout(t *testing.T) {
	start := time.Now()
	if _, ok := pingICMPHostTimeout("192.0.2.1", "", 300*time.Millisecond); ok {
		t.Skip("TEST-NET-1 неожиданно ответил — сеть подменяет ICMP, проверять нечего")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("проба заняла %v при бюджете 300ms", elapsed)
	}
}
