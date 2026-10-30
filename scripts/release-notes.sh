#!/usr/bin/env bash
# Текст релиза на GitHub: дата данных, как запустить, изменения с прошлого тега.
# Использование: scripts/release-notes.sh v0.2.0 > notes.md
set -euo pipefail
cd "$(dirname "$0")/.."
tag="${1:?укажите тег, например v0.2.0}"
ref="$tag"
git rev-parse -q --verify "$ref^{commit}" >/dev/null || ref=HEAD # тега ещё нет — проверка перед выпуском

# время файла в снимке — когда scripts/seed скачал данные
data=$(unzip -l assets/data.zip pve_maps.json | awk '$4 == "pve_maps.json" {print $2}')

echo "Данные карт tarkov.dev от $(date -d "$data" +%d.%m.%Y)."
echo
echo "### Как запустить"
echo
echo '1. Скачайте `TarkovMap.exe` из файлов ниже (Windows 10/11, 64 бита).'
echo '2. Windows предупредит о неизвестной программе: «Подробнее» → «Выполнить в любом случае».'
echo '3. Папки игры и скриншотов программа найдёт сама; если нет — вкладка «Настройки».'
echo
echo 'Контрольная сумма файла — в `TarkovMap.exe.sha256`.'
echo
echo "### Изменения"
echo
if prev=$(git describe --tags --abbrev=0 "$ref^" 2>/dev/null); then
  git log --no-merges --format='- %s' "$prev..$ref"
else
  echo "Первый релиз."
fi
