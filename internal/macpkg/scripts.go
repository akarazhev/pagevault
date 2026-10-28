package macpkg

import "fmt"

const targetHomePrefix = `#!/bin/sh
set -eu
if [ "$(/usr/bin/id -u)" -eq 0 ]; then
  echo "PageVault must be installed in CurrentUserHomeDirectory, not as root." >&2
  exit 1
fi
if [ "$#" -lt 3 ] || [ -z "$3" ]; then
  echo "PageVault installer target is missing." >&2
  exit 1
fi
pv_home=$3
if [ "$pv_home" = / ]; then
  pv_home=${HOME:?PageVault requires the current user home}
fi
case "$pv_home" in /*) ;; *) echo "PageVault requires an absolute user-home target." >&2; exit 1;; esac
`

func postinstall(browser string) string {
	return targetHomePrefix + fmt.Sprintf(`pv_binary="$pv_home/Library/Application Support/PageVault/Installer/pagevault-host"
if [ ! -x "$pv_binary" ]; then
  echo "PageVault payload not found under the selected user home: $pv_home" >&2
  exit 1
fi
exec "$pv_binary" install --browser %s --home "$pv_home" --preserve-registered-db
`, browser)
}

func preinstall() string {
	return targetHomePrefix + `exec "$(dirname "$0")/pagevault-host" package-preflight --home "$pv_home"
`
}
