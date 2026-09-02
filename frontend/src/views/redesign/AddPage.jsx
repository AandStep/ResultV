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
 * Страница «Добавить сервер». Figma "ResultV" -> App Design, ряд фреймов
 * `Add page`.
 *
 * Шесть фреймов — это одна раскладка в разных состояниях:
 *
 *   6533:1897  протокол VLESS, поле пустое, кнопка внизу погашена
 *   6557:2105  выбран AmneziaWG — меняются подпись поля и подсказка в нём
 *   6557:2208  поле в фокусе: рамка Main-color 20 %, каретка белым 50 %
 *   6557:2304  в поле есть текст — кнопка внизу оживает
 *   6557:2497  курсор на плитке «Из файла»
 *   6557:2596  плитка «Из файла» нажата
 *
 * Последние два состояния целиком лежат на компоненте кита BigBtn, а фокус и
 * заполнение поля — на Textarea; здесь только раскладка и то, что меняется от
 * выбранного протокола.
 */

import { BigBtn, Button, Textarea } from "../../components/kit";
import PageHeader from "./PageHeader";
import ScrollRow from "./ScrollRow";
import "./AddPage.css";

/*
 * Подписи, как они набраны в макете. Отдельным объектом, потому что в
 * приложении они пойдут через i18n.
 */
export const ADD_PAGE_TEXT = {
  title: "Добавить сервер",
  subtitle: "Добавьте подписку или сервер из файла, буфера обмена или вручную.",
  fromFile: "Из файла",
  fromClipboard: "Из буфера",
  protocol: "Выберите протокол",
  /* Подпись поля зависит от того, что в него вставляют. Обе — из макета. */
  link: "Вставьте ссылку",
  config: "Вставьте данные из файла JSON или .conf",
  submit: "Добавить сервер",
};

/*
 * Подсказка в поле нарисована для двух протоколов: у VLESS это ссылка, у
 * AmneziaWG — начало конфига. Остальным восьми подставляется своя схема:
 * ничего другого в этой строке не меняется.
 */
const linkHint = (scheme) => `https://example.com/sub или ${scheme}://...`;

/* У обычного WireGuard нет джиттера, поэтому строки `Jc` в его конфиге нет. */
const WG_HINT = "[Interface]\nPrivateKey = \nAddress = \nDNS = ";
const AWG_HINT = `${WG_HINT}\nJc = `;

/*
 * Порядок и написание — с макета (фрейм 6533:2107). Схемы взяты те, которые
 * разбирает сам парсер (utils/proxyParser.js).
 */
export const ADD_PAGE_PROTOCOLS = [
  { key: "vless", label: "VLESS", hint: linkHint("vless") },
  { key: "hysteria2", label: "Hysteria2", hint: linkHint("hysteria2") },
  { key: "amneziawg", label: "AmneziaWG", kind: "config", hint: AWG_HINT },
  { key: "wireguard", label: "Wireguard", kind: "config", hint: WG_HINT },
  { key: "trojan", label: "Trojan", hint: linkHint("trojan") },
  { key: "vmess", label: "VMESS", hint: linkHint("vmess") },
  { key: "ss", label: "SS", hint: linkHint("ss") },
  { key: "naive", label: "NaiveProxy", hint: linkHint("naive+https") },
  { key: "http", label: "HTTP(S)", hint: linkHint("http") },
  { key: "socks5", label: "Socks5", hint: linkHint("socks5") },
];

/*
 * Ряд протоколов шире панели (1664 против 814) и потому прокручивается —
 * этим занимается ScrollRow, здесь остаются только сами кнопки.
 */
function ProtocolRow({ protocols, value, onChange }) {
  return (
    <ScrollRow>
      {protocols.map((item) => (
        <Button
          key={item.key}
          variant="green"
          /* Выбранный протокол — залитое состояние кнопки из кита. */
          mode={item.key === value ? "idle" : undefined}
          aria-pressed={item.key === value}
          onClick={() => onChange?.(item.key)}
        >
          {item.label}
        </Button>
      ))}
    </ScrollRow>
  );
}

export default function AddPage({
  protocol = ADD_PAGE_PROTOCOLS[0].key,
  onProtocolChange,
  protocols = ADD_PAGE_PROTOCOLS,
  value = "",
  onValueChange,
  onFromFile,
  onFromClipboard,
  onSubmit,
  /* Пока разбираем вставленное, нажимать второй раз незачем. */
  busy = false,
  sidebar,
  text = ADD_PAGE_TEXT,
  className = "",
  ...rest
}) {
  const current = protocols.find((item) => item.key === protocol) ?? protocols[0];

  return (
    <div className={`rv-add-page ${className}`} {...rest}>
      {sidebar}

      <div className="rv-add-page__content">
        <PageHeader title={text.title} subtitle={text.subtitle} />

        <div className="rv-add-page__body">
          <div className="rv-add-page__sources">
            <BigBtn icon="uploadfile" label={text.fromFile} onClick={onFromFile} />
            <BigBtn icon="clipboard" label={text.fromClipboard} onClick={onFromClipboard} />
          </div>

          <div className="rv-add-page__panel">
            <div className="rv-add-page__field">
              <span className="rv-add-page__label">{text.protocol}</span>
              <ProtocolRow
                protocols={protocols}
                value={current?.key}
                onChange={onProtocolChange}
              />
            </div>

            <div className="rv-add-page__field rv-add-page__field--grow">
              <span className="rv-add-page__label">
                {current?.kind === "config" ? text.config : text.link}
              </span>
              <Textarea
                className="rv-add-page__textarea"
                placeholder={current?.hint}
                value={value}
                onChange={(event) => onValueChange?.(event.target.value)}
              />
            </div>

            <Button
              variant="green"
              className="rv-add-page__submit"
              /* Пустое поле — нечего добавлять: в макете кнопка погашена. */
              disabled={busy || value.trim() === ""}
              onClick={onSubmit}
            >
              {text.submit}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
