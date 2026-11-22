#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
repo=github.com/neokapi/neokapi
modules='{{with .Module}}{{if not .Main}}{{.Path}}{{end}}{{end}}'

for module in "$@"; do
  case "$module" in
    .|host|bowrain/core) allowed="" ;;
    cli|apps/kapi-desktop) allowed="host" ;;
    kapi|bowrain/plugin) allowed="host cli" ;;
    bowrain) allowed="host bowrain/core" ;;
    *) echo "No import policy for module $module" >&2; exit 1 ;;
  esac

  GOWORK=off "${GO:-go}" -C "$root/$module" list -mod=readonly -deps -f "$modules" ./... |
    sort -u | while IFS= read -r dependency; do
      case "$dependency" in
        "$repo"/*)
          [[ " $allowed " == *" ${dependency#"$repo"/} "* ]] && continue
          ;;
        github.com/spf13/cobra)
          case "$module" in .|host|apps/kapi-desktop|bowrain/core) ;; *) continue ;; esac
          ;;
        github.com/wailsapp/wails/*|github.com/labstack/echo/*|github.com/keycloak/*|github.com/neokapi/platform)
          [[ "$module" == kapi ]] || continue
          ;;
        *) continue ;;
      esac
      echo "$module imports $dependency" >&2
      exit 1
    done
done
