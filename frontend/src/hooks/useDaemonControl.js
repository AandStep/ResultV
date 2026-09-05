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

import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import wailsAPI from "../utils/wailsAPI";
import { isEndpointProtocol } from "../utils/proxyParser";
import { useToast } from "../context/ToastContext";

// Shown once per app launch. Deliberately a module variable rather than a
// setting: "once per launch" literally means it should reset on restart, and
// the running status label already carries the information on every later
// connect — this toast only explains WHY the wait is long, which is worth
// saying once.
let autoResolveHintShown = false;

export const useDaemonControl = (
    isConnected,
    setIsConnected,
    setIsConnecting,
    setIsDisconnecting,
    activeProxy,
    setActiveProxy,
    failedProxy,
    setFailedProxy,
    proxies,
    routingRules,
    settings,
    updateSetting,
    daemonStatus,
    isSwitchingRef,
    addLog,
    showAlertDialog,
    pings,
    statusGenerationRef,
) => {
    const { t } = useTranslation();
    const { showToast } = useToast();

    const AUTO_MAX_ATTEMPTS = 5;

    // True only while an AUTO group's candidates are being resolved. That
    // resolve runs a two-phase probe over every member and takes seconds, and
    // it used to happen with nothing at all on screen.
    const [isResolving, setIsResolving] = useState(false);

    const bumpGen = () => {
        if (statusGenerationRef) statusGenerationRef.current += 1;
    };

    /*
     * Номер запуска — на время между нажатием и тем местом, где запуск можно
     * бросить.
     *
     * Нужен из-за подбора узла авто-группы. Он идёт в своём await, у сметки
     * серверов свои таймауты и общий кэш на всех, кто её ждёт, и оборвать её
     * снаружи нечем — оттого подбор и считался неотменяемым. Но остановить
     * ЗАПУСК можно и не обрывая замер: подбор доработает сам и никому не
     * помешает, а подключаться после него мы просто не станем.
     *
     * Почему номер, а не флаг «запуск остановлен». Флаг снимал каждый новый
     * запуск — и отставший подбор авто-группы видел чистое поле. Отсюда был
     * баг: прервали подбор авто-группы, выбрали другой сервер, а через пару
     * секунд подбор досчитывался, флага уже не находил и переподключал на
     * авто-группу поверх выбранного узла. Номер же увеличивают и отмена, и
     * любой следующий запуск, поэтому свой номер остаётся только у последнего.
     */
    const connectEpochRef = useRef(0);
    const beginConnect = () => (connectEpochRef.current += 1);
    const isStale = (epoch) => epoch !== connectEpochRef.current;

    // Ranking lives in the backend (App.ResolveAutoCandidates) so the tray and
    // the UI cannot drift apart. This used to sort AUTO members by the cached
    // ping sweep, which measured a bare TCP connect and knew nothing about
    // past connect failures.
    const getConnectCandidates = useCallback(async (proxyToResolve) => {
        if (proxyToResolve?.type?.toUpperCase() !== "AUTO") {
            return [proxyToResolve];
        }
        const ranked = await wailsAPI.resolveAutoCandidates(proxyToResolve.id);
        /* Пусто — значит пусто. Раньше здесь стоял откат на саму AUTO-запись, и
           это был худший из возможных исходов: у заголовка группы нет ни
           адреса, ни протокола, ядро собирало из него http-аутбаунд с пустым
           сервером, поднималось без единой ошибки и рапортовало «Подключено».
           Кнопка загоралась зелёным, а каждое соединение падало с «invalid
           address» (логи 05.09.2026). Пустой список честнее: у него есть свой
           обработчик, а у зелёной кнопки поверх мёртвого туннеля — нет. */
        return ranked;
    }, []);

    /* Общий выход для «подбор не нашёл ни одного узла».
       Живое соединение не рвём: у авто-группы подбор идёт ДО отключения, и
       уронить рабочий туннель из-за того, что не нашлась замена, — худшее, что
       можно сделать. */
    const reportEmptyAutoGroup = useCallback(
        (group) => {
            addLog(
                `Авто-группа «${group?.name || ""}»: ни один узел не отвечает. ` +
                    `Подключение отменено, текущее соединение не тронуто.`,
                "error",
            );
            showToast({
                variant: "error",
                message: t("toast.autoNoNodes"),
                duration: 6000,
            });
        },
        [addLog, showToast, t],
    );

    // tun_adapter_unavailable — Windows не запустила Wintun-адаптер. Он один на
    // всё приложение и от выбранного узла не зависит, так что перебор остальных
    // кандидатов авто-группы упёрся бы в тот же отказ, только медленнее.
    const isTerminalErrorCode = (code) =>
        code === "tun_privileges" ||
        code === "proxy_not_supported" ||
        code === "tun_adapter_unavailable";

    // Понадобятся ли на это подключение права администратора. TUN-адаптер без
    // них не поднять, а туда ведут две дороги: включённый режим туннеля и сам
    // endpoint-протокол, который в режиме «прокси» просто не работает.
    //
    // Внутрь авто-группы не заглядываем нарочно: узел там выбирает ядро, и
    // требовать права заранее из-за одного возможного участника значило бы
    // показывать окно про администратора тем, кому оно не нужно. Такой участник
    // и так отсеется — на него вернётся терминальный `proxy_not_supported`, без
    // перебора остальных.
    const needsElevation = useCallback(
        (proxy) => settings?.mode === "tunnel" || isEndpointProtocol(proxy),
        [settings?.mode],
    );

    /*
     * Права спрашиваем ДО запуска, а не по ответу ядра.
     *
     * Раньше отсутствие прав выяснялось только из результата Connect: экран
     * успевал показать «Подключение...», и лишь потом всё обрывалось. У
     * авто-группы выходило хуже вдвойне — она перебирает узлы, и обрыв
     * растягивался на несколько попыток подряд.
     *
     * Если спросить не удалось, подключаться не мешаем: своя же диагностика не
     * должна отнимать у человека рабочую кнопку. Ядро в этом случае отработает
     * по-старому и покажет то же окно само.
     */
    const ensureElevated = useCallback(
        async (proxy) => {
            if (!needsElevation(proxy)) return true;
            let admin = true;
            try {
                admin = await wailsAPI.isAdmin();
            } catch {
                return true;
            }
            if (admin) return true;
            addLog(t("tunnel.adminMessage"), "error");
            showAlertDialog({
                title: t("tunnel.adminTitle"),
                message: t("tunnel.adminMessage"),
                variant: "warning",
                confirmText: t("tunnel.restartAsAdmin"),
                onConfirmAction: () => wailsAPI.restartAsAdmin(),
            });
            return false;
        },
        [needsElevation, addLog, showAlertDialog, t],
    );

    const disconnectOnly = useCallback(async () => {
        if (isSwitchingRef.current) return;

        try {
            bumpGen();
            isSwitchingRef.current = true;
            setIsConnecting(false);
            setIsDisconnecting(true);
            addLog("Отключение...", "info");
            await wailsAPI.disconnect();
            setIsConnected(false);
            setFailedProxy(null);
            addLog("Отключено успешно.", "success");
        } catch (error) {
            addLog(`Сбой отключения: ${error.message || error}`, "error");
        } finally {
            bumpGen();
            setIsDisconnecting(false);
            isSwitchingRef.current = false;
        }
    }, [addLog, isSwitchingRef, setFailedProxy, setIsConnected, setIsConnecting, setIsDisconnecting]);

    const toggleConnection = useCallback(async () => {
        if (isSwitchingRef.current) return;
        if (daemonStatus !== "online") {
            addLog("Служба недоступна.", "error");
            return;
        }

        const targetProxy =
            activeProxy ||
            proxies.find((p) => String(p.id) === String(settings?.lastSelectedProxyId)) ||
            proxies[0];
        if (proxies.length === 0 || !targetProxy) return;

        // Только на запуск: отключаться права не нужны, и спрашивать их,
        // чтобы разорвать соединение, было бы издевательством.
        //
        // На время вопроса вход закрыт: проверка асинхронная, а `isSwitchingRef`
        // поднимается ниже — без замка второе нажатие успело бы проскочить мимо
        // неё и запустить второе подключение.
        if (!isConnected) {
            isSwitchingRef.current = true;
            const allowed = await ensureElevated(targetProxy);
            isSwitchingRef.current = false;
            if (!allowed) return;
        }

        /* Объявлен снаружи try: номер запуска нужен и в catch — по нему видно,
           что ошибка прилетела от хода, который уже никому не нужен. */
        let epoch = connectEpochRef.current;

        try {
            bumpGen();
            isSwitchingRef.current = true;
            setFailedProxy(null);

            if (isConnected) {
                setIsConnecting(false);
                setIsDisconnecting(true);
                addLog("Отключение...", "info");
                await wailsAPI.disconnect();
                addLog("Отключено успешно.", "success");
                setIsConnected(false);
                setIsDisconnecting(false);
            } else {
                setIsConnecting(true);
                if (targetProxy?.type?.toUpperCase() === "SECTION") {
                    setIsConnecting(false);
                    // A toast, not a modal: there is nothing to confirm here,
                    // so an OK button is just an extra click.
                    showToast({
                        variant: "info",
                        message: t("proxyList.sectionNoConnect"),
                    });
                    bumpGen();
                    isSwitchingRef.current = false;
                    return;
                }
                const isAuto = targetProxy?.type?.toUpperCase() === "AUTO";
                epoch = beginConnect();

                // Tell the UI about the choice BEFORE awaiting the resolve.
                // For an AUTO group that await runs a two-phase probe over
                // every member and takes seconds; doing it first left the
                // screen completely unchanged for that whole time, which reads
                // as "the click didn't register".
                addLog(`Подключение к ${targetProxy.name}...`, "info");
                setActiveProxy(targetProxy);
                if (String(settings?.lastSelectedProxyId) !== String(targetProxy.id)) {
                    updateSetting("lastSelectedProxyId", targetProxy.id);
                }

                let candidates;
                if (isAuto && !autoResolveHintShown) {
                    autoResolveHintShown = true;
                    showToast({
                        variant: "info",
                        message: t("toast.autoResolveHint"),
                        // Longer than the default: this sentence takes more
                        // than four seconds to read.
                        duration: 6000,
                    });
                }
                // Scoped narrowly around the one await: this function and
                // selectAndConnect have no outer finally, they reset their
                // flags twice over (tail of try, and catch). Joining that
                // scheme is easy to desynchronise; this is shorter and safer.
                if (isAuto) setIsResolving(true);
                try {
                    candidates = (await getConnectCandidates(targetProxy)).slice(
                        0,
                        isAuto ? AUTO_MAX_ATTEMPTS : 1,
                    );
                } finally {
                    if (isAuto) setIsResolving(false);
                }

                // Запуск остановили или начали новый, пока шёл подбор. Общими
                // флагами теперь владеет тот, кто нас обогнал (cancelConnect
                // либо следующий запуск), — трогать их нельзя, иначе снимем
                // чужой замок посреди его работы.
                if (isStale(epoch)) {
                    addLog("Запуск остановлен.", "info");
                    return;
                }

                if (candidates.length === 0) {
                    setIsConnecting(false);
                    setActiveProxy(null);
                    reportEmptyAutoGroup(targetProxy);
                    bumpGen();
                    isSwitchingRef.current = false;
                    return;
                }

                let res = null;
                for (let i = 0; i < candidates.length; i++) {
                    /* Перебор узлов авто-группы тоже занимает секунды, и всё
                       это время кнопка питания прерывает запуск. Прервали —
                       следующий узел уже не пробуем. */
                    if (isStale(epoch)) {
                        addLog("Запуск остановлен.", "info");
                        return;
                    }
                    const candidate = candidates[i];
                    if (isAuto && i > 0) {
                        const label = candidate.name || `${candidate.ip}:${candidate.port}`;
                        addLog(`Auto: пробуем следующий узел (${label})...`, "info");
                        try { await wailsAPI.disconnect(); } catch {}
                    }
                    res = await wailsAPI.connect(
                        { ...candidate, port: parseInt(candidate.port, 10) || 0, id: targetProxy.id, name: targetProxy.name },
                        routingRules,
                        settings.killswitch || false
                    );
                    // Whether a connect actually succeeded is the most honest
                    // signal about a node, and it used to be discarded. Only
                    // AUTO reports: a single server has nothing to rank against.
                    if (isAuto) {
                        await wailsAPI.reportAutoConnectOutcome(
                            targetProxy.id,
                            i,
                            candidate.ip,
                            parseInt(candidate.port, 10) || 0,
                            !!res.success,
                        );
                    }
                    if (res.success) break;
                    if (isTerminalErrorCode(res.errorCode)) break;
                    if (!isAuto) break;
                }

                /* Пока шли попытки, запуск могли прервать — тогда соединение
                   уже разорвано, и объявлять успех или сбой нечего. */
                if (isStale(epoch)) {
                    addLog("Запуск остановлен.", "info");
                    return;
                }

                if (!res?.success) {
                    if (res?.errorCode === "tun_privileges") {
                        setIsConnecting(false);
                        addLog(
                            res.message || t("tunnel.adminMessage"),
                            "error",
                        );
                        showAlertDialog({
                            title: t("tunnel.adminTitle"),
                            message: t("tunnel.adminMessage"),
                            variant: "warning",
                            confirmText: t("tunnel.restartAsAdmin"),
                            onConfirmAction: () => wailsAPI.restartAsAdmin(),
                        });
                        bumpGen();
                        isSwitchingRef.current = false;
                        return;
                    }
                    const reason = res?.reason ? ` Причина: ${res.reason}` : "";
                    const code = res?.errorCode ? ` Код: ${res.errorCode}` : "";
                    const prefix = isAuto && candidates.length > 1
                        ? `Auto: все ${candidates.length} попытки не удались. `
                        : "";
                    throw new Error(prefix + (res?.message || "Unknown proxy connection error") + code + reason);
                }

                addLog("Соединение установлено.", "success");
                if (res.tunnelFailed) {
                    addLog(`Туннелирование не запущено: ${res.reason || "неизвестная причина"}`, "warning");
                    if (res.fallbackUsed) {
                        addLog("Подключение работает в fallback-режиме без TUN.", "warning");
                    }
                }



                setIsConnected(true);
                setIsConnecting(false);
            }

            bumpGen();
            isSwitchingRef.current = false;
        } catch (error) {
            /* Сбой устаревшего хода экрану не принадлежит: соединением уже
               занят кто-то другой, и красный «Сбой» лёг бы поверх его работы. */
            if (isStale(epoch)) {
                addLog(`Сбой прерванного запуска: ${error.message || error}`, "info");
                return;
            }
            bumpGen();
            isSwitchingRef.current = false;
            setIsConnected(false);
            setIsConnecting(false);
            setIsDisconnecting(false);
            setFailedProxy(targetProxy);
            addLog(`Сбой: ${error.message || error}`, "error");
        }
    }, [
        addLog,
        daemonStatus,
        activeProxy,
        proxies,
        isConnected,
        routingRules,
        settings,
        setIsConnected,
        setActiveProxy,
        setFailedProxy,
        isSwitchingRef,
        setIsConnecting,
        setIsDisconnecting,
        updateSetting,
        showAlertDialog,
        showToast,
        t,
        getConnectCandidates,
        reportEmptyAutoGroup,
        ensureElevated,
    ]);

    const selectAndConnect = useCallback(
        async (proxy, forceReconnect = false, setActiveTab) => {
            if (isSwitchingRef.current) return;
            if (!forceReconnect && activeProxy?.id === proxy.id && isConnected)
                return;

            if (proxy?.type?.toUpperCase() === "SECTION") {
                // A toast, not a modal: there is nothing to confirm here, so an
                // OK button is just an extra click.
                showToast({
                    variant: "info",
                    message: t("proxyList.sectionNoConnect"),
                });
                return;
            }

            // Замок на время асинхронной проверки — как в toggleConnection.
            isSwitchingRef.current = true;
            const allowed = await ensureElevated(proxy);
            isSwitchingRef.current = false;
            if (!allowed) return;

            /* Как в toggleConnection: номер запуска нужен и в catch. */
            let epoch = connectEpochRef.current;

            try {
                bumpGen();
                isSwitchingRef.current = true;
                setFailedProxy(null);
                if (setActiveTab) setActiveTab("home");

                const isAuto = proxy?.type?.toUpperCase() === "AUTO";
                epoch = beginConnect();

                // Tell the UI about the choice BEFORE awaiting the resolve.
                // For an AUTO group that await runs a two-phase probe over
                // every member and takes seconds; doing it first left the
                // screen completely unchanged for that whole time, which reads
                // as "the click didn't register".
                const previousActive = activeProxy;
                setActiveProxy(proxy);
                if (String(settings?.lastSelectedProxyId) !== String(proxy.id)) {
                    updateSetting("lastSelectedProxyId", proxy.id);
                }

                let candidates;
                if (isAuto && !autoResolveHintShown) {
                    autoResolveHintShown = true;
                    showToast({
                        variant: "info",
                        message: t("toast.autoResolveHint"),
                        // Longer than the default: this sentence takes more
                        // than four seconds to read.
                        duration: 6000,
                    });
                }
                // Scoped narrowly around the one await: this function and
                // toggleConnection have no outer finally, they reset their
                // flags twice over (tail of try, and catch). Joining that
                // scheme is easy to desynchronise; this is shorter and safer.
                if (isAuto) setIsResolving(true);
                try {
                    candidates = (await getConnectCandidates(proxy)).slice(
                        0,
                        isAuto ? AUTO_MAX_ATTEMPTS : 1,
                    );
                } finally {
                    if (isAuto) setIsResolving(false);
                }

                // Как в toggleConnection: подбор пережил остановку запуска или
                // выбор другого сервера — подключаться после этого не надо, и
                // общие флаги остаются тому, кто нас обогнал.
                if (isStale(epoch)) {
                    addLog("Запуск остановлен.", "info");
                    return;
                }

                if (candidates.length === 0) {
                    /* Строку сервера возвращаем на тот узел, что и правда
                       поднят: подбор шёл поверх живого соединения, и оставить
                       в шапке несостоявшуюся группу значило бы показывать не
                       то, к чему подключён пользователь. */
                    setActiveProxy(isConnected ? previousActive ?? null : null);
                    reportEmptyAutoGroup(proxy);
                    bumpGen();
                    isSwitchingRef.current = false;
                    return;
                }
                addLog(`Переключение на: ${proxy.name}...`, "info");

                if (isConnected) {
                    setIsDisconnecting(true);
                    await wailsAPI.disconnect();
                    setIsConnected(false);
                    setIsDisconnecting(false);
                }

                setIsConnecting(true);
                let res = null;
                for (let i = 0; i < candidates.length; i++) {
                    /* Перебор узлов авто-группы тоже занимает секунды, и всё
                       это время кнопка питания прерывает запуск. Прервали —
                       следующий узел уже не пробуем. */
                    if (isStale(epoch)) {
                        addLog("Запуск остановлен.", "info");
                        return;
                    }
                    const candidate = candidates[i];
                    if (isAuto && i > 0) {
                        const label = candidate.name || `${candidate.ip}:${candidate.port}`;
                        addLog(`Auto: пробуем следующий узел (${label})...`, "info");
                        try { await wailsAPI.disconnect(); } catch {}
                    }
                    res = await wailsAPI.connect(
                        { ...candidate, port: parseInt(candidate.port, 10) || 0, id: proxy.id, name: proxy.name },
                        routingRules,
                        settings.killswitch || false
                    );
                    // Whether a connect actually succeeded is the most honest
                    // signal about a node, and it used to be discarded. Only
                    // AUTO reports: a single server has nothing to rank against.
                    if (isAuto) {
                        await wailsAPI.reportAutoConnectOutcome(
                            proxy.id,
                            i,
                            candidate.ip,
                            parseInt(candidate.port, 10) || 0,
                            !!res.success,
                        );
                    }
                    if (res.success) break;
                    if (isTerminalErrorCode(res.errorCode)) break;
                    if (!isAuto) break;
                }

                /* Пока шли попытки, запуск могли прервать — тогда соединение
                   уже разорвано, и объявлять успех или сбой нечего. */
                if (isStale(epoch)) {
                    addLog("Запуск остановлен.", "info");
                    return;
                }

                if (!res?.success) {
                    if (res?.errorCode === "tun_privileges") {
                        setIsConnecting(false);
                        addLog(
                            res.message || t("tunnel.adminMessage"),
                            "error",
                        );
                        showAlertDialog({
                            title: t("tunnel.adminTitle"),
                            message: t("tunnel.adminMessage"),
                            variant: "warning",
                            confirmText: t("tunnel.restartAsAdmin"),
                            onConfirmAction: () => wailsAPI.restartAsAdmin(),
                        });
                        bumpGen();
                        isSwitchingRef.current = false;
                        return;
                    }
                    const reason = res?.reason ? ` Причина: ${res.reason}` : "";
                    const code = res?.errorCode ? ` Код: ${res.errorCode}` : "";
                    const prefix = isAuto && candidates.length > 1
                        ? `Auto: все ${candidates.length} попытки не удались. `
                        : "";
                    throw new Error(prefix + (res?.message || "Ошибка смены прокси: Узел отклонил подключение") + code + reason);
                }

                setIsConnected(true);
                setIsConnecting(false);
                addLog(`Успешно переключено на ${proxy.name}`, "success");
                if (res.tunnelFailed) {
                    addLog(`Туннелирование не запущено: ${res.reason || "неизвестная причина"}`, "warning");
                    if (res.fallbackUsed) {
                        addLog("Подключение работает в fallback-режиме без TUN.", "warning");
                    }
                }

                bumpGen();
                isSwitchingRef.current = false;
            } catch (error) {
                /* Сбой устаревшего хода экрану не принадлежит — см.
                   toggleConnection. */
                if (isStale(epoch)) {
                    addLog(`Сбой прерванного запуска: ${error.message || error}`, "info");
                    return;
                }
                bumpGen();
                isSwitchingRef.current = false;
                setIsConnected(false);
                setIsConnecting(false);
                setIsDisconnecting(false);
                setFailedProxy(proxy);
                addLog(`Сбой подключения: ${error.message || error}`, "error");
            }
        },
        [
            activeProxy,
            isConnected,
            routingRules,
            settings,
            addLog,
            setActiveProxy,
            setFailedProxy,
            setIsConnected,
            setIsConnecting,
            setIsDisconnecting,
            isSwitchingRef,
            updateSetting,
            showAlertDialog,
            showToast,
            t,
            getConnectCandidates,
            reportEmptyAutoGroup,
            ensureElevated,
        ],
    );

    const deleteProxy = useCallback(
        async (id, setProxies) => {
            const isDeletingActive = activeProxy?.id === id;
            setProxies((prev) => prev.filter((p) => p.id !== id));

            if (isDeletingActive) {
                if (isConnected) {
                    bumpGen();
                    isSwitchingRef.current = true;
                    setIsConnecting(false);
                    setIsDisconnecting(true);
                    addLog("Активный сервер удален. Разрыв соединения...", "info");
                    try {
                        await wailsAPI.disconnect();
                        addLog("Отключено успешно.", "success");
                    } catch (e) {}
                    setIsConnected(false);
                    setActiveProxy(null);
                    setIsDisconnecting(false);
                    bumpGen();
                    isSwitchingRef.current = false;
                } else {
                    setActiveProxy(null);
                }
            }
            if (failedProxy?.id === id) setFailedProxy(null);
        },
        [
            activeProxy,
            isConnected,
            failedProxy,
            addLog,
            setActiveProxy,
            setIsConnected,
            setIsConnecting,
            setIsDisconnecting,
            setFailedProxy,
            isSwitchingRef,
        ],
    );

    const cancelConnect = useCallback(async () => {
        // Full disconnect on cancel: backend CancelConnect aborts the probe ctx,
        // and Disconnect additionally stops the engine and clears sys proxy —
        // without this, sing-box keeps running and the next Connect fails with
        // "engine already running".
        beginConnect();
        bumpGen();
        isSwitchingRef.current = true;
        // Прерывание — это тоже отключение, и экран должен говорить именно так,
        // а не продолжать обещать подключение. Подбор гасим здесь же: замер
        // доработает сам, но ждать его человек уже не должен.
        setIsResolving(false);
        setIsConnecting(false);
        setIsDisconnecting(true);
        await wailsAPI.cancelConnect();
        try {
            await wailsAPI.disconnect();
        } catch (e) {
            // ignore
        }
        setIsConnected(false);
        setIsConnecting(false);
        setIsDisconnecting(false);
        setFailedProxy(null);
        bumpGen();
        isSwitchingRef.current = false;
    }, [setIsConnecting, setIsDisconnecting, setIsConnected, setFailedProxy, isSwitchingRef]);

    return {
        disconnectOnly,
        toggleConnection,
        selectAndConnect,
        deleteProxy,
        cancelConnect,
        isResolving,
    };
};
