// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package filter

import (
	"context"
	"errors"
	"strings"
)

func humanizeNetError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "отменено"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "превышено время ожидания"
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "context canceled"), strings.Contains(s, "context deadline"):
		return "превышено время ожидания"
	case strings.Contains(s, "timeout"), strings.Contains(s, "i/o timeout"):
		return "превышено время ожидания"
	case strings.Contains(s, "wsarecv"), strings.Contains(s, "connection reset"),
		strings.Contains(s, "connection refused"), strings.Contains(s, "forcibly closed"):
		return "сеть недоступна или хост блокирует загрузку"
	case strings.Contains(s, "no such host"):
		return "не удалось найти сервер (DNS)"
	case strings.Contains(s, "http 403"), strings.Contains(s, "http 404"):
		return "список недоступен на сервере"
	case strings.Contains(s, "http 5"):
		return "ошибка сервера при загрузке"
	case strings.Contains(s, "response too small"):
		return "получен пустой или повреждённый файл"
	default:
		if strings.Contains(s, "wsasend") {
			return "ошибка сети при загрузке"
		}
		if len(s) > 160 {
			return s[:157] + "..."
		}
		return err.Error()
	}
}
