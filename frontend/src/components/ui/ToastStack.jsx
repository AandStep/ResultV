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
import { AlertCircle, AlertTriangle, CheckCircle2, Info } from "lucide-react";

// The palette is the app's, not a new one: deep green rests, bright green
// confirms, amber is work in progress, rose is a failure — the same three
// hues the connect button and the server cards already speak in.
//
// `glow` reproduces the halo those surfaces use (see the active/connecting/
// failed card borders in ProxyListView). Inheriting it is the point: a toast
// should read as the same family of object as an active server card, not as a
// widget borrowed from somewhere else.
const VARIANT_STYLES = {
    info: {
        icon: Info,
        accent: "text-[#00A819]",
        border: "border-[#007E3A]/50",
        glow: "shadow-[0_0_24px_rgba(0,126,58,0.18)]",
        bar: "bg-[#007E3A]",
    },
    success: {
        icon: CheckCircle2,
        accent: "text-[#00A819]",
        border: "border-[#00A819]/50",
        glow: "shadow-[0_0_24px_rgba(0,168,25,0.20)]",
        bar: "bg-[#00A819]",
    },
    warning: {
        icon: AlertTriangle,
        accent: "text-amber-400",
        border: "border-amber-500/50",
        glow: "shadow-[0_0_24px_rgba(245,158,11,0.18)]",
        bar: "bg-amber-500",
    },
    error: {
        icon: AlertCircle,
        accent: "text-rose-400",
        border: "border-rose-500/50",
        glow: "shadow-[0_0_24px_rgba(244,63,94,0.18)]",
        bar: "bg-rose-500",
    },
};

const LEAVE_MS = 220;

const ToastItem = ({ toast, onDismiss }) => {
    // Mount at rest, then flip on the next frame so the entrance transition has
    // something to animate from. Without the frame gap the element is inserted
    // already in its final state and simply appears.
    const [entered, setEntered] = useState(false);
    const [leaving, setLeaving] = useState(false);

    useEffect(() => {
        const frame = requestAnimationFrame(() => setEntered(true));
        return () => cancelAnimationFrame(frame);
    }, []);

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
    const Icon = style.icon;

    const offstage = !entered || leaving;

    return (
        <div
            role="status"
            onClick={() => setLeaving(true)}
            className={`pointer-events-auto relative w-[340px] max-w-[calc(100vw-2rem)] cursor-pointer overflow-hidden rounded-2xl border bg-zinc-950/95 backdrop-blur-sm transition-all duration-200 ease-out motion-reduce:transition-none ${style.border} ${style.glow} ${
                offstage
                    ? "translate-x-3 scale-[0.98] opacity-0"
                    : "translate-x-0 scale-100 opacity-100"
            }`}
        >
            <div className="flex items-start gap-3 px-4 py-3.5">
                <Icon
                    size={18}
                    strokeWidth={2.25}
                    className={`mt-px shrink-0 ${style.accent}`}
                />
                {/* No title. The modal has one and it is exactly what makes the
                    modal heavy; a toast is read at a glance, so one line of
                    type doing one job is the whole content. */}
                <p className="text-sm font-medium leading-snug text-zinc-100">
                    {toast.message}
                </p>
            </div>

            <div
                aria-hidden
                style={{ animationDuration: `${toast.duration}ms` }}
                className={`absolute inset-x-0 bottom-0 h-0.5 origin-left animate-toast-drain motion-reduce:animate-none ${style.bar} ${
                    leaving ? "opacity-0" : "opacity-70"
                }`}
            />
        </div>
    );
};

export const ToastStack = ({ toasts, onDismiss }) => {
    if (!toasts || toasts.length === 0) return null;

    return (
        <div
            // top-11 clears the 32px title bar (TitleBar.jsx, h-8) with a 12px
            // gap, so toasts belong to the content area rather than floating
            // over the window chrome.
            //
            // The no-drag opt-out stays regardless: the title bar declares
            // itself a window-drag region, and anything that ends up over it —
            // a long stack, a smaller window — would inherit that and turn a
            // dismiss click into a window drag. TitleBar.jsx uses the same
            // opt-out for its own controls.
            style={{ "--wails-draggable": "no-drag" }}
            className="pointer-events-none fixed right-4 top-11 z-[100] flex flex-col gap-2.5"
        >
            {toasts.map((toast) => (
                <ToastItem key={toast.id} toast={toast} onDismiss={onDismiss} />
            ))}
        </div>
    );
};
