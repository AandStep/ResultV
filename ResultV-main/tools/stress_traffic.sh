#!/usr/bin/env bash
# stress_traffic.sh — симулирует «много вкладок открыл-закрыл» через туннель.
# Запускать при включённом туннеле в приложении. Скрипт сам по себе ничего не
# знает про туннель — он просто шлёт HTTP-запросы, которые ОС роутит через TUN.
#
# Usage:
#   bash tools/stress_traffic.sh                # бесконечно, 50 параллельно
#   bash tools/stress_traffic.sh 100 200        # 100 параллельно, 200 итераций
#
# Что делает:
#   - Каждую итерацию параллельно стучится на много разных доменов и быстро
#     закрывает коннект (1с timeout, HEAD-запрос). Поведение похоже на то, как
#     браузер открывает страницу: dns → tcp → tls → small request → close.

PARALLEL="${1:-50}"
ITERATIONS="${2:-0}"   # 0 = бесконечно

# Большой список уникальных хостов — чем разнообразнее, тем больше уникальных
# host→outTag пар в sync.Map (если он у тебя ещё не починен), и тем больше
# DNS-резолвов на стороне sing-box.
HOSTS=(
  google.com youtube.com facebook.com twitter.com instagram.com
  reddit.com wikipedia.org amazon.com github.com gitlab.com
  stackoverflow.com medium.com dev.to news.ycombinator.com bbc.com
  cnn.com nytimes.com theguardian.com bloomberg.com reuters.com
  netflix.com twitch.tv spotify.com soundcloud.com vimeo.com
  dropbox.com slack.com discord.com telegram.org whatsapp.com
  microsoft.com apple.com adobe.com oracle.com ibm.com
  cloudflare.com fastly.com akamai.com aws.amazon.com cloud.google.com
  azure.microsoft.com digitalocean.com heroku.com vercel.com netlify.com
  yandex.ru mail.ru vk.com ok.ru rambler.ru
  habr.com pikabu.ru lenta.ru rbc.ru kommersant.ru
  wildberries.ru ozon.ru avito.ru drom.ru auto.ru
  livejournal.com dzen.ru rutube.ru kinopoisk.ru ivi.ru
  npmjs.com pypi.org rubygems.org crates.io packagist.org
  docker.com kubernetes.io golang.org python.org rust-lang.org
  mozilla.org chromium.org webkit.org nodejs.org deno.land
  bitbucket.org sourceforge.net codeberg.org
  jsdelivr.net unpkg.com cdnjs.com
  ietf.org w3.org whatwg.org tc39.es
  archlinux.org debian.org ubuntu.com fedoraproject.org alpinelinux.org
  duckduckgo.com bing.com baidu.com yahoo.com
  imgur.com flickr.com 500px.com behance.net dribbble.com
  figma.com sketch.com canva.com
  notion.so trello.com asana.com monday.com clickup.com
  zoom.us meet.google.com teams.microsoft.com
)

NUM=${#HOSTS[@]}
echo "Stress test: $PARALLEL parallel, $NUM unique hosts, $([[ $ITERATIONS == 0 ]] && echo 'infinite' || echo $ITERATIONS) iterations"
echo "Press Ctrl+C to stop. Watch the browser meanwhile."
echo

count=0
start_time=$(date +%s)

trap 'echo; echo "Stopped after $count iterations in $(($(date +%s) - start_time))s"; exit 0' INT

while true; do
  pids=()
  for ((i=0; i<PARALLEL; i++)); do
    host="${HOSTS[$RANDOM % NUM]}"
    # -s silent, -o /dev/null discard body, --max-time 2 hard timeout,
    # -I HEAD only (small), -k allow self-signed (some hosts redirect oddly)
    curl -sk -I --max-time 2 -o /dev/null "https://$host" &
    pids+=($!)
  done
  # ждём всё текущее окно перед следующим
  for pid in "${pids[@]}"; do
    wait "$pid" 2>/dev/null
  done

  count=$((count + 1))
  if (( count % 10 == 0 )); then
    elapsed=$(($(date +%s) - start_time))
    echo "[+${elapsed}s] iteration $count, ~$((count * PARALLEL)) total requests"
  fi

  if (( ITERATIONS > 0 && count >= ITERATIONS )); then
    break
  fi
done

echo "Done: $count iterations, ~$((count * PARALLEL)) requests in $(($(date +%s) - start_time))s"
