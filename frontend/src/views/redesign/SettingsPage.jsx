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
 * Страница «Настройки». Figma "ResultV" -> App Design.
 *
 * Список пунктов — фрейм Settings (6628:3208): четыре карточки SettingsItem
 * и панель экспорта/импорта. Страницы самих пунктов нарисованы отдельно:
 * «Дополнительные настройки» (6793:6688), «Подписки» (6799:4682) и «Сеть»
 * (6799:4770). Из них взяты и раскладка страницы пункта, и вид строки
 * настройки; «Безопасность» и часть строк не нарисованы и собраны по тем же
 * правилам, см. docs/design/GAPS.md.
 */

import { useEffect, useState } from "react";
import {
  Button,
  Icon,
  Input,
  Select,
  SettingsItem,
  Tumbler,
} from "../../components/kit";
import PageHeader from "./PageHeader";
import ScrollRow from "./ScrollRow";
import "./SettingsPage.css";

/*
 * Порядок карточек и их значки — из макета. `key` совпадает с ключом группы
 * в переводах, чтобы подписи не пришлось раскладывать вторым списком.
 */
export const SETTINGS_GROUPS = [
  { key: "advanced", icon: "adsettings" },
  { key: "subscriptions", icon: "subscriptions" },
  { key: "security", icon: "security" },
  { key: "network", icon: "network" },
  { key: "ping", icon: "ping" },
  { key: "experimental", icon: "autoawesome" },
];

/*
 * Наборы вариантов. Значения технические (они уходят в конфиг), подписи
 * перебиваются пропами: в приложении они идут через i18n.
 */
export const TUN_STACKS = [
  { value: "default", label: "По умолчанию" },
  { value: "system", label: "system" },
  { value: "gvisor", label: "gvisor" },
];

export const LANGUAGES = [
  { value: "ru", label: "Русский" },
  { value: "en", label: "English" },
];

export const SUBSCRIPTION_HOURS = [
  { hours: 1, label: "1 ч" },
  { hours: 2, label: "2 ч" },
  { hours: 4, label: "4 ч" },
  { hours: 6, label: "6 ч" },
  { hours: 12, label: "12 ч" },
  { hours: 24, label: "24 ч" },
];

export const DNS_PRESETS = [
  { id: "auto", servers: [], label: "Авто (по умолчанию)" },
  { id: "google", servers: ["8.8.8.8", "8.8.4.4"], label: "Google DNS" },
  { id: "cloudflare", servers: ["1.1.1.1", "1.0.0.1"], label: "Cloudflare DNS" },
  { id: "quad9", servers: ["9.9.9.9", "149.112.112.112"], label: "Quad9 DNS" },
  { id: "yandex", servers: ["77.88.8.8", "77.88.8.1"], label: "Яндекс DNS" },
];

/*
 * Типы пинга. «Авто» — сегодняшнее поведение: проба подбирается под протокол
 * узла (TCP для VLESS и родственных, QUIC-хендшейк для Hysteria2, ICMP для
 * WireGuard). Отдельного «TCP» нет намеренно: для Hysteria2 и WireGuard он
 * бессмыслен — оба молча дропнут пакет, — и человек решил бы, что половина
 * серверов мертва.
 *
 * HTTP GET и HTTP HEAD — это «через прокси»: запрос тестового адреса идёт
 * через сам узел, поэтому в цифру входит его хендшейк.
 */
export const PING_TYPES = [
  { value: "auto", label: "Авто" },
  { value: "icmp", label: "ICMP" },
  { value: "http_get", label: "HTTP GET" },
  { value: "http_head", label: "HTTP HEAD" },
];

/*
 * Пресеты тестового адреса. Только https, и это требование корректности, а не
 * вкуса: по http ответ подделывает наш же локальный слушатель, и мёртвый узел
 * засчитался бы живым.
 */
export const PING_URL_PRESETS = [
  {
    id: "google",
    label: "Google 204",
    url: "https://www.gstatic.com/generate_204",
  },
  {
    id: "cloudflare",
    label: "Cloudflare",
    url: "https://cp.cloudflare.com/generate_204",
  },
  {
    id: "apple",
    label: "Apple",
    url: "https://captive.apple.com/hotspot-detect.html",
  },
];

export const PING_TIMEOUTS = [
  { value: 1, label: "1 с" },
  { value: 2, label: "2 с" },
  { value: 3, label: "3 с" },
  { value: 5, label: "5 с" },
  { value: 7, label: "7 с" },
  { value: 10, label: "10 с" },
];

/* Подписи в написании макета; приложение подставляет свои через i18n. */
export const SETTINGS_PAGE_TEXT = {
  title: "Настройки",
  subtitle: "Управление безопасностью и системой",
  back: "Назад",
  /*
   * `items` — строка под названием карточки в списке пунктов, `desc` —
   * подпись под названием на самой странице пункта.
   */
  groups: {
    advanced: {
      title: "Дополнительные настройки",
      items: "• Режим TUN  • Запуск при старте системы",
      desc: "Параметры запуска, TUN, язык приложения",
    },
    subscriptions: {
      title: "Подписки",
      items: "• Обновление • HWID  • UA",
      desc: "Обновление подписок и данные для провайдера.",
    },
    security: {
      title: "Безопасность",
      items: "• KillSwitch  • Защита утечки DNS",
      desc: "Защита соединения при сбоях и утечках DNS.",
    },
    network: {
      title: "Сеть",
      items: "• DNS  • IPv6 • Локальная сеть",
      desc: "DNS, локальный доступ и порт прокси.",
    },
    ping: {
      title: "Пинг",
      items: "• Тип пробы  • Тестовый адрес  • Тайм-аут",
      desc: "Чем и как долго измерять задержку до узлов.",
    },
    experimental: {
      title: "Экспериментально",
      items: "• Адаптивный умный режим",
      desc: "Недоделанное и необкатанное. Включать на свой страх.",
    },
  },
  exportImport: {
    title: "Экспорт / импорт конфигураций",
    desc: "Перенос настроек и серверов на другое устройство.",
    exportBtn: "Экспорт",
    importBtn: "Импорт",
  },
  rows: {
    tunStack: { title: "Режим TUN", desc: "Сетевой стек туннеля." },
    autostart: {
      title: "Запуск при старте системы",
      desc: "Запускать приложение вместе с Windows.",
    },
    language: { title: "Язык интерфейса", desc: "Язык подписей в приложении." },
    subAutoUpdate: {
      title: "Автоматическое обновление",
      desc: "Обновлять подписки по расписанию.",
    },
    subInterval: {
      title: "Интервал обновлений",
      desc: "Как часто обновлять подписки.",
    },
    subHwid: {
      title: "Передавать HWID",
      desc: "Идентификатор устройства для проверки лимита.",
    },
    subUserAgent: { title: "User agent", desc: "", placeholder: "" },
    killSwitch: { title: "Включить Kill Switch", desc: "" },
    dnsLeak: { title: "Защита от утечки DNS", desc: "" },
    dns: {
      title: "DNS-серверы",
      desc: "Выберите DNS-провайдера",
      customLabel: "Или укажите свои",
      placeholder: "",
    },
    ipv6: { title: "IPv6 в туннеле", desc: "" },
    listenLan: { title: "Слушать в локальной сети", desc: "" },
    port: { title: "Локальный порт", desc: "", placeholder: "", addrTitle: "" },
    adaptiveSmart: {
      title: "Адаптивный умный режим",
      desc: "Определять блокировки самостоятельно, не полагаясь на список. Экспериментально.",
    },
    adaptiveSmartMemoryOnly: {
      title: "Не сохранять вердикты на диск",
      desc: "Всё в памяти, забывается при перезапуске.",
    },
    adaptiveSmartBlockDoH: {
      title: "Блокировать DoH в браузере",
      desc: "Заставляет браузер вернуться к системному DNS. Может сломать сайты.",
    },
    pingType: {
      title: "Тип пинга",
      desc: "Чем измерять задержку. «Авто» подбирает пробу под протокол узла.",
    },
    pingUrl: {
      title: "Тестовый адрес",
      desc: "Запрашивается через сам узел. Только https.",
      customLabel: "Свой адрес",
      placeholder: "https://www.gstatic.com/generate_204",
      invalid: "Нужен адрес https:// — по http ответ подделает локальный слушатель",
      onlyHTTP: "Работает только с типами HTTP GET и HTTP HEAD.",
    },
    pingTimeout: {
      title: "Тайм-аут пинга",
      desc: "Сколько ждать ответа, прежде чем считать узел недоступным.",
    },
  },
};

/*
 * Строка настройки (Figma 6793:6695 и соседние): слева название с
 * пояснением, справа элемент. Высота 112 набегает от самого высокого
 * элемента — выбора значения: 24 + 64 + 24; строкам с тумблером она задана
 * тем же числом, чтобы ряд не сбивался.
 *
 * `stacked` — элемент во всю ширину под текстом (поле User agent, набор
 * DNS): справа ему не встать, и в макете он стоит именно так.
 */
function Row({ title, description, stacked = false, children }) {
  return (
    <div className="rv-settings-page__row" data-stacked={stacked || undefined}>
      <div className="rv-settings-page__row-text">
        <p className="rv-settings-page__row-title">{title}</p>
        {description && (
          <p className="rv-settings-page__row-desc">{description}</p>
        )}
      </div>
      <div className="rv-settings-page__row-control">{children}</div>
    </div>
  );
}

/*
 * Поле, которое отдаёт значение не на каждую букву, а по Enter и по уходу из
 * него — как поле тегов в умных правилах. Отдельной кнопки «Применить»
 * поэтому нет.
 *
 * Отказ приложение возвращает как `false`: сохранённое значение не менялось,
 * и следить за ним нечему — набранное надо вернуть к нему руками, иначе в
 * поле осталось бы то, что никуда не записалось.
 */
function CommitInput({ value, onCommit, ...rest }) {
  const [draft, setDraft] = useState(value);

  useEffect(() => setDraft(value), [value]);

  return (
    <Input
      value={draft}
      onChange={(event) => setDraft(event.target.value)}
      onBlur={() => {
        if (onCommit(draft) === false) setDraft(value);
      }}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          event.preventDefault();
          event.currentTarget.blur();
        }
      }}
      {...rest}
    />
  );
}

export default function SettingsPage({
  /* Пустая строка — список пунктов, иначе ключ открытой группы. */
  section = "",
  onOpenSection,
  onBack,
  values = {},
  onChange,
  tunStacks = TUN_STACKS,
  languages = LANGUAGES,
  intervals = SUBSCRIPTION_HOURS,
  dnsPresets = DNS_PRESETS,
  pingTypes = PING_TYPES,
  pingUrlPresets = PING_URL_PRESETS,
  pingTimeouts = PING_TIMEOUTS,
  /* Адреса, по которым прокси доступен в локальной сети. Строку собирает
     приложение: своих IP страница не знает. */
  lanAddress = "",
  onExport,
  onImport,
  sidebar,
  text = SETTINGS_PAGE_TEXT,
}) {
  const rows = text.rows;
  const set = (key) => (value) => onChange?.(key, value);

  const dnsServers = Array.isArray(values.dnsServers) ? values.dnsServers : [];
  const activePreset =
    dnsPresets.find(
      (preset) => preset.servers.join(",") === dnsServers.join(","),
    )?.id ?? "";

  const pingType = values.pingType || "auto";
  /* Тестовый адрес нужен только там, где запрос действительно уходит через
     узел. При «Авто» и ICMP запроса нет, и поле гасим, чтобы не обещать
     влияния, которого у него сейчас нет. */
  const pingUsesURL = pingType === "http_get" || pingType === "http_head";
  const pingTestUrl = values.pingTestUrl ?? "";
  const pingUrlInvalid =
    pingUsesURL &&
    pingTestUrl.trim() !== "" &&
    !/^https:\/\/[^/\s]+/i.test(pingTestUrl.trim());
  const activePingUrlPreset =
    pingUrlPresets.find((preset) => preset.url === pingTestUrl.trim())?.id ?? "";

  const groupBody = {
    advanced: (
      <>
        <Row title={rows.tunStack.title} description={rows.tunStack.desc}>
          <Select
            options={tunStacks}
            value={values.tunStack || "default"}
            onChange={set("tunStack")}
            aria-label={rows.tunStack.title}
          />
        </Row>
        <Row title={rows.autostart.title} description={rows.autostart.desc}>
          <Tumbler checked={!!values.autostart} onChange={set("autostart")} />
        </Row>
        <Row title={rows.language.title} description={rows.language.desc}>
          <Select
            options={languages}
            value={values.language}
            onChange={set("language")}
            aria-label={rows.language.title}
          />
        </Row>
      </>
    ),

    subscriptions: (
      <>
        <Row
          title={rows.subAutoUpdate.title}
          description={rows.subAutoUpdate.desc}
        >
          <Tumbler
            checked={values.subAutoUpdate !== false}
            onChange={set("subAutoUpdate")}
          />
        </Row>
        <Row title={rows.subInterval.title} description={rows.subInterval.desc}>
          <Select
            options={intervals.map((item) => ({
              value: item.hours,
              label: item.label,
            }))}
            value={values.subIntervalHours}
            onChange={set("subIntervalHours")}
            aria-label={rows.subInterval.title}
          />
        </Row>
        <Row title={rows.subHwid.title} description={rows.subHwid.desc}>
          <Tumbler checked={values.subHwid !== false} onChange={set("subHwid")} />
        </Row>
        <Row
          title={rows.subUserAgent.title}
          description={rows.subUserAgent.desc}
          stacked
        >
          <CommitInput
            value={values.subUserAgent || ""}
            onCommit={(next) => onChange?.("subUserAgent", next.trim())}
            placeholder={rows.subUserAgent.placeholder}
          />
        </Row>
      </>
    ),

    security: (
      <>
        <Row title={rows.killSwitch.title} description={rows.killSwitch.desc}>
          <Tumbler checked={!!values.killSwitch} onChange={set("killSwitch")} />
        </Row>
        <Row title={rows.dnsLeak.title} description={rows.dnsLeak.desc}>
          <Tumbler checked={values.dnsLeak !== false} onChange={set("dnsLeak")} />
        </Row>
      </>
    ),

    network: (
      <>
        <Row title={rows.dns.title} description={rows.dns.desc} stacked>
          {/* Набор DNS выбирается рядом кнопок, выбранная — зелёная. */}
          <ScrollRow>
            {dnsPresets.map((preset) => (
              <Button
                key={preset.id}
                variant={preset.id === activePreset ? "green" : "default"}
                aria-pressed={preset.id === activePreset}
                onClick={() => onChange?.("dnsServers", preset.servers)}
              >
                {preset.label}
              </Button>
            ))}
          </ScrollRow>
          <label className="rv-settings-page__custom">
            <span className="rv-settings-page__label">
              {rows.dns.customLabel}
            </span>
            <CommitInput
              value={dnsServers.join(", ")}
              onCommit={(next) => onChange?.("dnsServers", next)}
              placeholder={rows.dns.placeholder}
            />
          </label>
        </Row>
        <Row title={rows.ipv6.title} description={rows.ipv6.desc}>
          <Tumbler checked={!!values.ipv6} onChange={set("ipv6")} />
        </Row>
        <Row title={rows.listenLan.title} description={rows.listenLan.desc}>
          <Tumbler checked={!!values.listenLan} onChange={set("listenLan")} />
        </Row>
        <Row title={rows.port.title} description={rows.port.desc} stacked>
          <CommitInput
            className="rv-settings-page__port"
            inputMode="numeric"
            value={values.localPort ? String(values.localPort) : ""}
            onCommit={(next) => onChange?.("localPort", next)}
            placeholder={rows.port.placeholder}
          />
          {/* Адрес нужен, только когда прокси действительно слушает сеть. */}
          {values.listenLan && lanAddress && (
            <p className="rv-settings-page__hint">
              {rows.port.addrTitle}: {lanAddress}
            </p>
          )}
        </Row>
      </>
    ),

    /*
     * Пинг. Настройка управляет только ручным измерением — кнопкой в списке
     * серверов. Автовыбор узла её не видит намеренно: он свипает весь список
     * сам при каждом подключении, и «через прокси» там означало бы десятки
     * пробных движков на каждое нажатие «Подключиться».
     */
    ping: (
      <>
        <Row title={rows.pingType.title} description={rows.pingType.desc} stacked>
          <ScrollRow>
            {pingTypes.map((item) => (
              <Button
                key={item.value}
                variant={item.value === pingType ? "green" : "default"}
                aria-pressed={item.value === pingType}
                onClick={() => onChange?.("pingType", item.value)}
              >
                {item.label}
              </Button>
            ))}
          </ScrollRow>
        </Row>
        <Row title={rows.pingUrl.title} description={rows.pingUrl.desc} stacked>
          <ScrollRow>
            {pingUrlPresets.map((preset) => (
              <Button
                key={preset.id}
                variant={preset.id === activePingUrlPreset ? "green" : "default"}
                aria-pressed={preset.id === activePingUrlPreset}
                disabled={!pingUsesURL}
                onClick={() => onChange?.("pingTestUrl", preset.url)}
              >
                {preset.label}
              </Button>
            ))}
          </ScrollRow>
          <label className="rv-settings-page__custom">
            <span className="rv-settings-page__label">
              {rows.pingUrl.customLabel}
            </span>
            <CommitInput
              value={pingTestUrl}
              onCommit={(next) => onChange?.("pingTestUrl", next)}
              placeholder={rows.pingUrl.placeholder}
              disabled={!pingUsesURL}
              invalid={pingUrlInvalid}
            />
          </label>
          {pingUrlInvalid && (
            <p className="rv-settings-page__hint" role="alert">
              {rows.pingUrl.invalid}
            </p>
          )}
          {!pingUsesURL && (
            <p className="rv-settings-page__hint">{rows.pingUrl.onlyHTTP}</p>
          )}
        </Row>
        <Row title={rows.pingTimeout.title} description={rows.pingTimeout.desc}>
          <Select
            options={pingTimeouts.map((item) => ({
              value: String(item.value),
              label: item.label,
            }))}
            value={String(values.pingTimeoutSec || 3)}
            onChange={(next) => onChange?.("pingTimeoutSec", next)}
            aria-label={rows.pingTimeout.title}
          />
        </Row>
      </>
    ),

    /*
     * Экспериментальный блок. Два нижних тумблера ведомые: без верхнего
     * им нечем управлять — правило DoH живёт только при включённом FakeIP,
     * иначе оно отнимает у пользователя DoH и ничего не даёт взамен.
     */
    experimental: (
      <>
        <Row
          title={rows.adaptiveSmart.title}
          description={rows.adaptiveSmart.desc}
        >
          <Tumbler
            checked={!!values.adaptiveSmart}
            onChange={set("adaptiveSmart")}
          />
        </Row>
        <Row
          title={rows.adaptiveSmartMemoryOnly.title}
          description={rows.adaptiveSmartMemoryOnly.desc}
        >
          <Tumbler
            checked={!!values.adaptiveSmartMemoryOnly}
            onChange={set("adaptiveSmartMemoryOnly")}
            disabled={!values.adaptiveSmart}
          />
        </Row>
        <Row
          title={rows.adaptiveSmartBlockDoH.title}
          description={rows.adaptiveSmartBlockDoH.desc}
        >
          <Tumbler
            checked={!!values.adaptiveSmartBlockDoH}
            onChange={set("adaptiveSmartBlockDoH")}
            disabled={!values.adaptiveSmart}
          />
        </Row>
      </>
    ),
  };

  const group = section ? text.groups[section] : null;

  return (
    <div className="rv-settings-page">
      {sidebar}

      <div className="rv-settings-page__content rv-scroll">
        {group ? (
          <>
            <PageHeader title={group.title} subtitle={group.desc} />

            {/* Страница пункта: кнопка возврата и одна карточка со строками
                (Figma 6799:4689). */}
            <div className="rv-settings-page__section">
              <Button variant="green" onClick={onBack}>
                {text.back}
              </Button>
              <section className="rv-settings-page__card rv-border rv-border--static">
                {groupBody[section]}
              </section>
            </div>
          </>
        ) : (
          <>
            <PageHeader title={text.title} subtitle={text.subtitle} />

            <div className="rv-settings-page__body">
              <div className="rv-settings-page__groups">
                {SETTINGS_GROUPS.map((item) => (
                  <SettingsItem
                    key={item.key}
                    as="button"
                    type="button"
                    icon={item.icon}
                    title={text.groups[item.key].title}
                    description={text.groups[item.key].items}
                    onClick={() => onOpenSection?.(item.key)}
                  />
                ))}
              </div>

              <section className="rv-settings-page__export rv-border">
                <div className="rv-settings-page__export-text">
                  <h2 className="rv-settings-page__export-title">
                    {text.exportImport.title}
                  </h2>
                  <p className="rv-settings-page__export-desc">
                    {text.exportImport.desc}
                  </p>
                </div>
                <div className="rv-settings-page__export-actions">
                  <Button
                    icon={<Icon name="export" color="currentColor" />}
                    onClick={onExport}
                  >
                    {text.exportImport.exportBtn}
                  </Button>
                  <Button
                    icon={<Icon name="import" color="currentColor" />}
                    onClick={onImport}
                  >
                    {text.exportImport.importBtn}
                  </Button>
                </div>
              </section>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
