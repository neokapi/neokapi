#!/usr/bin/env bash
#
# PreToolUse hook for Bash: note, or refuse, a commit that does not say how it
# was verified.
#
# The verify skill ends a commit message with a trailer naming the recipes that
# were run, or saying why none was:
#
#   Verified: cli, server
#   Verified: none (comment-only change)
#
# The trailer reaches main in the squash commit, so `git log --grep '^Verified:
# none'` lists the changes nobody ran.
#
# The gate has three modes, set by KAPI_COMMIT_GATE:
#
#   off      does nothing.
#   observe  (the default) records a commit with no trailer in the log below and
#            lets it through, with a reminder. Read the log for false positives
#            before turning enforcement on.
#   enforce  refuses a commit with no trailer. After two refusals in one
#            session the third is let through and recorded, so a session
#            cannot get stuck behind the gate.
#
# The log is commit-gate.log in the repository's common git directory: one
# file for every worktree, never part of a commit.
#
#   .claude/hooks/commit-gate.sh --self-test
set -uo pipefail

mode="${KAPI_COMMIT_GATE:-observe}"

# has_trailer reads a command on stdin and succeeds when it carries a Verified
# trailer with a value.
has_trailer() {
  grep -qE '^[[:space:]]*Verified:[[:space:]]*[^[:space:]]'
}

# is_commit reads a command on stdin and succeeds when it runs `git commit`
# with a message this hook can see. A commit that reuses a message (--no-edit,
# -C, --fixup) or takes it from a file or an editor is out of its sight.
is_commit() {
  local command arguments
  command="$(cat)"
  grep -qE '(^|[;&|[:space:](])git([[:space:]]+-[^[:space:]]+([[:space:]]+[^-][^[:space:]]*)?)*[[:space:]]+commit([[:space:]]|$)' <<< "$command" || return 1
  # What follows the word commit: `git -C <dir> commit` names a directory
  # before it, `git commit -C <commit>` reuses a message after it.
  arguments="${command#*commit}"
  grep -qE -- '--no-edit|--fixup|--squash|[[:space:]]-C[[:space:]]|[[:space:]]-F[[:space:]]|--file' <<< "$arguments" && return 1
  grep -qE -- '[[:space:]]-[a-zA-Z]*m[[:space:]]|--message' <<< "$arguments"
}

decide() {
  local command="$1" session="$2" log="$3"
  [ "$mode" = "off" ] && return 0
  is_commit <<< "$command" || return 0
  has_trailer <<< "$command" && return 0

  local stamp refusals
  stamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  if [ "$mode" = "enforce" ]; then
    refusals="$(grep -c "	refused	$session\$" "$log" 2>/dev/null || true)"
    if [ "${refusals:-0}" -lt 2 ]; then
      printf '%s\trefused\t%s\n' "$stamp" "$session" >> "$log"
      echo "This commit message has no Verified: trailer. Run the verify skill, then end the message with 'Verified: <recipes>' or 'Verified: none (<reason>)'." >&2
      return 2
    fi
    printf '%s\tlet-through\t%s\n' "$stamp" "$session" >> "$log"
    return 0
  fi
  printf '%s\twould-refuse\t%s\n' "$stamp" "$session" >> "$log"
  jq -cn '{hookSpecificOutput: {hookEventName: "PreToolUse", additionalContext: "Commit gate (observe mode): this commit message has no Verified: trailer. The verify skill says how to verify a change and what the trailer records."}}'
  return 0
}

self_test() {
  local tmp log status
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand now: tmp is local to this function
  trap "rm -rf '$tmp'" RETURN
  log="$tmp/log"
  : > "$log"
  local with without amend other
  with=$'git commit -m "Fix it\n\nVerified: cli"'
  without=$'git commit -m "Fix it"'
  amend='git commit --amend --no-edit'
  other='git log --oneline -3 && echo "git commit is mentioned here"'

  mode=observe
  decide "$with" s1 "$log" > /dev/null || { echo "self-test: observe refused a verified commit" >&2; return 1; }
  [ ! -s "$log" ] || { echo "self-test: a verified commit was logged" >&2; return 1; }
  decide "$without" s1 "$log" | grep -q 'observe mode' || { echo "self-test: observe gave no reminder" >&2; return 1; }
  grep -q 'would-refuse' "$log" || { echo "self-test: observe did not log" >&2; return 1; }
  : > "$log"
  decide "$amend" s1 "$log" > /dev/null || { echo "self-test: an amend with no new message was refused" >&2; return 1; }
  decide "$other" s1 "$log" > /dev/null || { echo "self-test: a command that only mentions a commit was refused" >&2; return 1; }
  [ ! -s "$log" ] || { echo "self-test: a command that is not a new commit was logged" >&2; return 1; }
  decide 'git -C sub commit -m "x"' s1 "$log" > /dev/null
  grep -q 'would-refuse' "$log" || { echo "self-test: git -C <dir> commit was not recognised" >&2; return 1; }
  decide $'git commit -m "x\n\nVerified: none (docs only)"' s1 "$log" > /dev/null
  [ "$(wc -l < "$log")" -eq 1 ] || { echo "self-test: 'Verified: none (reason)' was not accepted" >&2; return 1; }

  mode=enforce
  : > "$log"
  for attempt in 1 2; do
    decide "$without" s2 "$log" 2> /dev/null; status=$?
    [ "$status" -eq 2 ] || { echo "self-test: enforce let refusal $attempt through" >&2; return 1; }
  done
  decide "$without" s2 "$log" 2> /dev/null || { echo "self-test: the third attempt was refused" >&2; return 1; }
  grep -q 'let-through' "$log" || { echo "self-test: the let-through was not logged" >&2; return 1; }
  decide "$without" s3 "$log" 2> /dev/null; status=$?
  [ "$status" -eq 2 ] || { echo "self-test: another session inherited the allowance" >&2; return 1; }
  decide "$with" s3 "$log" || { echo "self-test: enforce refused a verified commit" >&2; return 1; }

  mode=off
  : > "$log"
  decide "$without" s4 "$log" || { echo "self-test: off refused a commit" >&2; return 1; }
  [ ! -s "$log" ] || { echo "self-test: off wrote to the log" >&2; return 1; }
  echo "commit-gate: self-test passed"
}

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit $?
fi

input="$(cat)"
command="$(jq -r '.tool_input.command // empty' <<< "$input" 2>/dev/null)"
session="$(jq -r '.session_id // "unknown"' <<< "$input" 2>/dev/null)"
cwd="$(jq -r '.cwd // empty' <<< "$input" 2>/dev/null)"
[ -n "$command" ] || exit 0
common="$(git -C "${cwd:-.}" rev-parse --path-format=absolute --git-common-dir 2>/dev/null)" || exit 0
decide "$command" "$session" "$common/commit-gate.log"
