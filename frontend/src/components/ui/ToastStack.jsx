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
import Icon from "../kit/Icon";
import "./ToastStack.css";

/*
 * Тост нового дизайна — Figma "ResultV" -> App Design, слой 6725:4321 на
 * фрейме MainPage 6725:4223.
 *
 * Цвета у плашки одного набора при любом поводе: серая подложка, значок
 * белый 50 %, подпись белая 80 %. Это не упрощение — все четыре места, где
 * приложение вызывает тост, зовут его ради подсказки (`variant: "info"`), а
 * сам макет нарисован ровно с текстом одной из них. Всё, что требует другого
 * цвета, в новом дизайне идёт окном (Dialog с вариантами). Поэтому `variant`
 * в API остался, но на вид не влияет — см. docs/design/GAPS.md.
 */

const LEAVE_MS = 300;

const ToastItem = ({ toast, onDismiss }) => {
    // Плашка монтируется за краем окна и уезжает на место следующим кадром:
    // элемент, вставленный сразу в конечном виде, ехать неоткуда и он просто
    // появляется.
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

    return (
        <div
            role="status"
            className="rv-toast rv-border rv-border--static"
            data-offstage={!entered || leaving ? "true" : "false"}
            onClick={() => setLeaving(true)}
        >
            <span className="rv-toast__icon">
                <Icon name="comment" size={24} color="currentColor" />
            </span>
            <p className="rv-toast__text">{toast.message}</p>
        </div>
    );
};

export const ToastStack = ({ toasts, onDismiss }) => {
    if (!toasts || toasts.length === 0) return null;

    return (
        <div className="rv-toast-stack">
            {toasts.map((toast) => (
                <ToastItem key={toast.id} toast={toast} onDismiss={onDismiss} />
            ))}
        </div>
    );
};
