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

const AppContent = () => {
  const { t } = useTranslation();
  const { isConfigLoaded, activeTab } = useConfigContext();
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
            alt="ResultProxy"
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
      {activeTab === "list" && <ProxyListView />}
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
