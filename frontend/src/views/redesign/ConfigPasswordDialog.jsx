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
 * Пароль резервной копии. Кадра на это окно в макете нет — оно собрано из
 * Dialog кита той же раскладкой, что «Добавление серверов» (6721:3593):
 * значок, заголовок с подписью, содержимое и пара кнопок.
 *
 * Значок — замок: тот же, что стоит у карточки «Безопасность» на странице.
 * См. docs/design/GAPS.md.
 */

import { useEffect, useState } from "react";
import { Button, Dialog, Input } from "../../components/kit";
import "./ConfigPasswordDialog.css";

export default function ConfigPasswordDialog({
  open = false,
  title,
  message,
  placeholder,
  cancelLabel,
  submitLabel,
  onSubmit,
  onClose,
}) {
  const [password, setPassword] = useState("");

  /* Пароль не живёт дольше окна: закрыли — забыли. */
  useEffect(() => {
    if (!open) setPassword("");
  }, [open]);

  if (!open) return null;

  const submit = () => {
    if (!password) return;
    onSubmit?.(password);
  };

  return (
    <Dialog
      className="rv-config-pwd"
      icon="security"
      title={title}
      onClose={onClose}
      actions={
        <>
          <Button onClick={onClose}>{cancelLabel}</Button>
          <Button variant="green" mode={password ? undefined : "disable"} onClick={submit}>
            {submitLabel}
          </Button>
        </>
      }
    >
      <p className="rv-dialog__text">{message}</p>
      <Input
        type="password"
        className="rv-config-pwd__input"
        placeholder={placeholder}
        value={password}
        autoFocus
        onChange={(event) => setPassword(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault();
            submit();
          }
        }}
      />
    </Dialog>
  );
}
