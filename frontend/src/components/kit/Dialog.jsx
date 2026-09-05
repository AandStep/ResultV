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

import { createPortal } from "react-dom";
import { useEffect } from "react";
import Icon from "./Icon";
import "./Dialog.css";

/*
 * Окно поверх страницы. Figma "ResultV" -> App Design:
 *   6721:3593  «Добавление серверов» — щит, подзаголовок, содержимое, две кнопки
 *   6721:3875  «Уведомление» — кружок с восклицательным знаком, крестик, ОК
 *   6721:3993  «Обнаружены остатки прошлого сеанса» — тот же вид, длинный текст
 *   6721:4109  «Восстановление после некорректного выхода»
 *
 * Всё это одно окно с разными цветом и наполнением: 440 в ширину, отступ 24,
 * зазор 24, скругление 32; страница под ним темнеет на 50 % и размывается.
 */

export const DIALOG_VARIANTS = ["success", "warning", "error"];

export default function Dialog({
  open = true,
  variant = "success",
  icon = "alert",
  title,
  subtitle,
  onClose,
  /* Крестик есть у «Уведомления», а у «Добавления серверов» его нет — там
     закрывает «Отмена». Esc и щелчок по подложке работают в обоих. */
  showClose = true,
  children,
  actions,
  className = "",
  ...rest
}) {
  /* Escape закрывает окно — крестик в макете есть, клавиатура его повторяет. */
  useEffect(() => {
    if (!open || !onClose) return undefined;
    const onKey = (event) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  return createPortal(
    <div className="rv-dialog-overlay" onMouseDown={onClose}>
      {/* Клик по самому окну не должен доходить до подложки и закрывать его. */}
      <div
        className={`rv-dialog rv-border rv-border--static ${className}`}
        data-variant={variant}
        role="dialog"
        aria-modal="true"
        onMouseDown={(event) => event.stopPropagation()}
        {...rest}
      >
        <div className="rv-dialog__head">
          <div className="rv-dialog__id">
            {/* Обычно это значок кита в цвет варианта, но окно настроек
                подписки ставит на его место логотип провайдера — тогда
                содержимое подложки приходит готовым. */}
            <span className="rv-dialog__icon">
              {typeof icon === "string" ? (
                <Icon name={icon} size={32} color="currentColor" />
              ) : (
                icon
              )}
            </span>
            <div className="rv-dialog__titles">
              <p className="rv-dialog__title">{title}</p>
              {subtitle && <p className="rv-dialog__subtitle">{subtitle}</p>}
            </div>
          </div>
          {onClose && showClose && (
            <button type="button" className="rv-dialog__close" onClick={onClose}>
              <Icon name="close" size={24} color="currentColor" />
            </button>
          )}
        </div>

        {children}

        {actions && <div className="rv-dialog__actions">{actions}</div>}
      </div>
    </div>,
    document.body,
  );
}
