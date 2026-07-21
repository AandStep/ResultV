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

# Без --force не затираем уже существующие секреты.
if [[ $FORCE -eq 0 && -f .env ]]; then
    echo ".env уже существует — пропускаю распаковку. Запусти с --force, чтобы перезаписать." >&2
    exit 0
fi

PASS_ARGS=()
if [[ -n "${SECRETS_PASSWORD:-}" ]]; then
    PASS_ARGS=(-pass env:SECRETS_PASSWORD)
else
    echo "Введи пароль от secrets.enc:"
fi

openssl enc -d -aes-256-cbc -pbkdf2 "${PASS_ARGS[@]}" -in secrets.enc | tar -xf -
echo "Секреты восстановлены."
