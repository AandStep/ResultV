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
 * Главный экран нового дизайна, подключённый к настоящим данным приложения.
 *
 * Раскладку и внешний вид держит MainPage, здесь только связь с контекстами:
 * состояние подключения, выбранный сервер, трафик, пинг и режим.
 */

import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { BrowserOpenURL } from "../../../wailsjs/runtime/runtime";
import { useConfigContext } from "../../context/ConfigContext";
import { useConnectionContext } from "../../context/ConnectionContext";
import { FlagIcon } from "../../components/ui/FlagIcon";
import {
  formatProxyDisplayName,
  isEndpointProtocol,
} from "../../utils/proxyParser";
import {
  autoRowPingLabel,
  favoritesFirst,
  parseExtra,
  sortProxiesByOption,
} from "../../utils/pingSort";
import wailsAPI from "../../utils/wailsAPI";
import MainPage from "./MainPage";
import SortMenu from "./SortMenu";
import AppSidebar from "./AppSidebar";
import { formatRate, formatTraffic, protocolLabel } from "./format";

const WEBSITE_URL = "https://result-proxy.ru/";
const TELEGRAM_URL = "https://t.me/resultvpn";

/*
 * Замер приходит строкой: «90ms», «<1ms», «—» либо словом об отказе
 * («Timeout», «Refused», ...). В макете на этом месте «90мс», поэтому
 * миллисекунды переводим в русское сокращение, а отказ показываем как есть —
 * молчать о нём хуже, чем показать.
 */
function pingLabel(value, t) {
  const raw = value == null ? "" : String(value);
  if (!raw) return undefined;
  const ms = /^(<?\d+)ms$/.exec(raw);
  return ms ? `${ms[1]}${t("home.ms", "мс")}` : raw;
}

/*
 * Бейджи строки — это протокол и транспорт по отдельности: в макете у строк
 * списка их два («Hysteria2» + «gRPC»), а приложение отдаёт их одной строкой
 * через « + ».
 *
 * У авто-группы протокола нет: в ките на её месте набрано «Авто».
 */
function protocolBadges(proxy, t) {
  if (isAuto(proxy)) return [{ label: t("proxyList.autoType", "Авто") }];
  return protocolLabel(proxy)
    .split(" + ")
    .filter(Boolean)
    .map((label) => ({ label }));
}

function isAuto(proxy) {
  return proxy?.type?.toUpperCase() === "AUTO";
}

function formatUptime(totalSeconds) {
  const s = Math.max(0, Math.floor(totalSeconds || 0));
  const pad = (n) => String(n).padStart(2, "0");
  const hours = Math.floor(s / 3600);
  return hours > 0
    ? `${hours}:${pad(Math.floor((s % 3600) / 60))}:${pad(s % 60)}`
    : `${pad(Math.floor(s / 60))}:${pad(s % 60)}`;
}

/*
 * График трафика в том виде, в каком он нарисован в макете: ломаная линия
 * толщиной 3, без сглаживания и без заливки под ней.
 */
function TrafficChart({ data, color }) {
  if (!data || data.length < 2) return null;
  const max = Math.max(...data, 1024);
  const points = data
    .map((value, i) => `${(i / (data.length - 1)) * 100},${39.5 - (value / max) * 39.5}`)
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

export default function HomeScreen() {
  const { t } = useTranslation();
  const {
    proxies,
    subscriptions,
    settings,
    updateSetting,
    isApplyingMode,
    toggleFavorite,
    showAlertDialog,
    setActiveTab,
    setEditingProxy,
  } = useConfigContext();
  const {
    isConnected,
    isConnecting,
    isResolving,
    isDisconnecting,
    failedProxy,
    activeProxy,
    stats,
    speedHistory,
    uptime,
    pings,
    refreshPings,
    isManualPinging,
    isPingPending,
    selectAndConnect,
    toggleConnection,
    cancelConnect,
  } = useConnectionContext();

  const [listOpen, setListOpen] = useState(false);
  /*
   * Порядок списка и меню его выбора — те же семь вариантов, что были на
   * старой главной. Своего меню в макете пока нет, см. docs/design/GAPS.md;
   * до него работает старое, только вызванное от кнопки сортировки.
   */
  const [sortBy, setSortBy] = useState("default");
  const [sortAnchor, setSortAnchor] = useState(null);

  /*
   * Задержку авто-группы меряет не проба, а сам движок: он знает, какой узел
   * выбран, и отдаёт его RTT. Спрашиваем после каждого подключения — до него
   * узел не выбран, и строка показывает лучшее из членов группы.
   */
  const [autoStatusById, setAutoStatusById] = useState({});
  useEffect(() => {
    const autoIds = proxies.filter(isAuto).map((p) => String(p.id));
    if (autoIds.length === 0) return undefined;
    let cancelled = false;
    (async () => {
      const next = {};
      for (const id of autoIds) next[id] = await wailsAPI.getAutoGroupStatus(id);
      if (!cancelled) setAutoStatusById(next);
    })();
    return () => {
      cancelled = true;
    };
  }, [proxies, activeProxy, isConnected]);

  /*
   * Смена режима «прокси/туннель» — это переподключение: ядро рвёт соединение
   * и поднимает заново, а ConnectionContext на это время сам держит
   * `isConnecting`, чтобы экран не провалился в «отключено».
   *
   * Считать это подключением можно не всегда: на выключенном соединении
   * переключать нечего, и экран на кадр показывал «Подключение...». Но если
   * соединение было, переподключение настоящее — и раньше здесь стояло
   * огульное «пока режим применяется, занятость не считаем», из-за чего
   * `isConnected` на середине цикла падал в false и вместо жёлтого этапа
   * мелькало «Не подключено».
   *
   * Поэтому запоминаем, было ли соединение к началу применения режима: сам
   * `isConnected` внутри цикла мигает и на вопрос «а было ли что рвать» уже
   * не отвечает.
   */
  const wasConnectedRef = useRef(false);

  useEffect(() => {
    if (!isApplyingMode) wasConnectedRef.current = isConnected;
  }, [isConnected, isApplyingMode]);

  /* Считаем прямо в отрисовке, а не через состояние: состояние обновилось бы
     следующим кадром, и первый кадр смены режима успел бы показать старое. */
  const modeReconnect = isApplyingMode && wasConnectedRef.current;

  /*
   * Состояние экрана — то же, что в макете: четыре положения, каждое разом
   * задаёт цвет шапки, кнопки питания, подложки флага и бейджа.
   *
   * Переподключение под смену режима держит жёлтый этап до конца и потому
   * стоит выше `isConnected`: иначе экран моргал бы зелёным на первом кадре и
   * на последнем.
   */
  const busy =
    modeReconnect ||
    (!isApplyingMode && (isConnecting || isResolving || isDisconnecting));

  /*
   * Отключение стоит выше ошибки нарочно. Разрыв соединения гасит движок, и
   * пока он гаснет, наверх успевает всплыть упавший сервер — на кадр
   * загорался красный «Что-то пошло не так» на ровном месте. Пока рвём
   * соединение, показываем именно это, а на упавший сервер посмотрим, когда
   * разорвём.
   */
  const status = isDisconnecting
    ? "disconnecting"
    : failedProxy
      ? "error"
      : modeReconnect
        ? "connecting"
        : isConnected
          ? "connected"
          : busy
            ? "connecting"
            : "idle";

  /*
   * Нажатие на кнопку во время запуска его останавливает — в том числе на
   * подборе узла авто-группы: сам замер оборвать нечем, но подключаться после
   * него уже не станут, а экран уходит в «Отключение...» сразу.
   *
   * Не нажимается кнопка только там, где обратного хода нет: пока рвём
   * соединение и пока идёт переподключение под смену режима.
   */
  const canCancel = isResolving || (status === "connecting" && !modeReconnect);
  const powerBusy = isDisconnecting || modeReconnect;

  /*
   * Показываем тот же сервер, что и раньше: упавший, затем подключённый,
   * затем последний выбранный, затем первый в списке.
   */
  const proxy = useMemo(() => {
    const chain = [
      failedProxy,
      activeProxy,
      proxies.find((p) => String(p.id) === String(settings?.lastSelectedProxyId)),
      proxies[0],
    ];
    return chain.find(Boolean);
  }, [failedProxy, activeProxy, proxies, settings?.lastSelectedProxyId]);

  /*
   * WireGuard и AmneziaWG — не прокси, а сетевой интерфейс: работают они
   * только через TUN. Выбрали такой сервер — режим сам уходит в туннель, а
   * вернуть его в «прокси» нельзя, и тумблер об этом говорит вслух, а не
   * молча не срабатывает.
   */
  const endpointOnly = isEndpointProtocol(proxy);

  useEffect(() => {
    if (endpointOnly && settings?.mode !== "tunnel") updateSetting("mode", "tunnel");
  }, [endpointOnly, settings?.mode, updateSetting]);

  const onModeChange = (value) => {
    if (value === "proxy" && endpointOnly) {
      showAlertDialog({
        title: t("common.notice"),
        message: t("tunnel.endpointOnly"),
        variant: "warning",
      });
      return;
    }
    updateSetting("mode", value);
  };

  /*
   * Задержка строки: у обычного сервера это его собственный замер, у
   * авто-группы — RTT выбранного движком узла.
   */
  const rowPing = (p) =>
    pingLabel(
      isAuto(p) ? autoRowPingLabel(p, pings ?? {}, autoStatusById[p.id]) : pings?.[p.id],
      t,
    );

  const server = proxy && {
    auto: isAuto(proxy),
    flag: isAuto(proxy) ? undefined : (
      <FlagIcon code={proxy.country} className="rv-flag__img" />
    ),
    badges: protocolBadges(proxy, t),
    title: formatProxyDisplayName(proxy.name, proxy.country) || proxy.name,
    ping: rowPing(proxy),
    pingBusy: isPingPending(proxy),
  };

  const favorites = useMemo(
    () => new Set((settings?.favorites || []).map(String)),
    [settings?.favorites],
  );

  /*
   * Члены авто-групп в списке не показываются: в подписке их три десятка, и
   * рядом со своей же группой они только сбивают с толку — выбирают всегда
   * группу, а узел внутри неё подбирает движок. В списке остаются сами
   * авто-группы и обычные сервера.
   *
   * Подписку целиком можно убрать отсюда её же настройками («Показывать на
   * главной» на странице серверов): у кого их несколько, тому здесь нужна не
   * вся тысяча узлов, а та подписка, которой он пользуется.
   */
  const hidden = useMemo(
    () =>
      new Set(
        (subscriptions || []).filter((s) => s.showOnHome === false).map((s) => s.url),
      ),
    [subscriptions],
  );

  const listed = useMemo(() => {
    const members = new Set();
    for (const p of proxies) {
      if (!isAuto(p)) continue;
      for (const id of parseExtra(p.extra)?.members || []) members.add(String(id));
    }
    return proxies.filter(
      (p) => !members.has(String(p.id)) && !hidden.has(p.subscriptionUrl),
    );
  }, [proxies, hidden]);

  /*
   * Строка раскрытого списка. Выбор ведёт себя как на старой главной:
   * подключает и закрывает список.
   *
   * У авто-группы своя строка кита (`autoserver`): вместо флага — значок
   * автовыбора, а бейдж набран «Авто».
   */
  const toRow = (p) => ({
    key: String(p.id),
    variant: isAuto(p) ? "autoserver" : "row",
    flag: isAuto(p) ? undefined : <FlagIcon code={p.country} className="rv-flag__img" />,
    badges: protocolBadges(p, t),
    title: formatProxyDisplayName(p.name, p.country) || p.name,
    ping: rowPing(p),
    /* Задержку ещё меряют — на её месте спиннер, а не пустота. */
    pingBusy: isPingPending(p),
    favorite: favorites.has(String(p.id)),
    onFavorite: () => toggleFavorite(p.id),
    onSelect: () => {
      selectAndConnect(p);
      setListOpen(false);
    },
  });

  /*
   * Список разбит на группы по подпискам, «Мои сервера» идут последними
   * (фрейм 6504:3878). Группируем по `subscriptionUrl`, а не по имени
   * провайдера: имя подписки переименовывают, и группы разъехались бы.
   *
   * Подпись у группы появляется, только когда групп больше одной: с
   * единственной подпиской подписывать нечего.
   *
   * Внутри группы работает выбранный порядок, а избранное поверх него всплывает
   * наверх — звёздочка закрепляет сервер, а не задаёт ещё одну сортировку.
   */
  const serverGroups = useMemo(() => {
    const known = new Set((subscriptions || []).map((s) => s.url));
    const order = (list) =>
      favoritesFirst(
        sortProxiesByOption(list, sortBy, pings ?? {}, autoStatusById),
        favorites,
      );

    const out = [];
    for (const sub of subscriptions || []) {
      const own = listed.filter((p) => p.subscriptionUrl === sub.url);
      if (own.length === 0) continue;
      out.push({ key: String(sub.id), label: sub.name, proxies: own });
    }

    /* Всё, что не принадлежит ни одной заведённой подписке, включая сервера
       осиротевшей: иначе они пропали бы с главной совсем. */
    const manual = listed.filter(
      (p) => !p.subscriptionUrl || !known.has(p.subscriptionUrl),
    );
    if (manual.length > 0) {
      out.push({ key: "my", label: t("serversPage.myServers"), proxies: manual });
    }

    const single = out.length < 2;
    return out.map((g) => ({
      key: g.key,
      label: single ? null : g.label,
      servers: order(g.proxies).map(toRow),
    }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    listed,
    subscriptions,
    sortBy,
    pings,
    autoStatusById,
    isPingPending,
    favorites,
    toggleFavorite,
    selectAndConnect,
    t,
  ]);

  const rowCount = serverGroups.reduce((n, g) => n + g.servers.length, 0);

  /* Порядки те же и в том же составе, что на старой главной; «по провайдеру»
     показываем только когда провайдеры вообще есть. */
  const sortOptions = useMemo(
    () =>
      ["default", "newest", "oldest", "country", "type", "provider", "ping"]
        .filter((o) => o !== "provider" || proxies.some((p) => p.provider))
        .map((value) => ({ value, label: t(`proxyList.sort.${value}`) })),
    [proxies, t],
  );

  const text = {
    idle: t("home.status.unprotected"),
    connecting: t("home.status.connecting"),
    connected: t("home.status.protected"),
    error: t("home.status.error"),
    disconnecting: t("home.status.disconnecting"),
    resolving: t("home.status.resolving"),
    proxy: t("home.mode.proxy", "Прокси"),
    tunnel: t("home.mode.tunnel", "Туннель"),
    download: t("home.download"),
    upload: t("home.upload", "Отправлено"),
    editData: t("home.editData"),
    otherServer: t("home.chooseOther"),
    pingServers: t("home.pingTooltip"),
    sortServers: t("home.sortMenuTooltip"),
    empty: t("home.empty"),
  };

  return (
    <>
    <MainPage
      status={status}
      /* Подбор узла идёт и поверх живого соединения: страница остаётся
         зелёной, а в «идёт подбор» уходят заголовок и кнопка питания —
         кнопка в этот момент работает на отмену и зелёной быть не должна. */
      resolving={isResolving}
      text={text}
      time={formatUptime(uptime)}
      mode={settings?.mode === "tunnel" ? "tunnel" : "proxy"}
      onModeChange={onModeChange}
      onPower={canCancel ? cancelConnect : toggleConnection}
      powerDisabled={powerBusy}
      server={server}
      serverGroups={serverGroups}
      /* Серверов нет вовсе — вместо карточки приглашение их добавить. */
      empty={proxies.length === 0}
      onAddServer={() => {
        setEditingProxy(null);
        setActiveTab("add");
      }}
      /* Пустой список раскрывать незачем: карточка осталась бы такой же. */
      serverListOpen={listOpen && rowCount > 0}
      onToggleServerList={() => setListOpen((v) => !v)}
      /* Без обёртки сюда прилетел бы объект события, и `refreshPings` принял
         бы его за список идентификаторов. */
      onPingServers={() => refreshPings()}
      pingBusy={isManualPinging}
      /* Повторное нажатие закрывает меню. Кнопку берём сразу: к моменту
         обновления состояния `currentTarget` у события уже сброшен. */
      onSortServers={(event) => {
        const button = event.currentTarget;
        setSortAnchor((current) => (current ? null : button));
      }}
      download={{
        rate: formatRate(speedHistory?.down?.[19], t),
        total: formatTraffic(stats?.download, t),
      }}
      upload={{
        rate: formatRate(speedHistory?.up?.[19], t),
        total: formatTraffic(stats?.upload, t),
      }}
      downloadChart={
        <TrafficChart data={speedHistory?.down} color="var(--rv-main-color)" />
      }
      uploadChart={
        <TrafficChart data={speedHistory?.up} color="var(--rv-second-color)" />
      }
      onSite={() => BrowserOpenURL(WEBSITE_URL)}
      onTelegram={() => BrowserOpenURL(TELEGRAM_URL)}
      sidebar={<AppSidebar />}
    />
    <SortMenu
      anchor={sortAnchor}
      value={sortBy}
      options={sortOptions}
      onPick={(option) => {
        setSortBy(option);
        setSortAnchor(null);
      }}
      onClose={() => setSortAnchor(null)}
    />
    </>
  );
}
