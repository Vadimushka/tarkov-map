#!/usr/bin/env bash
# Сборка TarkovMap.exe под Windows из WSL.
set -euo pipefail
cd "$(dirname "$0")/.."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags desktop,production \
  -ldflags "-s -w -H windowsgui" -o build/bin/TarkovMap.exe .
