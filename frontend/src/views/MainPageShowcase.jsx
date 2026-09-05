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
 * Главный экран в размере окна 1000x740 — одной живой страницей.
 *
 * Состояния не разложены по кадрам, а проходятся кликами: кнопка питания ведёт
 * из «Не подключено» в «Подключение...» и дальше в «Защищено», повторный клик
 * возвращает обратно. Переключатель сверху нужен, чтобы попасть в состояние
 * сразу, минуя ожидание, — в том числе в ошибку.
 */

import { useEffect, useMemo, useRef, useState } from "react";
import { Sidebar } from "../components/kit";
import MainPage from "./redesign/MainPage";
import SortMenu from "./redesign/SortMenu";
import { FlagStub, MENU_ITEMS, SETTINGS_ITEM } from "./showcase-stubs";
import "./KitShowcase.css";

const SERVER = {
  flag: <FlagStub />,
  badges: [{ label: "Hysteria2" }],
  title: "Россия | Hysteria2",
  ping: "90мс",
};

/*
 * Строки раскрытого списка. Первые две — как в макете; дальше авто-группа
 * (у неё вместо флага значок автовыбора и бейдж «Авто») и ещё несколько
 * серверов, чтобы было видно, как список удлиняет страницу.
 */
const SERVER_ROWS = [
  { key: "1", badges: [{ label: "Hysteria2" }, { label: "gRPC" }] },
  { key: "2", badges: [{ label: "VLESS" }, { label: "gRPC" }] },
  { key: "3", variant: "autoserver", badges: [{ label: "Авто" }], title: "Авто сервер" },
  { key: "4", badges: [{ label: "Trojan" }] },
  { key: "5", badges: [{ label: "VLESS" }, { label: "Reality" }] },
];

const STATES = [
  { key: "idle", label: "Не подключено" },
  { key: "connecting", label: "Подключение..." },
  { key: "connected", label: "Защищено" },
  { key: "disconnecting", label: "Отключение..." },
  { key: "error", label: "Ошибка" },
];

const ZERO = { rate: "0 кб/с", total: "0 Мб" };

/* Временное меню порядка серверов — своего в макете пока нет. */
const SORT_OPTIONS = [
  { value: "default", label: "По умолчанию" },
  { value: "newest", label: "Новые" },
  { value: "oldest", label: "Старые" },
  { value: "country", label: "По стране" },
  { value: "type", label: "По типу" },
  { value: "provider", label: "По провайдеру" },
  { value: "ping", label: "По пингу" },
];

/* Ломаная той же формы, что в макете: толщина 3, без сглаживания и заливки. */
function Chart({ data, color }) {
  const points = data
    .map((value, i) => `${(i / (data.length - 1)) * 100},${39.5 - value * 39.5}`)
    .join(" ");
  return (
    <svg
      className="rv-main-page__chart"
      viewBox="0 0 100 39.5"
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      <polyline
        fill="none"
        stroke={color}
        strokeWidth="3"
        vectorEffect="non-scaling-stroke"
        points={points}
      />
    </svg>
  );
}

/* Ряд правдоподобных значений, чтобы график не был плоским. */
function useTraffic(active) {
  const [tick, setTick] = useState(0);
  useEffect(() => {
    if (!active) return undefined;
    const id = window.setInterval(() => setTick((v) => v + 1), 1000);
    return () => window.clearInterval(id);
  }, [active]);

  return useMemo(() => {
    const make = (seed) =>
      Array.from({ length: 20 }, (_, i) => {
        const x = i + tick;
        return 0.25 + 0.35 * Math.abs(Math.sin(x * seed)) + 0.2 * Math.abs(Math.cos(x * seed * 1.7));
      });
    return { down: make(0.7), up: make(1.1) };
  }, [tick]);
}

export default function MainPageShowcase() {
  const [status, setStatus] = useState("idle");
  const [mode, setMode] = useState("proxy");
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [listOpen, setListOpen] = useState(false);
  const [favorites, setFavorites] = useState(() => new Set());
  const [selected, setSelected] = useState(SERVER);
  const [resolving, setResolving] = useState(false);
  const [sortBy, setSortBy] = useState("default");
  const [sortAnchor, setSortAnchor] = useState(null);
  const [seconds, setSeconds] = useState(0);
  const timerRef = useRef(null);
  const traffic = useTraffic(status === "connected");

  /* Подключение занимает пару секунд, как в жизни. */
  useEffect(() => {
    if (status !== "connecting") return undefined;
    const id = window.setTimeout(() => setStatus("connected"), 2000);
    return () => window.clearTimeout(id);
  }, [status]);

  useEffect(() => () => pendingRef.current.forEach(window.clearTimeout), []);

  /* Время в шапке идёт, пока соединение живо. */
  useEffect(() => {
    if (status !== "connected") {
      setSeconds(0);
      return undefined;
    }
    timerRef.current = window.setInterval(() => setSeconds((v) => v + 1), 1000);
    return () => window.clearInterval(timerRef.current);
  }, [status]);

  const time = `${String(Math.floor(seconds / 60)).padStart(2, "0")}:${String(
    seconds % 60
  ).padStart(2, "0")}`;

  /*
   * Подбор узла — отдельная короткая фаза перед подключением: так ведёт себя
   * авто-группа. Поверх живого соединения она тоже случается, тогда страница
   * остаётся зелёной и жёлтым отзывается только заголовок.
   */
  const pendingRef = useRef([]);
  const later = (fn, ms) => pendingRef.current.push(window.setTimeout(fn, ms));

  /*
   * Подбор идёт уже внутри «Подключения»: в приложении он поднимает те же
   * флаги занятости, экран сразу жёлтый и вдавленный, а заголовок на это
   * время говорит «Подбор сервера».
   */
  const resolve = () => {
    setStatus("connecting");
    setResolving(true);
    later(() => setResolving(false), 900);
  };

  /*
   * Нажатие во время запуска его останавливает — в том числе на подборе. И
   * остановка, и обычное отключение проходят через одно состояние
   * «Отключение...»: жёлтая вдавленная кнопка, как при подключении.
   */
  const stop = () => {
    pendingRef.current.forEach(window.clearTimeout);
    pendingRef.current = [];
    setResolving(false);
    setStatus("disconnecting");
    later(() => setStatus("idle"), 900);
  };

  const onPower = () => {
    if (status === "idle" || status === "error") resolve();
    else stop();
  };

  /*
   * Смена режима при живом соединении — это переподключение: ядро рвёт
   * соединение и поднимает заново, и всё это время экран держит жёлтый этап,
   * не проваливаясь в «Не подключено». На выключенном рвать нечего, и
   * состояние не меняется вовсе.
   */
  const onModeChange = (value) => {
    setMode(value);
    if (status !== "connected") return;
    setStatus("connecting");
    window.setTimeout(() => setStatus("connected"), 1200);
  };

  const connected = status === "connected";

  const servers = SERVER_ROWS.map((row) => ({
    title: "Россия | Hysteria2",
    ...row,
    flag: row.variant === "autoserver" ? undefined : <FlagStub />,
    ping: "90мс",
    /* Выбор строки поднимает её в шапку карточки — как в приложении, а у
       авто-группы сперва проходит подбор узла. */
    onSelect: () => {
      if (row.variant === "autoserver") {
        /* Подбор поверх живого соединения статус не трогает: страница
           остаётся зелёной, жёлтым отзывается только заголовок. */
        setResolving(true);
        later(() => setResolving(false), 900);
      }
      setSelected({
        auto: row.variant === "autoserver",
        flag: row.variant === "autoserver" ? undefined : <FlagStub />,
        badges: row.badges,
        title: row.title ?? "Россия | Hysteria2",
        ping: "90мс",
      });
    },
    favorite: favorites.has(row.key),
    onFavorite: () =>
      setFavorites((prev) => {
        const next = new Set(prev);
        if (next.has(row.key)) next.delete(row.key);
        else next.add(row.key);
        return next;
      }),
  }));

  return (
    <div className="kit">
      <div className="kit__intro">
        <h1>Главный экран</h1>
        <p>
          Окно 1000×740, как в макете. Нажмите кнопку питания — экран пройдёт
          путь «Подбор сервера» → «Подключение...» → «Защищено», а нажатие на
          любом из этих шагов останавливает запуск через «Отключение...».
          Переключатель ниже перебрасывает в нужное состояние сразу.
        </p>
        <p>
          Переключатель режима и сайдбар тоже живые: логотип разворачивает
          меню. Карточка сервера раскрывается по нажатию на неё — в шапке
          появляются пинг и сортировка, а плитки скорости уступают место
          списку. Наведение и нажатие работают сами, ничего не форсируется.
          Флаг — заглушка из макета, трафик придуманный.
        </p>
      </div>

      <div className="kit__nav">
        {STATES.map((item) => (
          <a
            key={item.key}
            href="#main"
            aria-current={status === item.key ? "page" : undefined}
            onClick={(e) => {
              e.preventDefault();
              setStatus(item.key);
            }}
          >
            {item.label}
          </a>
        ))}
      </div>

      <div className="kit__frame">
        <MainPage
          status={status}
          resolving={resolving}
          time={time}
          mode={mode}
          onModeChange={onModeChange}
          onPower={onPower}
          powerDisabled={status === "disconnecting"}
          server={selected}
          /* Витрина показывает одну группу — без подписи, как в приложении
             с единственной подпиской. */
          serverGroups={[{ key: "all", label: null, servers }]}
          serverListOpen={listOpen}
          onToggleServerList={() => setListOpen((v) => !v)}
          onSortServers={(event) => {
            const button = event.currentTarget;
            setSortAnchor((current) => (current ? null : button));
          }}
          download={connected ? { rate: "312 кб/с", total: "312 Мб" } : ZERO}
          upload={connected ? { rate: "12 кб/с", total: "12 Мб" } : ZERO}
          downloadChart={<Chart data={traffic.down} color="var(--rv-main-color)" />}
          uploadChart={<Chart data={traffic.up} color="var(--rv-second-color)" />}
          sidebar={
            <Sidebar
              opened={sidebarOpen}
              onToggle={() => setSidebarOpen((v) => !v)}
              items={MENU_ITEMS}
              bottomItem={SETTINGS_ITEM}
              activeKey="home"
            />
          }
        />
      </div>

      <SortMenu
        anchor={sortAnchor}
        value={sortBy}
        options={SORT_OPTIONS}
        onPick={(option) => {
          setSortBy(option);
          setSortAnchor(null);
        }}
        onClose={() => setSortAnchor(null)}
      />
    </div>
  );
}
