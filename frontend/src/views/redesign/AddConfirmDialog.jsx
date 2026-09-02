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
 * Окно «Добавление серверов». Figma "ResultV" -> App Design, 6721:3593
 * (подписка со списками маршрутизации) и 6721:3702 (один сервер).
 *
 * Оно одно на все дороги: и вставка на странице добавления, и переход по
 * ссылке resultv:// показывают одно и то же — что нашли, брать ли списки
 * маршрутизации из подписки, и две кнопки.
 */

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Badge, Button, Dialog, Tumbler } from "../../components/kit";
import { PROTOCOL_CASE } from "./format";
import "./AddConfirmDialog.css";

/* Члены авто-групп не считаются: в списке они прячутся за своей группой. */
export function countByType(proxies) {
  const members = new Set();
  for (const p of proxies) {
    if (p.type === "AUTO") for (const id of p.extra?.members || []) members.add(id);
  }
  const counts = new Map();
  for (const p of proxies) {
    if (members.has(p.id) || p.type === "SECTION") continue;
    const key = String(p.type || "").toUpperCase();
    counts.set(key, (counts.get(key) || 0) + 1);
  }
  return [...counts].map(([type, count]) => ({
    type,
    count,
    label: PROTOCOL_CASE[type] ?? type,
  }));
}

export default function AddConfirmDialog({
  proxies = [],
  routingLists = [],
  routing = false,
  onRoutingChange,
  onCancel,
  onConfirm,
}) {
  const { t } = useTranslation();
  const found = useMemo(() => countByType(proxies), [proxies]);
  const total = found.reduce((sum, item) => sum + item.count, 0);

  /*
   * Одним окном добавляют и подписку, и отдельные серверы, а заголовок у них
   * разный. Подписка узнаётся по тому, что все найденные серверы пришли с
   * одного адреса подписки — так же её узнаёт и сама запись (useProxyImport).
   */
  const subscription = useMemo(() => {
    const url = proxies[0]?.subscriptionUrl;
    return Boolean(url) && proxies.every((p) => p.subscriptionUrl === url);
  }, [proxies]);

  return (
    <Dialog
      icon="shield"
      title={t(subscription ? "addPage.confirmTitleSubscription" : "addPage.confirmTitle")}
      subtitle={t("addPage.detected", { count: total })}
      onClose={onCancel}
      /* Крестика в макете у этого окна нет — закрывает «Отмена». */
      showClose={false}
    >
      <div className="rv-add-confirm">
        <div className="rv-add-confirm__found">
          {found.map((item) => (
            <Badge key={item.type} color="success">
              {item.label} ({item.count})
            </Badge>
          ))}
        </div>

        {routingLists.length > 0 && (
          <div className="rv-add-confirm__field">
            <span className="rv-add-confirm__label">
              {t("addPage.routingFromSubscription")}
            </span>
            <Tumbler checked={routing} onChange={onRoutingChange} />
          </div>
        )}

        <div className="rv-dialog__actions">
          <Button onClick={onCancel}>{t("common.cancel")}</Button>
          <Button variant="green" onClick={onConfirm}>
            {t("addPage.confirmSubmit", { count: total })}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
