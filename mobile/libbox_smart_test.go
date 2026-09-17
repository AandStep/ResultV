package mobile

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStartAdaptiveSmart_ReportsPortAndRuns(t *testing.T) {
	defer StopAdaptiveSmart()
	out, err := StartAdaptiveSmart(t.TempDir(), true)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	var parsed struct {
		Started bool `json:"started"`
		Port    int  `json:"port"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("ответ не JSON: %v (%s)", err, out)
	}
	if !parsed.Started {
		t.Errorf("started = false, ответ %s", out)
	}
	if parsed.Port != SmartRelayPort {
		t.Errorf("port = %d, ожидался %d — на него смотрит аутбаунд конфига", parsed.Port, SmartRelayPort)
	}
	if !AdaptiveSmartRunning() {
		t.Error("AdaptiveSmartRunning() = false сразу после старта")
	}
}

// Повторный старт не поднимает второе реле и не отдаёт «занят порт»: Kotlin
// зовёт это на каждом подключении, включая перезагрузку конфига.
func TestStartAdaptiveSmart_TwiceIsIdempotent(t *testing.T) {
	defer StopAdaptiveSmart()
	dir := t.TempDir()
	if _, err := StartAdaptiveSmart(dir, true); err != nil {
		t.Fatalf("первый старт: %v", err)
	}
	if _, err := StartAdaptiveSmart(dir, true); err != nil {
		t.Fatalf("второй старт: %v", err)
	}
	if !AdaptiveSmartRunning() {
		t.Error("реле не работает после второго старта")
	}
}

func TestStopAdaptiveSmart_WhenNotRunningIsSafe(t *testing.T) {
	StopAdaptiveSmart()
	StopAdaptiveSmart()
	if AdaptiveSmartRunning() {
		t.Error("AdaptiveSmartRunning() = true после двух остановок")
	}
}

func TestStartAdaptiveSmart_EmptyDataDirRefuses(t *testing.T) {
	out, err := StartAdaptiveSmart("  ", false)
	if err == nil {
		t.Fatalf("ожидался отказ на пустом dataDir, получено %q", out)
	}
	if !strings.Contains(err.Error(), "dataDir") {
		t.Errorf("причина отказа не названа: %v", err)
	}
}
