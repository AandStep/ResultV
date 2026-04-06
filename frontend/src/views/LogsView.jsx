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

import React from "react";
import { useLogContext } from "../context/LogContext";
import { useTranslation } from "react-i18next";

const translateLog = (msg, t) => {
  if (msg.includes("Интерфейс запущен. Загрузка конфигурации..."))
    return t("logs.msg.app_started");
  if (msg.includes("Служба недоступна.")) return t("logs.msg.daemon_offline");
  if (msg.includes("Отключено успешно.")) return t("logs.msg.disconnected");
  if (msg.includes("Отключение...")) return t("logs.msg.disconnecting");
  if (msg.startsWith("Подключение к "))
    return msg.replace("Подключение к", t("logs.msg.connecting_to"));
  if (msg.includes("Соединение установлено.")) return t("logs.msg.connected");
  if (msg.startsWith("Сбой подключения: "))
    return msg.replace("Сбой подключения:", t("logs.msg.conn_failed"));
  if (msg.startsWith("Сбой: "))
    return msg.replace("Сбой:", t("logs.msg.error"));
  if (msg.startsWith("Успешно переключено на "))
    return msg.replace("Успешно переключено на", t("logs.msg.switched_to"));
  if (msg.startsWith("Переключение на: "))
    return msg.replace("Переключение на:", t("logs.msg.switching_to"));
  if (msg.includes("Активный сервер удален. Разрыв соединения..."))
    return t("logs.msg.active_deleted");

  if (msg.startsWith("Внимание: Узел "))
    return msg
      .replace("Внимание: Узел", t("logs.msg.node_dead"))
      .replace("перестал отвечать!", t("logs.msg.stopped_responding"));
  if (msg.includes("Связь с узлом восстановлена."))
    return t("logs.msg.node_restored");

  if (msg.includes("--- НОВЫЙ ЗАПРОС НА ПОДКЛЮЧЕНИЕ ---"))
    return t("logs.msg.new_conn_request");
  if (msg.startsWith("Ошибка подключения: "))
    return msg.replace("Ошибка подключения:", t("logs.msg.backend_conn_error"));
  if (msg.includes("--- ЗАПРОС НА ОТКЛЮЧЕНИЕ ---"))
    return t("logs.msg.disconnect_request");
  if (msg.startsWith("Ошибка отключения: "))
    return msg.replace(
      "Ошибка отключения:",
      t("logs.msg.backend_disconn_error"),
    );
  if (msg.includes("[KILL SWITCH] Отключен вручную. Снимаем блокировку."))
    return t("logs.msg.killswitch_manual_off");

  if (typeof msg === "string") {
    if (msg.startsWith("[ПРОКСИ] ")) {
      return msg.replace("[ПРОКСИ]", t("logs.msg.proxy_prefix"));
    }
    if (msg.startsWith("[APP DEBUG] ")) {
      return msg
        .replace("[APP DEBUG]", t("logs.msg.app_debug_prefix"))
        .replace("(Процесс:", t("logs.msg.process"))
        .replace(
          ") не в белом списке. Идет в прокси.",
          t("logs.msg.not_in_whitelist"),
        )
        .replace(
          ") В БЕЛОМ СПИСКЕ. Идет напрямую.",
          t("logs.msg.in_whitelist"),
        );
    }
    if (msg.startsWith("[СИСТЕМА] ")) {
      return msg.replace("[СИСТЕМА]", t("logs.msg.system_prefix"));
    }
  }

  return msg;
};

export const LogsView = () => {
  const { t } = useTranslation();
  const { logs, backendLogs } = useLogContext();
  const allLogs = [...logs, ...(backendLogs || [])]
    .sort((a, b) => (b.timestamp || 0) - (a.timestamp || 0))
    .slice(0, 150);

  return (
    <div className="space-y-6 animate-in fade-in duration-300 h-full flex flex-col">
      <div>
        <h2 className="text-3xl font-bold text-white">{t("logs.title")}</h2>
        <p className="text-zinc-400 mt-2">{t("logs.desc")}</p>
      </div>
      <div className="bg-zinc-950 border border-zinc-800 rounded-3xl p-6 flex-1 overflow-y-auto font-mono text-sm scrollbar-hide">
        {allLogs.map((log, i) => (
          <div
            key={i}
            className={`flex items-start space-x-4 border-b border-zinc-800/50 py-3 last:border-0 ${log.type === "error" ? "text-rose-400" : log.type === "success" ? "text-[#007E3A]" : log.type === "warning" ? "text-[#00A819]" : "text-zinc-300"}`}
          >
            <span className="text-zinc-600 shrink-0">[{log.time}]</span>
            <div className="break-words w-full">
              {log.source && (
                <span className="text-zinc-500 text-xs mr-2 font-semibold">{log.source}</span>
              )}
              <span>{translateLog(log.msg, t)}</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};
