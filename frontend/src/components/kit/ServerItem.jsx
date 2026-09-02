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


import Icon from "./Icon";
import Badge from "./Badge";
import Flag from "./Flag";
import "./ServerItem.css";

export const SERVER_ITEM_VARIANTS = ["header", "row", "subitem", "myitem", "autoserver"];

/*
 * Шеврон справа. Он есть у Header, SubItem и MyItem; у Row и AutoServer
 * вместо него звёздочка избранного.
 *
 * Наведение направление стрелки не меняет — только яркость, с 50 % до 80 %.
 * Проверено по рендеру макета: в обоих состояниях шеврон смотрит вниз.
 * Цвет идёт через currentColor, поэтому меняется плавно вместе со строкой.
 */
function Chevron() {
  return <Icon name="open" color="currentColor" className="rv-server-item__chevron" />;
}

/* Счётчик серверов рядом с названием группы: число и значок mdi:dns. */
function Count({ value }) {
  return (
    <span className="rv-server-item__count">
      {value}
      <Icon name="dns" />
    </span>
  );
}

/**
 * Строка списка серверов (Figma node 6500:2700).
 *
 * `flagStatus` и `badgeColor` подсвечивают строку под состояние подключения:
 * на главной подложка флага и бейдж протокола окрашиваются вместе с шапкой и
 * кнопкой питания.
 *
 * Флаг и логотип провайдера приходят содержимым (`flag`, `logo`) — в макете
 * на их месте заглушки. Тексты тоже пропами: в макете это «Россия | Hysteria2»,
 * «impVPN Базовый», «Мои сервера», «Авто сервер».
 *
 * `mode` нужен витрине кита; в приложении наведение отрабатывает CSS.
 */
export default function ServerItem({
  variant = "header",
  mode,
  flag,
  flagStatus = "default",
  /* Авто-группа: вместо флага страны — значок автовыбора. Отдельным пропом,
     потому что головой карточки на главной может встать и она. */
  auto = false,
  logo,
  badges = [],
  badgeColor = "default",
  title,
  subtitle,
  count,
  ping,
  showPing = true,
  /* Замер этой строки ещё идёт — вместо числа крутится спиннер. Ждать
     молча нечестно: до первого замера на месте задержки пусто, и понять,
     меряется она или просто не измерилась, было нельзя. */
  pingBusy = false,
  showPingBtn = false,
  showSortBtn = false,
  /* Кнопка запущенного действия мигает, пока идёт работа: замер задержки по
     всей группе и обновление подписки занимают секунды. У обновления сверх
     того крутится сама иконка. */
  pingBtnBusy = false,
  syncBusy = false,
  favorite = false,
  /* Строка подключённого сервера: фрейм 6744:4162 страницы серверов. Цвет
     подложки флага и бейджа задают flagStatus/badgeColor, здесь — сама
     строка. */
  active = false,
  onPing,
  onSort,
  /* Подсказки к кнопкам-иконкам: в макете их нет, но кнопка без подписи без
     них не читается. */
  pingTitle,
  sortTitle,
  editTitle,
  syncTitle,
  deleteTitle,
  favoriteTitle,
  onFavorite,
  onEdit,
  onSync,
  onDelete,
  className = "",
  ...rest
}) {
  const isGroup = variant === "subitem" || variant === "myitem";
  const isAuto = auto || variant === "autoserver";
  const hasFlag = (variant === "header" || variant === "row") && !isAuto;
  const showsFavorite = variant === "row" || variant === "autoserver";

  /*
   * Строка целиком кликабельна (в шапке — раскрывает список, в списке —
   * выбирает сервер), поэтому кнопки внутри гасят всплытие: иначе нажатие на
   * «пинг» или «избранное» заодно срабатывало бы как нажатие на строку.
   */
  const stop = (handler) => (event) => {
    event.stopPropagation();
    handler?.(event);
  };

  /* `data-action` нужен стилям: удаление краснеет под курсором, обновление
     крутит иконку в работе. */
  const iconBtn = (name, handler, key, title, busy = false) => (
    <button
      key={key}
      type="button"
      className="rv-server-item__icon-btn"
      data-action={name}
      data-busy={busy || undefined}
      title={title}
      aria-label={title}
      onClick={stop(handler)}
    >
      {/* Цвет берёт кнопка: инлайн-цвет иконки перебил бы её ховер. */}
      <Icon name={name} color="currentColor" />
    </button>
  );

  return (
    <div
      className={`rv-server-item ${className}`}
      data-variant={variant}
      data-mode={mode}
      data-active={active || undefined}
      {...rest}
    >
      <div className="rv-server-item__main">
        {hasFlag && (
          <Flag size={variant === "row" ? "sm" : "md"} status={flagStatus}>
            {flag}
          </Flag>
        )}
        {variant === "subitem" && <span className="rv-server-item__badge">{logo}</span>}
        {isAuto && (
          /* Подложка окрашивается под состояние так же, как подложка флага —
             иначе шапка с авто-группой теряет цвет подключения. */
          <span className="rv-server-item__badge" data-status={flagStatus}>
            {/* Значок горит цветом состояния в полную силу — цвет держит
                подложка, иконка берёт его через currentColor. */}
            <Icon name="autoawesome" color="currentColor" />
          </span>
        )}

        {isGroup ? (
          <div className="rv-server-item__details">
            <div className="rv-server-item__title-row">
              <p className="rv-server-item__title">{title}</p>
              {count != null && <Count value={count} />}
            </div>
            {subtitle && <p className="rv-server-item__subtitle">{subtitle}</p>}
          </div>
        ) : (
          <div className="rv-server-item__details">
            {badges.length > 0 && (
              <div className="rv-server-item__tags">
                {badges.map((badge, i) => (
                  <Badge
                    key={badge.label}
                    variant={i === 0 ? "first" : "second"}
                    color={badge.color ?? badgeColor}
                    title={badge.title}
                  >
                    {badge.label}
                  </Badge>
                ))}
              </div>
            )}
            <p className="rv-server-item__title">{title}</p>
          </div>
        )}
      </div>

      <div className="rv-server-item__actions">
        {!isGroup &&
          showPing &&
          (pingBusy ? (
            <span className="rv-server-item__ping-spinner" aria-hidden="true" />
          ) : (
            ping != null && <p className="rv-server-item__ping">{ping}</p>
          ))}
        {showsFavorite && (
          <button
            type="button"
            className="rv-server-item__icon-btn"
            title={favoriteTitle}
            aria-label={favoriteTitle}
            onClick={stop(onFavorite)}
          >
            <Icon name="fav" state={favorite ? "active" : "default"} />
          </button>
        )}
        {variant === "header" &&
          showPingBtn &&
          iconBtn("ping", onPing, "ping", pingTitle, pingBtnBusy)}
        {variant === "header" && showSortBtn && iconBtn("sort", onSort, "sort", sortTitle)}
        {variant === "subitem" && iconBtn("edit", onEdit, "edit", editTitle)}
        {(variant === "subitem" || variant === "myitem") &&
          iconBtn("sync", onSync, "sync", syncTitle, syncBusy)}
        {(variant === "subitem" || variant === "myitem") &&
          iconBtn("delete", onDelete, "delete", deleteTitle)}
        {!showsFavorite && <Chevron />}
      </div>
    </div>
  );
}
