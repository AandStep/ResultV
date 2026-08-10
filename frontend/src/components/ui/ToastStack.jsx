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

// Same four levels AppDialogModal already understands
// (components/ui/AppDialogModal.jsx) — one severity scale for the whole app,
// not two.
const VARIANT_STYLES = {
    info: "border-sky-500/40 bg-sky-950/80 text-sky-100",
    success: "border-[#00A819]/40 bg-[#00280f]/85 text-[#7ee89a]",
    warning: "border-amber-500/40 bg-amber-950/80 text-amber-100",
    error: "border-rose-500/40 bg-rose-950/80 text-rose-100",
};

const LEAVE_MS = 200;

const ToastItem = ({ toast, onDismiss }) => {
    const [leaving, setLeaving] = useState(false);

    useEffect(() => {
        const timer = setTimeout(() => setLeaving(true), toast.duration);
        return () => clearTimeout(timer);
    }, [toast.duration]);

    useEffect(() => {
        if (!leaving) return undefined;
        const timer = setTimeout(() => onDismiss(toast.id), LEAVE_MS);
        return () => clearTimeout(timer);
    }, [leaving, onDismiss, toast.id]);

    const style = VARIANT_STYLES[toast.variant] || VARIANT_STYLES.info;

    return (
        <div
            onClick={() => setLeaving(true)}
            className={`pointer-events-auto w-[320px] max-w-[80vw] cursor-pointer rounded-[12px] border px-4 py-3 text-sm leading-snug shadow-lg backdrop-blur-sm transition-all duration-200 ${style} ${
                leaving
                    ? "translate-x-2 opacity-0"
                    : "translate-x-0 opacity-100"
            }`}
        >
            {toast.message}
        </div>
    );
};

export const ToastStack = ({ toasts, onDismiss }) => {
    if (!toasts || toasts.length === 0) return null;

    return (
        <div
            // The title bar declares itself a window-drag region
            // (components/layout/TitleBar.jsx). Anything floating over it
            // inherits that, and a click meant to dismiss a toast would be
            // swallowed and become a window drag instead. TitleBar.jsx already
            // uses this same opt-out for its own interactive controls.
            style={{ "--wails-draggable": "no-drag" }}
            className="pointer-events-none fixed right-4 top-4 z-[100] flex flex-col gap-2"
        >
            {toasts.map((toast) => (
                <ToastItem key={toast.id} toast={toast} onDismiss={onDismiss} />
            ))}
        </div>
    );
};
