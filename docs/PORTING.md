# Перенос проекта на другой ПК

## На текущей машине (один раз / после смены ключа)

```bash
bash scripts/pack-secrets.sh          # -> secrets.enc (спросит пароль)
git add secrets.enc && git commit -m "chore: update secrets bundle"
git push
```

Неинтерактивно (CI/скрипты): `SECRETS_PASSWORD=... bash scripts/pack-secrets.sh`.

## На новом ПК

Требуется предустановленное: Go, JDK 17+, Android Studio (SDK + NDK).

```bash
git clone <repo> && cd ResultV
bash scripts/bootstrap.sh             # проверит тулчейн, поставит gomobile,
                                      # создаст local.properties, спросит пароль
                                      # и распакует секреты
bash build-android.sh                 # пересоберёт AAR + APK
```

## Что НЕ переносится (регенерируется)

- `android/libs/libbox.aar` — собирается `scripts/build-android-aar.sh`
- `android/app/build/`, `node_modules` — билд-вывод
- `android/local.properties` — генерится bootstrap под путь SDK машины

## Секреты

`.env` и keystore едят только внутри зашифрованного `secrets.enc`.
Пароль передавай отдельным каналом (не в git).
