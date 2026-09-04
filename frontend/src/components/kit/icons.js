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
 * Иконки UI-kit поверх Material Design Icons (@mdi/js).
 *
 * Соответствие «имя в Figma -> экспорт библиотеки» не угадано, а доказано:
 * каждый путь из макета сверен с библиотечным геометрически (chamfer по
 * точкам вдоль контура). Проверка воспроизводима — `node
 * scripts/design/verify-icons.mjs`, эталон лежит в ./icons.figma.js.
 *
 * У 23 иконок расхождение ровно 0. У `side`, `open` и `smart` оно
 * не превышает 0.053 px на сетке 24 при совпадающих габаритах: это
 * погрешность Figma, которая при экспорте разворачивает дуги (A) в
 * кубические кривые (C). Другой иконкой это не является.
 *
 * `offset` — сдвиг, с которым иконка стоит в макете относительно своей
 * рамки; уходит в viewBox, чтобы позиция совпадала попиксельно.
 * `states` перечисляет только состояния, нарисованные в макете; чего там
 * нет — записано в docs/design/GAPS.md, а не дорисовано здесь.
 */

import {
  mdiAlertCircleOutline,
  mdiArrowDecision,
  mdiArrowLeft,
  mdiBrain,
  mdiCallSplit,
  mdiCart,
  mdiClipboardText,
  mdiClockTimeThreeOutline,
  mdiClipboardTextMultiple,
  mdiClose,
  mdiCog,
  mdiContentCopy,
  mdiContentSave,
  mdiEarth,
  mdiSync,
  mdiPencil,
  mdiDns,
  mdiDelete,
  mdiCreation,
  mdiFileUpload,
  mdiFormatListBulleted,
  mdiHome,
  mdiLightningBolt,
  mdiLinkVariant,
  mdiLock,
  mdiMagnify,
  mdiMenuDown,
  mdiMenuUp,
  mdiOpenInNew,
  mdiPageLayoutSidebarRight,
  mdiPlus,
  mdiPower,
  mdiShieldOutline,
  mdiSignalVariant,
  mdiSitemap,
  mdiSortVariant,
  mdiStarOutline,
  mdiTrayArrowDown,
  mdiTrayArrowUp,
  mdiTune,
  mdiWeb,
} from '@mdi/js';

/*
 * Логотип Telegram — брендовый знак, его нет ни в @mdi/js, ни в наборе
 * Google. Единственный путь, взятый напрямую из экспорта Figma.
 */
const TELEGRAM_BRAND =
  'M12 2C6.48 2 2 6.48 2 12C2 17.52 6.48 22 12 22C17.52 22 22 17.52 22 12C22 6.48 17.52 2 12 2ZM16.64 8.8C16.49 10.38 15.84 14.22 15.51 15.99C15.37 16.74 15.09 16.99 14.83 17.02C14.25 17.07 13.81 16.64 13.25 16.27C12.37 15.69 11.87 15.33 11.02 14.77C10.03 14.12 10.67 13.76 11.24 13.18C11.39 13.03 13.95 10.7 14 10.49C14.0069 10.4582 14.006 10.4252 13.9973 10.3938C13.9886 10.3624 13.9724 10.3337 13.95 10.31C13.89 10.26 13.81 10.28 13.74 10.29C13.65 10.31 12.25 11.24 9.52 13.08C9.12 13.35 8.76 13.49 8.44 13.48C8.08 13.47 7.4 13.28 6.89 13.11C6.26 12.91 5.77 12.8 5.81 12.45C5.83 12.27 6.08 12.09 6.55 11.9C9.47 10.63 11.41 9.79 12.38 9.39C15.16 8.23 15.73 8.03 16.11 8.03C16.19 8.03 16.38 8.05 16.5 8.15C16.6 8.23 16.63 8.34 16.64 8.42C16.63 8.48 16.65 8.66 16.64 8.8Z';

export const ICONS = {
  home: {
    path: mdiHome,
    size: 24,
    states: ['default', 'hover'],
  },
  add: {
    path: mdiPlus,
    size: 24,
    states: ['default', 'hover'],
  },
  list: {
    path: mdiFormatListBulleted,
    size: 24,
    states: ['default', 'hover'],
  },
  routing: {
    path: mdiCallSplit,
    size: 24,
    states: ['default', 'hover'],
  },
  buy: {
    path: mdiCart,
    size: 24,
    states: ['default', 'hover'],
  },
  logs: {
    path: mdiClipboardText,
    size: 24,
    states: ['default', 'hover'],
  },
  settings: {
    path: mdiCog,
    size: 24,
    states: ['default', 'hover'],
    offset: [0.2705, 0],
  },
  side: {
    path: mdiPageLayoutSidebarRight,
    size: 24,
    states: ['default', 'hover'],
  },
  fav: {
    path: mdiStarOutline,
    size: 24,
    states: ['default', 'hover', 'active'],
  },
  sort: {
    path: mdiSortVariant,
    size: 24,
    states: ['default', 'hover'],
  },
  ping: {
    path: mdiLightningBolt,
    size: 24,
    states: ['default', 'hover'],
  },
  open: {
    /*
     * Единственная иконка, у которой состояние меняет саму геометрию.
     *
     * В макете обе стрелки отражены по вертикали внутри самого компонента, а
     * экспорт отдаёт путь без этого отражения — проверено по рендеру: в покое
     * шеврон смотрит ВНИЗ, в состоянии Active ВВЕРХ. Поэтому здесь пути взяты
     * уже отражёнными, и сдвиг, который был нужен для сравнения с
     * неотражённым экспортом, исчез.
     *
     * `flipY` нужен только сверке (verify-icons): эталон в icons.figma.js
     * хранит неотражённый экспорт.
     */
    path: { default: mdiMenuDown, active: mdiMenuUp },
    size: 32,
    states: ['default', 'active'],
    flipY: true,
  },
  site: {
    path: mdiWeb,
    size: 24,
    states: ['default', 'hover'],
  },
  tg: {
    path: TELEGRAM_BRAND,
    size: 24,
    states: ['default', 'hover'],
    // Не из библиотеки — сверять не с чем, проверка эту иконку пропускает.
    source: 'figma',
  },
  uploadfile: {
    path: mdiFileUpload,
    size: 48,
    states: ['default'],
  },
  clipboard: {
    path: mdiClipboardTextMultiple,
    size: 48,
    states: ['default'],
  },
  globe: {
    path: mdiEarth,
    size: 48,
    states: ['default'],
  },
  smart: {
    path: mdiBrain,
    size: 48,
    states: ['default'],
  },
  copy: {
    path: mdiContentCopy,
    size: 24,
    states: ['default'],
  },
  subrouting: {
    path: mdiArrowDecision,
    size: 36,
    states: ['default'],
  },
  adsettings: {
    path: mdiTune,
    size: 36,
    states: ['default'],
  },
  subscriptions: {
    path: mdiSignalVariant,
    size: 36,
    states: ['default'],
  },
  security: {
    path: mdiLock,
    size: 36,
    states: ['default'],
  },
  network: {
    path: mdiSitemap,
    size: 36,
    states: ['default'],
  },
  import: {
    path: mdiTrayArrowDown,
    size: 24,
    states: ['default'],
  },
  export: {
    path: mdiTrayArrowUp,
    size: 24,
    states: ['default'],
  },
  /*
   * Во фрейме Icons этой иконки тоже нет, и в макете её негде взять: страниц
   * внутри пунктов настроек не нарисовано, а возвращаться с них надо.
   * См. docs/design/GAPS.md.
   */
  back: {
    path: mdiArrowLeft,
    size: 24,
    states: ['default', 'hover'],
    // Сверять не с чем: в макете этой иконки нет вовсе.
    reference: 'absent',
  },
  /*
   * Во фрейме Icons этой иконки нет — она встречается внутри SettingsItem
   * (слой «mdi:external-link», белый 50 %). Заведена здесь, чтобы компонент
   * не рисовал свой SVG. См. docs/design/GAPS.md I-6.
   */
  externallink: {
    path: mdiOpenInNew,
    size: 24,
    states: ['default'],
  },
  /*
   * Тоже нет во фрейме Icons — встречается в Header (слой
   * «mdi:clock-time-three-outline», белый 50 %). См. GAPS.md I-6.
   */
  clock: {
    path: mdiClockTimeThreeOutline,
    size: 24,
    states: ['default'],
  },
  /*
   * Тоже вне фрейма Icons — сердце PowerButton (слой «mdi:power»).
   * Размер там 110 или 106 и цвет свой в каждом варианте, поэтому базовым
   * взят 24: и размер, и цвет задаёт вызывающая сторона. См. GAPS.md I-6.
   */
  power: {
    path: mdiPower,
    size: 24,
    states: ['default'],
  },
  /*
   * Иконки из ServerItem — их тоже нет во фрейме Icons. Имена слоёв в
   * макете: mdi:dns, mdi:edit, mdi:sync, mdi:delete, mdi:auto-awesome.
   * См. GAPS.md I-6.
   */
  dns: {
    path: mdiDns,
    size: 14,
    states: ['default'],
  },
  edit: {
    path: mdiPencil,
    size: 24,
    states: ['default'],
  },
  sync: {
    path: mdiSync,
    size: 24,
    states: ['default'],
  },
  delete: {
    path: mdiDelete,
    size: 24,
    states: ['default'],
  },
  autoawesome: {
    path: mdiCreation,
    size: 24,
    states: ['default'],
  },
  /*
   * Сохранение журнала в файл — слой «mdi:floppy-disc» в шапке страницы
   * логов (6618:4194). Во фрейме Icons его тоже нет, поэтому и сырого
   * экспорта в icons.figma.js нет: соответствие доказано `find:icon`
   * (mdiContentSave, 0.0002). См. GAPS.md I-6.
   */
  save: {
    path: mdiContentSave,
    size: 24,
    states: ['default'],
    reference: 'pending',
  },
  /*
   * Иконки окон: щит у «Добавления серверов», кружок с восклицательным знаком
   * у «Уведомления», крестик закрытия.
   *
   * `reference: 'pending'` — сырого экспорта из Figma в icons.figma.js пока
   * нет: файл был закрыт, когда иконки понадобились. Соответствие при этом
   * доказано тем же способом, что и у остальных, — `npm run find:icon`:
   * shield 0.0000, alert 0.0002, close 0.0000. Эталон дописать, когда макет
   * снова под рукой.
   */
  shield: {
    path: mdiShieldOutline,
    size: 32,
    states: ['default'],
    reference: 'pending',
  },
  alert: {
    path: mdiAlertCircleOutline,
    size: 32,
    states: ['default'],
    reference: 'pending',
  },
  close: {
    path: mdiClose,
    size: 24,
    states: ['default', 'hover'],
    reference: 'pending',
  },
  /*
   * Во фрейме Icons её нет — встречается только в поле поиска на странице
   * серверов (слой «mdi:search», белый 20 %). Соответствие снято с экспорта
   * `npm run find:icon b3b09714ac855669772aed33d005bf61ae68b5e4`:
   * mdiMagnify, расхождение 0. См. docs/design/GAPS.md I-6.
   */
  search: {
    path: mdiMagnify,
    size: 24,
    states: ['default'],
    reference: 'pending',
  },
  /*
   * Тоже вне фрейма Icons — знак ссылки в теге «Поддержка» окна настроек
   * подписки (слой «mdi:link-variant», белый 50 %, 14 px). Соответствие:
   * `npm run find:icon 5cbb18f3ca01c2d24389f75609a16020de2b5e5b` — 
   * mdiLinkVariant, расхождение 0.0016. См. docs/design/GAPS.md I-6.
   */
  link: {
    path: mdiLinkVariant,
    size: 14,
    states: ['default'],
    reference: 'pending',
  },
};

/*
 * Цвет состояния. Базовое правило по всему фрейму Icons: default — белый
 * 50 %, hover — белый 80 %. Три иконки из него выпадают намеренно: им задан
 * сплошной фирменный цвет.
 */
export const ICON_STATE_COLOR = {
  default: 'var(--rv-icon-default)',
  hover: 'var(--rv-icon-hover)',
  active: 'var(--rv-icon-default)',
};

export const ICON_COLOR_OVERRIDE = {
  fav: { active: 'var(--rv-warning)' },
  site: { hover: 'var(--rv-main-color)' },
  tg: { hover: 'var(--rv-icon-tg-hover)' },
};

export const ICON_NAMES = Object.keys(ICONS);
