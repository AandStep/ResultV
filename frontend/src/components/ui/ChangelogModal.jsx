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

import { useTranslation } from "react-i18next";
import { Badge, Button, Dialog, Icon } from "../kit";
import "./ChangelogModal.css";

/*
 * Один род изменения — один цвет и один значок. Взято с окна 6816:4780:
 * исправление жёлтое, улучшение ярко-зелёное, новое — основным зелёным.
 * Красного здесь нет намеренно: в этом приложении он значит отказ, а в списке
 * изменений отказов не бывает.
 */
const TYPES = {
    feature: { icon: "feature", labelKey: "changelog.typeFeature" },
    improve: { icon: "improve", labelKey: "changelog.typeImprove" },
    fix: { icon: "fix", labelKey: "changelog.typeFix" },
};

/*
 * Строка без рода изменения — или с тем, что добавили в манифест уже после
 * этой сборки, — всё равно должна читаться. В макете такого кадра нет:
 * значок взят нейтральный, тот же, что у подсказки, и цвета у него нет.
 */
const OTHER = { icon: "comment", labelKey: "changelog.typeOther" };

/**
 * ChangelogModal — «Что нового?» после обновления.
 *
 * Только вид. Положено ли его показывать и что в нём написано, решает Go
 * (ShouldShowChangelog / GetChangelog), а приносит useChangelog.
 *
 * Props:
 *   changelog — { version, title, items: [{ type, text }] } либо null
 *   onClose   — закрытие
 */
export default function ChangelogModal({ changelog, onClose }) {
    const { t } = useTranslation();

    if (!changelog || !changelog.items?.length) return null;

    return (
        <Dialog
            className="rv-changelog"
            icon="fire"
            title={t("changelog.title", "Что нового?")}
            /* Версию приносит сама сборка; пустой она бывает только там, где
               окно вызвано принудительно, и тогда бейджа просто нет. */
            subtitle={
                changelog.version ? (
                    <Badge color="success">
                        {t("changelog.version", { version: changelog.version })}
                    </Badge>
                ) : null
            }
            onClose={onClose}
            actions={
                <Button variant="green" onClick={onClose}>
                    {t("changelog.dismiss", "Отлично")}
                </Button>
            }
        >
            <div className="rv-changelog__body">
                {changelog.title && (
                    <p className="rv-changelog__release">{changelog.title}</p>
                )}

                <ul className="rv-changelog__items">
                    {changelog.items.map((item, i) => {
                        const kind = TYPES[item.type];
                        const look = kind ?? OTHER;
                        return (
                            <li
                                key={i}
                                className="rv-changelog__item"
                                data-type={kind ? item.type : "other"}
                            >
                                <span
                                    className="rv-changelog__item-icon"
                                    title={t(look.labelKey)}
                                >
                                    <Icon
                                        name={look.icon}
                                        size={20}
                                        color="currentColor"
                                    />
                                </span>
                                <p className="rv-changelog__item-text">
                                    {item.text}
                                </p>
                            </li>
                        );
                    })}
                </ul>
            </div>
        </Dialog>
    );
}
