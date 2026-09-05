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

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import Icon from "./Icon";
import "./Select.css";

/* Отступ меню от кнопки — 8, как в макете (кнопка 238–302, меню от 311). */
const GAP = 8;

/**
 * Выбор одного значения из списка (Figma node 6799:4634 — закрытый,
 * 6799:4963 — раскрытый).
 *
 * Меню лежит не внутри кнопки, а поверх страницы: карточка настроек и
 * страница прокручиваются, а меню должно оставаться целым и у нижней строки
 * — тем же приёмом собрано меню порядка серверов.
 *
 * `options` — список `{ value, label }`; подписи приходят готовыми, потому
 * что в приложении они идут через i18n.
 */
export default function Select({
  value,
  options = [],
  onChange,
  disabled,
  className = "",
  ...rest
}) {
  const anchorRef = useRef(null);
  const menuRef = useRef(null);
  /* Фокус в меню ставится один раз за раскрытие: положение оно ещё
     пересчитывает за прокруткой, и на каждый пересчёт фокус не сбрасывается. */
  const focused = useRef(false);
  const [open, setOpen] = useState(false);
  /* `null` — меню ещё не мерили: до замера оно спрятано, иначе на кадр
     показалось бы в левом верхнем углу окна. */
  const [pos, setPos] = useState(null);

  const current = options.find((option) => option.value === value);

  const close = (focus = false) => {
    setOpen(false);
    setPos(null);
    if (focus) anchorRef.current?.focus();
  };

  /*
   * Меню правым краем равняется по кнопке и встаёт под неё, а если снизу не
   * помещается — над ней.
   *
   * Размер берётся из `offsetWidth`/`offsetHeight`, а не из прямоугольника:
   * меню появляется с уменьшения (0.94), и прямоугольник в этот момент отдаёт
   * размер вместе с этим уменьшением — меню вставало бы на полдесятка
   * пикселей правее кнопки.
   */
  const place = useCallback(() => {
    const btn = anchorRef.current?.getBoundingClientRect();
    const menu = menuRef.current;
    if (!btn || !menu) return;
    const width = menu.offsetWidth;
    const height = menu.offsetHeight;
    const left = Math.max(
      GAP,
      Math.min(btn.right - width, window.innerWidth - width - GAP),
    );
    const below = btn.bottom + GAP;
    const flip = below + height > window.innerHeight - GAP;
    setPos({ left, top: flip ? btn.top - height - GAP : below, flip });
  }, []);

  /* Место считается до отрисовки, а не после: иначе первый кадр меню успел
     бы показаться не там, где надо. */
  useLayoutEffect(() => {
    if (open) place();
  }, [open, place]);

  /*
   * Раскрытое меню сразу забирает фокус на текущее значение: иначе стрелкам
   * не от чего отсчитывать строку, и клавиатура до меню не доходит.
   *
   * Только после того, как меню встало на место (`pos`): до замера оно
   * спрятано, а спрятанное фокус не берёт — вызов проходил впустую.
   */
  useEffect(() => {
    if (!open) {
      focused.current = false;
      return;
    }
    if (!pos || focused.current) return;
    focused.current = true;
    const items = menuRef.current?.querySelectorAll(".rv-select__option");
    if (!items?.length) return;
    const index = options.findIndex((option) => option.value === value);
    items[index < 0 ? 0 : index].focus();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, pos]);

  /*
   * Меню держится на месте окна, а не страницы, поэтому за прокруткой и за
   * изменением окна оно идёт само — иначе отъехало бы от своей кнопки.
   * Щелчок мимо и Escape закрывают его так же, как любое окно приложения.
   */
  useEffect(() => {
    if (!open) return undefined;
    const onDown = (event) => {
      if (
        !menuRef.current?.contains(event.target) &&
        !anchorRef.current?.contains(event.target)
      ) {
        close();
      }
    };
    const onKey = (event) => {
      if (event.key === "Escape") close(true);
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    window.addEventListener("scroll", place, true);
    window.addEventListener("resize", place);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
      window.removeEventListener("scroll", place, true);
      window.removeEventListener("resize", place);
    };
  }, [open, place]);

  /* Раскрытое меню принимает клавиатуру: стрелки водят по строкам, Tab
     закрывает. Мышь не должна быть единственным путём к настройке. */
  const onMenuKeyDown = (event) => {
    if (event.key === "Tab") {
      close();
      return;
    }
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
    event.preventDefault();
    const items = [...menuRef.current.querySelectorAll(".rv-select__option")];
    const step = event.key === "ArrowDown" ? 1 : -1;
    const next = items.indexOf(document.activeElement) + step;
    items[(next + items.length) % items.length]?.focus();
  };

  return (
    <>
      <button
        ref={anchorRef}
        type="button"
        className={`rv-select ${className}`}
        disabled={disabled}
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => (open ? close() : setOpen(true))}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            setOpen(true);
          }
        }}
        {...rest}
      >
        <span className="rv-select__value">{current?.label ?? value}</span>
        {/* Шеврон из фрейма Icons: в покое смотрит вниз, у раскрытого меню
            вверх — это его состояние Active. */}
        <Icon
          name="open"
          state={open ? "active" : "default"}
          color="currentColor"
          className="rv-select__icon"
        />
      </button>

      {open && (
        <div
          ref={menuRef}
          className="rv-select__menu rv-border rv-border--static"
          /* Меню не уже своей кнопки, а шире — по самой длинной подписи:
             в макете они одной ширины. */
          style={{
            minWidth: anchorRef.current?.offsetWidth,
            ...(pos
              ? { left: pos.left, top: pos.top }
              : { visibility: "hidden" }),
          }}
          data-flip={pos?.flip ? "true" : undefined}
          onKeyDown={onMenuKeyDown}
        >
          {/* Строки залиты во всю ширину и упираются в углы меню, поэтому
              обрезка идёт по внутренней обёртке: обводке `rv-border` она
              противопоказана (см. design/borders.css). */}
          <div className="rv-select__list" role="listbox">
            {options.map((option) => (
              <button
                key={option.value}
                type="button"
                role="option"
                aria-selected={option.value === value}
                className="rv-select__option"
                data-current={option.value === value || undefined}
                onClick={() => {
                  onChange?.(option.value);
                  close(true);
                }}
              >
                {option.label}
              </button>
            ))}
          </div>
        </div>
      )}
    </>
  );
}
