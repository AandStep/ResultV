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
 * Страница «Умные правила». Figma "ResultV" -> App Design, ряд `SmartRules`.
 *
 * Три фрейма умного режима — это одна раскладка в трёх стадиях жизни одного
 * значения, а не три экрана:
 *
 *   6569:6229  поле пустое, видна подсказка
 *   6594:3104  строка набрана, но ещё не подтверждена — белым текстом
 *   6594:3178  после Enter строка стала тегом с крестиком
 *
 * Всю эту стадийность держит TagField из кита, здесь остаётся раскладка.
 *
 * Режим выбирается двумя плитками BigBtn, а не тумблером: у Global подписи
 * обоих блоков переворачиваются («через ВПН» -> «напрямую»), потому что и
 * поля конфига за ними стоят разные. Смысловая пара для каждого режима
 * приходит пропом `text`.
 */

import { BigBtn, Button, Icon, SettingsItem, TagField } from "../../components/kit";
import PageHeader from "./PageHeader";
import "./SmartRulesPage.css";

/*
 * Подписи ровно как в макете. Отдельным объектом, потому что в приложении они
 * идут через i18n: `SmartRulesScreen` подставляет свой набор.
 */
export const SMART_RULES_TEXT = {
  title: "Умные правила",
  subtitle:
    "Выбрав “Умный режим” маршрутизация настроится без сторонних правил. " +
    "Клиент всё сделает за вас.",
  smart: "Умный режим",
  global: "Глобальный режим",
  sitesTitle: "Сайты через ВПН",
  sitesSubtitle:
    "Если сайт оказался не в списке умного режима его можно добавить самостоятельно.",
  sitesPlaceholder: "Введите IP или хост и нажмите Enter чтобы добавить",
  appsTitle: "Приложения через ВПН",
  appsSubtitle: "Весь трафик этих приложений пойдет в туннель.",
  appsPlaceholder: "Например discord.exe",
  pickFile: "Выбрать файл на компьютере",
  clear: "Очистить список",
  remove: "Убрать из списка",
  profilesTitle: "Профили маршрутизации",
  profilesDesc:
    "Здесь вы можете настроить собственную маршрутизацию или использовать маршрутизацию провайдера",
};

/*
 * Блок списка: заголовок с подписью, корзина справа и поле-теги под ними.
 * Оба блока страницы устроены одинаково, у второго снизу добавляется кнопка.
 */
function RuleCard({ title, subtitle, clearLabel, onClear, children }) {
  return (
    <section className="rv-smart-rules__card">
      <div className="rv-smart-rules__card-head">
        <div className="rv-smart-rules__card-text">
          <h2 className="rv-smart-rules__card-title">{title}</h2>
          <p className="rv-smart-rules__card-subtitle">{subtitle}</p>
        </div>
        <button
          type="button"
          className="rv-smart-rules__clear"
          aria-label={clearLabel}
          title={clearLabel}
          onClick={onClear}
        >
          <Icon name="delete" size={24} color="currentColor" />
        </button>
      </div>
      {children}
    </section>
  );
}

export default function SmartRulesPage({
  mode = "smart",
  onModeChange,
  sites = [],
  onAddSite,
  onRemoveSite,
  onClearSites,
  apps = [],
  onAddApp,
  onRemoveApp,
  onClearApps,
  onPickApp,
  /* Профили работают только в глобальном режиме: в умном клиент считает
     маршруты сам. Кнопка появляется вместе с режимом. */
  onOpenProfiles,
  sidebar,
  text = SMART_RULES_TEXT,
}) {
  return (
    <div className="rv-smart-rules">
      {sidebar}
      <div className="rv-smart-rules__content rv-scroll">
        <PageHeader title={text.title} subtitle={text.subtitle} />
        <div className="rv-smart-rules__body">
          <div className="rv-smart-rules__modes">
            <BigBtn
              icon="smart"
              label={text.smart}
              /* Выбранный режим — залитое состояние плитки из кита. */
              state={mode === "smart" ? "idle" : undefined}
              aria-pressed={mode === "smart"}
              onClick={() => onModeChange?.("smart")}
            />
            <BigBtn
              icon="globe"
              label={text.global}
              state={mode === "global" ? "idle" : undefined}
              aria-pressed={mode === "global"}
              onClick={() => onModeChange?.("global")}
            />
          </div>

          <RuleCard
            title={text.sitesTitle}
            subtitle={text.sitesSubtitle}
            clearLabel={text.clear}
            onClear={onClearSites}
          >
            <TagField
              values={sites}
              onAdd={onAddSite}
              onRemove={onRemoveSite}
              placeholder={text.sitesPlaceholder}
              removeLabel={text.remove}
            />
          </RuleCard>

          <RuleCard
            title={text.appsTitle}
            subtitle={text.appsSubtitle}
            clearLabel={text.clear}
            onClear={onClearApps}
          >
            <TagField
              values={apps}
              onAdd={onAddApp}
              onRemove={onRemoveApp}
              placeholder={text.appsPlaceholder}
              removeLabel={text.remove}
            />
            <Button
              variant="green"
              className="rv-smart-rules__pick"
              onClick={onPickApp}
            >
              {text.pickFile}
            </Button>
          </RuleCard>

          {mode === "global" && (
            <SettingsItem
              as="button"
              type="button"
              icon="subrouting"
              title={text.profilesTitle}
              description={text.profilesDesc}
              external
              className="rv-smart-rules__profiles"
              onClick={onOpenProfiles}
            />
          )}
        </div>
      </div>
    </div>
  );
}
