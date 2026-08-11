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

import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Circle, Sparkles, TrendingUp, Wrench, X } from "lucide-react";

// One hue per kind of change, drawn from the palette the app already speaks in
// (see VARIANT_STYLES in ToastStack): bright green announces, deep green
// improves, amber is maintenance. Rose is deliberately absent — in this app it
// means failure, and nothing in a changelog is a failure.
const TYPE_STYLES = {
    feature: {
        icon: Sparkles,
        accent: "text-[#00A819]",
        chip: "bg-[#00A819]/10 border-[#00A819]/30",
        labelKey: "changelog.typeFeature",
        labelFallback: "Новое",
    },
    improve: {
        icon: TrendingUp,
        accent: "text-[#007E3A]",
        chip: "bg-[#007E3A]/10 border-[#007E3A]/30",
        labelKey: "changelog.typeImprove",
        labelFallback: "Улучшение",
    },
    fix: {
        icon: Wrench,
        accent: "text-amber-400",
        chip: "bg-amber-500/10 border-amber-500/30",
        labelKey: "changelog.typeFix",
        labelFallback: "Исправление",
    },
};

// An entry with no type, or a type added to the manifest after this build
// shipped, still deserves to be readable — it just gets a neutral bullet.
const NEUTRAL_STYLE = {
    icon: Circle,
    accent: "text-zinc-400",
    chip: "bg-zinc-800/60 border-zinc-700",
    labelKey: "changelog.typeOther",
    labelFallback: "Изменение",
};

const ITEM_STAGGER_MS = 60;

const ChangelogItemRow = ({ item, index }) => {
    const { t } = useTranslation();
    const style = TYPE_STYLES[item.type] || NEUTRAL_STYLE;
    const Icon = style.icon;

    // Mount at rest and flip on the next frame, the same way ToastItem does —
    // an element inserted already in its final state has nothing to animate
    // from and simply appears.
    const [entered, setEntered] = useState(false);
    useEffect(() => {
        const timer = setTimeout(() => setEntered(true), index * ITEM_STAGGER_MS);
        return () => clearTimeout(timer);
    }, [index]);

    return (
        <li
            className={`flex items-start gap-3 transition-all duration-300 ease-out motion-reduce:transition-none ${
                entered
                    ? "translate-y-0 opacity-100"
                    : "translate-y-1.5 opacity-0 motion-reduce:translate-y-0 motion-reduce:opacity-100"
            }`}
        >
            <span
                className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg border ${style.chip}`}
                title={t(style.labelKey, style.labelFallback)}
            >
                <Icon size={14} strokeWidth={2.25} className={style.accent} />
            </span>
            <span className="pt-1 text-sm leading-relaxed text-zinc-300">
                {item.text}
            </span>
        </li>
    );
};

/**
 * ChangelogModal — "what's new" after an update.
 *
 * Pure presentation. Whether it is due, and what it says, are both decided in
 * Go (ShouldShowChangelog / GetChangelog) and handed over by useChangelog.
 *
 * Props:
 *   changelog — { version, title, items: [{ type, text }] }, or null to render nothing
 *   onClose   — dismiss callback
 */
const ChangelogModal = ({ changelog, onClose }) => {
    const { t } = useTranslation();

    // Escape closes it. Every other modal in the app can be dismissed with a
    // click; this one has nothing destructive behind it, so a key is fine too.
    useEffect(() => {
        if (!changelog) return undefined;
        const onKeyDown = (e) => {
            if (e.key === "Escape") onClose();
        };
        window.addEventListener("keydown", onKeyDown);
        return () => window.removeEventListener("keydown", onKeyDown);
    }, [changelog, onClose]);

    if (!changelog || !changelog.items?.length) return null;

    return (
        <div
            className="fixed inset-0 z-[130] flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
            role="dialog"
            aria-modal="true"
            aria-labelledby="changelog-title"
        >
            <div className="relative flex max-h-[80vh] w-full max-w-md flex-col overflow-hidden rounded-3xl border border-zinc-800 bg-zinc-950 shadow-2xl animate-fade-in-up">
                {/* A single soft green wash behind the header, the same halo the
                    active server card and the connect button carry. It is what
                    makes this read as a moment rather than as another dialog. */}
                <div
                    aria-hidden
                    className="pointer-events-none absolute inset-x-0 top-0 h-32 bg-[radial-gradient(ellipse_at_top,rgba(0,168,25,0.18),transparent_70%)]"
                />

                <button
                    type="button"
                    onClick={onClose}
                    className="absolute right-4 top-4 z-10 text-zinc-500 transition-colors hover:text-[#00A819] outline-none focus:outline-none focus:ring-0"
                    aria-label={t("common.close", "Закрыть")}
                >
                    <X size={20} />
                </button>

                <div className="relative px-6 pt-6 pb-4">
                    <div className="mb-3 flex items-center gap-3 pr-8">
                        <Sparkles size={24} className="shrink-0 text-[#00A819]" />
                        <h3 id="changelog-title" className="text-xl font-bold text-white">
                            {t("changelog.title", "Что нового")}
                        </h3>
                        <span className="rounded-full border border-[#007E3A]/40 bg-[#007E3A]/15 px-2.5 py-0.5 text-xs font-bold text-[#00A819]">
                            {changelog.version}
                        </span>
                    </div>

                    {changelog.title && (
                        <p className="text-sm font-medium text-zinc-400">
                            {changelog.title}
                        </p>
                    )}
                </div>

                <ul className="min-h-0 flex-1 space-y-3.5 overflow-y-auto px-6 pb-5">
                    {changelog.items.map((item, i) => (
                        <ChangelogItemRow key={i} item={item} index={i} />
                    ))}
                </ul>

                <div className="border-t border-zinc-900 px-6 py-4">
                    <button
                        type="button"
                        onClick={onClose}
                        className="w-full rounded-xl border-transparent bg-[#007E3A] py-3 px-4 font-bold text-white transition-all hover:bg-[#00A819] outline-none focus:outline-none"
                    >
                        {t("changelog.dismiss", "Отлично")}
                    </button>
                </div>
            </div>
        </div>
    );
};

export default ChangelogModal;
