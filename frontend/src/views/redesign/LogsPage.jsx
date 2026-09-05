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
 * Страница «Журнал логов». Figma "ResultV" -> App Design, фрейм Logs
 * (6618:4063).
 *
 * Фрейм один. Шапка страницы с двумя кнопками справа — сохранить журнал в
 * файл и очистить его, — под ней панель во всю оставшуюся высоту со строками
 * лога. Строка это время, источник и сообщение; под каждой линия белым 10 %,
 * в том числе под последней — так в макете.
 */

import { Icon } from "../../components/kit";
import PageHeader from "./PageHeader";
import "./LogsPage.css";

/* Подписи в написании макета. В приложении они идут через i18n. */
export const LOGS_PAGE_TEXT = {
  title: "Журнал логов",
  subtitle:
    "Системные логи и информация о подключениях. Если вы нашли баг, логи помогут нам его исправить.",
  /* Подписей к кнопкам-иконкам в макете нет — это подсказки. */
  save: "Сохранить все логи в текстовый файл",
  clear: "Очистить журнал логов",
  /* Пустого журнала в макете нет, см. GAPS.md. */
  empty: "Нет доступных логов",
};

/**
 * `rows` — строки сверху вниз, уже в нужном порядке. У каждой `time`,
 * необязательный `source` и `text`; `type` красит сообщение (`error`,
 * `success`, `warning`), по умолчанию оно белое.
 */
export default function LogsPage({
  title,
  subtitle,
  rows = [],
  onSave,
  onClear,
  listRef,
  onListScroll,
  sidebar,
  text = LOGS_PAGE_TEXT,
  className = "",
  ...rest
}) {
  return (
    <div className={`rv-logs-page ${className}`} {...rest}>
      {sidebar}

      <div className="rv-logs-page__content">
        <PageHeader
          title={title ?? text.title}
          subtitle={subtitle ?? text.subtitle}
          actions={
            <>
              <button
                type="button"
                className="rv-page-header__btn"
                title={text.save}
                aria-label={text.save}
                onClick={onSave}
              >
                <Icon name="save" color="currentColor" />
              </button>
              <button
                type="button"
                className="rv-page-header__btn"
                title={text.clear}
                aria-label={text.clear}
                onClick={onClear}
              >
                <Icon name="delete" color="currentColor" />
              </button>
            </>
          }
        />

        {/*
          Прокручивается список внутри панели, а не сама панель: обводка и
          скругление живут на слоях, которые уехали бы вместе с содержимым.
        */}
        <div className="rv-logs-page__panel rv-border">
          <div
            ref={listRef}
            onScroll={onListScroll}
            className="rv-logs-page__list rv-scroll"
          >
            {rows.length === 0 ? (
              <p className="rv-logs-page__empty">{text.empty}</p>
            ) : (
              rows.map((row) => (
                <div key={row.key} className="rv-logs-page__row">
                  <span className="rv-logs-page__time">[{row.time}]</span>
                  <p className="rv-logs-page__msg" data-type={row.type}>
                    {row.source && (
                      <span className="rv-logs-page__source">{row.source} </span>
                    )}
                    {row.text}
                  </p>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
