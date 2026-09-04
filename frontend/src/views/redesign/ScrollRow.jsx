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
 * Ряд кнопок, который шире своей панели.
 *
 * Так нарисованы протоколы на странице добавления (6533:2107) и интервалы
 * обновления в окне настроек подписки (6566:4990): ряд не помещается, полосы
 * прокрутки в макете нет, а у края лежит растяжка в цвет панели —
 * прямоугольник 150x73 (6541:2164). Растяжка гаснет у того края, за которым
 * кнопок больше нет: она и нужна, чтобы показать, что ряд продолжается.
 * В макете нарисована только правая; левая — та же самая, отражённая, иначе
 * при прокрутке кнопки обрывались бы по живому.
 *
 * Колесо у мыши одно, поэтому вертикальное колесо крутит ряд вбок — иначе до
 * последних кнопок не добраться.
 */

import { Children, useLayoutEffect, useRef, useState } from "react";
import "./ScrollRow.css";

export default function ScrollRow({ children, className = "", ...rest }) {
  const ref = useRef(null);
  /*
   * `null` — ряд ещё не мерили, и растяжек в разметке нет вовсе.
   *
   * Начинать со спрятанных нельзя: измерить ряд можно только по готовой
   * разметке, и первый же замер зажигал бы правую растяжку — вход на
   * страницу каждый раз показывал её плавное появление на ровном месте.
   * Растяжка же не событие, она просто край ряда. Замер идёт до отрисовки
   * (`useLayoutEffect`), а только что вставленному элементу переход
   * анимировать не от чего — он появляется сразу таким, какой нужен.
   */
  const [edges, setEdges] = useState(null);
  /* Сами дети — новый массив на каждую отрисовку; пересобирать по ним
     слушатели незачем, меняется только их число. */
  const count = Children.count(children);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return undefined;

    const sync = () => {
      const rest = el.scrollWidth - el.clientWidth - el.scrollLeft;
      setEdges({ start: el.scrollLeft <= 1, end: rest <= 1 });
    };

    const onWheel = (event) => {
      /* Горизонтальное колесо и Shift браузер обрабатывает сам. */
      if (event.deltaY === 0 || event.shiftKey) return;
      const max = el.scrollWidth - el.clientWidth;
      if (max <= 0) return;
      event.preventDefault();
      el.scrollLeft = Math.max(0, Math.min(max, el.scrollLeft + event.deltaY));
    };

    sync();
    el.addEventListener("scroll", sync, { passive: true });
    /* Слушатель ставится руками: у React колесо всегда passive и
       preventDefault в нём не работает. */
    el.addEventListener("wheel", onWheel, { passive: false });
    const observer = new ResizeObserver(sync);
    observer.observe(el);
    return () => {
      el.removeEventListener("scroll", sync);
      el.removeEventListener("wheel", onWheel);
      observer.disconnect();
    };
  }, [count]);

  return (
    <div className={`rv-scroll-row ${className}`} {...rest}>
      <div className="rv-scroll-row__track" ref={ref}>
        {children}
      </div>
      {edges && (
        <>
          <div
            className="rv-scroll-row__fade rv-scroll-row__fade--start"
            data-hidden={edges.start || undefined}
          />
          <div className="rv-scroll-row__fade" data-hidden={edges.end || undefined} />
        </>
      )}
    </div>
  );
}
