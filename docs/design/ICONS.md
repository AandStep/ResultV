# Иконки: соответствие макета и библиотеки

Иконки UI-kit берутся из **Material Design Icons** (`@mdi/js`, набор
Templarian). Своих SVG в проекте нет — кроме логотипа Telegram, которого в
библиотеке не существует.

## Как получена таблица

Соответствие не подбиралось по названиям, а **доказано геометрически**.
Побайтовое сравнение путей не работает: Figma при экспорте разворачивает
дуги (`A`) в кубические кривые (`C`), поэтому одна и та же иконка в макете и
в библиотеке записана по-разному. Поэтому сравнивается форма: по 600 точек на
равных долях длины контура, симметричное chamfer-расстояние (не зависит ни от
направления обхода, ни от стартовой вершины).

Проверка воспроизводима:

```
cd frontend && npm run verify:icons
```

Эталон — `frontend/src/components/kit/icons.figma.js` (сырой экспорт из Figma,
в приложении не используется). Рабочая карта — `icons.js`.

## Таблица

Столбец «откл.» — расхождение с макетом в пикселях на сетке 24×24.

| Figma | размер | @mdi/js | откл. | примечание |
|---|---|---|---|---|
| Home | 24 | `mdiHome` | 0 | |
| Add | 24 | `mdiPlus` | 0 | |
| List | 24 | `mdiFormatListBulleted` | 0.0002 | |
| Routing | 24 | `mdiCallSplit` | 0 | |
| Buy | 24 | `mdiCart` | 0.0003 | |
| Logs | 24 | `mdiClipboardText` | 0 | |
| Settings | 24 | `mdiCog` | 0.0001 | в макете сдвинута на 0.27 px влево |
| Side | 24 | `mdiPageLayoutSidebarRight` | 0.042 | |
| Fav | 24 | `mdiStarOutline` | 0 | |
| Sort | 24 | `mdiSortVariant` | 0 | |
| Ping | 24 | `mdiLightningBolt` | 0 | |
| Open | 32 | `mdiMenuUp` / `mdiMenuDown` | 0.019 / 0.0003 | в макете сдвинута на 1 px вверх |
| Tg | 24 | **нет в библиотеке** | — | логотип Telegram, взят из макета |
| Site | 24 | `mdiWeb` | 0.0001 | |
| UploadFile | 48 | `mdiFileUpload` | 0 | |
| Сlipboard | 48 | `mdiClipboardTextMultiple` | 0 | имя слоя в Figma начинается с кириллической `С` |
| Globe | 48 | `mdiEarth` | 0.0008 | |
| Smart | 48 | `mdiBrain` | 0.026 | |
| Copy | 24 | `mdiContentCopy` | 0 | |
| SubRouting | 36 | `mdiArrowDecision` | 0.0009 | |
| AdSettings | 36 | `mdiTune` | 0 | |
| Subscriptions | 36 | `mdiSignalVariant` | 0 | |
| Security | 36 | `mdiLock` | 0.0003 | |
| Network | 36 | `mdiSitemap` | 0 | |
| Import | 24 | `mdiTrayArrowDown` | 0.0002 | |
| Export | 24 | `mdiTrayArrowUp` | 0.0002 | |
| mdi:shield-outline | 32 | `mdiShieldOutline` | 0 | окно «Добавление серверов» |
| mdi:warning-circle-outline | 32 | `mdiAlertCircleOutline` | 0.0002 | окно «Уведомление» |
| mdi:close | 24 | `mdiClose` | 0 | крестик в окне |
| mdi:search | 24 | `mdiMagnify` | 0 | поле поиска на странице серверов |
| mdi:link-variant | 14 | `mdiLinkVariant` | 0.0016 | тег «Поддержка» в окне настроек подписки |
| mdi:floppy-disc | 24 | `mdiContentSave` | 0.0002 | сохранение журнала в шапке страницы логов |
| **нет в макете** | 24 | `mdiArrowLeft` | — | возврат со страницы пункта настроек, см. GAPS.md |

26 иконок совпадают ровно. У `Side`, `Open` и `Smart` отклонение не больше
**0.053 px** при полностью совпадающих габаритах — это погрешность разворота
дуг в безье на стороне Figma, а не другая иконка. При отрисовке в 24 px это
заметно меньше физического пикселя.

## Почему не `@mdi/react`

`@mdi/react` — тонкая обёртка над теми же путями, но она умеет только
`size` / `color` / `rotate` и не позволяет задать сдвиг внутри рамки. Двум
иконкам макета (`Settings`, `Open`) он нужен, поэтому в проекте своя обёртка
`components/kit/Icon.jsx` на 40 строк, а пути берутся из `@mdi/js` — это тот
же самый набор Templarian.

## Цвета

| состояние | цвет |
|---|---|
| Default | белый 50 % (`--rv-icon-default`) |
| Hover | белый 80 % (`--rv-icon-hover`) |
| Fav / Active | `#F2CC0D` (`--rv-warning`) |
| Site / Hover | `#007E3A` (`--rv-main-color`) |
| Tg / Hover | `#0088CC` (бренд Telegram, вне палитры) |
