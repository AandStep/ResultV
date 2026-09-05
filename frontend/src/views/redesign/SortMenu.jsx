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
 * Временное меню порядка серверов.
 *
 * В макете у кнопки сортировки меню пока нет (docs/design/GAPS.md), а порядков
 * на старой главной было семь. Чтобы кнопка не висела мёртвой, до готового
 * макета работает это меню: набор порядков и сама сортировка — старые
 * (`sortProxiesByOption`), вид собран на токенах кита, чтобы не выбиваться.
 *
 * Когда меню нарисуют, файл заменяется целиком на компонент кита.
 */

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import "./SortMenu.css";

/* Отступ от кнопки, такой же, как зазор между строками карточки. */
const GAP = 4;

/**
 * `options` — список `{ value, label }`; подписи приходят готовыми, как и
 * тексты самой страницы, потому что в приложении они идут через i18n.
 */
export default function SortMenu({ anchor, value, options = [], onPick, onClose }) {
  const ref = useRef(null);
  const [pos, setPos] = useState(null);

  /*
   * Карточка обрезает содержимое по скруглению, поэтому меню не может лежать
   * внутри неё — оно висит поверх страницы и само встаёт под кнопку. Позицию
   * считаем до отрисовки, иначе меню на кадр мигает в левом верхнем углу.
   */
  useLayoutEffect(() => {
    if (!anchor) {
      setPos(null);
      return;
    }
    const btn = anchor.getBoundingClientRect();
    const menu = ref.current?.getBoundingClientRect();
    const width = menu?.width ?? 0;
    const height = menu?.height ?? 0;
    /* Не вылезать за окно: у правого края меню прижимается, снизу — встаёт
       над кнопкой. */
    const left = Math.max(GAP, Math.min(btn.right - width, window.innerWidth - width - GAP));
    const below = btn.bottom + GAP;
    const flip = below + height > window.innerHeight - GAP;
    setPos({ left, top: flip ? btn.top - height - GAP : below, flip });
  }, [anchor]);

  /* Закрывается как раньше: щелчком мимо и клавишей Escape. */
  useEffect(() => {
    if (!anchor) return undefined;
    const onDown = (event) => {
      if (!ref.current?.contains(event.target) && !anchor.contains(event.target)) onClose();
    };
    const onKey = (event) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [anchor, onClose]);

  if (!anchor) return null;

  return (
    <div
      ref={ref}
      className="rv-sort-menu rv-border rv-border--static"
      style={pos ? { left: pos.left, top: pos.top } : { visibility: "hidden" }}
      /* Меню раскрывается от кнопки, поэтому опорный угол зависит от того,
         встало оно под ней или над. */
      data-flip={pos?.flip ? "true" : undefined}
      role="menu"
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          role="menuitemradio"
          aria-checked={option.value === value}
          className="rv-sort-menu__item"
          data-current={option.value === value}
          onClick={() => onPick(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}
