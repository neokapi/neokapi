#!/usr/bin/env bash
#
# apt-retry.sh: install Debian packages within a fixed time budget.
#
#   apt-retry.sh <package>...      install, within APT_BUDGET seconds (900)
#   apt-retry.sh --self-test       prove the bounds with stub commands
#
# Each attempt first finishes any dpkg run a previous attempt left
# interrupted, then runs update, download and unpack, each under its own
# limit. The limits are shares of what is left of the budget, measured again
# after the dpkg step, and never below a floor: `timeout 0` would run the
# command with no limit at all, and a negative value is an error. An attempt
# starts only while enough budget is left for one. After the last attempt, or
# once the budget is spent, the script fails with apt's own error above it.
#
# APT_CMD, DPKG_CMD, TIMEOUT_CMD, SLEEP_CMD and NOW_CMD stand in for
# `sudo apt-get`, `sudo dpkg`, `timeout`, `sleep` and `date +%s` in the
# self-test.
set -uo pipefail

MIN_ATTEMPT=90 # the least budget worth starting an attempt with
MIN_STEP=20    # no single limit is shorter than this

now() { ${NOW_CMD:-date +%s}; }

# limit LEFT SHARE: LEFT/SHARE seconds, at least MIN_STEP, at most LEFT, and
# always a positive whole number.
limit() {
  local left=$1 share=$2 v
  v=$((left / share))
  [ "$v" -lt "$MIN_STEP" ] && v=$MIN_STEP
  [ "$v" -gt "$left" ] && v=$left
  [ "$v" -lt 1 ] && v=1
  echo "$v"
}

install() {
  local apt="${APT_CMD:-sudo apt-get}" dpkg="${DPKG_CMD:-sudo dpkg}" to="${TIMEOUT_CMD:-timeout}"
  local opts="-o Acquire::Retries=3 -o Acquire::http::Timeout=20 -o Acquire::https::Timeout=20 -o DPkg::Lock::Timeout=60"
  local budget="${APT_BUDGET:-900}" deadline left attempt
  deadline=$(($(now) + budget))
  for attempt in 1 2 3 4; do
    left=$((deadline - $(now)))
    [ "$left" -ge "$MIN_ATTEMPT" ] || break
    # shellcheck disable=SC2086 # $dpkg and $opts are word lists
    $to "$(limit "$left" 6)" $dpkg --configure -a || true
    left=$((deadline - $(now)))
    [ "$left" -ge "$MIN_ATTEMPT" ] || break
    # shellcheck disable=SC2086
    if $to "$(limit "$left" 6)" $apt $opts update &&
       $to "$(limit "$left" 3)" $apt $opts install -y --download-only "$@" &&
       $to "$(limit "$left" 2)" $apt $opts install -y --no-download "$@"; then
      return 0
    fi
    echo "::warning::apt attempt $attempt failed"
    # Drop the Azure mirror only if the list holds something else; an empty
    # mirrorlist turns a slow install into no install at all.
    if [ "$attempt" -eq 2 ] && [ -f /etc/apt/apt-mirrors.txt ] && [ -z "${APT_CMD:-}" ] &&
       grep -qv 'azure\.archive\.ubuntu\.com' /etc/apt/apt-mirrors.txt; then
      echo "::warning::dropping the Azure mirror; retrying against the rest"
      sudo sed -i '/azure\.archive\.ubuntu\.com/d' /etc/apt/apt-mirrors.txt
    fi
    # A network fault on a runner usually clears within a minute; no pause
    # after the last attempt.
    [ "$attempt" -lt 4 ] && ${SLEEP_CMD:-sleep} $((attempt * 15))
  done
  echo "::error::apt could not install '$*' within $budget seconds"
  return 1
}

self_test() {
  local work status=0 out
  work="$(mktemp -d)"
  # shellcheck disable=SC2064
  trap "rm -rf '$work'" RETURN
  # A clock the stubs advance: every timed command takes the whole limit it
  # was given, the worst case.
  echo 1000 >"$work/clock"
  cat >"$work/now" <<EOF
#!/bin/bash
cat "$work/clock"
EOF
  cat >"$work/timeout" <<EOF
#!/bin/bash
echo "\$1" >>"$work/limits"
case "\$1" in ''|*[!0-9]*|0) echo "bad limit \$1" >>"$work/bad" ;; esac
echo \$((\$(cat "$work/clock") + \$1)) >"$work/clock"
shift
"\$@"
EOF
  cat >"$work/fail" <<'EOF'
#!/bin/bash
exit 1
EOF
  chmod +x "$work/now" "$work/timeout" "$work/fail"
  run() {
    : >"$work/limits"
    rm -f "$work/bad"
    echo 1000 >"$work/clock"
    APT_BUDGET="$1" NOW_CMD="$work/now" TIMEOUT_CMD="$work/timeout" APT_CMD="$work/fail" \
      DPKG_CMD="$work/fail" SLEEP_CMD=true install pkg >"$work/out" 2>&1
  }

  run 900 && { echo "self-test: a failing apt must fail the install"; status=1; }
  if [ -f "$work/bad" ]; then echo "self-test: a limit of 0 or less was passed: $(cat "$work/bad")"; status=1; fi
  if [ "$(cat "$work/clock")" -gt $((1000 + 900 + MIN_STEP)) ]; then
    echo "self-test: the attempts overran the budget: clock at $(cat "$work/clock")"; status=1
  fi
  grep -q "within 900 seconds" "$work/out" || { echo "self-test: no final error"; status=1; }

  # A nearly spent budget: nothing starts, and nothing gets an unbounded run.
  run 60 && { echo "self-test: a spent budget must fail"; status=1; }
  if [ -s "$work/limits" ]; then echo "self-test: an attempt started with 60 s left: $(tr '\n' ' ' <"$work/limits")"; status=1; fi

  # Just enough to start: the dpkg step eats into it, and the step after it
  # must not start on what is left.
  run 100
  out="$(tr '\n' ' ' <"$work/limits")"
  if [ -f "$work/bad" ]; then echo "self-test: a limit of 0 or less with 100 s: $out"; status=1; fi
  [ "$(wc -l <"$work/limits")" -eq 1 ] || { echo "self-test: with 100 s only the dpkg step fits, got limits: $out"; status=1; }

  [ "$status" -eq 0 ] && echo "apt-retry self-test: OK"
  return "$status"
}

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit $?
fi
install "$@"
