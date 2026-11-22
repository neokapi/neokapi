#!/usr/bin/env bash
# Guard: the production server and worker images run as an unprivileged user
# with a fixed numeric id.
#
#   scripts/check-image-user.sh              check the repository
#   scripts/check-image-user.sh --self-test  prove it on scratch Dockerfiles
#
# The last USER instruction of each Dockerfile's final stage must be
# `USER <uid>:<gid>` with numeric, non-zero ids. A name depends on the base
# image's /etc/passwd, and a missing USER runs the process as root. The E2E
# stack (bowrain/e2e/server/compose.yaml) runs the same images with a
# read-only root filesystem, so a write outside the declared paths fails there.
#
# Exit status: 0 every image runs unprivileged, 1 one does not, 2 a file is
# missing.
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

IMAGES=(
  bowrain/docker/bowrain-server/Dockerfile
  bowrain/docker/bowrain-worker/Dockerfile
)

# final_user prints the argument of the last USER instruction after the last
# FROM, or nothing when the final stage sets none.
final_user() {
  awk '
    toupper($1) == "FROM" { user = "" }
    toupper($1) == "USER" { user = $2 }
    END { print user }' "$1"
}

# check_file <path> prints a problem and returns 1 when the image would run
# privileged.
check_file() {
  local f=$1 user uid gid
  if [ ! -f "$f" ]; then
    echo "check-image-user: $f is missing" >&2
    return 2
  fi
  user="$(final_user "$f")"
  if [ -z "$user" ]; then
    echo "check-image-user: $f sets no USER in its final stage, so the process runs as root" >&2
    return 1
  fi
  if ! [[ "$user" =~ ^([0-9]+):([0-9]+)$ ]]; then
    echo "check-image-user: $f runs as '$user'; use a numeric USER <uid>:<gid>" >&2
    return 1
  fi
  uid=${BASH_REMATCH[1]}
  gid=${BASH_REMATCH[2]}
  if [ "$uid" -eq 0 ] || [ "$gid" -eq 0 ]; then
    echo "check-image-user: $f runs as $user, which is root" >&2
    return 1
  fi
  return 0
}

self_test() {
  local tmp failures=0 rc
  tmp="$(mktemp -d)" || return 2
  expect() { # expect <want> <name> <dockerfile body>
    printf '%s\n' "$3" > "$tmp/Dockerfile"
    check_file "$tmp/Dockerfile" 2> /dev/null
    rc=$?
    if [ "$rc" -ne "$1" ]; then
      echo "self-test: $2: exit $rc, want $1" >&2
      failures=$((failures + 1))
    fi
  }
  expect 0 "a numeric user in the final stage" $'FROM golang AS b\nUSER 0:0\nFROM alpine\nUSER 10001:10001\nENTRYPOINT ["x"]'
  expect 1 "no USER in the final stage" $'FROM golang AS b\nUSER 10001:10001\nFROM alpine\nENTRYPOINT ["x"]'
  expect 1 "a named user" $'FROM alpine\nUSER bowrain\n'
  expect 1 "root by number" $'FROM alpine\nUSER 0:0\n'
  expect 1 "the root group" $'FROM alpine\nUSER 10001:0\n'
  expect 1 "a later USER switches back to root" $'FROM alpine\nUSER 10001:10001\nUSER root\n'
  rm -rf "${tmp:?}"
  if [ "$failures" -gt 0 ]; then
    return 1
  fi
  echo "check-image-user: self-test passed"
}

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit $?
fi

status=0
for f in "${IMAGES[@]}"; do
  check_file "$root/$f" || status=$?
done
[ "$status" -eq 0 ] && echo "check-image-user: the server and worker images run as an unprivileged numeric user"
exit "$status"
