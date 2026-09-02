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

import Icon from "./Icon";
import "./ProfileItem.css";

/**
 * Строка профиля маршрутизации (Figma 6600:3593 — активный, 6600:3618 —
 * остальные).
 *
 * Одна раскладка в двух состояниях: активный профиль залит Main 10 % и
 * плашка значка того же цвета, у прочих заливка Grey и плашка Light Gray.
 *
 * Счётчики под названием набраны одной строкой, но разными цветами: direct
 * фирменным 50 %, block цветом ошибки 50 %. Ноль не показывается — в макете
 * у профиля с четырьмя direct и тремя block нарисованы ровно две пары, а не
 * три с нулём.
 */
export default function ProfileItem({
  name,
  counts = {},
  active = false,
  onSelect,
  onEdit,
  onDelete,
  editLabel,
  deleteLabel,
  className = "",
  ...rest
}) {
  const parts = [
    { key: "direct", value: counts.direct },
    { key: "proxy", value: counts.proxy },
    { key: "block", value: counts.block },
  ].filter((p) => p.value > 0);

  return (
    <div
      className={`rv-profile-item rv-border ${className}`}
      data-active={active || undefined}
      {...rest}
    >
      {/*
        Выбор профиля — нажатие на саму строку. Кнопки справа лежат СНАРУЖИ
        этой кнопки, а не внутри: вложенная кнопка невалидна в HTML, и клик
        по карандашу заодно менял бы активный профиль.
      */}
      <button
        type="button"
        className="rv-profile-item__pick"
        aria-pressed={active}
        onClick={onSelect}
      >
        <span className="rv-profile-item__badge">
          <Icon name="globe" size={28} color="currentColor" />
        </span>
        <span className="rv-profile-item__text">
          <span className="rv-profile-item__name">{name}</span>
          {parts.length > 0 && (
            <span className="rv-profile-item__counts">
              {parts.map((p) => (
                <span key={p.key} className="rv-profile-item__count" data-action={p.key}>
                  • {p.value} {p.key}
                </span>
              ))}
            </span>
          )}
        </span>
      </button>
      <div className="rv-profile-item__actions">
        {/* Правка есть не у всякой строки: маршрутизация, встроенная в тело
            подписки, приходит готовой и править в ней нечего — карандаш там
            обещал бы то, чего нет. */}
        {onEdit && (
          <button
            type="button"
            className="rv-profile-item__action"
            aria-label={editLabel}
            title={editLabel}
            onClick={onEdit}
          >
            <Icon name="edit" size={24} color="currentColor" />
          </button>
        )}
        <button
          type="button"
          className="rv-profile-item__action"
          aria-label={deleteLabel}
          title={deleteLabel}
          onClick={onDelete}
        >
          <Icon name="delete" size={24} color="currentColor" />
        </button>
      </div>
    </div>
  );
}
