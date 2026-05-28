#!/usr/bin/env bash
# pprof_series.sh — снимает профили pprof через равные интервалы.
# Идея: ловим накопительную деградацию. Сравним t=0 и t=1h:
#  - count горутин растёт? → leak
#  - heap растёт? → leak
#  - CPU топ функции меняется? → hot path смещается
#
# Запускать ОДНОВРЕМЕННО со стресс-тестом.
#
# Usage:
#   bash tools/pprof_series.sh                 # каждые 15 мин, бесконечно
#   bash tools/pprof_series.sh 600 8           # каждые 10 мин, 8 раз = 80 мин

INTERVAL="${1:-900}"   # секунд между снапшотами; 900 = 15 мин
COUNT="${2:-0}"        # 0 = бесконечно

OUT_DIR="pprof_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$OUT_DIR"
echo "Saving profiles to ./$OUT_DIR/"
echo

i=0
trap 'echo; echo "Stopped after $i snapshots"; exit 0' INT

while true; do
  ts=$(date +%H%M%S)
  echo "[$i] snapshot at $ts..."

  # горутины и хип — быстро, миллисекунды
  curl -s "http://127.0.0.1:6060/debug/pprof/goroutine?debug=2" -o "$OUT_DIR/goroutine_${i}_${ts}.txt"
  curl -s "http://127.0.0.1:6060/debug/pprof/heap" -o "$OUT_DIR/heap_${i}_${ts}.pprof"

  # быстрая сводка прямо в консоль
  goroutines=$(grep -c "^goroutine " "$OUT_DIR/goroutine_${i}_${ts}.txt")
  heap_size=$(stat -c%s "$OUT_DIR/heap_${i}_${ts}.pprof" 2>/dev/null || stat -f%z "$OUT_DIR/heap_${i}_${ts}.pprof")
  echo "    goroutines=$goroutines  heap_dump=${heap_size}B"

  # CPU — 30 сек блокирующий вызов, запускаем в фоне чтобы не задерживать таймер
  (curl -s "http://127.0.0.1:6060/debug/pprof/profile?seconds=30" -o "$OUT_DIR/cpu_${i}_${ts}.pprof") &

  i=$((i + 1))
  if (( COUNT > 0 && i >= COUNT )); then
    wait
    break
  fi

  sleep "$INTERVAL"
done

echo
echo "Series done. $i snapshots saved in ./$OUT_DIR/"
echo "Compare with: go tool pprof -base ./$OUT_DIR/heap_0_*.pprof ./$OUT_DIR/heap_$((i-1))_*.pprof"
