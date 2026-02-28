import React, { useState, useEffect } from "react";
import { Lock } from "lucide-react";
import { useConfigContext } from "../context/ConfigContext";
import { useConnectionContext } from "../context/ConnectionContext";
import { useTranslation } from "react-i18next";

export const AddProxyView = () => {
  const { t } = useTranslation();
  const { handleSaveProxy, editingProxy, setEditingProxy, setActiveTab } =
    useConfigContext();
  const {
    activeProxy,
    failedProxy,
    setFailedProxy,
    setActiveProxy,
    isConnected,
    selectAndConnect,
  } = useConnectionContext();

  const onCancel = () => setActiveTab("list");

  const saveProxyWrapper = (proxyData) => {
    handleSaveProxy(
      proxyData,
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

  const [formData, setFormData] = useState({
    name: "",
    ip: "",
    port: "",
    type: "SOCKS5",
    username: "",
    password: "",
    country: "🌐",
  });

  useEffect(() => {
    if (editingProxy) setFormData(editingProxy);
    else
      setFormData({
        name: "",
        ip: "",
        port: "",
        type: "SOCKS5",
        username: "",
        password: "",
        country: "🌐",
      });
  }, [editingProxy]);

  const handleSubmit = (e) => {
    e.preventDefault();
    if (formData.ip && formData.port)
      saveProxyWrapper({
        ...formData,
        name: formData.name || t("add.newServer"),
      });
  };

  return (
    <div className="max-w-2xl mx-auto space-y-5 animate-in fade-in duration-300">
      <div>
        <h2 className="text-3xl font-bold text-white">
          {editingProxy ? t("add.titleEdit") : t("add.titleAdd")}
        </h2>
        <p className="text-zinc-400 mt-2">
          {editingProxy ? t("add.descEdit") : t("add.descAdd")}
        </p>
      </div>
      <form
        onSubmit={handleSubmit}
        className="bg-zinc-900 p-6 rounded-3xl border border-zinc-800 space-y-5"
      >
        <div>
          <label className="block text-sm font-medium text-zinc-400 mb-2">
            {t("add.profileName")}
          </label>
          <input
            type="text"
            placeholder={t("add.profilePlaceholder")}
            className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
            value={formData.name || ""}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
          />
        </div>
        <div className="grid grid-cols-3 gap-6">
          <div className="col-span-2">
            <label className="block text-sm font-medium text-zinc-400 mb-2">
              {t("add.ip")}
            </label>
            <input
              type="text"
              required
              placeholder="192.168.1.1"
              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
              value={formData.ip || ""}
              onChange={(e) => setFormData({ ...formData, ip: e.target.value })}
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-zinc-400 mb-2">
              {t("add.port")}
            </label>
            <input
              type="number"
              required
              placeholder="8000"
              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
              value={formData.port || ""}
              onChange={(e) =>
                setFormData({ ...formData, port: e.target.value })
              }
            />
          </div>
        </div>
        <div>
          <label className="block text-sm font-medium text-zinc-400 mb-3">
            {t("add.protocol")}
          </label>
          <div className="grid grid-cols-3 gap-3">
            {["HTTP", "HTTPS", "SOCKS5"].map((type) => (
              <button
                type="button"
                key={type}
                onClick={() => setFormData({ ...formData, type })}
                className={`py-3 rounded-xl text-sm font-bold border transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${formData.type === type ? "bg-[#007E3A] text-white border-[#007E3A]" : "bg-zinc-950 text-zinc-400 border-zinc-800 hover:border-[#00A819]"}`}
              >
                {type}
              </button>
            ))}
          </div>
        </div>
        <div className="pt-5 border-t border-zinc-800">
          <p className="text-sm font-medium text-zinc-400 mb-3 flex items-center">
            <Lock className="w-4 h-4 mr-2" /> {t("add.auth")}
          </p>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <input
              type="text"
              placeholder={t("add.loginPlaceholder")}
              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
              value={formData.username || ""}
              onChange={(e) =>
                setFormData({ ...formData, username: e.target.value })
              }
            />
            <input
              type="password"
              placeholder={t("add.passPlaceholder")}
              className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
              value={formData.password || ""}
              onChange={(e) =>
                setFormData({ ...formData, password: e.target.value })
              }
            />
          </div>
        </div>
        <div className="pt-5 flex space-x-4">
          {editingProxy && (
            <button
              type="button"
              onClick={onCancel}
              className="flex-1 bg-zinc-800 hover:bg-zinc-700 text-white font-bold py-3 rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
            >
              {t("add.cancel")}
            </button>
          )}
          <button
            type="submit"
            className="flex-[2] bg-[#007E3A] hover:bg-[#005C2A] text-white font-bold py-3 rounded-xl transition-colors shadow-lg shadow-[#007E3A]/20 border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none"
          >
            {editingProxy ? t("add.saveChanges") : t("add.saveProxy")}
          </button>
        </div>
      </form>
    </div>
  );
};
