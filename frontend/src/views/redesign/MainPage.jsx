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
 * Главный экран. Figma "ResultV" -> App Design, верхний ряд фреймов MainPage.
 *
 * Шесть фреймов макета — это не шесть раскладок, а четыре состояния плюс два
 * состояния кнопки питания:
 *
 *   6454:971   idle        — «Не подключено»
 *   6530:763   idle        — то же, курсор на кнопке питания
 *   6530:863   idle        — то же, кнопка нажата
 *   6530:963   connecting  — «Подключение...»
 *   6530:1063  connected   — «Защищено», появляется время и графики скорости
 *   6530:1163  error       — «Что-то пошло не так», вместо графиков две кнопки
 *
 * Состояние задаёт разом четыре вещи: вариант шапки, вариант кнопки питания,
 * подсветку флага и цвет бейджа в карточке сервера.
 *
 * Отдельным рядом идут фреймы `MainPage (Servers list)` (6530:2627, 6530:2870,
 * 6530:3042, 6530:3190, 6530:4158, 6532:1746) — та же страница с раскрытой
 * карточкой сервера: в шапке появляются кнопки пинга и сортировки, шеврон
 * разворачивается вверх, а под шапкой встаёт список серверов.
 *
 * Позже дозаказаны ещё два фрейма:
 *   6504:3878  раскрытый список, разбитый на группы по подпискам
 *   6752:4237  страница без серверов и новая типографика тумблера
 */

import { Fragment } from "react";
import {
  Header,
  Icon,
  PowerButton,
  ModeTumbler,
  HomeServerList,
  ServerItem,
  Speed,
  Button,
} from "../../components/kit";
import "./MainPage.css";

export const MAIN_PAGE_STATUSES = [
  "idle",
  "connecting",
  "connected",
  "disconnecting",
  "error",
];

/*
 * Одно состояние — один набор вариантов у вложенных компонентов.
 * Взято с фреймов макета, а не выведено логически.
 */
const BY_STATUS = {
  idle: { header: "default", power: "default", accent: "default", speed: "default" },
  connecting: {
    header: "processing",
    power: "warning",
    accent: "warning",
    speed: "default",
    /*
     * Пока идёт подключение, кнопка остаётся вдавленной — как в прежнем
     * дизайне: нажатие не отпускается, пока не подключились. Это и есть
     * признак работы, отдельной анимации ему не нужно.
     */
    powerPressed: true,
  },
  connected: { header: "success", power: "success", accent: "success", speed: "active" },
  /*
   * Отключение выглядит как подключение наоборот: та же жёлтая вдавленная
   * кнопка, только заголовок другой. Кадра в макете нет — там есть шапка
   * `Processisng` и жёлтая кнопка, из них состояние и собрано.
   */
  disconnecting: {
    header: "processing",
    power: "warning",
    accent: "warning",
    speed: "default",
    powerPressed: true,
  },
  error: { header: "error", power: "error", accent: "error", speed: "default" },
};

/*
 * Подписи, как они набраны в макете. Отдельным пропом, потому что в
 * приложении они пойдут через i18n.
 */
export const MAIN_PAGE_TEXT = {
  idle: "Не подключено",
  connecting: "Подключение...",
  connected: "Защищено",
  error: "Что-то пошло не так",
  disconnecting: "Отключение...",
  resolving: "Подбор сервера",
  proxy: "Прокси",
  tunnel: "Туннель",
  download: "Загружено",
  upload: "Отправлено",
  editData: "Изменить данные",
  otherServer: "Выбрать другой сервер",
  /* Подписей к кнопкам пинга и сортировки в макете нет — это подсказки. */
  pingServers: "Измерить задержку до серверов",
  sortServers: "Сортировать по задержке",
  empty: "Добавьте сервер или подписку",
};

export default function MainPage({
  status = "idle",
  /*
   * Авто-группа перед подключением сама выбирает узел. Фаза короткая и
   * поверх любого состояния: страница остаётся какой была (в том числе
   * зелёной, если соединение уже живо), а жёлтым отзывается только заголовок.
   */
  resolving = false,
  powerState,
  mode = "proxy",
  onModeChange,
  onPower,
  powerDisabled = false,
  time,
  server,
  /*
   * Строки списка приходят группами `{ key, label, servers }` — по подписке,
   * плюс «Мои сервера» последней (фрейм 6504:3878). `label` у группы пустой,
   * когда группа одна: подписывать нечего.
   */
  serverGroups = [],
  serverListOpen = false,
  onToggleServerList,
  /* Серверов нет вовсе — вместо карточки и плиток скорости встаёт
     приглашение добавить сервер (фрейм 6752:4237). */
  empty = false,
  onAddServer,
  onPingServers,
  /* Замер задержки уже идёт — кнопка мигает, пока он не кончится. */
  pingBusy = false,
  onSortServers,
  download,
  upload,
  downloadChart,
  uploadChart,
  sidebar,
  text = MAIN_PAGE_TEXT,
  onSite,
  onTelegram,
  className = "",
  ...rest
}) {
  const look = BY_STATUS[status] ?? BY_STATUS.idle;
  const showResolving = resolving && status !== "disconnecting";

  return (
    <div className={`rv-main-page ${className}`} data-status={status} {...rest}>
      {sidebar}

      <div className="rv-main-page__content rv-scroll">
        <Header
          /* Отключение перекрывает подбор: если рвём соединение, то говорим
             именно это, чем бы ход ни начинался. */
          variant={showResolving ? "processing" : look.header}
          title={showResolving ? text.resolving : text[status]}
          time={time}
          onSite={onSite}
          onTelegram={onTelegram}
        />

        <div className="rv-main-page__power">
          <PowerButton
            variant={look.power}
            /* Витрина может задать состояние силой; иначе его диктует статус. */
            state={powerState ?? (look.powerPressed ? "active" : undefined)}
            disabled={powerDisabled}
            onClick={onPower}
          />
          <ModeTumbler
            value={mode}
            onChange={onModeChange}
            options={[
              { value: "proxy", label: text.proxy },
              { value: "tunnel", label: text.tunnel },
            ]}
          />
        </div>

        <div className="rv-main-page__bottom">
          {empty ? (
            /*
             * Серверов нет — карточки выбора нет тоже, а на её месте стоит
             * приглашение. Нажатие ведёт на страницу добавления: другого
             * назначения у этого блока нет, хотя ховера в макете не нарисовано.
             */
            <button
              type="button"
              className="rv-main-page__empty rv-border"
              onClick={onAddServer}
            >
              <span className="rv-main-page__empty-icon">
                <Icon name="add" size={32} color="currentColor" />
              </span>
              <span className="rv-main-page__empty-text">{text.empty}</span>
            </button>
          ) : (
            <HomeServerList
              open={serverListOpen}
              header={
                <ServerItem
                  variant="header"
                  auto={server?.auto}
                  flag={server?.flag}
                  flagStatus={look.accent}
                  badges={server?.badges}
                  badgeColor={look.accent}
                  title={server?.title}
                  ping={server?.ping}
                  pingBusy={server?.pingBusy}
                  showPingBtn={serverListOpen}
                  showSortBtn={serverListOpen}
                  onPing={onPingServers}
                  pingBtnBusy={pingBusy}
                  onSort={onSortServers}
                  pingTitle={text.pingServers}
                  sortTitle={text.sortServers}
                  onClick={onToggleServerList}
                />
              }
            >
              {serverGroups.map((group) => (
                <Fragment key={group.key}>
                  {group.label && (
                    <p className="rv-home-server-list__group">{group.label}</p>
                  )}
                  {group.servers.map((item) => (
                    <ServerItem
                      key={item.key}
                      variant={item.variant ?? "row"}
                      flag={item.flag}
                      badges={item.badges}
                      title={item.title}
                      ping={item.ping}
                      pingBusy={item.pingBusy}
                      favorite={item.favorite}
                      onFavorite={item.onFavorite}
                      onClick={item.onSelect}
                    />
                  ))}
                </Fragment>
              ))}
            </HomeServerList>
          )}

          {/*
           * Раскрытый список плитки не прячет — он их выдавливает вниз, как в
           * макете; страница при этом становится длиннее и прокручивается.
           *
           * В состоянии ошибки ряд плиток в макете скрыт, а на его месте —
           * две кнопки в той же сетке.
           */}
          {empty ? null : status === "error" ? (
            <div className="rv-main-page__pair">
              <Button>{text.editData}</Button>
              <Button variant="red">{text.otherServer}</Button>
            </div>
          ) : (
            <div className="rv-main-page__pair">
              <Speed
                mode={look.speed}
                label={text.download}
                rate={download?.rate}
                total={download?.total}
              >
                {look.speed === "active" ? downloadChart : undefined}
              </Speed>
              <Speed
                variant="second"
                mode={look.speed}
                label={text.upload}
                rate={upload?.rate}
                total={upload?.total}
              >
                {look.speed === "active" ? uploadChart : undefined}
              </Speed>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
