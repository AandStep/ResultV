/*
 * Copyright (C) 2026 ResultV
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 */

const kind = (p) => String(p?.type || "").toUpperCase();

/*
 * Остался ли сервер в списке. Повторный импорт подписки выдаёт серверам новые
 * ID, поэтому без совпадения по ID сервер ищется по адресу, а группа «Авто» —
 * по имени в той же подписке.
 */
export function isProxyListed(list, target) {
    if (!target) return false;
    const id = String(target.id ?? "");
    if (id && list.some((p) => String(p.id) === id && kind(p) !== "SECTION")) return true;
    const type = kind(target);
    if (type === "SECTION") return false;
    if (type === "AUTO") {
        return list.some(
            (p) => kind(p) === "AUTO" && p.name === target.name && p.subscriptionUrl === target.subscriptionUrl,
        );
    }
    if (!target.ip) return false;
    const port = Number(target.port) || 0;
    return list.some(
        (p) => kind(p) === type && p.ip === target.ip && (Number(p.port) || 0) === port,
    );
}
