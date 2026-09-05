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

import React from "react";
import { AppProvider } from "./context/AppContext";
import { MainLayout } from "./components/layout/MainLayout";
import HomeScreen from "./views/redesign/HomeScreen";
import SmartRulesScreen from "./views/redesign/SmartRulesScreen";
import AddScreen from "./views/redesign/AddScreen";
import ServersScreen from "./views/redesign/ServersScreen";
import BuyScreen from "./views/redesign/BuyScreen";
import LogsScreen from "./views/redesign/LogsScreen";
import SettingsScreen from "./views/redesign/SettingsScreen";
import { formatBytes, formatSpeed } from "./utils/formatters";
import { useConfigContext } from "./context/ConfigContext";
import wailsAPI from "./utils/wailsAPI";
import { EventsOn, EventsOff } from "../wailsjs/runtime/runtime";
import logo from "./assets/logo.png";
import { useTranslation } from "react-i18next";
import { useCheckUpdate } from "./hooks/useCheckUpdate";
import { useChangelog } from "./hooks/useChangelog";
import UpdateNotificationModal from "./components/ui/UpdateNotificationModal";
import UpdaterModal from "./components/ui/UpdaterModal";
import ChangelogModal from "./components/ui/ChangelogModal";
import ProtocolWarningModal from "./components/ui/ProtocolWarningModal";
import AppDialogModal from "./components/ui/AppDialogModal";
import DeepLinkImportModal from "./components/ui/DeepLinkImportModal";

const AppContent = () => {
    const { t } = useTranslation();
    const {
        isConfigLoaded,
        activeTab,
        showProtocolModal,
        setShowProtocolModal,
        appDialog,
        closeAppDialog,
        handleAppDialogConfirm,
        showAlertDialog,
    } = useConfigContext();
    const { updateAvailable, latestVersionData, currentVersion, hasPlatformAsset } =
        useCheckUpdate();
    const { changelog, dismiss: dismissChangelog } = useChangelog(isConfigLoaded);
    const latestVersion = latestVersionData?.version || "";
    const [dismissedUpdateVersion, setDismissedUpdateVersion] = React.useState(
        () => window.sessionStorage.getItem("updateDismissedVersion") || "",
    );
    const isUpdateDismissed = latestVersion !== "" && dismissedUpdateVersion === latestVersion;

    const handleDismissUpdate = () => {
        if (!latestVersion) return;
        window.sessionStorage.setItem("updateDismissedVersion", latestVersion);
        // Legacy key from the old behavior could suppress notifications forever
        // in a long-running session if not removed.
        window.sessionStorage.removeItem("updateDismissed");
        setDismissedUpdateVersion(latestVersion);
    };

    // Startup recovery runs in Go and reliably removes OS-level leftovers from a
    // prior unclean exit (force-kill / crash) — including a stranded sing-tun
    // adapter that still owns the default route, which is what actually breaks
    // the internet in tunnel mode. Here we only show a one-time informational
    // notice of what was cleaned. The report is both pulled (recovery finished
    // before mount) and pushed via the "leftovers:recovered" event (recovery
    // finished after mount); a ref guard shows the notice exactly once.
    const leftoverNoticeShownRef = React.useRef(false);
    React.useEffect(() => {
        const showNotice = (rep) => {
            if (leftoverNoticeShownRef.current) return;
            if (!rep || (!rep.proxy && !rep.dns && !rep.tun && !rep.firewall)) return;
            leftoverNoticeShownRef.current = true;
            wailsAPI.resetLeftoverReport();
            const items = [];
            if (rep.tun) items.push(t("leftovers.itemTun"));
            if (rep.proxy) items.push(t("leftovers.itemProxy"));
            if (rep.dns) items.push(t("leftovers.itemDns"));
            if (rep.firewall) items.push(t("leftovers.itemFirewall"));
            const bullets = items.map((it) => `• ${it}`).join("\n");
            if (rep.needsElevation) {
                // Admin-requiring leftovers were only DETECTED (we run without
                // elevation): offer an explicit elevated restart instead of the
                // old surprise mid-startup UAC. The elevated instance cleans
                // them on its own startup pass and shows the normal notice.
                showAlertDialog({
                    title: t("leftovers.elevTitle"),
                    message: `${t("leftovers.elevLead")}\n\n${bullets}\n\n${t("leftovers.elevTail")}`,
                    variant: "warning",
                    confirmText: t("tunnel.restartAsAdmin"),
                    onConfirmAction: () => wailsAPI.restartAsAdmin(),
                });
                return;
            }
            showAlertDialog({
                title: t("leftovers.title"),
                message: `${t("leftovers.lead")}\n\n${bullets}\n\n${t("leftovers.tail")}`,
                variant: "warning",
                confirmText: t("common.ok"),
            });
        };
        (async () => {
            showNotice(await wailsAPI.getLeftoverRecoveryReport());
        })();
        EventsOn("leftovers:recovered", showNotice);
        return () => EventsOff("leftovers:recovered");
    }, [t, showAlertDialog]);

    if (!isConfigLoaded) {
        return (
            <div className="fixed inset-0 flex flex-col items-center justify-center bg-zinc-950">
                <div className="relative flex items-center justify-center">
                    <img
                        src={logo}
                        alt="ResultV"
                        className="w-10 h-10 absolute drop-shadow-[0_0_15px_rgba(0,126,58,0.8)] z-10"
                    />
                    <div className="w-20 h-20 border-4 border-zinc-800 border-t-[#00A819] rounded-full animate-spin"></div>
                </div>
                <p className="text-zinc-500 mt-6 font-medium animate-pulse">
                    {t("app.loading")}
                </p>
            </div>
        );
    }

    return (
        <MainLayout>
            {activeTab === "home" && <HomeScreen />}
            {activeTab === "list" && <ServersScreen />}
            {activeTab === "rules" && <SmartRulesScreen />}
            {activeTab === "add" && <AddScreen />}
            {activeTab === "buy" && <BuyScreen />}
            {activeTab === "logs" && <LogsScreen />}
            {activeTab === "settings" && <SettingsScreen />}

            <ChangelogModal changelog={changelog} onClose={dismissChangelog} />

            {/* Both can be due at once — you updated 3.1 → 3.2 while 3.3 is
                already out. What changed in the build you are running comes
                first; the offer to update again waits its turn. */}
            {!changelog && updateAvailable && !isUpdateDismissed && (
                hasPlatformAsset
                ? <UpdaterModal
                    currentVersion={currentVersion}
                    latestVersion={latestVersion}
                    downloadUrl={latestVersionData?.downloadUrl}
                    onClose={handleDismissUpdate}
                  />
                : <UpdateNotificationModal
                    currentVersion={currentVersion}
                    latestVersion={latestVersion}
                    downloadUrl={latestVersionData?.downloadUrl}
                    onClose={handleDismissUpdate}
                  />
            )}

            <ProtocolWarningModal
                isOpen={showProtocolModal}
                onClose={() => setShowProtocolModal(false)}
            />

            <AppDialogModal
                isOpen={appDialog?.isOpen}
                title={appDialog?.title}
                message={appDialog?.message}
                variant={appDialog?.variant}
                showCancel={appDialog?.showCancel}
                confirmText={appDialog?.confirmText}
                cancelText={appDialog?.cancelText}
                onClose={() => closeAppDialog(false)}
                onConfirm={handleAppDialogConfirm}
            />

            <DeepLinkImportModal />
        </MainLayout>
    );
};

export default function App() {
    return (
        <AppProvider>
            <AppContent />
        </AppProvider>
    );
}
