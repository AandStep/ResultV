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
 * Витрина UI-kit: все компоненты во всех состояниях, что есть в макете.
 * Нужна, чтобы открыть рядом с Figma и сверить попиксельно.
 *
 * Состояния наведения и нажатия здесь включены принудительно через пропы
 * `state` / `mode` — иначе увидеть их все разом нельзя.
 */

import {
  Icon,
  ICONS,
  Button,
  BigBtn,
  MenuBtn,
  PowerButton,
  Badge,
  Flag,
  Tumbler,
  ModeTumbler,
  Input,
  Textarea,
  SettingsItem,
  Header,
  Speed,
  Sidebar,
  ServerItem,
  HomeServerList,
} from "../components/kit";
import { FlagStub, LogoStub, MENU_ITEMS, SETTINGS_ITEM } from "./showcase-stubs";
import "./KitShowcase.css";

function Section({ title, node, children }) {
  return (
    <section className="kit__section">
      <div className="kit__head">
        <h2>{title}</h2>
        <span className="kit__node">{node}</span>
      </div>
      {children}
    </section>
  );
}

function Cell({ label, children }) {
  return (
    <div className="kit__cell">
      <span className="kit__label">{label}</span>
      {children}
    </div>
  );
}

const HEADER_TEXT = {
  default: "Не подключено",
  processing: "Подключение...",
  success: "Защищено",
  error: "Что-то пошло не так",
};

const BADGE_SAMPLE = { label: "Hysteria2" };

export default function KitShowcase() {
  const serverHeader = (showButtons) => (
    <ServerItem
      variant="header"
      flag={<FlagStub />}
      badges={[BADGE_SAMPLE]}
      title="Россия | Hysteria2"
      ping="90мс"
      showPingBtn={showButtons}
      showSortBtn={showButtons}
    />
  );

  return (
    <div className="kit">
      <div className="kit__intro">
        <h1>Витрина UI-kit</h1>
        <p>
          Все компоненты кита во всех состояниях, что нарисованы в Figma, на
          странице App UI-kit. Рядом с названием — id ноды: по нему компонент
          ищется в макете. Наведение и нажатие включены принудительно, чтобы их
          было видно одновременно.
        </p>
        <p>
          Чего в макете нет — здесь тоже нет: пробелы выписаны в{" "}
          <code>docs/design/GAPS.md</code>. Заглушки флага и логотипа провайдера
          повторяют заглушки из макета.
        </p>
      </div>

      <Section title="Icons" node="6492:1175">
        <div className="kit__grid">
          {Object.entries(ICONS).map(([name, icon]) =>
            icon.states.map((state) => (
              <Cell key={name + "-" + state} label={name + " / " + state}>
                <Icon name={name} state={state} />
              </Cell>
            ))
          )}
        </div>
      </Section>

      <Section title="Button" node="6523:385">
        <div className="kit__grid">
          {["default", "green", "yellow", "red"].map((variant) =>
            ["default", "hover", "active", "idle", "disable"].map((mode) => (
              <Cell key={variant + "-" + mode} label={variant + " / " + mode}>
                <Button variant={variant} mode={mode}>
                  Текст кнопки
                </Button>
              </Cell>
            ))
          )}
        </div>
      </Section>

      <Section title="BigBtn" node="6551:2825">
        <div className="kit__grid">
          {["default", "hover", "active", "idle"].map((state) => (
            <Cell key={state} label={state}>
              <div style={{ width: 421 }}>
                <BigBtn icon="uploadfile" label="Из файла" state={state} />
              </div>
            </Cell>
          ))}
        </div>
      </Section>

      <Section title="PowerButton" node="6481:7">
        <div className="kit__grid">
          {["default", "warning", "success", "error"].map((variant) =>
            ["default", "hover", "active"].map((state) => (
              <Cell key={variant + "-" + state} label={variant + " / " + state}>
                <PowerButton variant={variant} state={state} />
              </Cell>
            ))
          )}
        </div>
      </Section>

      <Section title="MenuBtn" node="6491:1071">
        <div className="kit__grid">
          {["default", "hover", "active", "activehover"].map((state) => (
            <Cell key={state} label={state}>
              <MenuBtn icon="home" label="Главная" state={state} />
            </Cell>
          ))}
        </div>
      </Section>

      <Section title="ModeTumbler" node="6488:48">
        <div className="kit__grid">
          {["proxy", "tunnel"].map((value) => (
            <Cell key={value} label={"выбран " + value}>
              <ModeTumbler
                value={value}
                options={[
                  { value: "proxy", label: "Прокси" },
                  { value: "tunnel", label: "Туннель" },
                ]}
              />
            </Cell>
          ))}
        </div>
      </Section>

      <Section title="Badges" node="6503:3035">
        <div className="kit__grid">
          {["first", "second"].map((variant) =>
            ["default", "warning", "success", "error"].map((color) => (
              <Cell key={variant + "-" + color} label={variant + " / " + color}>
                <Badge variant={variant} color={color}>
                  {variant === "first" ? "Hysteria2" : "gRPC"}
                </Badge>
              </Cell>
            ))
          )}
        </div>
      </Section>

      <Section title="Flag" node="6504:3256">
        <div className="kit__grid">
          {["md", "sm"].map((size) =>
            ["default", "warning", "success", "error"].map((status) => (
              <Cell key={size + "-" + status} label={size + " / " + status}>
                <Flag size={size} status={status}>
                  <FlagStub />
                </Flag>
              </Cell>
            ))
          )}
        </div>
      </Section>

      <Section title="Tumbler" node="6566:5073">
        <div className="kit__grid">
          <Cell label="default">
            <Tumbler checked={false} />
          </Cell>
          <Cell label="active">
            <Tumbler checked />
          </Cell>
        </div>
      </Section>

      <Section title="Input / Textarea" node="6566:5077 · 6557:2093">
        <div className="kit__stack" style={{ maxWidth: 752 }}>
          <Cell label="input / default">
            <div style={{ width: 588 }}>
              <Input placeholder="impVPM Базовый" readOnly />
            </div>
          </Cell>
          <Cell label="input / txt">
            <div style={{ width: 588 }}>
              <Input defaultValue="impVPM Базовый" />
            </div>
          </Cell>
          <Cell label="textarea / default">
            <Textarea placeholder="https://example.com/sub или vless://..." readOnly />
          </Cell>
          <Cell label="textarea / txt">
            <Textarea defaultValue="https://example.com" />
          </Cell>
        </div>
      </Section>

      <Section title="SettingsItem" node="6636:3953">
        <div className="kit__stack">
          {["default", "hover"].map((state) => (
            <Cell key={state} label={state}>
              <SettingsItem
                state={state}
                icon="subrouting"
                title="Профили маршрутизации"
                description="Здесь вы можете настроить собственную маршрутизацию или использовать маршрутизацию провайдера"
                external
              />
            </Cell>
          ))}
        </div>
      </Section>

      <Section title="Header" node="6521:263">
        <div className="kit__stack">
          {["default", "processing", "success", "error"].map((variant) => (
            <Cell key={variant} label={variant}>
              <Header variant={variant} title={HEADER_TEXT[variant]} time="12:23" />
            </Cell>
          ))}
        </div>
      </Section>

      <Section title="Speed" node="6528:714">
        <div className="kit__grid">
          <Cell label="default / default">
            <div style={{ width: 423 }}>
              <Speed label="Загружено" rate="0 кб/с" total="0 Мб" />
            </div>
          </Cell>
          <Cell label="second / default">
            <div style={{ width: 423 }}>
              <Speed variant="second" label="Отправлено" rate="0 кб/с" total="0 Мб" />
            </div>
          </Cell>
          <Cell label="default / active">
            <div style={{ width: 423 }}>
              <Speed mode="active" label="Загружено" rate="312 кб/с" total="312 Мб" />
            </div>
          </Cell>
          <Cell label="second / active">
            <div style={{ width: 423 }}>
              <Speed
                variant="second"
                mode="active"
                label="Отправлено"
                rate="12 кб/с"
                total="12 Мб"
              />
            </div>
          </Cell>
        </div>
      </Section>

      <Section title="Sidebar" node="6492:1648">
        <div className="kit__grid">
          <Cell label="default (свёрнут)">
            <div className="kit__sidebar-frame">
              <Sidebar items={MENU_ITEMS} bottomItem={SETTINGS_ITEM} activeKey="home" />
            </div>
          </Cell>
          <Cell label="openhover">
            <div className="kit__sidebar-frame">
              <Sidebar
                items={MENU_ITEMS}
                bottomItem={SETTINGS_ITEM}
                activeKey="home"
                state="openhover"
              />
            </div>
          </Cell>
          <Cell label="opened">
            <div className="kit__sidebar-frame">
              <Sidebar opened items={MENU_ITEMS} bottomItem={SETTINGS_ITEM} activeKey="home" />
            </div>
          </Cell>
        </div>
      </Section>

      <Section title="ServerItem" node="6500:2700">
        <div className="kit__stack" style={{ maxWidth: 785 }}>
          {["default", "hover"].map((mode) => (
            <Cell key={"header-" + mode} label={"header / " + mode}>
              <ServerItem
                variant="header"
                mode={mode}
                flag={<FlagStub />}
                badges={[BADGE_SAMPLE]}
                title="Россия | Hysteria2"
                ping="90мс"
              />
            </Cell>
          ))}
          {["default", "hover"].map((mode) => (
            <Cell key={"subitem-" + mode} label={"subitem / " + mode}>
              <ServerItem
                variant="subitem"
                mode={mode}
                logo={<LogoStub />}
                title="impVPN Базовый"
                count={39}
                subtitle="До 16.08.26 в 21:32 | 634.5 Гб / ∞"
              />
            </Cell>
          ))}
          {["default", "hover"].map((mode) => (
            <Cell key={"myitem-" + mode} label={"myitem / " + mode}>
              <ServerItem variant="myitem" mode={mode} title="Мои сервера" count={3} />
            </Cell>
          ))}
          {["default", "hover"].map((mode) => (
            <Cell key={"row-" + mode} label={"row / " + mode}>
              <ServerItem
                variant="row"
                mode={mode}
                flag={<FlagStub />}
                badges={[BADGE_SAMPLE]}
                title="Россия | Hysteria2"
                ping="90мс"
              />
            </Cell>
          ))}
          <Cell label="autoserver / default">
            <ServerItem
              variant="autoserver"
              badges={[{ label: "Авто" }]}
              title="Авто сервер"
              ping="90мс"
            />
          </Cell>
        </div>
      </Section>

      <Section title="HomeServerList" node="6504:3877">
        <div className="kit__stack" style={{ maxWidth: 778 }}>
          {["default", "hover"].map((status) => (
            <Cell key={"closed-" + status} label={"свёрнут / " + status}>
              <HomeServerList status={status} header={serverHeader(false)} />
            </Cell>
          ))}
          {["default", "hover"].map((status) => (
            <Cell key={"open-" + status} label={"раскрыт / " + status}>
              <HomeServerList status={status} open header={serverHeader(true)}>
                <ServerItem
                  variant="row"
                  flag={<FlagStub />}
                  badges={[BADGE_SAMPLE]}
                  title="Россия | Hysteria2"
                  ping="90мс"
                />
                <ServerItem
                  variant="row"
                  flag={<FlagStub />}
                  badges={[BADGE_SAMPLE]}
                  title="Россия | Hysteria2"
                  ping="90мс"
                />
              </HomeServerList>
            </Cell>
          ))}
        </div>
      </Section>
    </div>
  );
}
