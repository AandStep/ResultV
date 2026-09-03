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
 * Окно «Профили маршрутизации». Figma "ResultV" -> App Design, фрейм
 * SmartRules (Global mode) 6600:3444, слой 6600:3530.
 *
 * Dialog из кита шириной 700 (как окно настроек подписки), внутри три
 * раздела с зазором 16: «Активный профиль», «Все профили», «Действия».
 * Подписи разделов набраны Regular белым 50 %.
 */

import { Button, Dialog, ProfileItem } from "../../components/kit";
import "./RoutingProfilesDialog.css";

export const ROUTING_PROFILES_TEXT = {
  title: "Профили маршрутизации",
  subtitle: "Самостоятельно управляйте правилами трафика",
  active: "Активный профиль",
  all: "Все профили",
  actions: "Действия",
  create: "Создать профиль",
  import: "Импорт профиля",
  edit: "Изменить профиль",
  remove: "Удалить профиль",
  /* Пустого состояния в макете нет — до первого профиля показывать нечего,
     а раздел с одной подписью выглядел бы поломкой. См. GAPS.md. */
  empty: "Профилей пока нет. Создайте свой или импортируйте по ссылке.",
};

export default function RoutingProfilesDialog({
  open = true,
  profiles = [],
  activeId = "",
  onSelect,
  onEdit,
  onDelete,
  onCreate,
  onImport,
  onClose,
  text = ROUTING_PROFILES_TEXT,
}) {
  const active = profiles.find((p) => p.id === activeId) || null;
  /* «Все профили» — это остальные: активный уже показан выше своим разделом,
     и повторять его строкой ниже значило бы показать один профиль дважды. */
  const rest = profiles.filter((p) => p.id !== activeId);

  const row = (profile, isActive) => (
    <ProfileItem
      key={profile.id}
      name={profile.name}
      counts={profile.counts}
      active={isActive}
      editLabel={text.edit}
      deleteLabel={text.remove}
      onSelect={() => onSelect?.(profile)}
      onEdit={() => onEdit?.(profile)}
      onDelete={() => onDelete?.(profile)}
    />
  );

  return (
    <Dialog
      open={open}
      icon="subrouting"
      title={text.title}
      subtitle={text.subtitle}
      onClose={onClose}
      className="rv-profiles-dialog"
    >
      <div className="rv-profiles-dialog__body rv-scroll-dialog">
        {active && (
          <section className="rv-profiles-dialog__section">
            <p className="rv-profiles-dialog__label">{text.active}</p>
            {row(active, true)}
          </section>
        )}

        {rest.length > 0 && (
          <section className="rv-profiles-dialog__section">
            <p className="rv-profiles-dialog__label">{text.all}</p>
            <div className="rv-profiles-dialog__list">
              {rest.map((p) => row(p, false))}
            </div>
          </section>
        )}

        {profiles.length === 0 && (
          <p className="rv-profiles-dialog__empty">{text.empty}</p>
        )}

        <section className="rv-profiles-dialog__section">
          <p className="rv-profiles-dialog__label">{text.actions}</p>
          <div className="rv-profiles-dialog__actions">
            <Button className="rv-profiles-dialog__btn" onClick={onCreate}>
              {text.create}
            </Button>
            <Button variant="green" className="rv-profiles-dialog__btn" onClick={onImport}>
              {text.import}
            </Button>
          </div>
        </section>
      </div>
    </Dialog>
  );
}
