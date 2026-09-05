/*
 * Copyright (C) 2026 ResultV
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

/*
 * Страница «Журнал логов», подключённая к приложению.
 *
 * Вид держит LogsPage, здесь всё остальное: слияние двух источников записей,
 * перевод сообщений ядра, сохранение журнала в файл и очистка. Всё это
 * перенесено с прежней страницы как есть: менялся только вид.
 */

import { useCallback, useLayoutEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useLogContext } from "../../context/LogContext";
import LogsPage from "./LogsPage";
import AppSidebar from "./AppSidebar";

/* Насколько близко к началу списка нужно быть, чтобы он продолжал держаться
   за него при новых записях. */
const SCROLL_TOP_THRESHOLD = 40;
/* Больше этого числа записей на странице не показываем: журнал длинный, а
   строки рисуются все разом. */
const DISPLAY_LIMIT = 150;

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

const mergeLogs = (logs, backendLogs) =>
  [...logs, ...(backendLogs || [])].sort(
    (a, b) => (b.timestamp || 0) - (a.timestamp || 0),
  );

const formatLogLine = (log, t) => {
  const time = log.time ? `[${log.time}]` : "";
  const source = log.source ? ` ${log.source}` : "";
  const msg = translateLog(log.msg, t);
  return `${time}${source} ${msg}`.trim();
};

export default function LogsScreen() {
  const { t } = useTranslation();
  const { logs, backendLogs, clearLogs } = useLogContext();
  const listRef = useRef(null);
  const pinnedToTopRef = useRef(true);
  const scrollMetricsRef = useRef({ scrollHeight: 0, scrollTop: 0 });

  const allLogs = mergeLogs(logs, backendLogs).slice(0, DISPLAY_LIMIT);

  const handleScroll = useCallback(() => {
    const el = listRef.current;
    if (!el) return;
    pinnedToTopRef.current = el.scrollTop <= SCROLL_TOP_THRESHOLD;
    scrollMetricsRef.current = {
      scrollHeight: el.scrollHeight,
      scrollTop: el.scrollTop,
    };
  }, []);

  /*
   * Новые записи приходят сверху. Пока список стоит у начала, он там и
   * остаётся; если его отмотали вниз — сдвигаем на столько же, на сколько
   * подрос, чтобы читаемое место не уезжало из-под глаз.
   */
  useLayoutEffect(() => {
    const el = listRef.current;
    if (!el) return;

    const { scrollHeight: prevHeight, scrollTop: prevTop } = scrollMetricsRef.current;

    if (pinnedToTopRef.current) {
      el.scrollTop = 0;
    } else {
      el.scrollTop = prevTop + (el.scrollHeight - prevHeight);
    }

    scrollMetricsRef.current = {
      scrollHeight: el.scrollHeight,
      scrollTop: el.scrollTop,
    };
  }, [allLogs]);

  const handleSave = useCallback(() => {
    /* В файл журнал уходит в обычном порядке — от старых записей к новым. */
    const exportLogs = mergeLogs(logs, backendLogs)
      .slice()
      .sort((a, b) => (a.timestamp || 0) - (b.timestamp || 0));

    const body = exportLogs.map((log) => formatLogLine(log, t)).join("\n");
    const now = new Date();
    const pad = (n) => String(n).padStart(2, "0");
    const filename = `resultv-logs-${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}_${pad(now.getHours())}-${pad(now.getMinutes())}-${pad(now.getSeconds())}.txt`;

    const blob = new Blob([body], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = filename;
    document.body.appendChild(anchor);
    anchor.click();
    document.body.removeChild(anchor);
    URL.revokeObjectURL(url);
  }, [logs, backendLogs, t]);

  const handleClear = useCallback(() => {
    clearLogs();
    pinnedToTopRef.current = true;
    const el = listRef.current;
    if (el) {
      el.scrollTop = 0;
      scrollMetricsRef.current = { scrollHeight: el.scrollHeight, scrollTop: 0 };
    }
  }, [clearLogs]);

  const rows = allLogs.map((log, i) => ({
    key: `${log.timestamp ?? 0}-${log.msg?.slice(0, 32) ?? ""}-${i}`,
    time: log.time,
    source: log.source,
    text: translateLog(log.msg, t),
    type: log.type,
  }));

  return (
    <LogsPage
      title={t("logs.title")}
      subtitle={t("logs.desc")}
      rows={rows}
      onSave={handleSave}
      onClear={handleClear}
      listRef={listRef}
      onListScroll={handleScroll}
      text={{
        save: t("logs.save_tooltip"),
        clear: t("logs.clear_tooltip"),
        empty: t("logs.empty"),
      }}
      sidebar={<AppSidebar />}
    />
  );
}
