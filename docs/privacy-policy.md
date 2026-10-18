# Политика конфиденциальности ResultV — текст для сайта

Это исходник для страницы `https://result-proxy.ru/privacy.html`. Ссылка на неё
стоит в приложении (Настройки → Политика конфиденциальности) и указывается в
Play Console. Страница одна на оба языка: сначала русский текст, затем
английский. Ревьюер Google читает английский, а адрес страницы у приложения
один.

## Перед публикацией

Каждое утверждение ниже сверено с кодом (ветка `android` и `C:\ResultVPC`,
2026-10-13), кроме трёх пунктов: их знаете только вы.

1. **`[Оператор]`** — ФИО или название разработчика, ровно как в профиле Play
   Console. 152-ФЗ требует, чтобы оператор был назван, а Google сверяет имя
   с аккаунтом разработчика.
2. **Журналы `countryAPI`.** IP сервера уходит в строке запроса
   (`api.php?ip=…`), поэтому попадает в access-лог веб-сервера. Текст ниже
   обещает для таких запросов тот же срок хранения, что и для остальных
   журналов (до 12 месяцев). Если можно, отключите запись query string для
   `/countryAPI/` или сократите срок, и тогда поправьте абзац «Определение
   страны».
3. **Дата редакции** — поставьте дату фактической публикации.

Если код начнёт отправлять что-то новое, политику нужно обновить **до**
выпуска версии: Play сверяет анкету Data safety с этим текстом.

---

# Политика конфиденциальности

Редакция от 13 октября 2026 г.

Эта политика описывает, какие данные обрабатывают приложение ResultV (Windows,
Android и другие платформы) и сайт result-proxy.ru, зачем и куда они
передаются.

Оператор: [Оператор]. Контакты — в разделе 10.

## 1. Коротко

- В приложении нет аккаунтов, рекламы, аналитики и сборщиков сбоев.
- Мы не видим ваш трафик. Он идёт на сервер, который выбрали вы, и
  обрабатывается владельцем этого сервера.
- Настройки, серверы, подписки и журналы хранятся на вашем устройстве.
- Несколько технических запросов всё же уходят с устройства. Они перечислены
  в разделе 3: куда, что и зачем.

## 2. Что хранится только на устройстве

Конфигурации и ключи серверов, ссылки на подписки, профили и правила
маршрутизации, настройки, журнал работы приложения и счётчики переданного
трафика. Эти данные не отправляются ни нам, ни третьим лицам. Удаляются они
вместе с приложением или его данными.

Журнал работы показывает события приложения и соединения: какой сайт через
какой сервер открылся. Он хранится только в памяти (последние 500 записей) и
стирается, когда приложение закрывается. На диск он не записывается.
Устройство журнал покидает только тогда, когда вы сами копируете его и
отправляете, например в поддержку. Учтите, что в нём могут быть адреса
серверов, посещённые сайты и ссылки на подписки с токенами.

## 3. Что приложение отправляет по сети

**Ваш трафик.** Когда VPN включён, трафик идёт на сервер, который вы добавили
сами или получили от своего провайдера. Мы этот сервер не контролируем. Что
он записывает, определяет его владелец и его политика.

**Запрос подписки.** Когда приложение загружает или обновляет подписку, запрос
уходит на адрес, который вы ввели, то есть провайдеру. Вместе с ним
передаются:

- идентификатор устройства (HWID) — хеш системного идентификатора
  (на Android — `ANDROID_ID`, на Windows и Linux — идентификатора
  компьютера). Провайдеры используют его, чтобы ограничивать число устройств
  на одну подписку. **Передачу HWID можно отключить**: Настройки → Подписки →
  «Передавать HWID»;
- название и версия операционной системы и модель устройства. Панель
  провайдера показывает их в вашем списке устройств. Эти данные передаются
  всегда, когда загружается подписка;
- строка User-Agent (по умолчанию `ResultV/<версия>`, её можно изменить в
  настройках).

Получатель этих данных — ваш провайдер, а не мы. Их хранение регулирует
политика провайдера.

**Определение страны.** Чтобы показать флаги стран у серверов, приложение
отправляет IP-адреса ваших серверов на наш сервис
`result-proxy.ru/countryAPI`. Если у сервера указано доменное имя, оно сначала
превращается в IP-адрес на устройстве. Если в режиме Smart регион не выбран
вручную, приложение один раз запрашивает страну по адресу, с которого пришёл
запрос, то есть по вашему текущему IP. Сервис возвращает код страны.
Запросы попадают в журналы веб-сервера (раздел 6). Для других целей мы их не
используем и не связываем с какими-либо другими данными.

**Списки маршрутизации.** В режиме Smart приложение скачивает списки
адресов с `raw.githubusercontent.com` и `cdn.jsdelivr.net`. Профили и списки
маршрутизации, которые добавили вы или ваш провайдер, скачиваются с
указанных там адресов. Ваших данных эти запросы не содержат. Как и любой
сайт, получатель видит IP-адрес, с которого пришёл запрос.

**Проверка соединения и пинг.** Приложение проверяет, работает ли интернет,
запросами к `www.gstatic.com/generate_204` (Google). Версия для Windows также
использует `cp.cloudflare.com` и `www.google.com`, а при проверке скорости —
`speed.cloudflare.com`. Для замера задержки приложение подключается к вашим
серверам. Ваших данных эти запросы не содержат.

**Обновления (Windows).** Версия для Windows проверяет наличие обновлений,
загружая файл `update.json` с GitHub. Версия из Google Play обновляется
только через Google Play.

**Сканер QR-кодов (Android).** Сканер встроен в Google Play services
(Google Code Scanner). Изображение с камеры обрабатывает Google Play services
на устройстве, само приложение доступа к камере не получает. Google собирает
технические сведения о работе сканера: модель и версию ОС, версию
приложения, показатели производительности, коды ошибок и идентификатор
установки. Это происходит по правилам Google:
https://developers.google.com/ml-kit/android-data-disclosure и
https://policies.google.com/privacy.

**Блокировка рекламы (версии с сайта).** Блокировки рекламы нет в версии из
Google Play. В версиях, которые скачиваются с сайта, она загружает списки
фильтров с серверов их авторов (AdGuard, EasyList и других) и с GitHub. Для
фильтрации в браузере на устройстве создаётся собственный корневой
сертификат. Его закрытый ключ не покидает устройство, а HTTPS-трафик
расшифровывается и фильтруется только локально и никуда не передаётся.

## 4. Сайт result-proxy.ru

Веб-сервер записывает стандартные технические данные: IP-адрес, тип
браузера, запрошенные страницы и время запроса. На главной странице работают
Яндекс Метрика и Google Analytics, которые устанавливают собственные cookie.
Их можно заблокировать в браузере, сайт продолжит работать.

## 5. Зачем мы используем данные

Чтобы приложение работало (определение страны серверов, обновления), чтобы
сайт был доступен и защищён от атак, чтобы находить ошибки и понимать общую
посещаемость сайта. Мы не продаём и не сдаём данные, не передаём их для
рекламы и не составляем профили пользователей.

## 6. Хранение и защита

Журналы веб-сервера, в том числе запросы к сервису определения страны,
хранятся до 12 месяцев, затем удаляются. Срок хранения данных аналитики
определяют Яндекс и Google. Доступ к журналам есть только у разработчиков
проекта. Все запросы приложения к нашим серверам и к провайдерам подписок
идут по HTTPS.

## 7. Ссылки на партнёров

На сайте есть ссылки на сторонние сервисы, которые продают доступ к серверам.
У них свои политики конфиденциальности, за их обработку данных мы не
отвечаем.

## 8. Дети

Приложение и сайт не предназначены для детей младше 13 лет, и мы сознательно
не собираем их данные.

## 9. Ваши права

По 152-ФЗ, а для жителей ЕС — по GDPR вы можете запросить доступ к своим
данным, их исправление или удаление, а также отозвать согласие. Большая часть
данных хранится только на вашем устройстве: чтобы их удалить, удалите
приложение или очистите его данные. Чтобы удалить записи о ваших запросах из
наших журналов, напишите нам и прямо укажите, что запрос касается удаления.
Мы ответим в течение 30 дней. Данные, которые получил ваш провайдер подписки,
удаляет он сам.

## 10. Контакты

- E-mail: resultpoint.dev@gmail.com
- Чат поддержки в Telegram: https://t.me/+nIOT_k5HJmo4YzUy
- Канал: https://t.me/resultvpn

## 11. Изменения

Новая редакция публикуется на этой странице с новой датой. О существенных
изменениях мы сообщаем в Telegram-канале.

---

# Privacy Policy

Effective: October 13, 2026

This policy explains what data the ResultV app (Windows, Android and other
platforms) and the result-proxy.ru website process, why, and where it goes.

Operator: [Operator]. Contacts are in section 10.

## 1. Summary

- The app has no accounts, no ads, no analytics and no crash reporting.
- We do not see your traffic. It goes to the server you chose and is handled
  by that server's owner.
- Your settings, servers, subscriptions and logs stay on your device.
- A few technical requests do leave the device. Section 3 lists each one:
  where it goes, what it contains and why.

## 2. Data kept only on your device

Server configurations and keys, subscription links, routing profiles and
rules, settings, the app log and traffic counters. None of this is sent to us
or to third parties. It is deleted when you uninstall the app or clear its
data.

The app log shows app events and connections: which site was opened through
which server. It is kept only in memory (the last 500 entries) and erased when
the app closes. It is never written to disk. The log leaves the device only if
you copy it and send it yourself, for example to support. Note that it may
contain server addresses, visited sites and subscription links with access
tokens.

## 3. What the app sends over the network

**Your traffic.** While the VPN is on, your traffic goes to a server that you
added yourself or received from your provider. We do not control that server.
What it records is up to its owner and their policy.

**Subscription requests.** When the app downloads or refreshes a
subscription, the request goes to the address you entered, which is your
provider. It includes:

- a device identifier (HWID): a hash of the system identifier (`ANDROID_ID`
  on Android, the machine identifier on Windows and Linux). Providers use it
  to limit the number of devices per subscription. **You can turn HWID off**
  under Settings → Subscriptions → "Send HWID";
- the operating system name and version and the device model. Your
  provider's panel shows these in your device list. They are sent with every
  subscription request;
- a User-Agent string (`ResultV/<version>` by default; you can change it in
  settings).

This data is received by your provider, not by us, and is governed by your
provider's policy.

**Country lookup.** To show country flags next to your servers, the app sends
the IP addresses of your servers to our service at `result-proxy.ru/countryAPI`.
If a server is given as a domain name, it is resolved to an IP address on the
device first. In Smart mode, if you have not picked a region manually, the app
asks once for the country of the address the request comes from, which is
your current IP. The service returns a country code. These requests are
recorded in our web server logs (section 6). We use them for nothing else and
do not link them to any other data.

**Routing lists.** In Smart mode the app downloads address lists from
`raw.githubusercontent.com` and `cdn.jsdelivr.net`. Routing profiles and lists
that you or your provider add are downloaded from the addresses they
specify. These requests contain none of your data. Like any website, the
receiving server sees the IP address the request comes from.

**Connectivity checks and ping.** The app checks whether the internet works
by requesting `www.gstatic.com/generate_204` (Google). The Windows version
also uses `cp.cloudflare.com` and `www.google.com`, plus
`speed.cloudflare.com` for its speed test. To measure latency the app
connects to your servers. These requests contain none of your data.

**Updates (Windows).** The Windows version checks for updates by downloading
an `update.json` file from GitHub. The Google Play version is updated only
through Google Play.

**QR code scanner (Android).** The scanner is part of Google Play services
(Google Code Scanner). Google Play services processes the camera image on the
device, and the app itself gets no camera access. Google collects technical
information about how the scanner works: the device model and OS version,
the app version, performance metrics, error codes and an installation
identifier. This happens under Google's terms:
https://developers.google.com/ml-kit/android-data-disclosure and
https://policies.google.com/privacy.

**Ad blocking (website versions only).** The Google Play version has no ad
blocking. In the versions downloaded from our website, ad blocking downloads
filter lists from their authors' servers (AdGuard, EasyList and others) and
from GitHub. For in-browser filtering, the device creates its own root
certificate. Its private key never leaves the device, and HTTPS traffic is
decrypted and filtered only locally and is not sent anywhere.

## 4. The result-proxy.ru website

The web server records standard technical data: IP address, browser type,
pages requested and time of request. The home page uses Yandex Metrica and
Google Analytics, which set their own cookies. You can block them in your
browser, and the site keeps working.

## 5. Why we use data

To make the app work (server country lookup, updates), to keep the website
available and protected from attacks, to find errors and to understand
overall website traffic. We do not sell or rent data, do not share it for
advertising and do not build user profiles.

## 6. Retention and security

Web server logs, including country lookup requests, are kept for up to 12
months and then deleted. Analytics retention is set by Yandex and Google.
Only the project's developers can access the logs. All app requests to our
servers and to subscription providers use HTTPS.

## 7. Partner links

The website links to third-party services that sell server access. They have
their own privacy policies, and we are not responsible for how they handle
data.

## 8. Children

The app and the website are not directed at children under 13, and we do not
knowingly collect their data.

## 9. Your rights

Under Russian Federal Law No. 152-FZ, and the GDPR for EU residents, you can
request access to, correction or deletion of your data, and withdraw consent.
Most of your data exists only on your device: to delete it, uninstall the app
or clear its data. To have records of your requests removed from our logs,
write to us and say clearly that the request is about deletion. We reply
within 30 days. Data received by your subscription provider is deleted by
that provider.

## 10. Contacts

- Email: resultpoint.dev@gmail.com
- Telegram support chat: https://t.me/+nIOT_k5HJmo4YzUy
- Channel: https://t.me/resultvpn

## 11. Changes

Updated versions are posted on this page with a new date. We announce
significant changes in our Telegram channel.
