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

import React, { useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { ListPlus } from "lucide-react";
import AppSelect from "./AppSelect";

// Add/edit form for a user-managed routing-list subscription. In "edit" mode
// the URL is read-only: UpdateRoutingList (Go side) always preserves the
// existing URL/AllowInsecure and only lets name/action/enabled change —
// changing the source requires deleting and re-adding the list.
const RoutingListModal = ({ isOpen, mode = "add", initial, onClose, onSubmit }) => {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [action, setAction] = useState("proxy");
  const [allowInsecure, setAllowInsecure] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!isOpen) return;
    setName(initial?.name || "");
    setUrl(initial?.url || "");
    setAction(initial?.action || "proxy");
    setAllowInsecure(!!initial?.allowInsecure);
    setSaving(false);
    setError("");
  }, [isOpen, initial]);

  const isEdit = mode === "edit";
  const isInsecureURL = useMemo(
    () => /^http:\/\//i.test(url.trim()),
    [url],
  );

  const actionOptions = useMemo(
    () => [
      { value: "proxy", label: t("routingLists.actionProxy") },
      { value: "direct", label: t("routingLists.actionDirect") },
      { value: "block", label: t("routingLists.actionBlock") },
    ],
    [t],
  );

  if (!isOpen) return null;

  const canSubmit =
    name.trim() &&
    url.trim() &&
    !(isInsecureURL && !isEdit && !allowInsecure) &&
    !saving;

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!canSubmit) return;
    setSaving(true);
    setError("");
    try {
      await onSubmit({
        name: name.trim(),
        url: url.trim(),
        action,
        allowInsecure,
      });
      onClose();
    } catch (err) {
      setError(String(err?.message || err));
      setSaving(false);
    }
  };

  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4">
      <div
        className="absolute inset-0 bg-black/70 backdrop-blur-md"
        onClick={saving ? undefined : onClose}
      />
      <form
        onSubmit={handleSubmit}
        className="relative bg-zinc-900 border border-zinc-800 w-full max-w-md p-6 rounded-3xl shadow-2xl space-y-5 animate-in zoom-in-95 duration-200"
      >
        <div className="flex items-center space-x-3">
          <div className="w-10 h-10 bg-[#007E3A]/10 rounded-xl flex items-center justify-center shrink-0">
            <ListPlus className="w-5 h-5 text-[#00A819]" />
          </div>
          <h3 className="text-xl font-bold text-white">
            {isEdit ? t("common.edit") : t("routingLists.add")}
          </h3>
        </div>

        <div className="space-y-2">
          <label className="text-sm font-medium text-zinc-400">
            {t("routingLists.name")}
          </label>
          <input
            type="text"
            autoFocus
            placeholder={t("routingLists.namePlaceholder")}
            className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>

        <div className="space-y-2">
          <label className="text-sm font-medium text-zinc-400">
            {t("routingLists.url")}
          </label>
          <input
            type="text"
            disabled={isEdit}
            placeholder={t("routingLists.urlPlaceholder")}
            className="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-white outline-none focus:outline-none focus:ring-0 focus-visible:outline-none focus:border-[#007E3A] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
        </div>

        <div className="space-y-2">
          <label className="text-sm font-medium text-zinc-400">
            {t("routingLists.action")}
          </label>
          <AppSelect
            value={action}
            options={actionOptions}
            onChange={setAction}
            ariaLabel={t("routingLists.action")}
            className="w-full"
            buttonClassName="w-full bg-zinc-950 border border-zinc-800 px-4 py-3 text-base hover:border-zinc-700 focus:ring-[#00A819]/35"
            listClassName="w-full"
            align="left"
          />
        </div>

        {!isEdit && isInsecureURL && (
          <label className="flex items-start gap-3 p-4 bg-rose-500/10 border border-rose-500/20 rounded-xl cursor-pointer">
            <input
              type="checkbox"
              checked={allowInsecure}
              onChange={(e) => setAllowInsecure(e.target.checked)}
              className="mt-0.5 w-4 h-4 accent-[#00A819] shrink-0"
            />
            <span className="text-sm text-rose-300">
              {t("routingLists.insecureConsent")}
            </span>
          </label>
        )}

        {error && (
          <p className="text-sm text-rose-500 break-words">{error}</p>
        )}

        <div className="flex space-x-3 pt-1">
          <button
            type="button"
            onClick={onClose}
            disabled={saving}
            className="flex-1 bg-zinc-800 hover:bg-zinc-700 text-white font-bold py-3 rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none disabled:opacity-50"
          >
            {t("common.cancel")}
          </button>
          <button
            type="submit"
            disabled={!canSubmit}
            className="flex-1 bg-[#007E3A] hover:bg-[#00A819] text-white font-bold py-3 rounded-xl transition-colors border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {saving ? "…" : t("common.save")}
          </button>
        </div>
      </form>
    </div>,
    document.body,
  );
};

export default RoutingListModal;
