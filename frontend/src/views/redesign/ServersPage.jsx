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
 * Страница серверов. Figma "ResultV" -> App Design, ряд фреймов ServersPage
 * (6557:3436 и соседние, см. ServersPage.css).
 *
 * От главной она отличается двумя вещами: шапкой (название страницы плюс
 * кнопки замера задержки и порядка вместо состояния подключения) и тем, что
 * карточек с серверами несколько — по одной на подписку и одна на серверы,
 * заведённые вручную. Сами карточки и строки те же самые, компоненты кита.
 */

import { HomeServerList, Icon, ServerItem } from "../../components/kit";
import PageHeader from "./PageHeader";
import "./ServersPage.css";

/* Подписи в написании макета. В приложении они идут через i18n. */
export const SERVERS_PAGE_TEXT = {
  title: "Список серверов",
  search: "Поиск серверов...",
  /* Подписей к кнопкам-иконкам в макете нет — это подсказки. */
  pingServers: "Измерить задержку до серверов",
  sortServers: "Порядок списка",
  editSubscription: "Настройки подписки",
  refreshSubscription: "Обновить подписку",
  deleteSubscription: "Удалить подписку",
  pingGroup: "Измерить задержку до серверов группы",
  deleteGroup: "Удалить все серверы группы",
  favorite: "В избранное",
  empty: "Серверы не найдены",
};

/**
 * `groups` — карточки страницы сверху вниз. Каждая описывает свою голову
 * (подписка `subitem` с логотипом и строкой о сроке и трафике, либо
 * `myitem` — «Мои сервера») и свои строки.
 */
export default function ServersPage({
  title,
  subtitle,
  search = "",
  onSearchChange,
  onPingServers,
  /* Замер по всей странице уже идёт — кнопка мигает, пока он не кончится. */
  pingBusy = false,
  onSortServers,
  groups = [],
  empty = false,
  sidebar,
  text = SERVERS_PAGE_TEXT,
  className = "",
  ...rest
}) {
  return (
    <div className={`rv-servers-page ${className}`} {...rest}>
      {sidebar}

      <div className="rv-servers-page__content rv-scroll">
        <PageHeader
          title={title ?? text.title}
          subtitle={subtitle}
          actions={
            <>
              <button
                type="button"
                className="rv-page-header__btn"
                data-busy={pingBusy || undefined}
                title={text.pingServers}
                aria-label={text.pingServers}
                onClick={onPingServers}
              >
                <Icon name="ping" color="currentColor" />
              </button>
              <button
                type="button"
                className="rv-page-header__btn"
                title={text.sortServers}
                aria-label={text.sortServers}
                onClick={onSortServers}
              >
                <Icon name="sort" color="currentColor" />
              </button>
            </>
          }
        />

        <div className="rv-servers-page__body">
          {/* Метка, а не div: щелчок по всему полю ставит курсор в строку. */}
          <label className="rv-servers-page__search rv-border">
            <input
              type="text"
              className="rv-servers-page__search-input"
              value={search}
              placeholder={text.search}
              onChange={(event) => onSearchChange?.(event.target.value)}
            />
            <Icon name="search" color="currentColor" className="rv-servers-page__search-icon" />
          </label>

          {groups.map((group) => (
            <HomeServerList
              key={group.key}
              open={group.open}
              header={
                <ServerItem
                  variant={group.variant}
                  logo={group.logo}
                  title={group.title}
                  count={group.count}
                  subtitle={group.subtitle}
                  onClick={group.onToggle}
                  onEdit={group.onEdit}
                  onSync={group.onSync}
                  syncBusy={group.syncBusy}
                  onDelete={group.onDelete}
                  editTitle={text.editSubscription}
                  syncTitle={
                    group.variant === "subitem" ? text.refreshSubscription : text.pingGroup
                  }
                  deleteTitle={
                    group.variant === "subitem" ? text.deleteSubscription : text.deleteGroup
                  }
                />
              }
            >
              {group.servers.map((item) => (
                <ServerItem
                  key={item.key}
                  variant={item.variant ?? "row"}
                  flag={item.flag}
                  flagStatus={item.accent}
                  badges={item.badges}
                  badgeColor={item.accent}
                  title={item.title}
                  ping={item.ping}
                  pingBusy={item.pingBusy}
                  favorite={item.favorite}
                  favoriteTitle={text.favorite}
                  active={item.active}
                  onFavorite={item.onFavorite}
                  onClick={item.onSelect}
                />
              ))}
            </HomeServerList>
          ))}

          {empty && <p className="rv-servers-page__empty">{text.empty}</p>}
        </div>
      </div>
    </div>
  );
}
