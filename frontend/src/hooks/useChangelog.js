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

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import wailsAPI from "../utils/wailsAPI";

/**
 * useChangelog — decides nothing on its own. Go owns the "is it due" policy
 * (ShouldShowChangelog) and the notes themselves (GetChangelog, read from the
 * update.json embedded in this very build). This hook only fetches once and
 * remembers the dismissal.
 *
 * @param {boolean} ready — pass the app's isConfigLoaded. ShouldShowChangelog
 *   reads the config manager, which Go fills in during startup; asking before
 *   that would race and silently answer "no". isConfigLoaded is only true once
 *   GetConfig has come back, so it is exactly the signal we need.
 *
 * Returns { changelog, dismiss } where changelog is null when there is nothing
 * to show — either not due, or the embedded manifest carries no notes.
 */
export const useChangelog = (ready = true) => {
    const { i18n } = useTranslation();
    const [changelog, setChangelog] = useState(null);

    // The notes belong to the build, so they never change while the app runs.
    // Fetching once keeps a language switch from re-opening a modal the user
    // has already closed.
    const requestedRef = useRef(false);

    useEffect(() => {
        if (!ready || requestedRef.current) return undefined;
        requestedRef.current = true;

        let cancelled = false;
        (async () => {
            const due = await wailsAPI.shouldShowChangelog();
            if (!due || cancelled) return;
            const data = await wailsAPI.getChangelog(i18n.language);
            if (cancelled) return;
            if (data && Array.isArray(data.items) && data.items.length > 0) {
                setChangelog(data);
            }
        })();

        return () => {
            cancelled = true;
        };
    }, [ready, i18n.language]);

    const dismiss = useCallback(() => {
        setChangelog(null);
        // Records the running version as seen, so the modal stays away until
        // the next update.
        wailsAPI.ackChangelog();
    }, []);

    return { changelog, dismiss };
};
