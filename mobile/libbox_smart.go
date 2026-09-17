// SPDX-License-Identifier: GPL-3.0-or-later
// Proprietary license and copyright notice at https://vpncore.org/legal

package mobile

import (
	"fmt"
	"strings"
	"sync"

	"resultproxy-wails/internal/proxy"
)

var (
	smartRelayMu sync.Mutex
	smartRelay   *proxy.SmartRelay
)

// StartAdaptiveSmart поднимает реле адаптивного Smart на loopback.
//
// Зовётся ПЕРЕД стартом движка и на том же флаге, на котором собран конфиг:
// аутбаунд smart-relay, за которым никто не слушает, проглотит весь трафик,
// не покрытый другими правилами.
//
// Повторный вызов ничего не делает: Kotlin зовёт это на каждом подключении, а
// перезагрузка конфига не повод терять выученное этой сессией.
//
// Возвращает JSON {"started":true,"port":<порт>} — тот же контракт
// «JSON или ошибка», что у остальных биндингов этого файла.
func StartAdaptiveSmart(dataDir string, memoryOnly bool) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("dataDir is required")
	}
	smartRelayMu.Lock()
	defer smartRelayMu.Unlock()
	if smartRelay != nil {
		return fmt.Sprintf(`{"started":true,"port":%d}`, smartRelay.Port()), nil
	}
	relay, err := proxy.StartSmartRelay(proxy.SmartRelayOptions{
		DataDir:    dataDir,
		ListenPort: SmartRelayPort,
		TunnelPort: SmartRaceInboundPort,
		MemoryOnly: memoryOnly,
	})
	if err != nil {
		return "", err
	}
	smartRelay = relay
	return fmt.Sprintf(`{"started":true,"port":%d}`, relay.Port()), nil
}

// StopAdaptiveSmart гасит реле и сохраняет выученное. Безопасно звать, когда
// оно не поднято.
func StopAdaptiveSmart() {
	smartRelayMu.Lock()
	relay := smartRelay
	smartRelay = nil
	smartRelayMu.Unlock()
	if relay != nil {
		_ = relay.Close()
	}
}

// AdaptiveSmartRunning — поднято ли реле. Нужно Kotlin для строки в логе и
// тестам, чтобы не гадать по порту.
func AdaptiveSmartRunning() bool {
	smartRelayMu.Lock()
	defer smartRelayMu.Unlock()
	return smartRelay != nil
}
