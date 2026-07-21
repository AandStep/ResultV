# Переносимое окружение Android-сборки — дизайн

Дата: 2026-07-21

## Проблема

Проект нужно уметь целиком перенести на другой ПК так, чтобы он сразу
собирался, при этом:

- тяжёлые артефакты не должны «висеть» в git (и не пролезут — лимит 100 МБ);
- секреты не должны утекать в открытом виде.

Тяжёлые каталоги на текущей машине:

- `android/app/build/` — ~639 МБ (вывод Gradle, уже в `.gitignore`);
- `android/libs/libbox.aar` — ~134 МБ (gomobile-сборка, уже в `.gitignore`);
- `node_modules`, `ResultV-main` — уже игнорируются.

Все они **регенерируются** штатными скриптами, поэтому переносить их не нужно —
на новой машине они привязаны к путям и тулчейну этой машины.

## Решение: бутстрап + зашифрованный бандл секретов

Переносим только код (через git) и секреты (через зашифрованный файл в git).
Всё остальное восстанавливает bootstrap-скрипт на целевой машине.

### Что чем переносится

| Что | Способ | Почему |
|---|---|---|
| Код | `git clone` | как обычно |
| `.env` (`SUBSCRIPTION_ENCRYPT_KEY`) + keystore (если появится) | зашифрованный `secrets.enc` в git | безопасно; расшифровка по паролю |
| `android/local.properties` | генерируется бутстрапом из найденного SDK | путь к SDK на каждой машине свой |
| `libbox.aar`, `app/build/`, `node_modules` | пересборка штатными скриптами | привязаны к машине |
| Тулчейн (Go / JDK / SDK / NDK / gomobile) | бутстрап проверяет; gomobile доустанавливает | ставится один раз на машине |

### Компоненты

Все скрипты — bash, работают в git-bash (Windows) и в macOS/Linux.
ОС определяется через `uname` / `$OSTYPE`.

1. **`scripts/pack-secrets.sh`**
   - Назначение: упаковать секретные файлы в один зашифрованный файл.
   - Собирает существующие из списка: `.env`, `android/keystore.properties`,
     `android/*.jks`, `android/*.keystore`.
   - `tar` → `openssl enc -aes-256-cbc -pbkdf2 -salt` с паролем (промпт дважды
     на подтверждение) → `secrets.enc` в корне.
   - Зависит от: `openssl`, `tar` (оба есть в git-for-windows и на *nix).
   - Выход: `secrets.enc` (машинно-независимый, коммитится в git).

2. **`scripts/unpack-secrets.sh`**
   - Назначение: восстановить секреты из `secrets.enc`.
   - Запрашивает пароль, `openssl enc -d` → `tar -x` в корень репозитория.
   - Идемпотентен; при отсутствии `secrets.enc` завершается с понятным сообщением.
   - Не перезаписывает существующие файлы без флага `--force` (чтобы не затереть
     локальные секреты по ошибке).

3. **`scripts/bootstrap.sh`** — точка входа на новой машине.
   - Детект ОС (Windows / macOS / Linux).
   - Проверки тулчейна, каждая с чёткой инструкцией при отсутствии:
     - `go` в PATH;
     - JDK 17+ (`JAVA_HOME` или системный `java`/`javac`);
     - Android SDK (`ANDROID_HOME`, либо `android/local.properties`, либо
       типичные пути: `%LOCALAPPDATA%\Android\Sdk`, `~/Library/Android/sdk`,
       `~/Android/Sdk`);
     - NDK под `$ANDROID_HOME/ndk`.
   - `gomobile`: если нет в PATH — `go install github.com/sagernet/gomobile/cmd/gomobile@v0.1.12`,
     затем `gomobile init` (однократно).
   - Генерирует `android/local.properties` из найденного SDK (с корректным
     экранированием пути под Windows), если файла нет.
   - Если есть `secrets.enc` и нет `.env` — зовёт `unpack-secrets.sh`.
   - В конце печатает следующий шаг: `bash build-android.sh`.
   - Не ставит SDK/NDK/JDK автоматически (гигабайты; ставятся штатными
     установщиками) — только проверяет и даёт ссылки.

4. **`.env.example`** и **`android/keystore.properties.example`**
   - Шаблоны в git, показывают какие ключи нужны, без значений.

### Изменения в git-конфигурации

- `.gitignore`: снять/уточнить игнор так, чтобы `secrets.enc`, `.env.example`,
  `keystore.properties.example` **коммитились**, а сами `.env`/`keystore.properties`
  оставались игнорируемыми (уже так). Убедиться, что `!.env.example` присутствует.

### Порядок использования

Упаковать секреты (на текущей машине, один раз или после смены ключа):

```
bash scripts/pack-secrets.sh      # → secrets.enc, git add && commit
```

На новом ПК:

```
git clone <repo> && cd ResultV
bash scripts/bootstrap.sh         # тулчейн + local.properties + расшифровка
bash build-android.sh             # пересборка AAR + APK
```

## Границы (YAGNI)

- Не автоустанавливаем SDK/NDK/JDK.
- Не копируем build-артефакты и `node_modules`.
- Keystore-подписи сейчас нет — скрипты работают с ней опционально (если файлы
  появятся, `pack-secrets` их подхватит).

## Критерии готовности

- `bootstrap.sh` на «чистой» машине доводит до состояния, из которого
  `build-android.sh` собирает APK.
- `pack-secrets.sh` → `unpack-secrets.sh` — round-trip восстанавливает `.env`
  байт-в-байт.
- Скрипты не падают в git-bash на Windows и в bash на *nix.
- Никаких секретов в открытом виде в git; только `secrets.enc`.
