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
import { Badge, Button, Dialog } from "../kit";

/*
 * update.json приезжает с raw.githubusercontent.com, поэтому подменённая
 * ветка или MITM с чужим корневым сертификатом могут вписать в downloadUrl
 * что угодно — хоть "javascript:evil()", хоть чужой установщик по http.
 * Список хостов ограничивает последствия: адрес вне его никуда не ведёт.
 */
const ALLOWED_DOWNLOAD_HOSTS = new Set([
  "result-proxy.ru",
  "www.result-proxy.ru",
  "github.com",
]);

function isSafeDownloadURL(raw) {
  if (!raw || typeof raw !== "string") return false;
  let u;
  try {
    u = new URL(raw);
  } catch {
    return false;
  }
  if (u.protocol !== "https:") return false;
  return ALLOWED_DOWNLOAD_HOSTS.has(u.hostname.toLowerCase());
}

/**
 * UpdateNotificationModal — «вышла новая версия» без встроенной загрузки.
 *
 * Показывается вместо UpdaterModal, когда в манифесте нет готового файла
 * под эту платформу: обновляться приходится вручную, с сайта.
 */
const UpdateNotificationModal = ({
  currentVersion,
  latestVersion,
  downloadUrl,
  onClose,
}) => {
  const { t } = useTranslation();

  if (!latestVersion) return null;

  const openDownloadPage = () => {
    if (isSafeDownloadURL(downloadUrl)) {
      window.open(downloadUrl, "_blank", "noopener,noreferrer");
    } else {
      if (downloadUrl) {
        console.warn("Update downloadUrl rejected by host allow list:", downloadUrl);
      }
      document.dispatchEvent(new CustomEvent("open-download-modal"));
    }
    onClose();
  };

  return (
    <Dialog
      icon="import"
      title={t("update.title", "Доступно обновление")}
      subtitle={<Badge color="success">{latestVersion}</Badge>}
      onClose={onClose}
      actions={
        <>
          <Button onClick={onClose}>{t("update.later", "Позже")}</Button>
          <Button variant="green" onClick={openDownloadPage}>
            {t("update.download", "Обновить")}
          </Button>
        </>
      }
    >
      <p className="rv-dialog__text">
        {t(
          "update.message",
          "У вас установлена версия {{current}}, доступна новая версия {{latest}}.",
          { current: currentVersion, latest: latestVersion },
        )}
      </p>
    </Dialog>
  );
};

export default UpdateNotificationModal;
