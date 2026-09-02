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
 * Страница «Добавить сервер» в размере окна 1000x740 — одной живой страницей.
 *
 * Все шесть фреймов макета проходятся руками: протокол переключается
 * кнопками, поле в фокусе получает рамку, с текстом оживает кнопка внизу, а
 * плитки отзываются на наведение и нажатие сами.
 */

import { useRef, useState } from "react";
import { Sidebar } from "../components/kit";
import AddPage, { ADD_PAGE_PROTOCOLS } from "./redesign/AddPage";
import { MENU_ITEMS, SETTINGS_ITEM } from "./showcase-stubs";
import "./KitShowcase.css";

export default function AddPageShowcase() {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [protocol, setProtocol] = useState(ADD_PAGE_PROTOCOLS[0].key);
  const [value, setValue] = useState("");
  const fileRef = useRef(null);

  /*
   * В приложении на этих плитках будут диалог Wails и буфер обмена ядра;
   * витрине хватает браузерных — важно видеть, что текст попадает в поле.
   */
  const onFile = (event) => {
    const file = event.target.files?.[0];
    if (!file) return;
    file.text().then(setValue);
    event.target.value = "";
  };

  const onClipboard = () => {
    navigator.clipboard?.readText().then(setValue, () => {});
  };

  return (
    <div className="kit">
      <div className="kit__intro">
        <h1>Добавить сервер</h1>
        <p>
          Окно 1000×740, как в макете. Протокол переключается кнопками — у
          AmneziaWG и Wireguard меняются подпись поля и подсказка в нём.
          Поле в фокусе получает рамку, а как только в нём появляется текст,
          кнопка внизу оживает.
        </p>
        <p>
          Ряд протоколов шире панели: он прокручивается колесом, а растяжка
          справа гаснет, когда доехали до конца. Плитки «Из файла» и «Из
          буфера» здесь открывают браузерный диалог и читают буфер — в
          приложении на их месте будут диалог Wails и чтение через ядро.
        </p>
      </div>

      <div className="kit__frame">
        <AddPage
          protocol={protocol}
          onProtocolChange={setProtocol}
          value={value}
          onValueChange={setValue}
          onFromFile={() => fileRef.current?.click()}
          onFromClipboard={onClipboard}
          sidebar={
            <Sidebar
              opened={sidebarOpen}
              onToggle={() => setSidebarOpen((v) => !v)}
              items={MENU_ITEMS}
              bottomItem={SETTINGS_ITEM}
              activeKey="add"
            />
          }
        />
      </div>

      <input ref={fileRef} type="file" hidden onChange={onFile} />
    </div>
  );
}
