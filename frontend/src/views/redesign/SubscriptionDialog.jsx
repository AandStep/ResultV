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
 * Окно настроек подписки. Figma "ResultV" -> App Design, фрейм ServersPage
 * 6566:4737, слой 6566:4928: оно открывается карандашом в строке подписки.
 *
 * Само окно — Dialog из кита, только шире (700 против 440) и с логотипом
 * провайдера вместо значка. Внутри три поля и кнопка: имя, показывать ли
 * серверы подписки на главной, свой интервал обновления.
 */

import { useState } from "react";
import { Button, Dialog, Icon, Input, Tumbler } from "../../components/kit";
import ScrollRow from "./ScrollRow";
import "./SubscriptionDialog.css";

/*
 * Интервалы ровно те, что набраны в макете (6566:4990). Ноль — «Никогда»:
 * подписка не обновляется по часам, только вручную.
 */
export const SUBSCRIPTION_INTERVALS = [
  { minutes: 0, label: "Никогда" },
  { minutes: 30, label: "30 мин" },
  { minutes: 60, label: "1 час" },
  { minutes: 120, label: "2 часа" },
  { minutes: 240, label: "4 часа" },
  { minutes: 360, label: "6 часов" },
];

export const SUBSCRIPTION_DIALOG_TEXT = {
  support: "Поддержка",
  name: "Изменить имя",
  showOnHome: "Показывать на главной",
  interval: "Свой интервал обновлений",
  save: "Сохранить",
};

export default function SubscriptionDialog({
  logo,
  title,
  /* Тег «Поддержка» — ссылка провайдера. Нет ссылки — нет и тега. */
  supportUrl,
  onSupport,
  name = "",
  showOnHome = true,
  interval = 360,
  intervals = SUBSCRIPTION_INTERVALS,
  onClose,
  onSave,
  text = SUBSCRIPTION_DIALOG_TEXT,
}) {
  const [draftName, setDraftName] = useState(name);
  const [draftShow, setDraftShow] = useState(showOnHome);
  const [draftInterval, setDraftInterval] = useState(interval);

  return (
    <Dialog
      className="rv-sub-dialog"
      icon={logo}
      title={title}
      subtitle={
        supportUrl ? (
          <a
            className="rv-sub-dialog__tag"
            href={supportUrl}
            onClick={(event) => {
              /* Ссылку открывает приложение во внешнем браузере: окно Wails —
                 не вкладка, уходить из него страницей нельзя. */
              event.preventDefault();
              onSupport?.(supportUrl);
            }}
          >
            {text.support}
            <Icon name="link" color="currentColor" />
          </a>
        ) : undefined
      }
      onClose={onClose}
    >
      <div className="rv-sub-dialog__fields">
        <label className="rv-sub-dialog__field">
          <span className="rv-sub-dialog__label">{text.name}</span>
          <Input
            value={draftName}
            onChange={(event) => setDraftName(event.target.value)}
          />
        </label>

        <div className="rv-sub-dialog__field">
          <span className="rv-sub-dialog__label">{text.showOnHome}</span>
          <Tumbler checked={draftShow} onChange={setDraftShow} />
        </div>

        <div className="rv-sub-dialog__field">
          <span className="rv-sub-dialog__label">{text.interval}</span>
          {/* Выбранный интервал в макете ничем не отмечен; отмечен так же, как
              выбранный протокол на странице добавления — залитым состоянием
              кнопки. См. docs/design/GAPS.md. */}
          <ScrollRow>
            {intervals.map((item) => (
              <Button
                key={item.minutes}
                mode={item.minutes === draftInterval ? "idle" : undefined}
                aria-pressed={item.minutes === draftInterval}
                onClick={() => setDraftInterval(item.minutes)}
              >
                {item.label}
              </Button>
            ))}
          </ScrollRow>
        </div>

        <Button
          variant="green"
          className="rv-sub-dialog__save"
          onClick={() =>
            onSave?.({
              name: draftName.trim(),
              showOnHome: draftShow,
              intervalMinutes: draftInterval,
            })
          }
        >
          {text.save}
        </Button>
      </div>
    </Dialog>
  );
}
