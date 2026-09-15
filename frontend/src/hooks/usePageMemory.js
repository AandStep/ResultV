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
 * Память страницы: положение прокрутки и состояние её органов управления
 * переживают уход на другую страницу.
 *
 * Зачем она нужна. Страницу выбирает `activeTab`, и App.jsx рисует ровно
 * одну — остальные в этот момент размонтированы. Вместе с экраном умирает
 * весь его `useState` и прокрутка его DOM-узла, поэтому раскрытый список
 * серверов, набранный поиск и отмотанная страница возвращались в исходное
 * положение на каждом переходе по меню. Тем же самым уже болело боковое
 * меню — его раскрытость поэтому и живёт в ConfigContext.
 *
 * Почему хранилище лежит вне React-дерева. Пережить нужно именно
 * размонтирование, а состояние размонтированного поддерева не переживает
 * ничего. Контекст сработал бы тоже, но тогда каждая запомненная мелочь
 * (скролл на каждое движение колеса!) перерисовывала бы всё приложение
 * целиком. Модульная карта не перерисовывает никого.
 *
 * Живёт до перезапуска приложения и на диск не пишется: «вернулся на
 * страницу — она такая, какой я её оставил» — это про один сеанс работы, а
 * не про восстановление позиции спустя сутки.
 *
 * Чего память нарочно НЕ хранит — открытые диалоги. Вернуться на страницу и
 * получить всплывшее окно правки подписки это не сохранённое состояние, а
 * сюрприз; страницы держат такие окна в обычном `useState`.
 */

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";

/* Ключи страниц. Строкой их легко разойтись, поэтому они собраны здесь. */
export const PAGE_HOME = "home";
export const PAGE_SERVERS = "servers";
export const PAGE_RULES = "rules";
export const PAGE_ADD = "add";
export const PAGE_BUY = "buy";
export const PAGE_LOGS = "logs";
export const PAGE_SETTINGS = "settings";

/* pageKey -> { state: Map<stateKey, value>, scrollTop: number } */
const pages = new Map();

function bucket(pageKey) {
  let found = pages.get(pageKey);
  if (!found) {
    found = { state: new Map(), scrollTop: 0 };
    pages.set(pageKey, found);
  }
  return found;
}

/*
 * Сколько кадров ждём, пока содержимое дорастёт до запомненной высоты.
 *
 * Первым кадром список бывает короче, чем был: часть страниц дорисовывает
 * себя следом за первым рендером (карточки подписок ждут `getConfig`), и
 * прокрутка на 800 px по 200-пиксельному списку упёрлась бы в его низ. Тогда
 * пробуем ещё несколько кадров подряд — а не дождавшись, оставляем как
 * вышло: лучше показать начало страницы, чем дёргать её под курсором спустя
 * секунду.
 */
const RESTORE_FRAMES = 10;

/** Запомненное положение прокрутки страницы. */
export function readPageScroll(pageKey) {
  return bucket(pageKey).scrollTop;
}

/** Запомнить положение прокрутки страницы. */
export function writePageScroll(pageKey, scrollTop) {
  bucket(pageKey).scrollTop = Math.max(0, scrollTop || 0);
}

/**
 * `useState`, переживающий уход со страницы.
 *
 * Возвращает ту же пару, что и `useState`, и так же принимает начальное
 * значение либо функцию, которая его считает. При первом заходе берётся
 * начальное, при возвращении — то, на чём страницу оставили.
 */
export function usePageState(pageKey, stateKey, initial) {
  const store = bucket(pageKey).state;
  const [value, setValue] = useState(() =>
    store.has(stateKey)
      ? store.get(stateKey)
      : typeof initial === "function"
        ? initial()
        : initial,
  );

  /* Пишем в эффекте, а не внутри `setValue`: обновляющая функция обязана
     быть чистой — в строгом режиме React зовёт её дважды. */
  useEffect(() => {
    bucket(pageKey).state.set(stateKey, value);
  }, [pageKey, stateKey, value]);

  return [value, setValue];
}

/**
 * Прокрутка, переживающая уход со страницы.
 *
 * Возвращает `ref`, который вешается на прокручиваемый узел страницы
 * (`.rv-scroll`). Положение восстанавливается до первой отрисовки кадра —
 * в `useLayoutEffect`, иначе страница успела бы мигнуть началом.
 */
export function useScrollMemory(pageKey) {
  const ref = useRef(null);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return undefined;

    const saved = readPageScroll(pageKey);
    let frames = 0;
    let raf = 0;

    const restore = () => {
      raf = 0;
      el.scrollTop = saved;
      /* Браузер обрезает прокрутку по фактической высоте: не дотянули —
         значит содержимое ещё не всё. */
      if (el.scrollTop >= saved || frames >= RESTORE_FRAMES) return;
      frames += 1;
      raf = requestAnimationFrame(restore);
    };

    if (saved > 0) restore();

    const onScroll = () => writePageScroll(pageKey, el.scrollTop);
    el.addEventListener("scroll", onScroll, { passive: true });

    return () => {
      if (raf) cancelAnimationFrame(raf);
      el.removeEventListener("scroll", onScroll);
      /* Последнее движение колеса могло не успеть дойти событием. */
      writePageScroll(pageKey, el.scrollTop);
    };
  }, [pageKey]);

  return ref;
}

/**
 * Забыть всё, что запомнено о странице. Нужен там, где состояние страницы
 * перестало иметь смысл — например, форма добавления отработала и её
 * черновик держать больше незачем.
 */
export function forgetPage(pageKey) {
  pages.delete(pageKey);
}

/* Для тестов и отладки: снести память целиком. */
export function forgetAllPages() {
  pages.clear();
}
