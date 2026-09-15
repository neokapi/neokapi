#!/usr/bin/env bash
#
# Guard: only a final kapi CLI release becomes the repository's "latest".
#
# One repository publishes the kapi CLI, Bowrain and every plugin, and GitHub
# answers https://github.com/neokapi/neokapi/releases/latest with the one release
# marked latest. A publishing step that passes no `make_latest` takes the API
# default, which marks a newly published full release latest: check-v0.1.0 took
# the slot that way. `kapi update` sends a user without a managed install to
# that URL.
#
# So every publishing step says what it wants:
#   - release.yml passes the `latest` output of scripts/release-channel.sh,
#     which is "true" only for a vX.Y.Z tag. The script's self-test covers the
#     tag shapes.
#   - every release-*.yml (Bowrain and each plugin) passes `make_latest: false`.
#
# The rule is per publishing step, not per workflow: release.yml and
# release-bowrain.yml each publish three times, and one step that says nothing
# is enough to hand the slot to whichever release publishes last.
set -euo pipefail

cd "$(dirname "$0")/.."

# make_latest_values prints the `make_latest` value of every step in a workflow
# that uses action-gh-release, one line each, or "<none>" for such a step that
# sets none. A step starts at a list item six columns in; a job header ends it.
make_latest_values() {
  awk '
    function flush() {
      if (publishes) print (value == "" ? "<none>" : value)
      publishes = 0; value = ""
    }
    /^  [A-Za-z0-9_-]+:[[:space:]]*$/ { flush(); next }
    /^      - / { flush() }
    /uses:[[:space:]]*softprops\/action-gh-release/ { publishes = 1 }
    /^[[:space:]]+make_latest:/ {
      value = $0
      sub(/^[[:space:]]+make_latest:[[:space:]]*/, "", value)
      sub(/[[:space:]]+#.*$/, "", value)
    }
    END { flush() }
  ' "$1"
}

count_lines() {
  if [ -z "$1" ]; then echo 0; else printf '%s\n' "$1" | wc -l | tr -d ' '; fi
}

fail=0

# The kapi CLI's own release: latest on a final tag, and on nothing else.
wf=.github/workflows/release.yml
values=$(make_latest_values "$wf")
total=$(count_lines "$values")
passing=$(printf '%s\n' "$values" |
  grep -c -E '^\$\{\{ (steps\.channel|needs\.package-cli)\.outputs\.latest \}\}$' || true)
if [ "$total" -eq 0 ]; then
  echo "check-plugin-release-latest: $wf publishes no release; this guard needs updating" >&2
  fail=1
elif [ "$passing" -lt "$total" ]; then
  printf 'check-plugin-release-latest: %s publishes %s release(s), %s pass the latest output of scripts/release-channel.sh\n' \
    "$wf" "$total" "$passing" >&2
  printf '%s\n' "$values" | sed 's/^/    make_latest: /' >&2
  fail=1
fi

# Bowrain and every plugin: never latest.
for wf in .github/workflows/release-*.yml; do
  values=$(make_latest_values "$wf")
  total=$(count_lines "$values")
  [ "$total" -eq 0 ] && continue
  guarded=$(printf '%s\n' "$values" | grep -c -x 'false' || true)
  if [ "$guarded" -lt "$total" ]; then
    printf 'check-plugin-release-latest: %s publishes %s release(s), %s pass make_latest: false\n' \
      "$wf" "$total" "$guarded" >&2
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then
  cat >&2 <<'MSG'

Only a final kapi CLI release may become what
https://github.com/neokapi/neokapi/releases/latest resolves to. Every
publishing step in release.yml passes `make_latest: ${{ steps.channel.outputs.latest }}`,
or the package-cli job output of the same name, and every step in another
release workflow passes `make_latest: false`.
MSG
  exit 1
fi

echo "check-plugin-release-latest: only a final kapi CLI release claims the repo's latest"
