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
 * Числа трафика так, как они набраны в макете: «312 кб/с», «0 Мб», «634.5 Гб».
 *
 * Отличий от общего `utils/formatters.js` два: единицы идут через i18n (в
 * макете они русские, в общем форматтере зашиты английские) и мелкие величины
 * округляются до целого — дробей в макете нет нигде, кроме гигабайтов.
 *
 * Общий форматтер намеренно не трогаем: на нём держатся экраны старого
 * дизайна.
 */

import { getProtocolLabel } from "../../utils/proxyParser";

const KB = 1024;
const MB = KB * 1024;
const GB = MB * 1024;

/** Накопленный объём: «0 Мб», «312 Мб», «1.5 Гб». */
export function formatTraffic(bytes, t) {
  const value = Number(bytes) || 0;
  if (value >= GB) return `${(value / GB).toFixed(1)} ${t("units.gb")}`;
  return `${Math.round(value / MB)} ${t("units.mb")}`;
}

/** Текущая скорость: «0 кб/с», «312 кб/с», «1.5 Мб/с». */
export function formatRate(bytesPerSec, t) {
  const value = Number(bytesPerSec) || 0;
  if (value >= MB) return `${(value / MB).toFixed(1)} ${t("units.mbps")}`;
  return `${Math.round(value / KB)} ${t("units.kbps")}`;
}

/*
 * Приложение отдаёт протокол капсом («HYSTERIA2»), а в макете он набран как
 * имя продукта — «Hysteria2». Правим регистр только у первого слова: хвост
 * («+ Reality», «+ gRPC») уже приходит в нужном виде.
 */
export const PROTOCOL_CASE = {
  HYSTERIA2: "Hysteria2",
  HYSTERIA: "Hysteria",
  VLESS: "VLESS",
  VMESS: "VMess",
  TROJAN: "Trojan",
  SHADOWSOCKS: "Shadowsocks",
  SS: "Shadowsocks",
  WIREGUARD: "WireGuard",
  AMNEZIAWG: "AmneziaWG",
  TUIC: "TUIC",
  ANYTLS: "AnyTLS",
  SOCKS: "SOCKS",
  SOCKS5: "SOCKS5",
  HTTP: "HTTP",
  HTTPS: "HTTPS",
  SSH: "SSH",
  AUTO: "Auto",
};

/** Подпись протокола сервера в написании макета. */
export function protocolLabel(proxy) {
  const raw = getProtocolLabel(proxy);
  if (!raw) return "";
  const [head, ...rest] = raw.split(" + ");
  return [PROTOCOL_CASE[head.toUpperCase()] ?? head, ...rest].join(" + ");
}
