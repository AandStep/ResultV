import React, { useState } from "react";
import { Activity, Check, Shield, Download, Upload, Lock } from "lucide-react";
import { SettingToggle } from "../components/ui/SettingToggle";
import { encryptWithPassword, decryptWithPassword } from "../utils/crypto";
import { useConfigContext } from "../context/ConfigContext";
import { useTranslation } from "react-i18next";

export const SettingsView = () => {
  const { t } = useTranslation();
  const {
    proxies,
    setProxies,
    routingRules,
    setRoutingRules,
    settings,
    setSettings,
    updateSetting,
  } = useConfigContext();
  const [pwdModal, setPwdModal] = useState({
    isOpen: false,
    mode: "",
    data: null,
  });
  const [pwdInput, setPwdInput] = useState("");
  const [notify, setNotify] = useState(null);

  const showNotify = (msg, isError = false) => {
    setNotify({ msg, isError });
    setTimeout(() => setNotify(null), 4000);
  };

  const handleExportClick = () => {
    setPwdInput("");
    setPwdModal({ isOpen: true, mode: "export", data: null });
  };

  const executeExport = async () => {
    if (!pwdInput) return;

    const fullConfig = { proxies, routingRules, settings };
    const encrypted = await encryptWithPassword(fullConfig, pwdInput);
    if (!encrypted) {
      showNotify(t("settings.notify.read_error"), true);
      return;
    }
    const securePayload = {
      _isSecure: true,
      _version: 2,
      data: encrypted,
    };

    const dataStr =
      "data:text/json;charset=utf-8," +
      encodeURIComponent(JSON.stringify(securePayload, null, 2));
    const downloadAnchorNode = document.createElement("a");
    downloadAnchorNode.setAttribute("href", dataStr);
    downloadAnchorNode.setAttribute(
      "download",
      "resultproxy-secure-config.json",
    );
    document.body.appendChild(downloadAnchorNode);
    downloadAnchorNode.click();
    document.body.removeChild(downloadAnchorNode);

    setTimeout(() => {
      setPwdModal({ isOpen: false, mode: "", data: null });
    }, 100);
  };

  const handleImportClick = (e) => {
    const inputEl = e.target;
    const fileReader = new FileReader();
    if (!inputEl.files[0]) return;

    fileReader.readAsText(inputEl.files[0], "UTF-8");
    fileReader.onload = (event) => {
      try {
        const imported = JSON.parse(event.target.result);

        if (imported._isSecure && imported.data) {
          setPwdInput("");
          setPwdModal({ isOpen: true, mode: "import", data: imported.data });
        } else if (Array.isArray(imported)) {
          setProxies(imported);
          showNotify(t("settings.notify.import_old"));
        } else if (imported && typeof imported === "object") {
          if (imported.proxies) {
            if (Array.isArray(imported.proxies)) {
              setProxies(imported.proxies);
            } else {
              showNotify(t("settings.notify.invalid_format"), true);
              return;
            }
          }
          if (imported.routingRules) setRoutingRules(imported.routingRules);
          if (imported.settings) {
            if (imported.settings.autostart !== undefined)
              updateSetting("autostart", imported.settings.autostart);
            if (imported.settings.killswitch !== undefined)
              updateSetting("killswitch", imported.settings.killswitch);
            setSettings(imported.settings);
          }
          showNotify(t("settings.notify.import_unsecured"));
        } else {
          showNotify(t("settings.notify.invalid_format"), true);
        }
      } catch (err) {
        showNotify(t("settings.notify.read_error"), true);
      }
      inputEl.value = "";
    };
  };

  const executeImport = async () => {
    if (!pwdInput || !pwdModal.data) return;
    const dec = await decryptWithPassword(pwdModal.data, pwdInput);
    if (dec) {
      setProxies(dec.proxies || []);
      setRoutingRules(dec.routingRules || routingRules);
      setSettings(dec.settings || settings);
      showNotify(t("settings.notify.decrypt_success"));
      setPwdModal({ isOpen: false, mode: "", data: null });
    } else {
      showNotify(t("settings.notify.decrypt_error"), true);
    }
  };

  return (
    <div className="max-w-3xl mx-auto space-y-10 animate-in fade-in duration-300 relative">
      <div>
        <h2 className="text-3xl font-bold text-white">{t("settings.title")}</h2>
        <p className="text-zinc-400 mt-2">{t("settings.desc")}</p>
      </div>

      {notify && (
        <div
          className={`p-4 rounded-2xl border ${notify.isError ? "bg-rose-500/10 border-rose-500/20 text-rose-500" : "bg-[#007E3A]/10 border-[#007E3A]/20 text-[#00A819]"} flex items-center space-x-3 animate-in fade-in slide-in-from-top-4`}
        >
          {notify.isError ? (
            <Activity className="w-5 h-5 shrink-0" />
          ) : (
            <Check className="w-5 h-5 shrink-0" />
          )}
          <span className="font-medium">{notify.msg}</span>
        </div>
      )}

      <div className="space-y-4">
        <SettingToggle
          title={t("settings.autostart.title")}
          description={t("settings.autostart.desc")}
          isOn={settings.autostart}
          onToggle={() => updateSetting("autostart", !settings.autostart)}
        />
        <SettingToggle
          title={t("settings.killswitch.title")}
          description={t("settings.killswitch.desc")}
          isOn={settings.killswitch}
          onToggle={() => updateSetting("killswitch", !settings.killswitch)}
        />
        <SettingToggle
          title={t("settings.adblock.title")}
          description={t("settings.adblock.desc")}
          isOn={settings.adblock}
          onToggle={() => updateSetting("adblock", !settings.adblock)}
        />
      </div>

      <div className="p-8 bg-zinc-900 rounded-3xl border border-zinc-800 mt-10">
        <h3 className="text-white font-bold mb-2 text-xl">
          {t("settings.export_import.title")}
        </h3>
        <p className="text-zinc-400 mb-6">
          <Shield className="inline-block w-4 h-4 mr-1 text-[#00A819]" />
          {t("settings.export_import.desc")}
        </p>
        <div className="flex space-x-4">
          <button
            onClick={handleExportClick}
            className="flex items-center px-6 py-3 bg-zinc-800 hover:bg-zinc-700 text-white hover:text-[#00A819] rounded-xl font-medium transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
          >
            <Download className="w-5 h-5 mr-2" />{" "}
            {t("settings.export_import.export_btn")}
          </button>

          <input
            type="file"
            id="import-file"
            onChange={handleImportClick}
            accept=".json"
            className="hidden"
          />
          <button
            onClick={() => document.getElementById("import-file").click()}
            className="flex items-center px-6 py-3 bg-zinc-800 hover:bg-zinc-700 text-white hover:text-[#00A819] rounded-xl font-medium transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
          >
            <Upload className="w-5 h-5 mr-2" />{" "}
            {t("settings.export_import.import_btn")}
          </button>
        </div>
      </div>

      {pwdModal.isOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-zinc-950/80 backdrop-blur-sm p-4 animate-in fade-in duration-200">
          <div className="bg-zinc-900 border border-zinc-800 rounded-3xl p-8 max-w-md w-full shadow-2xl animate-in zoom-in-95 duration-200">
            <div className="flex items-center space-x-3 mb-4">
              <Lock className="w-6 h-6 text-[#007E3A]" />
              <h3 className="text-xl font-bold text-white">
                {pwdModal.mode === "export"
                  ? t("settings.modal.title_export")
                  : t("settings.modal.title_import")}
              </h3>
            </div>
            <p className="text-zinc-400 text-sm mb-6">
              {pwdModal.mode === "export"
                ? t("settings.modal.desc_export")
                : t("settings.modal.desc_import")}
            </p>
            <input
              type="password"
              placeholder={t("settings.modal.placeholder")}
              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:border-[#007E3A] transition-colors mb-6 focus:ring-0"
              value={pwdInput}
              onChange={(e) => setPwdInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  pwdModal.mode === "export"
                    ? executeExport()
                    : executeImport();
                }
              }}
              autoFocus
            />
            <div className="flex space-x-3">
              <button
                onClick={() =>
                  setPwdModal({ isOpen: false, mode: "", data: null })
                }
                className="flex-1 bg-zinc-800 hover:bg-zinc-700 text-white font-bold py-3 rounded-xl transition-colors border-transparent outline-none focus:outline-none"
              >
                {t("settings.modal.cancel")}
              </button>
              <button
                onClick={
                  pwdModal.mode === "export" ? executeExport : executeImport
                }
                className="flex-1 bg-[#007E3A] hover:bg-[#005C2A] text-white font-bold py-3 rounded-xl transition-colors border-transparent outline-none focus:outline-none"
              >
                {pwdModal.mode === "export"
                  ? t("settings.modal.encrypt")
                  : t("settings.modal.open")}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
