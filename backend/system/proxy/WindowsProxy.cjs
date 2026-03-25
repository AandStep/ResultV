/*
 * Copyright (C) 2026 ResultProxy
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

const BaseProxyManager = require("./BaseProxyManager.cjs");
const { getSafeOSWhitelist } = require("../../utils/domain.cjs");

const fs = require("fs");
const path = require("path");
const util = require("util");
const { execFile, execSync } = require("child_process");
const execFileAsync = util.promisify(execFile);

const FIREWALL_RULE_NAME = "ResultProxy_KillSwitch";

// Ветки реестра, в которых GPO хранит прокси-настройки (имеют приоритет над HKCU\Internet Settings)
const GPO_REG_PATHS = [
  "HKCU\\Software\\Policies\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
  "HKLM\\Software\\Policies\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
];
// Значения реестра, которые GPO использует для управления прокси
const GPO_VALUE_NAMES = ["ProxyEnable", "ProxyServer", "ProxyOverride", "AutoConfigURL", "ProxySettingsPerUser"];

class WindowsProxy extends BaseProxyManager {
  constructor(loggerService) {
    super(loggerService);
    this._gpoBackupPath = null; // Устанавливается через setUserDataPath()
    this._gpoWasDisabled = false;
  }

  /**
   * Устанавливает путь для хранения backup GPO-настроек.
   * Вызывается из system.factory при инициализации.
   */
  setUserDataPath(userDataPath) {
    this._gpoBackupPath = path.join(userDataPath, "gpo_backup.json");
  }

  // ─── GPO Detection ───────────────────────────────────────

  /**
   * Считывает значение из реестра. Возвращает { type, value } или null.
   */
  async _readRegValue(regPath, valueName) {
    try {
      const { stdout } = await execFileAsync("reg", [
        "query", regPath, "/v", valueName,
      ]);
      // Парсим вывод reg query: "    ValueName    REG_TYPE    Data"
      const lines = stdout.split("\n").map(l => l.trim()).filter(l => l.length > 0);
      for (const line of lines) {
        const match = line.match(new RegExp(`^${valueName}\\s+(REG_\\w+)\\s+(.*)$`, "i"));
        if (match) {
          return { type: match[1], value: match[2].trim() };
        }
      }
    } catch (e) {
      // Значение не существует — это нормально
    }
    return null;
  }

  /**
   * Обнаруживает GPO-настройки прокси в ветках Policies.
   * Возвращает объект { hasGpo, entries: [...] } где entries — массив обнаруженных записей.
   */
  async _detectGpoProxy() {
    const entries = [];

    for (const regPath of GPO_REG_PATHS) {
      for (const valueName of GPO_VALUE_NAMES) {
        const result = await this._readRegValue(regPath, valueName);
        if (result) {
          entries.push({
            regPath,
            valueName,
            regType: result.type,
            regValue: result.value,
          });
        }
      }
    }

    return { hasGpo: entries.length > 0, entries };
  }

  /**
   * Сохраняет GPO-настройки в backup-файл и удаляет GPO-записи из реестра.
   * Возвращает true если GPO были обнаружены и нейтрализованы.
   */
  async _backupAndDisableGpo() {
    const detection = await this._detectGpoProxy();
    if (!detection.hasGpo) return false;

    // Сохраняем backup
    if (this._gpoBackupPath) {
      try {
        const dir = path.dirname(this._gpoBackupPath);
        if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
        fs.writeFileSync(this._gpoBackupPath, JSON.stringify(detection.entries, null, 2));
      } catch (e) {
        this.log(`[GPO] Ошибка сохранения backup: ${e.message}`, "warning");
      }
    }

    // Удаляем GPO-записи
    let removedCount = 0;
    for (const entry of detection.entries) {
      try {
        await execFileAsync("reg", [
          "delete", entry.regPath, "/v", entry.valueName, "/f",
        ]);
        removedCount++;
      } catch (e) {
        // HKLM может потребовать админ-прав
        this.log(
          `[GPO] Не удалось удалить ${entry.valueName} из ${entry.regPath}: ${e.message}`,
          "warning",
        );
      }
    }

    if (removedCount > 0) {
      this._gpoWasDisabled = true;
      this.log(
        `[GPO] Обнаружены и временно деактивированы настройки прокси из групповых политик (${removedCount} записей).`,
        "warning",
      );
    }

    return removedCount > 0;
  }

  /**
   * Восстанавливает GPO-настройки из backup-файла.
   */
  async _restoreGpo() {
    if (!this._gpoBackupPath || !fs.existsSync(this._gpoBackupPath)) return;

    try {
      const raw = fs.readFileSync(this._gpoBackupPath, "utf8");
      const entries = JSON.parse(raw);

      for (const entry of entries) {
        try {
          await execFileAsync("reg", [
            "add", entry.regPath,
            "/v", entry.valueName,
            "/t", entry.regType,
            "/d", entry.regValue,
            "/f",
          ]);
        } catch (e) {
          this.log(
            `[GPO] Не удалось восстановить ${entry.valueName} в ${entry.regPath}: ${e.message}`,
            "warning",
          );
        }
      }

      // Удаляем файл backup после успешного восстановления
      try { fs.unlinkSync(this._gpoBackupPath); } catch (e) {}
      this._gpoWasDisabled = false;
      this.log("[GPO] Настройки групповых политик восстановлены.", "info");
    } catch (e) {
      this.log(`[GPO] Ошибка восстановления backup: ${e.message}`, "error");
    }
  }

  /**
   * Синхронное восстановление GPO (для использования при завершении процесса).
   */
  _restoreGpoSync() {
    if (!this._gpoBackupPath || !fs.existsSync(this._gpoBackupPath)) return;

    try {
      const raw = fs.readFileSync(this._gpoBackupPath, "utf8");
      const entries = JSON.parse(raw);

      for (const entry of entries) {
        try {
          execSync(
            `reg add "${entry.regPath}" /v ${entry.valueName} /t ${entry.regType} /d "${entry.regValue}" /f`,
            { stdio: "ignore" },
          );
        } catch (e) {
          // Игнорируем — при синхронном закрытии лучше не крашить процесс
        }
      }

      try { fs.unlinkSync(this._gpoBackupPath); } catch (e) {}
      this._gpoWasDisabled = false;
    } catch (e) {
      // Игнорируем ошибки при синхронном закрытии
    }
  }

  /**
   * Статический метод для восстановления GPO при запуске (аварийное завершение).
   * Вызывается из electron-main.cjs.
   */
  static restoreGpoOnStartup(userDataPath, logCallback) {
    const backupPath = path.join(userDataPath, "gpo_backup.json");
    if (!fs.existsSync(backupPath)) return false;

    if (logCallback) logCallback("[GPO] Обнаружен backup GPO от предыдущего сеанса. Восстановление...", "warning");

    try {
      const raw = fs.readFileSync(backupPath, "utf8");
      const entries = JSON.parse(raw);

      for (const entry of entries) {
        try {
          execSync(
            `reg add "${entry.regPath}" /v ${entry.valueName} /t ${entry.regType} /d "${entry.regValue}" /f`,
            { stdio: "ignore" },
          );
        } catch (e) {
          if (logCallback) logCallback(`[GPO] Не удалось восстановить ${entry.valueName}: ${e.message}`, "warning");
        }
      }

      fs.unlinkSync(backupPath);
      if (logCallback) logCallback("[GPO] Настройки групповых политик успешно восстановлены из backup.", "success");
      return true;
    } catch (e) {
      if (logCallback) logCallback(`[GPO] Ошибка восстановления backup: ${e.message}`, "error");
      return false;
    }
  }

  // ─── Core Proxy Methods ───────────────────────────────────

  formatBypassList(whitelist) {
    let override = "<local>";
    if (whitelist && whitelist.length > 0) {
      const safeList = getSafeOSWhitelist(whitelist);
      const bypassStr = safeList.map((d) => `*.${d};*${d}*`).join(";");
      override = `${bypassStr};<local>`;
    }
    return override;
  }

  async setSystemProxy(proxyIp, proxyPort, proxyType, whitelist) {
    // Обнаруживаем и нейтрализуем GPO-настройки прокси перед установкой
    let gpoConflict = false;
    try {
      gpoConflict = await this._backupAndDisableGpo();
    } catch (e) {
      this.log(`[GPO] Ошибка при обработке GPO: ${e.message}`, "warning");
    }

    let proxyStr = "";
    if (proxyType === "SOCKS5") {
      proxyStr = `socks=${proxyIp}:${proxyPort}`;
    } else if (proxyType === "ALL") {
      proxyStr = `${proxyIp}:${proxyPort}`;
    } else {
      proxyStr = `http=${proxyIp}:${proxyPort};https=${proxyIp}:${proxyPort}`;
    }

    const override = this.formatBypassList(whitelist);

    try {
      await execFileAsync("reg", [
        "add",
        "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
        "/v",
        "ProxyEnable",
        "/t",
        "REG_DWORD",
        "/d",
        "1",
        "/f",
      ]);
      await execFileAsync("reg", [
        "add",
        "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
        "/v",
        "ProxyServer",
        "/t",
        "REG_SZ",
        "/d",
        proxyStr,
        "/f",
      ]);
      await execFileAsync("reg", [
        "add",
        "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
        "/v",
        "ProxyOverride",
        "/t",
        "REG_SZ",
        "/d",
        override,
        "/f",
      ]);
      try {
        await execFileAsync("reg", [
          "delete",
          "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
          "/v",
          "AutoConfigURL",
          "/f",
        ]);
      } catch (e) {} // Игнорируем, если ключа нет
      await execFileAsync("ipconfig", ["/flushdns"]);

      this.log(`[СИСТЕМА] Прокси применен к Windows успешно.`, "success");
    } catch (error) {
      this.log(
        `[ОШИБКА СИСТЕМЫ] Ошибка установки прокси: ${error.message}`,
        "error",
      );
      throw error;
    }

    return { gpoConflict };
  }

  async disableSystemProxy() {
    this.log("[СИСТЕМА] Очистка настроек прокси из реестра Windows...", "info");

    // Всегда снимаем правила файрвола kill switch
    await this.removeKillSwitchFirewall();

    try {
      await execFileAsync("reg", [
        "add",
        "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
        "/v",
        "ProxyEnable",
        "/t",
        "REG_DWORD",
        "/d",
        "0",
        "/f",
      ]);
      try {
        await execFileAsync("reg", [
          "delete",
          "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
          "/v",
          "ProxyServer",
          "/f",
        ]);
      } catch (e) {}
      try {
        await execFileAsync("reg", [
          "delete",
          "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
          "/v",
          "ProxyOverride",
          "/f",
        ]);
      } catch (e) {}
      try {
        await execFileAsync("reg", [
          "delete",
          "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
          "/v",
          "AutoConfigURL",
          "/f",
        ]);
      } catch (e) {}
      await execFileAsync("ipconfig", ["/flushdns"]);
    } catch (error) {
      this.log(
        `[ОШИБКА СИСТЕМЫ] Ошибка очистки прокси: ${error.message}`,
        "error",
      );
    }

    // Восстанавливаем GPO-настройки после снятия прокси
    try {
      await this._restoreGpo();
    } catch (e) {
      this.log(`[GPO] Ошибка восстановления GPO: ${e.message}`, "warning");
    }
  }

  disableSystemProxySync() {
    // Файрвольные правила очищаются ТОЛЬКО через async removeKillSwitchFirewall()
    // Не вызываем netsh advfirewall здесь — синхронный вызов может показать интерактивный промпт

    try {
      execSync(
        'reg add "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyEnable /t REG_DWORD /d 0 /f',
        { stdio: "ignore" },
      );
      execSync(
        'reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyServer /f',
        { stdio: "ignore" },
      );
      execSync(
        'reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings" /v ProxyOverride /f',
        { stdio: "ignore" },
      );
    } catch (e) {
      // Игнорируем ошибки при синхронном закрытии
    }

    // Синхронно восстанавливаем GPO-настройки
    this._restoreGpoSync();
  }

  async applyKillSwitch() {
    this.log(
      "[KILL SWITCH] Активирована полная блокировка интернета!",
      "error",
    );

    // Основной механизм: файрвол Windows (блокирует ВСЁ, включая приложения, не использующие системный прокси)
    let firewallApplied = false;
    try {
      // Удаляем старые правила, если остались
      try {
        await execFileAsync("netsh", [
          "advfirewall",
          "firewall",
          "delete",
          "rule",
          `name=${FIREWALL_RULE_NAME}`,
        ]);
      } catch (e) {}

      // Блокировка всего исходящего трафика
      await execFileAsync("netsh", [
        "advfirewall",
        "firewall",
        "add",
        "rule",
        `name=${FIREWALL_RULE_NAME}`,
        "dir=out",
        "action=block",
        "enable=yes",
        "profile=any",
        "protocol=any",
      ]);
      // Блокировка всего входящего трафика
      await execFileAsync("netsh", [
        "advfirewall",
        "firewall",
        "add",
        "rule",
        `name=${FIREWALL_RULE_NAME}_in`,
        "dir=in",
        "action=block",
        "enable=yes",
        "profile=any",
        "protocol=any",
      ]);

      firewallApplied = true;
      this.log("[KILL SWITCH] Файрвол Windows активирован.", "error");
    } catch (error) {
      this.log(
        `[KILL SWITCH] Не удалось применить файрвол (нет прав администратора?): ${error.message}. Используем fallback.`,
        "warning",
      );
    }

    // Fallback: мёртвый прокси через реестр
    try {
      await execFileAsync("reg", [
        "add",
        "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
        "/v",
        "ProxyEnable",
        "/t",
        "REG_DWORD",
        "/d",
        "1",
        "/f",
      ]);
      await execFileAsync("reg", [
        "add",
        "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
        "/v",
        "ProxyServer",
        "/t",
        "REG_SZ",
        "/d",
        "127.0.0.1:65535",
        "/f",
      ]);
      try {
        await execFileAsync("reg", [
          "delete",
          "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
          "/v",
          "ProxyOverride",
          "/f",
        ]);
      } catch (e) {}
      await execFileAsync("ipconfig", ["/flushdns"]);
    } catch (error) {
      this.log(`[ОШИБКА KILL SWITCH] ${error.message}`, "error");
    }
  }

  async removeKillSwitchFirewall() {
    try {
      await execFileAsync("netsh", [
        "advfirewall",
        "firewall",
        "delete",
        "rule",
        `name=${FIREWALL_RULE_NAME}`,
      ]);
    } catch (e) {} // Правило могло не существовать
    try {
      await execFileAsync("netsh", [
        "advfirewall",
        "firewall",
        "delete",
        "rule",
        `name=${FIREWALL_RULE_NAME}_in`,
      ]);
    } catch (e) {}
  }
}

module.exports = WindowsProxy;
