#!/usr/bin/env bash
# unpack-secrets.sh — восстановить секреты из secrets.enc. Пароль тот же, что в pack-secrets.
# Неинтерактивно: export SECRETS_PASSWORD=... перед запуском.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}"

FORCE=0
[[ "${1:-}" == "--force" ]] && FORCE=1

if [[ ! -f secrets.enc ]]; then
    echo "secrets.enc не найден в ${REPO_ROOT}. Нечего распаковывать." >&2
    exit 1
fi

# Без --force не затираем НИ ОДИН уже существующий секрет.
if [[ $FORCE -eq 0 ]]; then
    shopt -s nullglob
    EXISTING=(.env android/keystore.properties android/*.jks android/*.keystore)
    shopt -u nullglob
    for f in "${EXISTING[@]}"; do
        if [[ -e "$f" ]]; then
            echo "$f уже существует — пропускаю распаковку. Запусти с --force, чтобы перезаписать." >&2
            exit 0
        fi
    done
fi

PASS_ARGS=()
if [[ -n "${SECRETS_PASSWORD:-}" ]]; then
    PASS_ARGS=(-pass env:SECRETS_PASSWORD)
else
    echo "Введи пароль от secrets.enc:"
fi

openssl enc -d -aes-256-cbc -pbkdf2 "${PASS_ARGS[@]}" -in secrets.enc | tar -xf -
echo "Секреты восстановлены."
