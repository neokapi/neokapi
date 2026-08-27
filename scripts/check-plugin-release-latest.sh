#!/usr/bin/env bash
#
# Guard: a plugin release must never become the repository's "latest".
#
# GitHub answers /releases/latest with the newest release that is not a draft
# and not a prerelease. Every kapi CLI release is a prerelease while 1.2.0 is in
# its rc line, so any plugin published as a full release wins that slot — and
# setup-kapi's resolve-version, `kapi update`'s "install a release" link and the
# server install docs all read it. check-v0.1.0 took the slot exactly this way:
# its release step was the one of eight that never passed `make_latest: false`.
#
# The rule is per publishing step, not per workflow: release-bowrain.yml
# publishes three, and one unguarded step is enough to hand the slot away.
set -euo pipefail

cd "$(dirname "$0")/.."

fail=0
for wf in .github/workflows/release-*.yml; do
  # The kapi CLI's own release IS allowed to be latest; every other tag line
  # here is a plugin or a platform binary with its own versioning.
  case "$wf" in
    *release-kapi.yml) continue ;;
  esac

  # Count publishing steps and how many opt out. awk over the block that follows
  # each action-gh-release use, so a `with:` further down the file cannot be
  # mistaken for this step's.
  total=$(grep -c "uses: softprops/action-gh-release" "$wf" || true)
  [ "$total" -eq 0 ] && continue

  guarded=$(awk '
    /uses: softprops\/action-gh-release/ { instep = 1; seen = 0; next }
    instep && /make_latest:[[:space:]]*false/ { seen = 1 }
    instep && /^[[:space:]]{0,8}- / { if (seen) count++; instep = 0 }
    END { if (instep && seen) count++; print count + 0 }
  ' "$wf")

  if [ "$guarded" -lt "$total" ]; then
    printf 'check-plugin-release-latest: %s publishes %s release(s), %s guarded\n' \
      "$wf" "$total" "$guarded" >&2
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then
  cat >&2 <<'MSG'

Every plugin release step needs `make_latest: false`, or the plugin becomes
what https://github.com/neokapi/neokapi/releases/latest resolves to and the
kapi CLI stops being installable by "latest".
MSG
  exit 1
fi

echo "check-plugin-release-latest: no plugin release claims the repo's latest"
