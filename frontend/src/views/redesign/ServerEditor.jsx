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
 * Окно правки своего сервера.
 *
 * В макете его нет — собрано из кита по правилам самого макета, см.
 * docs/design/GAPS.md. Раньше правка открывалась целой страницей на вкладке
 * «Добавить» и вываливалась из нового дизайна: список серверов оставался
 * позади, а форма занимала весь экран ради полудюжины строк.
 *
 * Раскладка одна на все протоколы: сетка из одинаковых коробок по 64 —
 * ровно как у Select кита, чтобы поле, список и тумблер в одном ряду были
 * одним прямоугольником. Подписей над полями нет: каждое поле само носит свою
 * (Field), и пока оно пусто, видна только подсказка. Меняется от протокола
 * лишь набор полей — их размер и порядок следования не меняются никогда.
 */

import { useEffect, useState } from "react";
import { Button, Dialog, Field, Select, Textarea, Tumbler } from "../../components/kit";
import { VPN_NETWORK_OPTIONS } from "../../utils/proxyParser";
import {
  AMNEZIA_AWG3_KEYS,
  AMNEZIA_INT_KEYS,
  AMNEZIA_STR_KEYS,
  PLAIN_TYPES,
  SECURITY_OPTIONS,
  VPN_TYPES,
  buildServerData,
  readServerForm,
} from "./serverForm";
import "./ServerEditor.css";

/* Подписи. В приложении они идут через i18n, здесь — написание по умолчанию. */
export const SERVER_EDITOR_TEXT = {
  save: "Сохранить",
  cancel: "Отмена",

  name: "Название",
  host: "Адрес или хост",
  port: "Порт",
  protocol: "Протокол",
  login: "Логин",
  password: "Пароль",

  uuid: "ID",
  security: "Шифрование",
  network: "Транспорт",
  method: "Метод шифрования",
  path: "Путь",
  transportHost: "Host",
  grpcService: "Имя сервиса gRPC",
  httpHost: "HTTP host",
  httpPath: "HTTP path",
  xhttpMode: "Режим XHTTP",

  sni: "SNI",
  alpn: "ALPN",
  up: "Отдача, Мбит/с",
  down: "Приём, Мбит/с",
  obfsType: "Тип обфускации",
  obfsPassword: "Пароль обфускации",
  insecure: "Не проверять сертификат",
  naiveListen: "Listen (для справки)",
  naiveLog: "Log (для справки)",

  wgAddress: "Адрес, CIDR",
  wgAllowedIps: "Allowed IPs",
  wgPrivateKey: "Private key",
  wgPublicKey: "Peer public key",
  wgPsk: "Pre-shared key",
  wgKeepalive: "Keepalive, сек",
  wgReserved: "Reserved",
  wgMtu: "MTU",
  wgName: "Имя интерфейса",
  wgSystem: "Системный интерфейс",

  obfuscation: "Обфускация",
  useFields: "Поля",
  useJSON: "JSON",
  awg3: "AWG 3.0",
  headerKey: "Ключ защиты заголовков",
  contentPadding: "Добивка содержимого",
  rekeyAfter: "Rekey after",
  rekeyTimeout: "Rekey timeout",
  rejectAfter: "Reject after",
  keepaliveTimeout: "Keepalive timeout",
  maxHandshakes: "Max handshakes",
};

/* Тайминги AWG 3.0 идут парами «ключ — подпись»: подписи у них длиннее
   собственных имён, и списком ключей тут не обойтись. */
const AWG3_TIMINGS = [
  ["rekey_after_time", "rekeyAfter"],
  ["rekey_timeout", "rekeyTimeout"],
  ["reject_after_time", "rejectAfter"],
  ["keepalive_timeout", "keepaliveTimeout"],
  ["max_handshake_attempts", "maxHandshakes"],
];

/*
 * Что именно заменяет собой JSON. Только классический набор — junk-пакеты,
 * размеры, заголовки и signature-пакеты: у провайдеров обфускация приходит
 * такой, и вставляют её целиком. Ручки AWG 3.0 в него не входят, они остаются
 * полями в обоих видах.
 */
const AMNEZIA_JSON_KEYS = [...AMNEZIA_INT_KEYS, ...AMNEZIA_STR_KEYS];

const options = (list) => list.map((value) => ({ value, label: value }));

/* Ряд сетки. `cols` — сколько полей в нём помещается; по умолчанию два. */
function Grid({ cols = 2, children }) {
  return (
    <div className="rv-server-editor__grid" data-cols={cols}>
      {children}
    </div>
  );
}

/*
 * Коробка под то, что не является полем ввода, — список и тумблер. Держит ту
 * же высоту и ту же заливку, что Field, поэтому в сетке они одного роста;
 * подпись у неё постоянная, потому что скрывать её нечем: у списка значение
 * видно всегда.
 */
function Row({ label, children }) {
  return (
    <div className="rv-server-editor__row">
      <span className="rv-server-editor__row-label">{label}</span>
      {children}
    </div>
  );
}

export default function ServerEditor({
  open = true,
  proxy,
  onSave,
  onClose,
  busy = false,
  text = SERVER_EDITOR_TEXT,
}) {
  const [form, setForm] = useState(() => readServerForm(proxy));

  /* Поля набираются заново на каждое открытие: окно живёт дольше одной правки,
     и остатки прошлой сюда попадать не должны. */
  useEffect(() => {
    if (open) setForm(readServerForm(proxy));
  }, [open, proxy]);

  const type = form.type;
  const isPlain = !VPN_TYPES.includes(type);
  const isWg = type === "WIREGUARD" || type === "AMNEZIAWG";
  /* Сеть и её параметры есть только у тех трёх, у кого транспорт вообще
     выбирается: у остальных он задан протоколом. */
  const hasTransport = ["VLESS", "VMESS", "TROJAN"].includes(type);
  const net = form.network;

  const set = (key) => (event) => setForm((f) => ({ ...f, [key]: event.target.value }));
  const pick = (key) => (value) => setForm((f) => ({ ...f, [key]: value }));
  const setIn = (group, key) => (event) => {
    const { value } = event.target;
    setForm((f) => ({ ...f, [group]: { ...f[group], [key]: value } }));
  };
  const flagIn = (group, key) => (value) =>
    setForm((f) => ({ ...f, [group]: { ...f[group], [key]: value } }));
  const setTf = (key) => (event) => {
    const { value } = event.target;
    setForm((f) => ({ ...f, tf: { ...f.tf, [key]: value } }));
  };
  const setAm = (key) => (event) => {
    const { value } = event.target;
    setForm((f) => ({
      ...f,
      wg: { ...f.wg, amnezia: { ...f.wg.amnezia, [key]: value } },
    }));
  };

  /*
   * Переключение «Поля / JSON» переносит уже набранное из одного вида в
   * другой, а не начинает с чистого листа: иначе нажатие на переключатель
   * молча выбрасывало бы настроенную обфускацию.
   */
  const toggleRaw = () =>
    setForm((f) => {
      const wg = f.wg;
      if (!wg.amneziaUseRaw) {
        const obj = {};
        for (const k of AMNEZIA_JSON_KEYS) if (String(wg.amnezia[k]).trim()) obj[k] = wg.amnezia[k];
        const json = Object.keys(obj).length ? JSON.stringify(obj, null, 2) : wg.amneziaJSON;
        return { ...f, wg: { ...wg, amneziaUseRaw: true, amneziaJSON: json } };
      }
      const raw = (wg.amneziaJSON || "").trim();
      let amnezia = wg.amnezia;
      if (raw) {
        try {
          const parsed = JSON.parse(raw);
          if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
            amnezia = { ...wg.amnezia };
            for (const k of AMNEZIA_JSON_KEYS) {
              amnezia[k] = parsed[k] == null ? "" : String(parsed[k]);
            }
          }
        } catch {
          /* Не разобралось — поля остаются те, что были до JSON. */
        }
      }
      return { ...f, wg: { ...wg, amneziaUseRaw: false, amnezia } };
    });

  /*
   * Вставленный блок мог прийти с ручками AWG 3.0 внутри. Они переезжают в свои
   * поля, а из текста уходят: иначе одно и то же значение стояло бы в двух
   * местах сразу, и было бы непонятно, какое из них поедет в конфиг.
   *
   * Делается по уходу из поля, а не на каждую букву: посреди набора текст ещё
   * не разбирается, и перекладывать было бы нечего.
   */
  const liftAwg3 = () =>
    setForm((f) => {
      const raw = (f.wg.amneziaJSON || "").trim();
      if (!raw) return f;
      let parsed;
      try {
        parsed = JSON.parse(raw);
      } catch {
        return f;
      }
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return f;
      const moved = AMNEZIA_AWG3_KEYS.filter((k) => parsed[k] != null && parsed[k] !== "");
      if (moved.length === 0) return f;

      const amnezia = { ...f.wg.amnezia };
      const rest = { ...parsed };
      for (const k of moved) {
        amnezia[k] = String(parsed[k]);
        delete rest[k];
      }
      return {
        ...f,
        wg: {
          ...f.wg,
          amnezia,
          amneziaJSON: Object.keys(rest).length ? JSON.stringify(rest, null, 2) : "",
        },
      };
    });

  /* Узел без адреса или порта не запустится, и сохранять такое незачем. */
  const incomplete = !form.ip.trim() || !String(form.port).trim();

  return (
    <Dialog
      open={open}
      className="rv-server-editor"
      icon="edit"
      title={form.name || proxy?.name}
      subtitle={`${type} · ${form.ip || "—"}:${form.port || "—"}`}
      onClose={onClose}
    >
      <div className="rv-server-editor__body rv-scroll-dialog">
        <Grid>
          <Field className="rv-server-editor__wide" label={text.name} value={form.name} onChange={set("name")} />
          <Field label={text.host} value={form.ip} onChange={set("ip")} />
          <Field label={text.port} inputMode="numeric" value={form.port} onChange={set("port")} />
        </Grid>

        {isPlain && (
          <Grid>
            <Row label={text.protocol}>
              <Select value={type} options={options(PLAIN_TYPES)} onChange={pick("type")} />
            </Row>
            <Field label={text.login} autoComplete="off" value={form.username} onChange={set("username")} />
            <Field
              className="rv-server-editor__wide"
              label={text.password}
              type="password"
              autoComplete="off"
              value={form.password}
              onChange={set("password")}
            />
          </Grid>
        )}

        {(type === "VLESS" || type === "VMESS") && (
          <Grid>
            <Field
              className="rv-server-editor__wide"
              label={text.uuid}
              autoComplete="off"
              value={form.uuid}
              onChange={set("uuid")}
            />
            <Row label={text.security}>
              <Select
                value={form.security}
                options={options(SECURITY_OPTIONS)}
                onChange={pick("security")}
              />
            </Row>
          </Grid>
        )}

        {(type === "TROJAN" || type === "SS") && (
          <Grid>
            <Field
              className={type === "SS" ? "" : "rv-server-editor__wide"}
              label={text.password}
              type="password"
              autoComplete="off"
              value={form.password}
              onChange={set("password")}
            />
            {type === "SS" && (
              <Field label={text.method} value={form.ssMethod} onChange={set("ssMethod")} />
            )}
          </Grid>
        )}

        {hasTransport && (
          <Grid>
            <Row label={text.network}>
              <Select
                value={net}
                options={options(VPN_NETWORK_OPTIONS)}
                onChange={pick("network")}
              />
            </Row>
            {(net === "ws" || net === "httpupgrade" || net === "xhttp") && (
              <>
                <Field label={text.path} value={form.tf.transPath} onChange={setTf("transPath")} />
                <Field label={text.transportHost} value={form.tf.transHost} onChange={setTf("transHost")} />
              </>
            )}
            {net === "xhttp" && (
              <Field label={text.xhttpMode} value={form.tf.xhttpMode} onChange={setTf("xhttpMode")} />
            )}
            {net === "grpc" && (
              <Field label={text.grpcService} value={form.tf.grpcService} onChange={setTf("grpcService")} />
            )}
            {(net === "http" || net === "h2") && (
              <>
                <Field label={text.httpHost} value={form.tf.httpHost} onChange={setTf("httpHost")} />
                <Field label={text.httpPath} value={form.tf.httpPath} onChange={setTf("httpPath")} />
              </>
            )}
          </Grid>
        )}

        {type === "HYSTERIA2" && (
          <Grid>
            <Field
              className="rv-server-editor__wide"
              label={text.password}
              type="password"
              autoComplete="off"
              value={form.hy2.password}
              onChange={setIn("hy2", "password")}
            />
            <Field label={text.sni} value={form.hy2.sni} onChange={setIn("hy2", "sni")} />
            <Field label={text.alpn} value={form.hy2.alpn} onChange={setIn("hy2", "alpn")} />
            <Field
              label={text.up}
              inputMode="numeric"
              value={form.hy2.upMbps}
              onChange={setIn("hy2", "upMbps")}
            />
            <Field
              label={text.down}
              inputMode="numeric"
              value={form.hy2.downMbps}
              onChange={setIn("hy2", "downMbps")}
            />
            <Field
              label={text.obfsType}
              value={form.hy2.obfsType}
              onChange={setIn("hy2", "obfsType")}
            />
            <Field
              label={text.obfsPassword}
              type="password"
              autoComplete="off"
              value={form.hy2.obfsPassword}
              onChange={setIn("hy2", "obfsPassword")}
            />
            <Row label={text.insecure}>
              <Tumbler checked={form.hy2.insecure} onChange={flagIn("hy2", "insecure")} />
            </Row>
          </Grid>
        )}

        {type === "NAIVEPROXY" && (
          <Grid>
            <Field label={text.login} autoComplete="off" value={form.username} onChange={set("username")} />
            <Field
              label={text.password}
              type="password"
              autoComplete="off"
              value={form.password}
              onChange={set("password")}
            />
            <Field
              className="rv-server-editor__wide"
              label={text.sni}
              value={form.naive.sni}
              onChange={setIn("naive", "sni")}
            />
            <Field label={text.naiveListen} value={form.naive.listen} onChange={setIn("naive", "listen")} />
            <Field label={text.naiveLog} value={form.naive.log} onChange={setIn("naive", "log")} />
            <Row label={text.insecure}>
              <Tumbler checked={form.naive.insecure} onChange={flagIn("naive", "insecure")} />
            </Row>
          </Grid>
        )}

        {isWg && (
          <Grid>
            <Field label={text.wgAddress} value={form.wg.address} onChange={setIn("wg", "address")} />
            <Field label={text.wgAllowedIps} value={form.wg.allowedIps} onChange={setIn("wg", "allowedIps")} />
            <Field
              className="rv-server-editor__wide"
              label={text.wgPrivateKey}
              autoComplete="off"
              value={form.wg.privateKey}
              onChange={setIn("wg", "privateKey")}
            />
            <Field
              className="rv-server-editor__wide"
              label={text.wgPublicKey}
              autoComplete="off"
              value={form.wg.publicKey}
              onChange={setIn("wg", "publicKey")}
            />
            <Field
              label={text.wgPsk}
              autoComplete="off"
              value={form.wg.preSharedKey}
              onChange={setIn("wg", "preSharedKey")}
            />
            <Field
              label={text.wgKeepalive}
              inputMode="numeric"
              value={form.wg.keepalive}
              onChange={setIn("wg", "keepalive")}
            />
            <Field label={text.wgReserved} value={form.wg.reserved} onChange={setIn("wg", "reserved")} />
            <Field
              label={text.wgMtu}
              inputMode="numeric"
              value={form.wg.mtu}
              onChange={setIn("wg", "mtu")}
            />
            <Field label={text.wgName} value={form.wg.name} onChange={setIn("wg", "name")} />
            <Row label={text.wgSystem}>
              <Tumbler checked={form.wg.system} onChange={flagIn("wg", "system")} />
            </Row>
          </Grid>
        )}

        {type === "AMNEZIAWG" && (
          <>
            <div className="rv-server-editor__caption">
              <span>{text.obfuscation}</span>
              {/* Тот же переключатель, что был в прежней форме: набирать
                  два десятка чисел руками нужно не всегда — иногда обфускация
                  приходит готовым JSON и её проще вставить целиком. */}
              <button type="button" className="rv-server-editor__switch" onClick={toggleRaw}>
                {form.wg.amneziaUseRaw ? text.useFields : text.useJSON}
              </button>
            </div>

            {form.wg.amneziaUseRaw ? (
              <Textarea
                className="rv-server-editor__json rv-scroll-dialog"
                placeholder='{"jc":4,"jmin":1,"jmax":3,"s1":15,"s2":13}'
                value={form.wg.amneziaJSON}
                onChange={setIn("wg", "amneziaJSON")}
                onBlur={liftAwg3}
              />
            ) : (
              /* Все шестнадцать чисел — один ряд по четыре: имена у них
                 короткие и одинаковой длины, и группировать их подписями
                 значило бы разрезать ровную сетку на три лесенки. */
              <Grid cols={4}>
                {AMNEZIA_INT_KEYS.map((key) => (
                  <Field
                    key={key}
                    label={key.toUpperCase()}
                    inputMode="numeric"
                    value={form.wg.amnezia[key]}
                    onChange={setAm(key)}
                  />
                ))}
                {AMNEZIA_STR_KEYS.map((key) => (
                  <Field
                    key={key}
                    label={key.toUpperCase()}
                    value={form.wg.amnezia[key]}
                    onChange={setAm(key)}
                  />
                ))}
              </Grid>
            )}

            {/* AWG 3.0 стоит и при JSON: провайдерский блок обфускации этих
                ручек не носит, их набирают руками — и прятать их вместе с
                классическим набором значило бы прятать не то. */}
            <div className="rv-server-editor__caption">
              <span>{text.awg3}</span>
            </div>
            <Grid>
              <Field
                className="rv-server-editor__wide"
                label={text.headerKey}
                autoComplete="off"
                value={form.wg.amnezia.header_protection_key}
                onChange={setAm("header_protection_key")}
              />
              <Field
                label={text.contentPadding}
                value={form.wg.amnezia.content_padding_addition}
                onChange={setAm("content_padding_addition")}
              />
              {AWG3_TIMINGS.map(([key, label]) => (
                <Field
                  key={key}
                  label={text[label]}
                  value={form.wg.amnezia[key]}
                  onChange={setAm(key)}
                />
              ))}
            </Grid>
          </>
        )}

        <div className="rv-server-editor__buttons">
          <Button className="rv-server-editor__btn" onClick={onClose}>
            {text.cancel}
          </Button>
          <Button
            variant="green"
            className="rv-server-editor__btn"
            disabled={busy || incomplete}
            onClick={() => onSave?.(buildServerData(proxy, form))}
          >
            {text.save}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
