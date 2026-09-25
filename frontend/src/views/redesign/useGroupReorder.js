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
 * Перетаскивание карточек групп на странице серверов. Карточку поднимает
 * удержание шапки в течение секунды: обычное нажатие по-прежнему раскрывает
 * группу, а сдвиг до срабатывания отменяет удержание, чтобы не мешать
 * прокрутке и выделению.
 *
 * Пока карточка на весу, её `translate` пишется прямо в DOM на каждом
 * движении, а соседи едут через состояние — их всего несколько. После
 * отпускания карточка доезжает до своего места, и только потом меняется
 * порядок: сдвиги снимаются в том же кадре без анимации.
 */

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";

export const HOLD_MS = 1000;
const MOVE_TOLERANCE = 6;
const SETTLE_MS = 240;
const EDGE = 64;
const MAX_SCROLL_SPEED = 14;

export default function useGroupReorder({ keys, enabled, onReorder }) {
  const nodes = useRef(new Map());
  const press = useRef(null);
  const drag = useRef(null);
  const [pressing, setPressing] = useState(null);
  const [lifted, setLifted] = useState(null);
  const [target, setTarget] = useState(null);
  const [dropping, setDropping] = useState(false);
  const [settling, setSettling] = useState(false);
  const pendingClear = useRef(null);
  const onReorderRef = useRef(onReorder);
  onReorderRef.current = onReorder;

  const register = useCallback(
    (key) => (node) => {
      if (node) nodes.current.set(key, node);
      else nodes.current.delete(key);
    },
    [],
  );

  const stopScroll = () => {
    if (drag.current?.raf) cancelAnimationFrame(drag.current.raf);
  };

  const cleanupListeners = useRef(() => {});

  const reset = useCallback(() => {
    clearTimeout(press.current?.timer);
    press.current = null;
    stopScroll();
    drag.current = null;
    cleanupListeners.current();
    document.body.classList.remove("rv-grabbing");
    setPressing(null);
    setLifted(null);
    setTarget(null);
    setDropping(false);
  }, []);

  /* Отпускание породит click по шапке, а он свернул бы или раскрыл группу. */
  const swallowClick = () => {
    const eat = (event) => {
      event.stopPropagation();
      event.preventDefault();
    };
    window.addEventListener("click", eat, { capture: true, once: true });
    setTimeout(() => window.removeEventListener("click", eat, { capture: true }), 100);
  };

  const indexFor = (d, center) => {
    let to = 0;
    d.centers.forEach((c, i) => {
      if (i !== d.from && c < center) to += 1;
    });
    return to;
  };

  const place = useCallback((clientY) => {
    const d = drag.current;
    if (!d) return;
    d.lastY = clientY;
    const dy = clientY - d.startY + (d.scroller.scrollTop - d.scroll0);
    d.node.style.translate = `0 ${dy}px`;
    const to = indexFor(d, d.centers[d.from] + dy);
    if (to !== d.to) {
      d.to = to;
      setTarget(to);
    }
  }, []);

  const autoScroll = useCallback(() => {
    const d = drag.current;
    if (!d) return;
    const box = d.scroller.getBoundingClientRect();
    let speed = 0;
    if (d.lastY < box.top + EDGE) speed = -((box.top + EDGE - d.lastY) / EDGE);
    else if (d.lastY > box.bottom - EDGE) speed = (d.lastY - (box.bottom - EDGE)) / EDGE;
    if (speed !== 0) {
      const before = d.scroller.scrollTop;
      d.scroller.scrollTop += Math.max(-1, Math.min(1, speed)) * MAX_SCROLL_SPEED;
      if (d.scroller.scrollTop !== before) place(d.lastY);
    }
    d.raf = requestAnimationFrame(autoScroll);
  }, [place]);

  const lift = useCallback(() => {
    const p = press.current;
    if (!p) return;
    const order = keys;
    const from = order.indexOf(p.key);
    const node = nodes.current.get(p.key);
    if (from === -1 || !node) return reset();

    const rects = order.map((k) => nodes.current.get(k)?.getBoundingClientRect());
    if (rects.some((r) => !r)) return reset();
    const gap = rects.length > 1 ? rects[1].top - rects[0].bottom : 0;
    const scroller = node.closest(".rv-scroll") || document.scrollingElement;

    drag.current = {
      key: p.key,
      order,
      from,
      to: from,
      node,
      tops: rects.map((r) => r.top),
      heights: rects.map((r) => r.height),
      centers: rects.map((r) => r.top + r.height / 2),
      gap,
      startY: p.y,
      lastY: p.y,
      scroller,
      scroll0: scroller.scrollTop,
      raf: 0,
    };
    press.current = null;
    document.body.classList.add("rv-grabbing");
    /* Атрибуты ставим сразу, не дожидаясь рендера: от них зависит, какие
       свойства анимируются, а первое движение может прийти раньше. */
    node.removeAttribute("data-pressing");
    node.setAttribute("data-lifted", "true");
    setPressing(null);
    setLifted(p.key);
    setTarget(from);

    drag.current.raf = requestAnimationFrame(autoScroll);
  }, [keys, reset, autoScroll]);

  const drop = useCallback(
    (cancel = false, byKey = false) => {
      const d = drag.current;
      if (!d) return reset();
      stopScroll();
      cleanupListeners.current();
      const to = cancel ? d.from : d.to;
      let offset = 0;
      if (to > d.from) offset = d.tops[to] + d.heights[to] - d.heights[d.from] - d.tops[d.from];
      else if (to < d.from) offset = d.tops[to] - d.tops[d.from];
      if (cancel) setTarget(d.from);

      if (byKey) window.addEventListener("pointerup", swallowClick, { once: true });
      else swallowClick();

      setDropping(true);
      d.node.setAttribute("data-dropping", "true");
      d.node.style.translate = `0 ${offset}px`;
      document.body.classList.remove("rv-grabbing");

      setTimeout(() => {
        const node = d.node;
        drag.current = null;
        if (to !== d.from) {
          const next = d.order.filter((k) => k !== d.key);
          next.splice(to, 0, d.key);
          pendingClear.current = node;
          setSettling(true);
          onReorderRef.current?.(next);
        } else {
          node.style.translate = "";
        }
        setLifted(null);
        setTarget(null);
        setDropping(false);
      }, SETTLE_MS);
    },
    [reset],
  );

  /* Новый порядок уже в DOM: сдвиг поднятой карточки снимаем до отрисовки. */
  useLayoutEffect(() => {
    if (!settling) return undefined;
    if (pendingClear.current) {
      pendingClear.current.style.translate = "";
      pendingClear.current = null;
    }
    const raf = requestAnimationFrame(() =>
      requestAnimationFrame(() => setSettling(false)),
    );
    return () => cancelAnimationFrame(raf);
  }, [settling]);

  const onPointerDown = useCallback(
    (key) => (event) => {
      if (!enabled || event.button !== 0 || drag.current || press.current) return;
      const head = event.target.closest('.rv-server-item[data-variant="subitem"], .rv-server-item[data-variant="myitem"]');
      if (!head || event.target.closest("button")) return;

      const p = { key, x: event.clientX, y: event.clientY, id: event.pointerId };
      p.timer = setTimeout(lift, HOLD_MS);
      press.current = p;
      setPressing(key);

      const move = (e) => {
        if (e.pointerId !== p.id) return;
        if (drag.current) {
          e.preventDefault();
          place(e.clientY);
        } else if (
          press.current &&
          Math.hypot(e.clientX - p.x, e.clientY - p.y) > MOVE_TOLERANCE
        ) {
          reset();
        }
      };
      const up = (e) => {
        if (e.pointerId !== p.id) return;
        if (drag.current) drop(e.type === "pointercancel");
        else reset();
      };
      const key_ = (e) => {
        if (e.key === "Escape") {
          if (drag.current) drop(true, true);
          else reset();
        }
      };
      const noSelect = (e) => e.preventDefault();
      window.addEventListener("pointermove", move, { passive: false });
      window.addEventListener("pointerup", up);
      window.addEventListener("pointercancel", up);
      window.addEventListener("keydown", key_);
      window.addEventListener("selectstart", noSelect);
      cleanupListeners.current = () => {
        window.removeEventListener("pointermove", move);
        window.removeEventListener("pointerup", up);
        window.removeEventListener("pointercancel", up);
        window.removeEventListener("keydown", key_);
        window.removeEventListener("selectstart", noSelect);
        cleanupListeners.current = () => {};
      };
    },
    [enabled, lift, place, drop, reset],
  );

  useEffect(() => reset, [reset]);

  /* Сдвиг соседа, пока поднятая карточка висит над чужим местом. */
  const shiftFor = (key) => {
    const d = drag.current;
    if (lifted == null || target == null || !d || key === lifted) return 0;
    const i = d.order.indexOf(key);
    const step = d.heights[d.from] + d.gap;
    if (d.from < target && i > d.from && i <= target) return -step;
    if (target < d.from && i >= target && i < d.from) return step;
    return 0;
  };

  const itemProps = (key) => {
    const shift = shiftFor(key);
    return {
      ref: register(key),
      onPointerDown: onPointerDown(key),
      onDragStart: (event) => event.preventDefault(),
      "data-pressing": pressing === key || undefined,
      "data-lifted": lifted === key || undefined,
      "data-dropping": (lifted === key && dropping) || undefined,
      style: lifted != null && key !== lifted ? { translate: `0 ${shift}px` } : undefined,
    };
  };

  return { itemProps, active: lifted != null, settling };
}
