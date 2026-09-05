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
 * Перевод сервера в поля окна правки и обратно.
 *
 * Вынесено из самого окна, потому что это не разметка, а разбор: у каждого
 * протокола свой набор ключей в `extra`, и половина их читается из двух
 * написаний сразу (`up_mbps` и `upMbps`, `private_key` и `privateKey`) —
 * подписки приходят и в snake_case, и в camelCase.
 *
 * Правило сборки одно и важное: `extra` пересобирается ПОВЕРХ прежнего, а не
 * с нуля. В нём лежат поля, которых в окне нет вовсе — отпечаток клиента,
 * ключи Reality, флаг xtls. Собери его заново — и правка имени вырубила бы
 * узлу шифрование. Пустые значения при этом удаляются: ядро разбирает конфиг
 * строго, и пустая строка на месте числа роняет старт целиком.
 */

import {
  applyVpnTransportFieldsToExtra,
  normalizeNetworkForSelect,
  normalizeSecurityForSelect,
  parseProxyExtra,
  readVpnTransportFieldsFromExtra,
  sanitizeVpnExtraForEdit,
} from "../../utils/proxyParser";

/* Обычные прокси: тип у них меняется прямо в окне — он ниоткуда не выводится. */
export const PLAIN_TYPES = ["HTTP", "HTTPS", "SOCKS5"];

export const VPN_TYPES = [
  "VLESS",
  "VMESS",
  "TROJAN",
  "SS",
  "WIREGUARD",
  "AMNEZIAWG",
  "HYSTERIA2",
  "NAIVEPROXY",
];

export const SECURITY_OPTIONS = ["none", "tls", "reality"];

/*
 * j1-j3 и itime отсутствуют намеренно: у форка wireguard-go под движком нет
 * ключей UAPI под них, и любое значение здесь только уронило бы IpcSet.
 */
export const AMNEZIA_INT_KEYS = [
  "jc",
  "jmin",
  "jmax",
  "s1",
  "s2",
  "s3",
  "s4",
  "h1",
  "h2",
  "h3",
  "h4",
];
export const AMNEZIA_STR_KEYS = ["i1", "i2", "i3", "i4", "i5"];
/* Ручки AmneziaWG 3.0. Все строковые: тайминги и добивка принимают и «n», и
   диапазон «от-до», а ключ защиты заголовков — 64 знака hex. */
export const AMNEZIA_AWG3_KEYS = [
  "header_protection_key",
  "content_padding_addition",
  "rekey_after_time",
  "rekey_timeout",
  "reject_after_time",
  "keepalive_timeout",
  "max_handshake_attempts",
];

const AMNEZIA_KEYS = [...AMNEZIA_INT_KEYS, ...AMNEZIA_STR_KEYS, ...AMNEZIA_AWG3_KEYS];

const EMPTY_AMNEZIA = Object.fromEntries(AMNEZIA_KEYS.map((k) => [k, ""]));

export const typeOf = (proxy) => String(proxy?.type || "").toUpperCase();
export const isVpn = (proxy) => VPN_TYPES.includes(typeOf(proxy));

const str = (v) => (v == null ? "" : String(v));
/* Число могло прийти нулём, и `||` съел бы его вместе с пустотой. */
const num = (v) => (v == null || v === "" ? "" : String(v));
const list = (v) => (Array.isArray(v) ? v.join(",") : str(v));

const tokens = (text) =>
  String(text || "")
    .split(/[,;\n\r\t]+/g)
    .map((s) => s.trim())
    .filter(Boolean);

export const amneziaFromObject = (raw) => {
  const out = { ...EMPTY_AMNEZIA };
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return out;
  for (const k of AMNEZIA_KEYS) {
    const v = raw[k];
    if (v === undefined || v === null || v === "") continue;
    out[k] = String(v);
  }
  return out;
};

export const amneziaToObject = (fields) => {
  const obj = {};
  for (const k of AMNEZIA_INT_KEYS) {
    const s = String(fields?.[k] ?? "").trim();
    if (s === "") continue;
    const n = Number(s);
    if (Number.isFinite(n) && n >= 0) obj[k] = n;
  }
  for (const k of [...AMNEZIA_STR_KEYS, ...AMNEZIA_AWG3_KEYS]) {
    const s = String(fields?.[k] ?? "").trim();
    if (s) obj[k] = s;
  }
  return obj;
};

/** Сервер -> поля окна. */
export function readServerForm(proxy) {
  const type = typeOf(proxy);
  const ex = parseProxyExtra(proxy?.extra);
  const amnezia =
    ex.amnezia && typeof ex.amnezia === "object" && !Array.isArray(ex.amnezia)
      ? ex.amnezia
      : null;

  return {
    name: str(proxy?.name),
    ip: str(proxy?.ip),
    port: num(proxy?.port),
    type: type || "HTTP",
    username: str(proxy?.username),
    password: str(proxy?.password),

    uuid: str(ex.uuid),
    security: normalizeSecurityForSelect(ex.security),
    network: normalizeNetworkForSelect(ex.network),
    ssMethod: str(ex.method) || "aes-256-gcm",
    tf: readVpnTransportFieldsFromExtra(proxy?.extra),

    hy2: {
      password: str(ex.password || proxy?.password),
      sni: str(ex.sni || ex.server_name),
      alpn: str(ex.alpn) || "h3",
      insecure: Boolean(ex.insecure),
      upMbps: num(ex.up_mbps ?? ex.upMbps),
      downMbps: num(ex.down_mbps ?? ex.downMbps),
      obfsType: str(ex.obfs_type || ex.obfsType),
      obfsPassword: str(ex.obfs_password || ex.obfsPassword),
    },

    naive: {
      sni: str(ex.sni || ex.server_name),
      insecure: Boolean(ex.insecure),
      listen: str(ex.naive_listen),
      log: str(ex.naive_log),
    },

    wg: {
      address: list(ex.address) || "10.0.0.2/32",
      privateKey: str(ex.private_key || ex.privateKey),
      publicKey: str(ex.public_key || ex.publicKey),
      preSharedKey: str(ex.pre_shared_key || ex.preSharedKey),
      allowedIps: list(ex.allowed_ips) || "0.0.0.0/0",
      reserved: list(ex.reserved),
      keepalive: num(ex.persistent_keepalive_interval ?? ex.persistentKeepaliveInterval),
      system: Boolean(ex.system),
      name: str(ex.name),
      mtu: num(ex.mtu),
      amnezia: amneziaFromObject(amnezia),
      amneziaJSON: amnezia ? JSON.stringify(amnezia, null, 2) : "",
      amneziaUseRaw: false,
    },
  };
}

/*
 * Обфускация приходит либо полями, либо текстом JSON — окно даёт переключить
 * одно на другое. Битый JSON не должен затирать то, что уже настроено, поэтому
 * при неудачном разборе возвращается `undefined` и прежнее значение остаётся.
 *
 * Ручки AWG 3.0 остаются полями в обоих видах: JSON у провайдеров — это
 * классический набор junk-пакетов и размеров, а тайминги и защиту заголовков
 * набирают руками. Что видно на экране, то и главнее: заполненное поле AWG 3.0
 * перекрывает одноимённый ключ из JSON. Ключ, которому поле пусто, из JSON не
 * пропадает — иначе вставленный целиком блок терял бы часть себя.
 */
function amneziaOf(wg) {
  if (!wg?.amneziaUseRaw) return amneziaToObject(wg?.amnezia);

  const awg3 = {};
  for (const k of AMNEZIA_AWG3_KEYS) {
    const v = String(wg?.amnezia?.[k] ?? "").trim();
    if (v) awg3[k] = v;
  }

  const raw = String(wg?.amneziaJSON || "").trim();
  if (!raw) return awg3;
  try {
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return { ...parsed, ...awg3 };
    }
  } catch {
    /* Разобрать не вышло — правки обфускации в этот раз не применяются. */
  }
  return undefined;
}

/** Поля окна -> сервер, готовый к сохранению. */
export function buildServerData(proxy, form) {
  const type = String(form.type || "").toUpperCase();
  const base = {
    ...proxy,
    name: form.name.trim(),
    ip: form.ip.trim(),
    port: form.port,
    type,
    username: form.username,
    password: form.password,
  };

  if (!VPN_TYPES.includes(type)) {
    /* У обычного прокси весь профиль — это адрес, порт и пара логин/пароль:
       `extra` ему взяться неоткуда, и трогать его нечем. */
    return base;
  }

  if (type === "HYSTERIA2") {
    const ex = { ...parseProxyExtra(proxy?.extra) };
    ex.password = form.hy2.password.trim();
    ex.sni = form.hy2.sni.trim();
    ex.alpn = form.hy2.alpn.trim();
    ex.insecure = Boolean(form.hy2.insecure);
    ex.up_mbps = parseInt(form.hy2.upMbps, 10) || 0;
    ex.down_mbps = parseInt(form.hy2.downMbps, 10) || 0;
    ex.obfs_type = form.hy2.obfsType.trim();
    ex.obfs_password = form.hy2.obfsPassword.trim();
    if (!ex.up_mbps) delete ex.up_mbps;
    if (!ex.down_mbps) delete ex.down_mbps;
    /* Пароль обфускации без её типа ядру не нужен и только сбивает разбор. */
    if (!ex.obfs_type) {
      delete ex.obfs_type;
      delete ex.obfs_password;
    }
    return { ...base, extra: ex };
  }

  if (type === "NAIVEPROXY") {
    const ex = { ...parseProxyExtra(proxy?.extra) };
    ex.sni = form.naive.sni.trim();
    ex.insecure = Boolean(form.naive.insecure);
    if (form.naive.listen.trim()) ex.naive_listen = form.naive.listen.trim();
    else delete ex.naive_listen;
    if (form.naive.log.trim()) ex.naive_log = form.naive.log.trim();
    else delete ex.naive_log;
    /* Отпечаток клиента naive задаёт Chromium, и наш ему только мешает. */
    delete ex.fp;
    return { ...base, extra: ex };
  }

  if (type === "WIREGUARD" || type === "AMNEZIAWG") {
    const wg = form.wg;
    const ex = { ...parseProxyExtra(proxy?.extra) };
    ex.address = tokens(wg.address);
    ex.private_key = wg.privateKey.trim();
    ex.public_key = wg.publicKey.trim();
    ex.pre_shared_key = wg.preSharedKey.trim();
    ex.allowed_ips = tokens(wg.allowedIps);
    ex.system = Boolean(wg.system);
    ex.name = wg.name.trim();

    const reserved = tokens(wg.reserved)
      .map((s) => parseInt(s, 10))
      .filter((n) => Number.isFinite(n));
    ex.reserved = reserved.length ? reserved : undefined;
    ex.persistent_keepalive_interval = parseInt(wg.keepalive, 10) || 0;
    ex.mtu = parseInt(wg.mtu, 10) || 0;

    if (!ex.pre_shared_key) delete ex.pre_shared_key;
    if (!ex.reserved) delete ex.reserved;
    if (!ex.persistent_keepalive_interval) delete ex.persistent_keepalive_interval;
    if (!ex.name) delete ex.name;
    if (!ex.mtu) delete ex.mtu;

    if (type === "AMNEZIAWG") {
      const am = amneziaOf(wg);
      if (am === undefined) {
        /* JSON не разобрался — прежняя обфускация остаётся как была. */
      } else if (Object.keys(am).length > 0) {
        ex.amnezia = am;
      } else {
        delete ex.amnezia;
      }
    } else {
      delete ex.amnezia;
    }
    return { ...base, extra: ex };
  }

  /* VLESS, VMESS, TROJAN, SS: сеть, шифрование и параметры транспорта. */
  const clean = sanitizeVpnExtraForEdit(proxy?.extra, {
    type,
    network: form.network,
    security: form.security,
    uuid: form.uuid.trim(),
    ssMethod: form.ssMethod,
  });
  return { ...base, extra: applyVpnTransportFieldsToExtra(clean, form.network, form.tf) };
}
