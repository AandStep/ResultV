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

import { useTranslation } from "react-i18next";
import { Button, Dialog } from "../kit";

/* Пояснение про протокол при добавлении узла — то же окно сообщения, что и
   у AppDialogModal, с одной кнопкой. */
const ProtocolWarningModal = ({ isOpen, onClose }) => {
  const { t } = useTranslation();

  if (!isOpen) return null;

  return (
    <Dialog
      icon="alert"
      title={t("add.protocolWarningTitle", "Важное уточнение")}
      onClose={onClose}
      actions={
        <Button variant="green" onClick={onClose}>
          {t("add.gotIt", "Понятно")}
        </Button>
      }
    >
      <p className="rv-dialog__text">{t("add.protocolWarning")}</p>
    </Dialog>
  );
};

export default ProtocolWarningModal;
