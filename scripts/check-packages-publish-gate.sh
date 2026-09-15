#!/usr/bin/env bash
#
# Guard: the apt and yum repository publishes only when it is turned on.
#
# release.yml builds a .deb and an .rpm for every tag and attaches both to the
# release. Publishing them into the signed repository at
# https://neokapi.github.io/packages/ is a separate step, and it runs only when
# the repository variable PUBLISH_PACKAGES is "true". GitHub reads an unset
# variable as the empty string, so the repository stays off until someone sets
# the variable. The step's own check for its secrets is no gate: the secrets are
# configured, so a step guarded by them alone publishes on the next final tag.
#
# See docs/internals/auto-update.md, "Phase 3".
set -euo pipefail

cd "$(dirname "$0")/.."

wf=.github/workflows/release.yml
gate="vars.PUBLISH_PACKAGES == 'true'"

# Print the `if:` of every step that runs the publish script, one line each, or
# "<none>" for such a step with no condition. A step starts at a list item six
# columns in; its keys sit at eight. A job header ends the step before it.
conditions=$(awk '
  function flush() {
    if (instep && publishes) print (cond == "" ? "<none>" : cond)
    instep = 0; publishes = 0; cond = ""
  }
  /^  [A-Za-z0-9_-]+:[[:space:]]*$/ { flush(); next }
  /^      - / {
    flush(); instep = 1
    if ($0 ~ /^      - if:/) { cond = $0; sub(/^      - if:[[:space:]]*/, "", cond) }
    next
  }
  instep && /^        if:/ { cond = $0; sub(/^        if:[[:space:]]*/, "", cond) }
  instep && /scripts\/publish-packages\.sh/ { publishes = 1 }
  END { flush() }
' "$wf")

if [ -z "$conditions" ]; then
  echo "check-packages-publish-gate: no step in $wf runs scripts/publish-packages.sh" >&2
  echo "A renamed or moved publish step would leave this guard checking nothing; update it." >&2
  exit 1
fi

fail=0
while IFS= read -r cond; do
  case "$cond" in
    *"||"*)
      printf 'check-packages-publish-gate: the publish step condition has an alternative: %s\n' "$cond" >&2
      fail=1
      ;;
    *"$gate"*) ;;
    *)
      printf 'check-packages-publish-gate: the publish step runs without %s: %s\n' "$gate" "$cond" >&2
      fail=1
      ;;
  esac
done <<<"$conditions"

if [ "$fail" -ne 0 ]; then
  cat >&2 <<'MSG'

The apt and yum repository is off until the repository variable
PUBLISH_PACKAGES is "true". Every step that runs scripts/publish-packages.sh
needs `vars.PUBLISH_PACKAGES == 'true'` in a condition that has no `||`.
MSG
  exit 1
fi

echo "check-packages-publish-gate: the apt and yum publish waits for PUBLISH_PACKAGES"
