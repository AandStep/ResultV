# ResultProxy

**ResultProxy** — это кроссплатформенное прокси-приложение, созданное с использованием Electron, React и Vite.

🌐 **Официальный сайт проекта:** [https://result-proxy.ru/](https://result-proxy.ru/)

## ✨ Особенности

- Встроенный блокировщик рекламы на базе `@ghostery/adblocker`
- Поддержка HTTP и SOCKS прокси
- Современный пользовательский интерфейс, созданный на React и Tailwind CSS
- Кроссплатформенность (Windows, macOS, Linux)
- Многоязычный интерфейс (интеграция с `i18next`)

## 🚀 Установка и запуск (для разработчиков)

### Требования

- Node.js (рекомендуется LTS версия)
- npm

### Шаги

1. Клонируйте репозиторий и перейдите в папку проекта:
   ```bash
   git clone <ссылка_на_ваш_репозиторий>
   cd ResultProxy
   ```

2. Установите зависимости:
   ```bash
   npm install --legacy-peer-deps
   ```

3. Запустите проект в режиме разработчика:
   ```bash
   npm run dev
   ```
   *Эта команда одновременно запустит процесс Vite для React и основное окно приложения Electron.*

## 📦 Сборка приложения

Для того чтобы собрать установочные файлы `.exe`, `.AppImage` и другие форматы, используйте следующие команды:

- **Для Windows:**
  ```bash
  npm run package
  ```

- **Для Linux:**
  ```bash
  npm run package:linux
  ```

## 🛠 Технологический стек

- **Кроссплатформенная оболочка:** [Electron](https://www.electronjs.org/)
- **Frontend:** [React](https://reactjs.org/), [Vite](https://vitejs.dev/), [Tailwind CSS](https://tailwindcss.com/)
- **Проксирование и сеть:** `proxy-chain`, `socks`, `express`
- **Блокировка рекламы:** [@ghostery/adblocker](https://github.com/ghostery/adblocker)

---

**Больше информации и загрузка приложения:** [https://result-proxy.ru/](https://result-proxy.ru/)
