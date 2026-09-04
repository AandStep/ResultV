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
 * Страница «Настройки». Figma "ResultV" -> App Design, фрейм Settings
 * (6628:3208).
 *
 * В макете нарисован только список: четыре карточки SettingsItem и панель
 * экспорта/импорта под ними. Карточка — кнопка, и уводит она на свою
 * страницу; сами настройки в макете не нарисованы и собраны из кита, см.
 * docs/design/GAPS.md.
 */

import { useEffect, useState } from "react";
import {
  Button,
  Icon,
  Input,
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

/* Подписи в написании макета; приложение подставляет свои через i18n. */
export const SETTINGS_PAGE_TEXT = {
  title: "Настройки",
  subtitle: "Управление безопасностью и системой",
  back: "Назад к настройкам",
  /*
   * `items` — строка под названием карточки, как в макете. `desc` в макете
   * нет: это подпись под названием на самой странице пункта.
   */
  groups: {
    advanced: {
      title: "Дополнительные настройки",
      items: "• Режим TUN  • Запуск при старте системы",
      desc: "Параметры запуска, TUN и фильтрации трафика.",
    },
    subscriptions: {
      title: "Подписки",
      items: "• Обновление • HWID  • UA",
      desc: "Автообновление и данные, которые приложение отправляет провайдеру подписки.",
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
  },
  exportImport: {
    title: "Экспорт / импорт конфигураций",
    desc: "Сохраняйте все настройки и сервера приложения, для переноса на другое устройство.",
    exportBtn: "Экспорт",
    importBtn: "Импорт",
  },
  /* Строк настроек в макете нет — набор повторяет прежние настройки. */
  rows: {
    tunStack: { title: "Режим TUN", desc: "" },
    autostart: { title: "Запуск при старте системы", desc: "" },
    language: { title: "Язык интерфейса", desc: "" },
    subAutoUpdate: { title: "Автоматическое обновление", desc: "" },
    subInterval: { title: "Интервал обновления", desc: "" },
    subHwid: { title: "Передавать HWID", desc: "" },
    subUserAgent: { title: "User agent", desc: "", placeholder: "" },
    killSwitch: { title: "Включить Kill Switch", desc: "" },
    dnsLeak: { title: "Защита от утечки DNS", desc: "" },
    dns: { title: "DNS-серверы", desc: "", placeholder: "" },
    ipv6: { title: "IPv6 в туннеле", desc: "" },
    listenLan: { title: "Слушать в локальной сети", desc: "" },
    port: { title: "Локальный порт", desc: "", placeholder: "", addrTitle: "" },
  },
};

/*
 * Строка настройки: слева название с пояснением, справа элемент. Широкому
 * элементу справа не встать — ряд кнопок и поле уходят под текст, как поля в
 * окне настроек подписки.
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

/* Ряд кнопок-вариантов: выбранный отмечен залитым состоянием кнопки — так же,
   как выбранный протокол на странице добавления. */
function Choice({ options, value, onSelect }) {
  return (
    <ScrollRow>
      {options.map((option) => (
        <Button
          key={option.value}
          mode={option.value === value ? "idle" : undefined}
          aria-pressed={option.value === value}
          onClick={() => onSelect(option.value)}
        >
          {option.label}
        </Button>
      ))}
    </ScrollRow>
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

  const groupBody = {
    advanced: (
      <>
        <Row
          title={rows.tunStack.title}
          description={rows.tunStack.desc}
          stacked
        >
          <Choice
            options={tunStacks}
            value={values.tunStack || "default"}
            onSelect={set("tunStack")}
          />
        </Row>
        <Row title={rows.autostart.title} description={rows.autostart.desc}>
          <Tumbler checked={!!values.autostart} onChange={set("autostart")} />
        </Row>
        <Row
          title={rows.language.title}
          description={rows.language.desc}
          stacked
        >
          <Choice
            options={languages}
            value={values.language}
            onSelect={set("language")}
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
        <Row
          title={rows.subInterval.title}
          description={rows.subInterval.desc}
          stacked
        >
          <ScrollRow>
            {intervals.map((item) => (
              <Button
                key={item.hours}
                mode={
                  item.hours === values.subIntervalHours ? "idle" : undefined
                }
                aria-pressed={item.hours === values.subIntervalHours}
                onClick={() => onChange?.("subIntervalHours", item.hours)}
              >
                {item.label}
              </Button>
            ))}
          </ScrollRow>
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
          <ScrollRow>
            {dnsPresets.map((preset) => (
              <Button
                key={preset.id}
                mode={preset.id === activePreset ? "idle" : undefined}
                aria-pressed={preset.id === activePreset}
                onClick={() => onChange?.("dnsServers", preset.servers)}
              >
                {preset.label}
              </Button>
            ))}
          </ScrollRow>
          <CommitInput
            value={dnsServers.join(", ")}
            onCommit={(next) => onChange?.("dnsServers", next)}
            placeholder={rows.dns.placeholder}
          />
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
  };

  const group = section ? text.groups[section] : null;

  return (
    <div className="rv-settings-page">
      {sidebar}

      <div className="rv-settings-page__content rv-scroll">
        {group ? (
          <>
            <PageHeader
              title={group.title}
              subtitle={group.desc}
              onBack={onBack}
              backLabel={text.back}
            />
            <section className="rv-settings-page__panel rv-border">
              {groupBody[section]}
            </section>
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
