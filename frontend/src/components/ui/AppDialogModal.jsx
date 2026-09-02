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

import React from "react";
import { useTranslation } from "react-i18next";
import { Button, Dialog } from "../kit";

/*
 * Общее окно сообщений приложения. Figma "ResultV" -> App Design, фреймы
 * `Errors` (6721:3993, 6721:4109) и «Прокси не найдены» на Add page
 * (6721:3845): жёлтый кружок с восклицательным знаком, заголовок, текст и
 * одна кнопка.
 *
 * В макете нарисовано только предупреждение. `info` и `danger` берут ту же
 * раскладку в своих цветах палитры — см. docs/design/GAPS.md.
 */
const VARIANTS = {
  info: "success",
  warning: "warning",
  danger: "error",
};

const BUTTON_VARIANT = {
  success: "green",
  warning: "yellow",
  error: "red",
};

const AppDialogModal = ({
  isOpen = false,
  title = "",
  message = "",
  variant = "info",
  showCancel = false,
  confirmText,
  cancelText,
  onConfirm,
  onClose,
}) => {
  const { t } = useTranslation();

  if (!isOpen) return null;

  const look = VARIANTS[variant] ?? VARIANTS.info;

  return (
    <Dialog
      variant={look}
      icon="alert"
      title={title || t("common.notice", "Уведомление")}
      onClose={onClose}
      actions={
        <>
          {showCancel && (
            <Button onClick={onClose}>{cancelText || t("common.cancel", "Отмена")}</Button>
          )}
          <Button variant={BUTTON_VARIANT[look]} onClick={onConfirm}>
            {confirmText ||
              (showCancel ? t("common.confirm", "Подтвердить") : t("common.ok", "OK"))}
          </Button>
        </>
      }
    >
      <p className="rv-dialog__text">{message}</p>
    </Dialog>
  );
};

export default AppDialogModal;
