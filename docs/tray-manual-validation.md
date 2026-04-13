# Tray Manual Validation

## Window lifecycle
- Start app, close main window using `X`: app must stay running in tray.
- Click `Показать окно` in tray 5-10 times after repeated close/minimize cycles: window must restore every time.
- Click `Выход` in tray: app must terminate completely.

## Connect/disconnect from tray
- Open tray menu, select a server, click `Подключиться к выбранному`.
- Confirm status changes to connected (green status line) and selected server is marked as connected.
- Click `Отключить`: status returns to disconnected and server markers update.

## Grouped server menu
- Ensure servers are grouped by provider first, then by country.
- Verify country fallback for unknown country and provider fallback to `Мои прокси`.
- Verify long country groups show `... еще N серверов (полный список в окне)` when limit is exceeded.

## Ping updates
- Keep app running for at least 40 seconds.
- Confirm server labels update with ping values (`NNms`) without app restart.
- Disconnect/connect and ensure ping labels continue updating.
