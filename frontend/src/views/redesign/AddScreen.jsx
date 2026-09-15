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
 * Страница добавления сервера, подключённая к настоящему импорту.
 *
 * Вид держит AddPage, разбор — useProxyImport, а между ними стоит окно
 * «Добавление серверов» из макета (Figma 6721:3593 и 6721:3702): что нашли,
 * нужны ли списки маршрутизации из подписки, и две кнопки.
 */

import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { useConfigContext } from "../../context/ConfigContext";
import { PAGE_ADD, forgetPage, usePageState } from "../../hooks/usePageMemory";
import useProxyImport from "../../hooks/useProxyImport";
import AddPage, { ADD_PAGE_PROTOCOLS } from "./AddPage";
import AddConfirmDialog from "./AddConfirmDialog";
import AppSidebar from "./AppSidebar";

/*
 * Голому `ip:port` протокол не из чего взять — его задаёт выбор наверху
 * страницы. У ссылок протокол свой, и подставлять им ничего не нужно.
 */
const PLAIN_PROTOCOL = {
  http: "HTTP",
  socks5: "SOCKS5",
};

export default function AddScreen() {
  const { t } = useTranslation();
  const { showAlertDialog } = useConfigContext();
  const { busy, preview, importText, confirm, cancel } = useProxyImport();

  /*
   * Набранное переживает уход на другую страницу: экран размонтируется
   * целиком, и на полпути отойти посмотреть список серверов значило потерять
   * вставленную ссылку.
   *
   * Черновик гасится, когда импорт приняли, — см. `onConfirm` ниже. Иначе на
   * странице так и висела бы ссылка, которую уже разобрали и сохранили.
   */
  const [protocol, setProtocol] = usePageState(
    PAGE_ADD,
    "protocol",
    ADD_PAGE_PROTOCOLS[0].key,
  );
  const [value, setValue] = usePageState(PAGE_ADD, "value", "");
  /* Списки маршрутизации из подписки по умолчанию не берём — так нарисовано
     в макете и так подтвердил дизайнер. */
  const [routing, setRouting] = usePageState(PAGE_ADD, "routing", false);
  const fileRef = useRef(null);

  const onFile = (event) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (e) => importText(String(e.target.result || ""), file.name || "");
    reader.readAsText(file);
  };

  const onClipboard = async () => {
    try {
      const text = await navigator.clipboard.readText();
      if (!text) {
        showAlertDialog({
          title: t("common.notice"),
          message: t("add.clipboardEmpty"),
          variant: "warning",
        });
        return;
      }
      setValue(text);
      await importText(text);
    } catch (err) {
      console.error("Failed to read clipboard:", err);
      showAlertDialog({
        title: t("common.error"),
        message: t("add.clipboardError"),
        variant: "danger",
      });
    }
  };

  const text = {
    title: t("addPage.title"),
    subtitle: t("addPage.subtitle"),
    fromFile: t("addPage.fromFile"),
    fromClipboard: t("addPage.fromClipboard"),
    protocol: t("addPage.protocol"),
    link: t("addPage.link"),
    config: t("addPage.config"),
    submit: t("addPage.submit"),
  };

  return (
    <>
      <AddPage
        protocol={protocol}
        onProtocolChange={setProtocol}
        value={value}
        onValueChange={setValue}
        onFromFile={() => fileRef.current?.click()}
        onFromClipboard={onClipboard}
        onSubmit={() => importText(value)}
        busy={busy}
        text={text}
        sidebar={<AppSidebar />}
      />

      <input
        ref={fileRef}
        type="file"
        hidden
        accept=".txt,.json,.conf,.yaml,.yml,.b64"
        onChange={onFile}
      />

      {preview && (
        <AddConfirmDialog
          proxies={preview.proxies}
          routingLists={preview.routingLists}
          routing={routing}
          onRoutingChange={setRouting}
          onCancel={cancel}
          onConfirm={async () => {
            const imported = await confirm({
              protocol: PLAIN_PROTOCOL[protocol],
              routing,
            });
            /* Приняли — черновик больше не нужен; не приняли — он нужен
               человеку на месте, чтобы не набирать ссылку заново. К этому
               моменту страница обычно уже сменилась на список серверов, и
               настоящую работу делает `forgetPage`, а не `setValue`. */
            if (imported) {
              setValue("");
              forgetPage(PAGE_ADD);
            }
          }}
        />
      )}

    </>
  );
}
