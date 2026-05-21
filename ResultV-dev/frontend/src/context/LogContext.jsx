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

import React, { createContext, useContext } from "react";
import { useLogs } from "../hooks/useLogs";

const LogContext = createContext();

export const LogProvider = ({ children }) => {
    const { logs, backendLogs, addLog } = useLogs();

    return (
        <LogContext.Provider value={{ logs, backendLogs, addLog }}>
            {children}
        </LogContext.Provider>
    );
};

export const useLogContext = () => {
    const context = useContext(LogContext);
    if (!context) throw new Error("useLogContext must be used within LogProvider");
    return context;
};
