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

import React, {
    createContext,
    useCallback,
    useContext,
    useMemo,
    useRef,
    useState,
} from "react";
import { ToastStack } from "../components/ui/ToastStack";

const ToastContext = createContext(null);

// How many toasts are on screen at once. Beyond this they wait rather than
// stack — a burst of events must not paper over the app.
const MAX_VISIBLE = 3;

const DEFAULT_DURATION = 4000;

export const ToastProvider = ({ children }) => {
    const [toasts, setToasts] = useState([]);
    const nextIdRef = useRef(0);

    const dismissToast = useCallback((id) => {
        setToasts((prev) => prev.filter((toast) => toast.id !== id));
    }, []);

    const showToast = useCallback(
        ({ variant = "info", message, duration = DEFAULT_DURATION } = {}) => {
            if (!message) return;
            nextIdRef.current += 1;
            const toast = { id: nextIdRef.current, variant, message, duration };
            // Newest first. When more than MAX_VISIBLE are queued the oldest
            // are the ones dropped from view, which is the right trade for
            // status messages: a status that waited its turn is already stale.
            setToasts((prev) => [toast, ...prev]);
        },
        [],
    );

    // Only showToast is exposed, so a consumer re-renders on nothing but its
    // own state — the toast list itself lives here and never leaves.
    const value = useMemo(() => ({ showToast }), [showToast]);

    return (
        <ToastContext.Provider value={value}>
            {children}
            <ToastStack
                toasts={toasts.slice(0, MAX_VISIBLE)}
                onDismiss={dismissToast}
            />
        </ToastContext.Provider>
    );
};

export const useToast = () => {
    const ctx = useContext(ToastContext);
    if (!ctx) {
        throw new Error("useToast must be used inside ToastProvider");
    }
    return ctx;
};
