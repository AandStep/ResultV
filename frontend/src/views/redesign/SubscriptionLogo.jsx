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
 * Логотип провайдера в строке подписки и в окне её настроек. В макете на его
 * месте лежит картинка impVPN (6569:5111), поэтому подложку рисует ServerItem,
 * а здесь только сам знак 32x32.
 *
 * Своей картинки у произвольной подписки нет, поэтому берётся то, что можно
 * достать: присланная провайдером иконка, затем favicon его сайта, а у
 * подписок ResultV — знак impVPN, лежащий в приложении.
 */

import { useEffect, useMemo, useState } from "react";
import { Icon } from "../../components/kit";
import impLogo from "../../assets/implogo.png";

/* Подписка ResultV (`resultv://rvsub/...`) и всё, что зовётся impVPN. */
function usesImpLogo(subscription) {
  if (subscription?.source === "rvsub") return true;
  return String(subscription?.name || "")
    .toLowerCase()
    .includes("impvpn");
}

/*
 * Адрес поддержки для тега «Поддержка» в окне настроек подписки. Его
 * присылает сам провайдер в ответе на запрос подписки (заголовок `Support-Url`
 * и его родня, см. `subscriptionSupportURL` в app.go), и ядро кладёт его в
 * запись подписки. Не прислал — тега нет: придумывать за провайдера, куда
 * писать в поддержку, нельзя.
 */
export function subscriptionSupportURL(subscription) {
  return String(subscription?.supportUrl || "").trim();
}

export default function SubscriptionLogo({ subscription, className = "rv-server-item__logo" }) {
  const [index, setIndex] = useState(0);
  const impLogoWanted = usesImpLogo(subscription);
  const iconUrl = subscription?.iconUrl;
  const url = subscription?.url;

  const candidates = useMemo(() => {
    const out = [];
    if (typeof iconUrl === "string" && iconUrl.startsWith("data:image/")) out.push(iconUrl);
    try {
      const parsed = new URL(url || "");
      const base = `${parsed.protocol}//${parsed.host}`;
      out.push(`${base}/assets/favicon-32x32.png`, `${base}/assets/favicon.ico`, `${base}/favicon.ico`);
    } catch {
      /* Подписка без разбираемого адреса — значка у неё не будет. */
    }
    return out;
  }, [iconUrl, url]);

  useEffect(() => setIndex(0), [candidates, impLogoWanted]);

  if (impLogoWanted) return <img src={impLogo} alt="" className={className} />;

  const src = candidates[index];
  if (!src) return <Icon name="subscriptions" size={32} color="currentColor" />;

  return (
    <img
      src={src}
      alt=""
      className={className}
      onError={() => setIndex((i) => i + 1)}
    />
  );
}
