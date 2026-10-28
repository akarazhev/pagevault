#!/usr/bin/env bash
set -euo pipefail
pagevault_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
case "$(uname -s)" in
  Linux) pagevault_os=linux ;;
  Darwin) pagevault_os=darwin ;;
  *) echo "Используйте install-windows.cmd для Windows." >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) pagevault_arch=amd64 ;;
  arm64|aarch64) pagevault_arch=arm64 ;;
  *) echo "Эта архитектура не входит в готовый комплект." >&2; exit 1 ;;
esac
pagevault_binary="$pagevault_root/bin/$pagevault_os-$pagevault_arch/pagevault-host"
if [[ ! -f "$pagevault_binary" ]]; then
  echo "Не найден $pagevault_binary. Распакуйте полный выпуск или соберите его: go run ./cmd/release" >&2
  exit 1
fi
chmod u+x "$pagevault_binary"
pagevault_browser="${1:-chrome}"
if [[ $# -gt 0 ]]; then shift; fi
"$pagevault_binary" install --browser "$pagevault_browser" --extension-manifest "$pagevault_root/extension/manifest.json" "$@"
echo "Подробности подключения расширения и обновления: README.md"
