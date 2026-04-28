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
import { HomeView } from "./views/HomeView";
import { ProxyListView } from "./views/ProxyListView";
import { RulesView } from "./views/RulesView";
import { AddProxyView } from "./views/AddProxyView";
import { BuyProxyView } from "./views/BuyProxyView";
import { LogsView } from "./views/LogsView";
import { SettingsView } from "./views/SettingsView";
import { formatBytes, formatSpeed } from "./utils/formatters";
import { useConfigContext } from "./context/ConfigContext";
import logo from "./assets/logo.png";
import { useTranslation } from "react-i18next";
import { useCheckUpdate } from "./hooks/useCheckUpdate";
import UpdateNotificationModal from "./components/ui/UpdateNotificationModal";
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
    } = useConfigContext();
    const { updateAvailable, latestVersionData, currentVersion } =
        useCheckUpdate();

    const [isUpdateDismissed, setIsUpdateDismissed] = React.useState(
        () => window.sessionStorage.getItem("updateDismissed") === "true",
    );

    const handleDismissUpdate = () => {
        window.sessionStorage.setItem("updateDismissed", "true");
        setIsUpdateDismissed(true);
    };

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
            {activeTab === "home" && <HomeView />}
            <div className={activeTab === "list" ? "" : "hidden"}>
                <ProxyListView />
            </div>
            {activeTab === "rules" && <RulesView />}
            {activeTab === "add" && <AddProxyView />}
            {activeTab === "buy" && <BuyProxyView />}
            {activeTab === "logs" && <LogsView />}
            {activeTab === "settings" && <SettingsView />}

            {updateAvailable && !isUpdateDismissed && (
                <UpdateNotificationModal
                    currentVersion={currentVersion}
                    latestVersion={latestVersionData?.version}
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
