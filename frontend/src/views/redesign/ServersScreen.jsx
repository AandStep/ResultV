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
 * Страница серверов нового дизайна, подключённая к данным приложения.
 *
 * Вид держит ServersPage, здесь только связь с контекстами: подписки и их
 * серверы, серверы, заведённые вручную, поиск, порядок, замер задержки и окно
 * настроек подписки.
 *
 * Группы собираются по подпискам, а не по полю `provider`, как это делала
 * прежняя страница: имя подписки теперь можно менять, и группировка по имени
 * провайдера разъезжалась бы на две после первого же переименования.
 */

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { BrowserOpenURL, EventsOff, EventsOn } from "../../../wailsjs/runtime/runtime";
import { useConfigContext } from "../../context/ConfigContext";
import { useConnectionContext } from "../../context/ConnectionContext";
import { useToast } from "../../context/ToastContext";
import { FlagIcon } from "../../components/ui/FlagIcon";
import {
  formatProxyDisplayName,
  mergeSubscriptionRefreshCountries,
} from "../../utils/proxyParser";
import {
  autoRowPingLabel,
  favoritesFirst,
  parseExtra,
  sortProxiesByOption,
} from "../../utils/pingSort";
import wailsAPI from "../../utils/wailsAPI";
import ServerEditor, { SERVER_EDITOR_TEXT } from "./ServerEditor";
import ServersPage from "./ServersPage";
import SortMenu from "./SortMenu";
import SubscriptionDialog, { SUBSCRIPTION_INTERVALS } from "./SubscriptionDialog";
import SubscriptionLogo, { subscriptionSupportURL } from "./SubscriptionLogo";
import AppSidebar from "./AppSidebar";
import { formatTraffic, protocolLabel } from "./format";

const isAuto = (proxy) => proxy?.type?.toUpperCase() === "AUTO";

/*
 * Подтверждение на удаление своего сервера спрашиваем один раз за запуск:
 * дальше корзина срабатывает сразу, а о смене поведения говорит тост. Флаг
 * живёт вне компонента — страница перемонтируется на каждом переходе по
 * меню, и в состоянии он обнулялся бы вместе с ней. Подписку и группу «Мои
 * сервера» это не касается: там за одно нажатие уходит весь список.
 */
let deleteConfirmShown = false;

/* Те же правила, что на главной: «90ms» -> «90мс», отказ показываем как есть. */
function pingLabel(value, t) {
  const raw = value == null ? "" : String(value);
  if (!raw) return undefined;
  const ms = /^(<?\d+)ms$/.exec(raw);
  return ms ? `${ms[1]}${t("home.ms", "мс")}` : raw;
}

function protocolBadges(proxy, t) {
  if (isAuto(proxy)) return [{ label: t("proxyList.autoType", "Авто") }];
  return protocolLabel(proxy)
    .split(" + ")
    .filter(Boolean)
    .map((label) => ({ label }));
}

/* «16.08.26 в 21:32» — как набрано в макете. */
function expireDate(unix, t) {
  if (!unix || unix <= 0) return "";
  const d = new Date(unix * 1000);
  const pad = (n) => String(n).padStart(2, "0");
  const date = `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${String(d.getFullYear()).slice(-2)}`;
  return t("serversPage.subDate", {
    date,
    time: `${pad(d.getHours())}:${pad(d.getMinutes())}`,
  });
}

export default function ServersScreen() {
  const { t } = useTranslation();
  const {
    proxies,
    setProxies,
    subscriptions,
    setSubscriptions,
    syncRoutingLists,
    settings,
    toggleFavorite,
    showConfirmDialog,
    handleSaveProxy,
    setActiveTab,
    setEditingProxy,
  } = useConfigContext();
  const {
    activeProxy,
    setActiveProxy,
    failedProxy,
    setFailedProxy,
    isConnected,
    isConnecting,
    isResolving,
    isDisconnecting,
    pings,
    refreshPings,
    isManualPinging,
    isPingPending,
    isManualPingPending,
    selectAndConnect,
    deleteProxy,
  } = useConnectionContext();

  const { showToast } = useToast();

  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState("default");
  const [sortAnchor, setSortAnchor] = useState(null);
  const [openGroups, setOpenGroups] = useState({});
  const [editingSub, setEditingSub] = useState(null);
  /* Сервер, открытый в окне правки. Только свой: узел подписки править
     бессмысленно — ближайшее её обновление вернёт всё как было. */
  const [editingServer, setEditingServer] = useState(null);
  /* Подписки, которые сейчас перечитываются: у них крутится значок обновления.
     Набор, а не флаг, — обновлять можно каждую по отдельности. */
  const [refreshingSubs, setRefreshingSubs] = useState(() => new Set());

  /* Задержку авто-группы отдаёт движок, а не проба, — как и на главной. */
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
   * Узел может пройти все проверки и всё-таки не пропускать UDP — тогда на нём
   * молча не работают звонки и демонстрации Discord. Ядро сообщает об этом
   * через пару секунд после подключения в туннеле. В макете места под такую
   * отметку нет, поэтому она сделана третьим бейджем и только у тех узлов, где
   * UDP не прошёл: у остальных отмечать нечего. См. docs/design/GAPS.md.
   */
  const [udpRelay, setUdpRelay] = useState({});
  useEffect(() => {
    EventsOn("proxy:udp-relay", (payload) => {
      if (!payload?.proxyId) return;
      setUdpRelay((prev) => ({ ...prev, [payload.proxyId]: !!payload.ok }));
    });
    return () => EventsOff("proxy:udp-relay");
  }, []);

  const favorites = useMemo(
    () => new Set((settings?.favorites || []).map(String)),
    [settings?.favorites],
  );

  /* Члены авто-групп в списке не показываются: выбирают всегда группу. */
  const listed = useMemo(() => {
    const members = new Set();
    for (const p of proxies) {
      if (!isAuto(p)) continue;
      for (const id of parseExtra(p.extra)?.members || []) members.add(String(id));
    }
    return proxies.filter((p) => !members.has(String(p.id)));
  }, [proxies]);

  const matches = useCallback(
    (proxy) => {
      const q = search.trim().toLowerCase();
      if (!q) return true;
      return (
        String(proxy.name || "").toLowerCase().includes(q) ||
        String(proxy.ip || "").toLowerCase().includes(q)
      );
    },
    [search],
  );

  const rowPing = useCallback(
    (p) =>
      pingLabel(
        isAuto(p) ? autoRowPingLabel(p, pings ?? {}, autoStatusById[p.id]) : pings?.[p.id],
        t,
      ),
    [pings, autoStatusById, t],
  );

  /* Запуск идёт — те же флаги, из которых главная собирает свой жёлтый этап. */
  const busy = isConnecting || isResolving || isDisconnecting;

  const removeServer = useCallback(
    async (proxy) => {
      const first = !deleteConfirmShown;
      if (first) {
        const ok = await showConfirmDialog({
          title: t("common.confirmAction"),
          message: t("proxyList.confirmDelete", { name: proxy.name }),
          variant: "danger",
          confirmText: t("common.delete"),
          cancelText: t("common.cancel"),
        });
        if (!ok) return;
      }
      await deleteProxy(proxy.id, setProxies);
      if (first) {
        deleteConfirmShown = true;
        showToast({
          variant: "info",
          message: t("toast.deleteWithoutConfirm"),
          /* Длиннее обычного: фразу такой длины за четыре секунды не прочесть. */
          duration: 6000,
        });
      }
    },
    [showConfirmDialog, showToast, t, deleteProxy, setProxies],
  );

  /* `own` — строка своего сервера: только у неё есть карандаш и корзина. */
  const toRow = useCallback(
    (p, own = false) => {
      /*
       * Подключённый сервер подсвечен зелёным — фрейм 6744:4162. Пока к
       * выбранному только идёт подключение, флаг и бейджи держат жёлтый, как
       * шапка на главной: у авто-группы один подбор узла занимает секунды, и
       * без этого нажатие на строку проваливалось в тишину до самого конца
       * запуска. Подложку строки жёлтый не трогает — она в макете есть только
       * у подключённого.
       */
      const target = String(activeProxy?.id) === String(p.id);
      const current = isConnected && target;
      const accent = !target ? "default" : busy ? "warning" : current ? "success" : "default";
      const badges = protocolBadges(p, t);
      if (udpRelay[p.id] === false) {
        badges.push({
          label: "UDP",
          color: "error",
          title: t("proxyList.udpRelayFailTooltip"),
        });
      }
      return {
        key: String(p.id),
        variant: isAuto(p) ? "autoserver" : "row",
        flag: isAuto(p) ? undefined : <FlagIcon code={p.country} className="rv-flag__img" />,
        badges,
        title: formatProxyDisplayName(p.name, p.country) || p.name,
        ping: rowPing(p),
        pingBusy: isPingPending(p),
        favorite: favorites.has(String(p.id)),
        active: current,
        accent,
        onFavorite: () => toggleFavorite(p.id),
        /* Состав авто-группы задаёт провайдер, а не пользователь: править в
           ней нечего, поэтому карандаша у неё нет. Удалить её можно. */
        onEdit: own && !isAuto(p) ? () => setEditingServer(p) : undefined,
        onDelete: own ? () => removeServer(p) : undefined,
        onSelect: () => selectAndConnect(p),
      };
    },
    [
      t,
      rowPing,
      isPingPending,
      favorites,
      isConnected,
      busy,
      activeProxy,
      toggleFavorite,
      selectAndConnect,
      removeServer,
      udpRelay,
    ],
  );

  /* Избранное поднимается наверх своей группы поверх выбранного порядка:
     звёздочка — это закрепление, а не ещё одна сортировка. */
  const sortRows = useCallback(
    (list) =>
      favoritesFirst(
        sortProxiesByOption(list, sortBy, pings ?? {}, autoStatusById),
        favorites,
      ),
    [sortBy, pings, autoStatusById, favorites],
  );

  /* --- Действия над подписками -------------------------------------------- */

  const reloadConfig = useCallback(async () => {
    const cfg = await wailsAPI.getConfig();
    /* Именно `Array.isArray`, а не «поле непустое»: когда удалили последнюю
       подписку, список приходит пустым, и проверка на истинность приняла бы
       это за «ответа нет» и оставила бы в состоянии удалённую подписку. */
    if (Array.isArray(cfg?.subscriptions)) setSubscriptions(cfg.subscriptions);
    if (cfg?.routingRules?.routingLists) syncRoutingLists(cfg.routingRules.routingLists);
  }, [setSubscriptions, syncRoutingLists]);

  const markRefreshing = useCallback((subID, busy) => {
    setRefreshingSubs((prev) => {
      const next = new Set(prev);
      if (busy) next.add(subID);
      else next.delete(subID);
      return next;
    });
  }, []);

  const refreshSubscription = useCallback(
    async (sub) => {
      markRefreshing(sub.id, true);
      try {
        const updated = await wailsAPI.refreshSubscription(sub.id);
        if (updated?.length) {
          setProxies((prev) => [
            ...prev.filter((p) => p.subscriptionUrl !== sub.url),
            ...mergeSubscriptionRefreshCountries(prev, updated, sub.url),
          ]);
        }
        await reloadConfig();
      } catch (err) {
        console.error("Refresh subscription error:", err);
      } finally {
        markRefreshing(sub.id, false);
      }
    },
    [setProxies, reloadConfig, markRefreshing],
  );

  const removeSubscription = useCallback(
    async (sub) => {
      const ok = await showConfirmDialog({
        title: t("common.confirmAction"),
        message: t("proxyList.confirmDeleteSubscription", { name: sub.name || sub.url }),
        variant: "danger",
        confirmText: t("common.delete"),
        cancelText: t("common.cancel"),
      });
      if (!ok) return;
      try {
        await wailsAPI.deleteSubscription(sub.id);
        /*
         * Убираем и серверы, и саму подписку, и делаем это здесь, а не ждём
         * перечитывания конфигурации: на диске подписка ушла одним сохранением
         * вместе со своими серверами, и на странице она должна пропасть тем же
         * движением. Раньше от удаления оставался её заголовок над пустотой.
         */
        setProxies((prev) => prev.filter((p) => p.subscriptionUrl !== sub.url));
        setSubscriptions((prev) => prev.filter((item) => item.id !== sub.id));
        await reloadConfig();
      } catch (err) {
        console.error("Delete subscription error:", err);
      }
    },
    [showConfirmDialog, t, setProxies, setSubscriptions, reloadConfig],
  );

  const removeManual = useCallback(
    async (list) => {
      if (list.length === 0) return;
      const ok = await showConfirmDialog({
        title: t("common.confirmAction"),
        message: t("proxyList.confirmDeleteMyProxies", { count: list.length }),
        variant: "danger",
        confirmText: t("common.delete"),
        cancelText: t("common.cancel"),
      });
      if (!ok) return;
      for (const p of list) await deleteProxy(p.id, setProxies);
    },
    [showConfirmDialog, t, deleteProxy, setProxies],
  );

  /* --- Группы -------------------------------------------------------------- */

  const groups = useMemo(() => {
    const known = new Set((subscriptions || []).map((s) => s.url));
    const searching = search.trim() !== "";
    const out = [];

    for (const sub of subscriptions || []) {
      const own = listed.filter((p) => p.subscriptionUrl === sub.url);
      const found = own.filter(matches);
      if (searching && found.length === 0) continue;

      const used = (sub.trafficUpload ?? 0) + (sub.trafficDownload ?? 0);
      const meta = {
        used: formatTraffic(used, t),
        total:
          sub.trafficTotal > 0
            ? formatTraffic(sub.trafficTotal, t)
            : t("proxyList.subUnlimited"),
      };
      const date = expireDate(sub.expireUnix, t);

      out.push({
        key: sub.id,
        variant: "subitem",
        logo: <SubscriptionLogo subscription={sub} />,
        title: sub.name,
        count: own.length,
        subtitle: date
          ? t("serversPage.subMeta", { date, ...meta })
          : t("serversPage.subMetaNoDate", meta),
        /* Во время поиска карточка раскрыта сама: иначе найденное осталось бы
           спрятанным внутри свёрнутой группы. */
        open: searching || !!openGroups[sub.id],
        onToggle: () => setOpenGroups((prev) => ({ ...prev, [sub.id]: !prev[sub.id] })),
        onEdit: () => setEditingSub(sub),
        onSync: () => refreshSubscription(sub),
        syncBusy: refreshingSubs.has(sub.id),
        onDelete: () => removeSubscription(sub),
        servers: sortRows(found).map((p) => toRow(p)),
      });
    }

    /*
     * «Мои сервера» — всё, что не принадлежит ни одной заведённой подписке.
     * Сервер осиротевшей подписки попадает сюда же: иначе он пропал бы со
     * страницы совсем.
     */
    const manual = listed.filter((p) => !p.subscriptionUrl || !known.has(p.subscriptionUrl));
    const manualFound = manual.filter(matches);
    if (!searching || manualFound.length > 0) {
      out.push({
        key: "my",
        variant: "myitem",
        title: t("serversPage.myServers"),
        count: manual.length,
        open: searching || !!openGroups.my,
        onToggle: () => setOpenGroups((prev) => ({ ...prev, my: !prev.my })),
        /* Обновлять у своих серверов нечего: обновление здесь означает
           переизмерить задержку до них. См. docs/design/GAPS.md. */
        onSync: () => refreshPings(manual.map((p) => p.id)),
        /* Значок тот же, работа та же — значит, и крутится он по тому же
           поводу: пока идёт запрошенный отсюда замер. */
        syncBusy: manual.some(isManualPingPending),
        onDelete: () => removeManual(manual),
        /* Свои серверы: у каждой строки своя правка и своё удаление. */
        servers: sortRows(manualFound).map((p) => toRow(p, true)),
      });
    }

    return out;
  }, [
    subscriptions,
    listed,
    search,
    matches,
    openGroups,
    sortRows,
    toRow,
    t,
    refreshSubscription,
    refreshingSubs,
    removeSubscription,
    removeManual,
    refreshPings,
    isManualPingPending,
  ]);

  const sortOptions = useMemo(
    () =>
      ["default", "newest", "oldest", "country", "type", "provider", "ping"]
        .filter((o) => o !== "provider" || proxies.some((p) => p.provider))
        .map((value) => ({ value, label: t(`proxyList.sort.${value}`) })),
    [proxies, t],
  );

  const text = {
    title: t("serversPage.title"),
    search: t("serversPage.search"),
    pingServers: t("serversPage.pingServers"),
    sortServers: t("serversPage.sortServers"),
    editSubscription: t("serversPage.editSubscription"),
    refreshSubscription: t("proxyList.refreshSubAria"),
    deleteSubscription: t("proxyList.deleteSubscriptionAria"),
    pingGroup: t("proxyList.manualPingMyServersAria"),
    deleteGroup: t("proxyList.deleteManualGroupAria"),
    editServer: t("serversPage.editServer"),
    deleteServer: t("serversPage.deleteServer"),
    favorite: t("proxyList.favoriteAria"),
    empty: t("proxyList.noResults"),
  };

  /* Подписи окна правки. Ключи те же, что имена полей формы, — так видно,
     какая подпись какому полю принадлежит, без сверки по порядку. */
  const editorText = Object.fromEntries(
    Object.keys(SERVER_EDITOR_TEXT).map((key) => [key, t(`serverEditor.${key}`)]),
  );

  /*
   * Сохранение идёт тем же путём, что и прежняя форма правки: подключённый в
   * этот момент узел переподнимается с новыми настройками, а не остаётся жить
   * со старыми до следующего переключения.
   */
  const saveServer = (data) => {
    setEditingServer(null);
    handleSaveProxy(
      data,
      activeProxy,
      failedProxy,
      setFailedProxy,
      setActiveProxy,
      isConnected,
      selectAndConnect,
      setActiveTab,
      setEditingProxy,
    );
  };

  const saveSubscription = async ({ name, showOnHome, intervalMinutes }) => {
    const sub = editingSub;
    setEditingSub(null);
    if (!sub) return;
    try {
      await wailsAPI.updateSubscription(sub.id, name, showOnHome, intervalMinutes);
      /*
       * Переименование пишется и на сами серверы (`provider`), поэтому список
       * забираем целиком, а не правим одну запись подписки.
       */
      const cfg = await wailsAPI.getConfig();
      if (cfg?.subscriptions) setSubscriptions(cfg.subscriptions);
      if (Array.isArray(cfg?.proxies)) {
        setProxies(
          cfg.proxies.map((p) => ({ ...p, port: parseInt(p.port, 10) || 0, id: String(p.id) })),
        );
      }
    } catch (err) {
      console.error("Update subscription error:", err);
    }
  };

  return (
    <>
      <ServersPage
        title={text.title}
        subtitle={t("serversPage.subtitle", {
          servers: t("serversPage.countServers", { count: listed.length }),
          subscriptions: t("serversPage.countSubs", {
            count: (subscriptions || []).length,
          }),
        })}
        search={search}
        onSearchChange={setSearch}
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
        groups={groups}
        empty={groups.length === 0}
        text={text}
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

      {editingServer && (
        <ServerEditor
          /* Поля набираются из сервера при появлении окна, поэтому у каждого
             сервера оно должно быть своим. */
          key={editingServer.id}
          proxy={editingServer}
          text={editorText}
          onClose={() => setEditingServer(null)}
          onSave={saveServer}
        />
      )}

      {editingSub && (
        <SubscriptionDialog
          /* Своё состояние окно набирает из пропов при появлении, поэтому у
             каждой подписки оно должно быть своим. */
          key={editingSub.id}
          logo={
            <SubscriptionLogo subscription={editingSub} className="rv-sub-dialog__logo" />
          }
          title={editingSub.name}
          supportUrl={subscriptionSupportURL(editingSub)}
          onSupport={(url) => BrowserOpenURL(url)}
          name={editingSub.name}
          showOnHome={editingSub.showOnHome !== false}
          /* Своего интервала у подписки может не быть — тогда в окне отмечен
             общий из настроек. */
          interval={
            editingSub.updateIntervalMinutes ??
            (settings?.subscriptionUpdateIntervalHours || 6) * 60
          }
          intervals={SUBSCRIPTION_INTERVALS.map((item) => ({
            ...item,
            label: t(`serversPage.interval.${item.minutes}`),
          }))}
          text={{
            support: t("serversPage.support"),
            name: t("serversPage.renameLabel"),
            showOnHome: t("serversPage.showOnHome"),
            interval: t("serversPage.intervalLabel"),
            save: t("common.save"),
          }}
          onClose={() => setEditingSub(null)}
          onSave={saveSubscription}
        />
      )}
    </>
  );
}
