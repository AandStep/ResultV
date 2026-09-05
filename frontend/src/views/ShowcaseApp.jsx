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

/*
 * Оболочка витрины: разделы переключаются хешем, чтобы ссылку на нужный
 * раздел можно было держать открытой рядом с Figma.
 *   kit.html        — UI-kit
 *   kit.html#main   — главный экран
 *   kit.html#add    — добавление сервера
 */

import { useEffect, useState } from "react";
import KitShowcase from "./KitShowcase";
import MainPageShowcase from "./MainPageShowcase";
import AddPageShowcase from "./AddPageShowcase";

const SECTIONS = [
  { hash: "", title: "UI-kit", render: () => <KitShowcase /> },
  { hash: "#main", title: "Главный экран", render: () => <MainPageShowcase /> },
  { hash: "#add", title: "Добавить сервер", render: () => <AddPageShowcase /> },
];

export default function ShowcaseApp() {
  const [hash, setHash] = useState(() => window.location.hash);

  useEffect(() => {
    const onChange = () => setHash(window.location.hash);
    window.addEventListener("hashchange", onChange);
    return () => window.removeEventListener("hashchange", onChange);
  }, []);

  const current = SECTIONS.find((s) => s.hash === hash) ?? SECTIONS[0];

  return (
    <>
      <nav className="kit__nav" style={{ padding: "32px 48px 0" }}>
        {SECTIONS.map((section) => (
          <a
            key={section.title}
            href={section.hash || "#"}
            aria-current={section === current ? "page" : undefined}
          >
            {section.title}
          </a>
        ))}
      </nav>
      {current.render()}
    </>
  );
}
