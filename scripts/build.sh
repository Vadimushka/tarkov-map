#!/usr/bin/env bash
# Сборка TarkovMap.exe под Windows из WSL или Linux (так же собирает GitHub Actions).
# VERSION — версия для «О программе»; без неё — git describe или dev.
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags desktop,production \
  -ldflags "-s -w -H windowsgui -X main.version=$VERSION" -o build/bin/TarkovMap.exe .
echo "build/bin/TarkovMap.exe $VERSION"
