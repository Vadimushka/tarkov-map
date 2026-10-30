#!/usr/bin/env bash
# Собирает тесты пакета под Windows и запускает их через interop WSL.
# Использование: scripts/wintest.sh internal/shots [аргументы go test]
set -euo pipefail
cd "$(dirname "$0")/.."
pkg="${1:?укажите каталог пакета, например internal/shots}"
shift
out="build/test/$(basename "$pkg").test.exe"
mkdir -p build/test
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o "$out" "./$pkg"
"./$out" -test.v "$@"
