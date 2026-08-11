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
import { LogProvider } from "./LogContext";
import { ToastProvider } from "./ToastContext";
import { ConfigProvider } from "./ConfigContext";
import { ConnectionProvider } from "./ConnectionContext";

export const AppProvider = ({ children }) => {
    return (
        <LogProvider>
            {/* Outside ConnectionProvider: useDaemonControl raises toasts, so
                the toast context has to already exist by the time it runs. */}
            <ToastProvider>
                <ConfigProvider>
                    <ConnectionProvider>{children}</ConnectionProvider>
                </ConfigProvider>
            </ToastProvider>
        </LogProvider>
    );
};
