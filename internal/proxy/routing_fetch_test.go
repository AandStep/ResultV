// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchRoutingPayloadRefusesPrivateTargets(t *testing.T) {
	// httptest слушает на 127.0.0.1 — ровно тот класс адресов, который защита
	// обязана отсечь. Диплинк приходит от атакующего и несёт URL geo-баз,
	// поэтому запрос не должен дойти до сокета.
	reached := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if _, err := FetchRoutingPayload(context.Background(), srv.URL, true); err == nil {
		t.Fatal("загрузка с loopback прошла, ждали отказ")
	}
	if reached {
		t.Error("запрос дошёл до приватного адреса")
	}
}

func TestFetchRoutingPayloadRefusesPlaintextWithoutConsent(t *testing.T) {
	_, err := FetchRoutingPayload(context.Background(), "http://example.com/list.txt", false)
	if err == nil {
		t.Fatal("http:// без согласия принят")
	}
	if !strings.Contains(err.Error(), "insecure") {
		t.Errorf("причина отказа не про plaintext: %v", err)
	}
}

func TestFetchRoutingPayloadRefusesOtherSchemes(t *testing.T) {
	for _, u := range []string{
		"file:///etc/passwd",
		"javascript:alert(1)",
		"ftp://example.com/list.txt",
		"",
		"   ",
	} {
		if _, err := FetchRoutingPayload(context.Background(), u, true); err == nil {
			t.Errorf("схема %q принята", u)
		}
	}
}

func TestFetchRoutingPayloadHonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := FetchRoutingPayload(ctx, "https://example.com/list.txt", false); err == nil {
		t.Fatal("отменённый контекст не остановил загрузку")
	}
}

func TestFetchRoutingPayloadStopsOnClientError(t *testing.T) {
	// 404 не починится повтором. Ждём одну попытку, а не три: лишние два
	// круга — это лишние секунды на каждой сборке профиля.
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := fetchRoutingPayloadVia(context.Background(), srv.Client(), srv.URL, true); err == nil {
		t.Fatal("404 принят как успех")
	}
	if attempts != 1 {
		t.Errorf("попыток %d, ждали 1 — 404 повтором не лечится", attempts)
	}
}

func TestFetchRoutingPayloadRetriesServerError(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write([]byte("example.com\n"))
	}))
	defer srv.Close()

	body, err := fetchRoutingPayloadVia(context.Background(), srv.Client(), srv.URL, true)
	if err != nil {
		t.Fatalf("третья попытка должна была пройти: %v", err)
	}
	if attempts != 3 {
		t.Errorf("попыток %d, ждали 3", attempts)
	}
	if !strings.Contains(string(body), "example.com") {
		t.Errorf("тело не то: %q", body)
	}
}

func TestFetchRoutingPayloadBoundsBodySize(t *testing.T) {
	// Тело больше потолка обрезается, а не съедает память целиком.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := strings.Repeat("a.example\n", 1024) // 10 КиБ
		for i := 0; i < (routingFetchMaxBytes/len(chunk))+2; i++ {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	body, err := fetchRoutingPayloadVia(context.Background(), srv.Client(), srv.URL, true)
	if err != nil {
		t.Fatalf("FetchRoutingPayload: %v", err)
	}
	if len(body) > routingFetchMaxBytes {
		t.Errorf("прочитано %d байт, потолок %d", len(body), routingFetchMaxBytes)
	}
}

// Переписывание github.com/blob в raw должно случаться ДО проверки схемы.
// Это видно без сети: blob-ссылка по http превращается в https, поэтому отказ
// «insecure http:// requires explicit consent» не должен прозвучать — хотя
// вход был именно http и согласия не давали.
func TestFetchRoutingPayloadRewritesBlobURLBeforeSchemeCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // сеть не нужна: нас интересует, до какой проверки дошло

	_, err := FetchRoutingPayload(ctx, "http://github.com/o/r/blob/main/x.lst", false)
	if err == nil {
		t.Fatal("ждали ошибку от отменённого контекста")
	}
	if strings.Contains(err.Error(), "insecure") {
		t.Errorf("blob-ссылка не переписана в https до проверки схемы: %v", err)
	}

	// А обычный http, который переписывать не во что, отвергается как раньше.
	_, err = FetchRoutingPayload(ctx, "http://example.com/x.lst", false)
	if err == nil || !strings.Contains(err.Error(), "insecure") {
		t.Errorf("обычный http прошёл без согласия: %v", err)
	}
}
