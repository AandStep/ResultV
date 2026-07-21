#!/usr/bin/env bash
# pack-secrets.sh — собрать секреты в зашифрованный secrets.enc (коммитится в git).
# Расшифровка: scripts/unpack-secrets.sh
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}"

CANDIDATES=(.env android/keystore.properties)
# globs, которые могут не совпасть — собираем аккуратно
shopt -s nullglob
CANDIDATES+=(android/*.jks android/*.keystore)
shopt -u nullglob

FILES=()
for f in "${CANDIDATES[@]}"; do
    [[ -f "$f" ]] && FILES+=("$f")
done

if [[ ${#FILES[@]} -eq 0 ]]; then
    echo "Нет секретных файлов для упаковки (искал: .env, android/keystore.properties, android/*.jks, android/*.keystore)." >&2
    exit 1
fi

echo "Упаковываю в secrets.enc:"
printf '  - %s\n' "${FILES[@]}"
echo "Задай пароль (запомни — он нужен на другом ПК):"

PASS_ARGS=()
if [[ -n "${SECRETS_PASSWORD:-}" ]]; then
    PASS_ARGS=(-pass env:SECRETS_PASSWORD)
fi

tar -cf - "${FILES[@]}" | openssl enc -aes-256-cbc -pbkdf2 -salt "${PASS_ARGS[@]}" -out secrets.enc

if [[ -n "${SECRETS_PASSWORD:-}" ]]; then
    if ! openssl enc -d -aes-256-cbc -pbkdf2 "${PASS_ARGS[@]}" -in secrets.enc | tar -tf - >/dev/null 2>&1; then
        echo "ERROR: проверка secrets.enc не прошла — бандл битый, не коммить." >&2
        exit 1
    fi
fi

echo "Готово: secrets.enc"
echo "Теперь: git add secrets.enc && git commit -m \"chore: update secrets bundle\""
