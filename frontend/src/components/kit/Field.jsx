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


import "./Field.css";

/**
 * Однострочное поле ввода (Figma node 6566:5077).
 * Ширина берётся от родителя; в макете это 588.
 *
 * `invalid` подсвечивает поле рамкой цвета ошибки. В макете такого состояния
 * нет — сделано по правилам кита, см. docs/design/GAPS.md.
 */
export function Input({ invalid = false, className = "", ...rest }) {
  return (
    <input
      type="text"
      className={`rv-input ${className}`}
      aria-invalid={invalid || undefined}
      {...rest}
    />
  );
}

/**
 * Многострочное поле (Figma node 6557:2093). Высота фиксированная — 156,
 * ручного изменения размера в макете нет.
 */
export function Textarea({ invalid = false, className = "", ...rest }) {
  return (
    <textarea
      className={`rv-textarea ${className}`}
      aria-invalid={invalid || undefined}
      {...rest}
    />
  );
}

/**
 * Поле с плавающей подписью. В макете его нет — собрано из готового Input по
 * правилам кита, см. docs/design/GAPS.md.
 *
 * Зачем оно: окно правки сервера — это два-три десятка полей подряд, и обе
 * привычные раскладки там плохи. Подпись над каждым полем растит окно вдвое и
 * превращает сетку в лестницу из строк разной высоты. Одни подсказки внутри
 * (так сделан AmneziaWG на Андроиде) выглядят чисто ровно до первого
 * заполнения: заполненное поле остаётся без имени, и «4» в ряду из четырёх
 * чисел уже не отличить от соседних.
 *
 * Здесь оба состояния в одной коробке: пустое поле показывает только
 * подсказку, у заполненного (и у того, что в фокусе) подпись уезжает наверх
 * внутрь той же рамки. Размер не меняется никогда — ради этого высота задана
 * жёстко: 64, как у Select, чтобы поля, списки и тумблеры в одной сетке были
 * одним прямоугольником.
 *
 * Состояния переключает сам CSS через `:placeholder-shown`, поэтому подсказка
 * обязана быть непустой — ею и служит подпись.
 */
export function Field({ label, invalid = false, className = "", ...rest }) {
  return (
    <label className={`rv-field ${className}`}>
      {/* Ввод стоит первым: подпись ловится от него соседним селектором. */}
      <input
        type="text"
        className="rv-field__input"
        placeholder={label}
        aria-label={label}
        aria-invalid={invalid || undefined}
        {...rest}
      />
      <span className="rv-field__label">{label}</span>
    </label>
  );
}
