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
 * Окно правки профиля маршрутизации. Figma "ResultV" -> App Design:
 *   6648:4105  «Добавление профиля» — пустое, значок плюса
 *   6636:4310  «Изменение профиля» — заполненное, значок карандаша
 *
 * Одно окно в двух ролях; отличаются заголовок, подзаголовок и значок.
 *
 * Внутри, зазор 18: название, три раздела действий (у каждого складные
 * панели «Домены» и «IP адреса»), «Стратегия», «Geo данные», «Действия».
 */

import { useEffect, useMemo, useState } from "react";
import { Button, Dialog, Icon, Input, Textarea } from "../../components/kit";
import "./RoutingProfileEditor.css";

export const ROUTING_EDITOR_TEXT = {
  createTitle: "Добавление профиля",
  createSubtitle: "Создание нового профиля маршрутизации",
  editTitle: "Изменение профиля",
  editSubtitle: "Редактирование профиля",
  name: "Название",
  namePlaceholder: "Мои правила",
  direct: "Direct (напрямую)",
  proxy: "Proxy (через прокси)",
  block: "Block (блокировать)",
  domains: "Домены",
  ips: "IP адреса",
  /*
   * Подсказки полей. В макете внутри панели стоит Textarea со своей
   * подсказкой из страницы «Добавить сервер» («https://example.com/sub или
   * vless://...») — очевидный остаток копирования, к спискам правил она
   * отношения не имеет. См. docs/design/GAPS.md.
   */
  domainsPlaceholder: "geosite:private\nexample.com\ndomain:nalog.ru",
  ipsPlaceholder: "geoip:private\n10.0.0.0/8",
  strategy: "Стратегия",
  strategyHint: "Порядок обработки правил — перетащите, чтобы изменить",
  geo: "Geo данные",
  geoip: "URL GeoIP",
  geosite: "URL GeoSite",
  urlPlaceholder: "https://example.com",
  actions: "Действия",
  reset: "Сбросить",
  /* В макете набрано «Соханить» — опечатка, см. docs/design/GAPS.md. */
  save: "Сохранить",
};

const ACTIONS = ["direct", "proxy", "block"];

/* Складная панель со списком правил (Figma 6648:4159 и соседние). */
function RulePanel({ label, placeholder, value, onChange, open, onToggle }) {
  return (
    <div className="rv-profile-editor__panel rv-border rv-border--static">
      <button
        type="button"
        className="rv-profile-editor__panel-head"
        aria-expanded={open}
        onClick={onToggle}
      >
        <span className="rv-profile-editor__panel-label">{label}</span>
        <Icon name="open" state={open ? "active" : "default"} size={24} color="currentColor" />
      </button>
      {open && (
        <Textarea
          className="rv-profile-editor__area"
          placeholder={placeholder}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      )}
    </div>
  );
}

/*
 * Порядок правил перетаскиванием (Figma 6648:4197).
 *
 * Сделано на указательных событиях, а не на HTML5 drag-and-drop: последний в
 * WebView2 требует зажать и «сорвать» элемент, рисует свой призрак поверх
 * страницы и не даёт менять порядок под пальцем — то есть ровно то поведение,
 * которого просили избежать. Здесь бейдж просто едет за курсором, а соседи
 * расступаются.
 *
 * С клавиатуры порядок меняют стрелками: перетаскивание мышью — не
 * единственный способ добраться до настройки.
 */
function StrategyOrder({ order, onChange, labels }) {
  const [dragging, setDragging] = useState(null);

  const move = (from, to) => {
    if (to < 0 || to >= order.length || from === to) return;
    const next = order.slice();
    const [item] = next.splice(from, 1);
    next.splice(to, 0, item);
    onChange(next);
  };

  const onPointerDown = (index) => (event) => {
    /* Только основная кнопка: правый клик — это контекстное меню. */
    if (event.button !== 0) return;
    event.preventDefault();
    setDragging(index);
    event.currentTarget.setPointerCapture?.(event.pointerId);
  };

  const onPointerMove = (index) => (event) => {
    if (dragging === null) return;
    const badge = event.currentTarget;
    const rect = badge.getBoundingClientRect();
    /* Курсор ушёл за середину соседа — меняем их местами. Порог по середине,
       а не по краю, иначе бейджи прыгали бы туда-обратно на границе. */
    if (event.clientX < rect.left + rect.width / 2 && index > 0) {
      move(dragging, index - 1);
      setDragging(index - 1);
    } else if (event.clientX > rect.left + rect.width / 2 && index < order.length - 1) {
      move(dragging, index + 1);
      setDragging(index + 1);
    }
  };

  const stop = () => setDragging(null);

  return (
    <div className="rv-profile-editor__strategy rv-border rv-border--static">
      {order.map((action, index) => (
        <button
          key={action}
          type="button"
          className="rv-profile-editor__badge"
          data-action={action}
          data-dragging={dragging === index || undefined}
          onPointerDown={onPointerDown(index)}
          onPointerMove={onPointerMove(index)}
          onPointerUp={stop}
          onPointerCancel={stop}
          onKeyDown={(event) => {
            if (event.key === "ArrowLeft") {
              event.preventDefault();
              move(index, index - 1);
            } else if (event.key === "ArrowRight") {
              event.preventDefault();
              move(index, index + 1);
            }
          }}
        >
          {labels[action]}
        </button>
      ))}
    </div>
  );
}

const linesOf = (list) => (list || []).join("\n");
const tokensOf = (text) =>
  text
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);

export default function RoutingProfileEditor({
  open = true,
  /* Пустой профиль => окно «Добавление профиля». */
  profile = null,
  onSave,
  onClose,
  busy = false,
  text = ROUTING_EDITOR_TEXT,
}) {
  const isEdit = Boolean(profile?.id);

  const [name, setName] = useState("");
  const [fields, setFields] = useState({});
  const [order, setOrder] = useState(["block", "proxy", "direct"]);
  const [geoip, setGeoip] = useState("");
  const [geosite, setGeosite] = useState("");
  /* В макете раскрыта первая панель Direct, остальные свёрнуты. */
  const [opened, setOpened] = useState({ "direct-sites": true });

  /* Форма наполняется из профиля при каждом открытии: окно живёт дольше
     одной правки, и остатки прошлой сюда попадать не должны. */
  useEffect(() => {
    if (!open) return;
    setName(profile?.name || "");
    setFields({
      "direct-sites": linesOf(profile?.directSites),
      "direct-ips": linesOf(profile?.directIp),
      "proxy-sites": linesOf(profile?.proxySites),
      "proxy-ips": linesOf(profile?.proxyIp),
      "block-sites": linesOf(profile?.blockSites),
      "block-ips": linesOf(profile?.blockIp),
    });
    setOrder(
      profile?.routeOrder ? profile.routeOrder.split("-") : ["block", "proxy", "direct"]
    );
    setGeoip(profile?.geoipUrl || "");
    setGeosite(profile?.geositeUrl || "");
    setOpened({ "direct-sites": true });
  }, [open, profile]);

  const setField = (key) => (value) => setFields((f) => ({ ...f, [key]: value }));
  const toggle = (key) => () => setOpened((o) => ({ ...o, [key]: !o[key] }));

  const empty = useMemo(
    () => Object.values(fields).every((v) => tokensOf(v || "").length === 0),
    [fields]
  );

  const submit = () => {
    onSave?.({
      ...(profile || {}),
      name: name.trim(),
      directSites: tokensOf(fields["direct-sites"] || ""),
      directIp: tokensOf(fields["direct-ips"] || ""),
      proxySites: tokensOf(fields["proxy-sites"] || ""),
      proxyIp: tokensOf(fields["proxy-ips"] || ""),
      blockSites: tokensOf(fields["block-sites"] || ""),
      blockIp: tokensOf(fields["block-ips"] || ""),
      routeOrder: order.join("-"),
      geoipUrl: geoip.trim(),
      geositeUrl: geosite.trim(),
    });
  };

  return (
    <Dialog
      open={open}
      icon={isEdit ? "edit" : "add"}
      title={isEdit ? text.editTitle : text.createTitle}
      subtitle={
        isEdit ? `${text.editSubtitle} ${profile?.name || ""}`.trim() : text.createSubtitle
      }
      onClose={onClose}
      className="rv-profile-editor"
    >
      <div className="rv-profile-editor__body">
        <section className="rv-profile-editor__section">
          <p className="rv-profile-editor__label">{text.name}</p>
          <Input
            placeholder={text.namePlaceholder}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </section>

        <div className="rv-profile-editor__actions-list">
          {ACTIONS.map((action) => (
            <section key={action} className="rv-profile-editor__section">
              <p className="rv-profile-editor__action" data-action={action}>
                {text[action]}
              </p>
              <div className="rv-profile-editor__panels">
                <RulePanel
                  label={text.domains}
                  placeholder={text.domainsPlaceholder}
                  value={fields[`${action}-sites`] || ""}
                  onChange={setField(`${action}-sites`)}
                  open={Boolean(opened[`${action}-sites`])}
                  onToggle={toggle(`${action}-sites`)}
                />
                <RulePanel
                  label={text.ips}
                  placeholder={text.ipsPlaceholder}
                  value={fields[`${action}-ips`] || ""}
                  onChange={setField(`${action}-ips`)}
                  open={Boolean(opened[`${action}-ips`])}
                  onToggle={toggle(`${action}-ips`)}
                />
              </div>
            </section>
          ))}
        </div>

        <section className="rv-profile-editor__section">
          <p className="rv-profile-editor__label">{text.strategy}</p>
          <StrategyOrder
            order={order}
            onChange={setOrder}
            labels={{ direct: "Direct", proxy: "Proxy", block: "Block" }}
          />
          <p className="rv-profile-editor__hint">{text.strategyHint}</p>
        </section>

        <section className="rv-profile-editor__section">
          <p className="rv-profile-editor__label">{text.geo}</p>
          <div className="rv-profile-editor__geo rv-border rv-border--static">
            <p className="rv-profile-editor__label">{text.geoip}</p>
            <Input
              placeholder={text.urlPlaceholder}
              value={geoip}
              onChange={(e) => setGeoip(e.target.value)}
            />
            <p className="rv-profile-editor__label">{text.geosite}</p>
            <Input
              placeholder={text.urlPlaceholder}
              value={geosite}
              onChange={(e) => setGeosite(e.target.value)}
            />
          </div>
        </section>

        <section className="rv-profile-editor__section">
          <p className="rv-profile-editor__label">{text.actions}</p>
          <div className="rv-profile-editor__buttons">
            <Button className="rv-profile-editor__btn" onClick={onClose}>
              {text.reset}
            </Button>
            <Button
              variant="green"
              className="rv-profile-editor__btn"
              /* Профиль без имени или без единого правила сохранять нечего —
                 бэкенд их всё равно отклонит, и лучше это видно до нажатия. */
              disabled={busy || !name.trim() || empty}
              onClick={submit}
            >
              {text.save}
            </Button>
          </div>
        </section>
      </div>
    </Dialog>
  );
}
