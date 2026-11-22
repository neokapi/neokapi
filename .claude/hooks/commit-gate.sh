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
# The gate reads the message a `git commit` command carries: the -m arguments,
# or the file a -F names when that file exists already. A commit made while a
# merge is in progress needs no trailer. A message the gate cannot see (a reused
# one, an editor, a file the same command writes first, a path held in a
# variable) passes unjudged.
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
# file for every worktree, never part of a commit. Each line holds the time,
# what the gate did, the session and the commit's subject, tab-separated.
#
#   .claude/hooks/commit-gate.sh --self-test
set -uo pipefail

mode="${KAPI_COMMIT_GATE:-observe}"

# has_trailer reads a message on stdin and succeeds when it carries a Verified
# trailer with a value.
has_trailer() {
  grep -qE '^[[:space:]]*Verified:[[:space:]]*[^[:space:]]'
}

# is_commit reads a command on stdin and succeeds when it runs `git commit`.
is_commit() {
  grep -qE '(^|[;&|[:space:](])git([[:space:]]+-[^[:space:]]+([[:space:]]+[^-][^[:space:]]*)?)*[[:space:]]+commit([[:space:]]|$)'
}

# message_file reads the arguments of a commit on stdin and prints the path a
# -F or --file names. It prints nothing for a path the shell would still have
# to expand.
message_file() {
  local path
  path="$(sed -nE "s/.*[[:space:]](-F|--file)[=[:space:]]+(\"([^\"]+)\"|'([^']+)'|([^[:space:];&|]+)).*/\\3\\4\\5/p" | head -1)"
  case "$path" in *'$'* | *'`'* | '~'*) return 0 ;; esac
  printf '%s' "$path"
}

# commit_message reads a command on stdin and prints the message its commit
# carries: the command itself for -m, the file's content for -F. $1 is the
# directory the command runs in. It fails when the message is out of sight.
commit_message() {
  local command arguments file
  command="$(cat)"
  # What follows the word commit: `git -C <dir> commit` names a directory
  # before it, `git commit -C <commit>` reuses a message after it.
  arguments="${command#*commit}"
  grep -qE -- '--no-edit|--fixup|--squash|[[:space:]]-C[[:space:]]' <<< "$arguments" && return 1
  if grep -qE -- '[[:space:]]-[a-zA-Z]*m[[:space:]]|--message' <<< "$arguments"; then
    printf '%s\n' "$command"
    return 0
  fi
  file="$(message_file <<< "$arguments")"
  [ -n "$file" ] || return 1
  case "$file" in /*) ;; *) file="${1:-.}/$file" ;; esac
  [ -f "$file" ] || return 1
  cat "$file"
}

# subject reads a command on stdin and prints the first line of the message
# its -m carries, for the log. A message written as a here-document has its
# first line on the line after the -m.
subject() {
  awk '
    heredoc == 1 { print; exit }
    match($0, /-[a-zA-Z]*m[[:space:]]+/) {
      rest = substr($0, RSTART + RLENGTH)
      if (rest ~ /^"?\$\(cat/) { heredoc = 1; next }
      sub(/^\$?["'"'"']/, "", rest)
      sub(/["'"'"'].*$/, "", rest)
      print rest
      exit
    }'
}

# commit_dirs reads a command on stdin and prints the directories its commit
# may run in, one per line: $1 (where the command starts), each directory a
# `cd` names, and the one `git -C` names. A relative one is taken from $1.
commit_dirs() {
  local start="${1:-.}" dir
  printf '%s\n' "$start"
  grep -oE "(^|[;&|[:space:]])(cd|git[[:space:]]+-C)[[:space:]]+(\"[^\"]+\"|'[^']+'|[^[:space:];&|]+)" |
    sed -E "s/^[;&|[:space:]]*(cd|git[[:space:]]+-C)[[:space:]]+//; s/^[\"']//; s/[\"']\$//" |
    while IFS= read -r dir; do
      case "$dir" in *'$'* | *'`'* | '~'*) continue ;; esac
      case "$dir" in /*) printf '%s\n' "$dir" ;; *) printf '%s\n' "$start/$dir" ;; esac
    done
}

# concludes_merge reads a command on stdin and succeeds when a merge is in
# progress in a directory its commit may run in. The message is not asked: a
# subject that says "Merge" proves nothing.
concludes_merge() {
  local dir
  while IFS= read -r dir; do
    git -C "$dir" rev-parse -q --verify MERGE_HEAD > /dev/null 2>&1 && return 0
  done < <(commit_dirs "$1")
  return 1
}

decide() {
  local command="$1" session="$2" log="$3" cwd="${4:-.}"
  [ "$mode" = "off" ] && return 0
  is_commit <<< "$command" || return 0
  local message title
  message="$(commit_message "$cwd" <<< "$command")" || return 0
  has_trailer <<< "$message" && return 0
  if [ "$message" = "$command" ]; then
    title="$(subject <<< "$command")"
  else
    title="$(head -1 <<< "$message")"
  fi
  concludes_merge "$cwd" <<< "$command" && return 0
  title="$(tr '\t' ' ' <<< "$title" | cut -c1-80)"

  local stamp refusals
  stamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  if [ "$mode" = "enforce" ]; then
    refusals="$(grep -c "	refused	$session	" "$log" 2>/dev/null || true)"
    if [ "${refusals:-0}" -lt 2 ]; then
      printf '%s\trefused\t%s\t%s\n' "$stamp" "$session" "$title" >> "$log"
      echo "This commit message has no Verified: trailer. Run the verify skill, then end the message with 'Verified: <recipes>' or 'Verified: none (<reason>)'." >&2
      return 2
    fi
    printf '%s\tlet-through\t%s\t%s\n' "$stamp" "$session" "$title" >> "$log"
    return 0
  fi
  printf '%s\twould-refuse\t%s\t%s\n' "$stamp" "$session" "$title" >> "$log"
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
  decide "$with" s1 "$log" "$tmp" > /dev/null || { echo "self-test: observe refused a verified commit" >&2; return 1; }
  [ ! -s "$log" ] || { echo "self-test: a verified commit was logged" >&2; return 1; }
  decide "$without" s1 "$log" "$tmp" | grep -q 'observe mode' || { echo "self-test: observe gave no reminder" >&2; return 1; }
  grep -q "would-refuse	s1	Fix it\$" "$log" || { echo "self-test: observe did not log the subject" >&2; return 1; }
  : > "$log"
  decide "$amend" s1 "$log" "$tmp" > /dev/null || { echo "self-test: an amend with no new message was refused" >&2; return 1; }
  decide "$other" s1 "$log" "$tmp" > /dev/null || { echo "self-test: a command that only mentions a commit was refused" >&2; return 1; }
  [ ! -s "$log" ] || { echo "self-test: a command that is not a new commit was logged" >&2; return 1; }
  decide 'git -C sub commit -m "x"' s1 "$log" "$tmp" > /dev/null
  grep -q 'would-refuse' "$log" || { echo "self-test: git -C <dir> commit was not recognised" >&2; return 1; }
  decide $'git commit -m "x\n\nVerified: none (docs only)"' s1 "$log" "$tmp" > /dev/null
  [ "$(wc -l < "$log")" -eq 1 ] || { echo "self-test: 'Verified: none (reason)' was not accepted" >&2; return 1; }

  # A message in a file is read when the file exists.
  : > "$log"
  printf 'Fix it\n\nVerified: cli\n' > "$tmp/good.txt"
  printf 'Fix it from a file\n\nNo trailer here.\n' > "$tmp/bad.txt"
  decide 'git add -A && git commit -q -F good.txt' s1 "$log" "$tmp" > /dev/null
  [ ! -s "$log" ] || { echo "self-test: a verified message in a file was logged" >&2; return 1; }
  decide 'git commit -F bad.txt' s1 "$log" "$tmp" > /dev/null
  grep -q "would-refuse	s1	Fix it from a file\$" "$log" || { echo "self-test: a message file with no trailer was not logged" >&2; return 1; }
  : > "$log"
  decide "git commit --file=\"$tmp/bad.txt\"" s1 "$log" /nonexistent > /dev/null
  grep -q 'would-refuse' "$log" || { echo "self-test: an absolute --file= path was not read" >&2; return 1; }
  : > "$log"
  # shellcheck disable=SC2016  # the unexpanded variable is the case under test
  decide 'git commit -F "$S/msg.txt"' s1 "$log" "$tmp" > /dev/null
  decide 'git commit -F later.txt' s1 "$log" "$tmp" > /dev/null
  [ ! -s "$log" ] || { echo "self-test: a message file out of sight was judged" >&2; return 1; }

  # A commit made while a merge is in progress needs no trailer. A subject
  # that says "Merge" does not make one.
  : > "$log"
  decide 'git commit -m "Merge origin/main"' s1 "$log" "$tmp" > /dev/null
  grep -q 'would-refuse' "$log" || { echo "self-test: a subject saying Merge passed with no merge in progress" >&2; return 1; }
  : > "$log"
  git -C "$tmp" init -q 2> /dev/null
  git -C "$tmp" -c user.name=t -c user.email=t@example.test commit -q --allow-empty -m base 2> /dev/null
  local gitdir
  gitdir="$(git -C "$tmp" rev-parse --absolute-git-dir)"
  git -C "$tmp" rev-parse HEAD > "$gitdir/MERGE_HEAD"
  decide 'git commit -m "Take main"' s1 "$log" "$tmp" > /dev/null
  [ ! -s "$log" ] || { echo "self-test: a commit during a merge was logged" >&2; return 1; }
  # The merge is found where the command goes, not only where it starts.
  decide "cd \"$tmp\" && git commit -m \"Take main\"" s1 "$log" /nonexistent > /dev/null
  decide "git -C $tmp commit -m \"Take main\"" s1 "$log" /nonexistent > /dev/null
  mkdir -p "$tmp/sub"
  decide 'cd sub && git commit -m "Take main"' s1 "$log" "$tmp" > /dev/null
  [ ! -s "$log" ] || { echo "self-test: a merge in the directory the command names was logged" >&2; return 1; }
  rm -f "$gitdir/MERGE_HEAD"
  decide 'git commit -m "Take main"' s1 "$log" "$tmp" > /dev/null
  grep -q 'would-refuse' "$log" || { echo "self-test: the same commit outside a merge was not logged" >&2; return 1; }

  # A message written as a here-document logs its first line.
  : > "$log"
  decide $'git commit -m "$(cat <<\'EOF\'\nShip the thing\n\nBody.\nEOF\n)"' s1 "$log" "$tmp" > /dev/null
  grep -q "would-refuse	s1	Ship the thing\$" "$log" || { echo "self-test: a here-document subject was not logged" >&2; return 1; }

  mode=enforce
  : > "$log"
  for attempt in 1 2; do
    decide "$without" s2 "$log" "$tmp" 2> /dev/null; status=$?
    [ "$status" -eq 2 ] || { echo "self-test: enforce let refusal $attempt through" >&2; return 1; }
  done
  decide "$without" s2 "$log" "$tmp" 2> /dev/null || { echo "self-test: the third attempt was refused" >&2; return 1; }
  grep -q 'let-through' "$log" || { echo "self-test: the let-through was not logged" >&2; return 1; }
  decide "$without" s3 "$log" "$tmp" 2> /dev/null; status=$?
  [ "$status" -eq 2 ] || { echo "self-test: another session inherited the allowance" >&2; return 1; }
  decide "$with" s3 "$log" "$tmp" || { echo "self-test: enforce refused a verified commit" >&2; return 1; }
  decide 'git commit -F bad.txt' s4 "$log" "$tmp" 2> /dev/null; status=$?
  [ "$status" -eq 2 ] || { echo "self-test: enforce let a message file with no trailer through" >&2; return 1; }
  git -C "$tmp" rev-parse HEAD > "$gitdir/MERGE_HEAD"
  decide 'git commit -m "Merge origin/main"' s5 "$log" "$tmp" 2> /dev/null || { echo "self-test: enforce refused a commit during a merge" >&2; return 1; }
  rm -f "$gitdir/MERGE_HEAD"

  mode=off
  : > "$log"
  decide "$without" s4 "$log" "$tmp" || { echo "self-test: off refused a commit" >&2; return 1; }
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
decide "$command" "$session" "$common/commit-gate.log" "${cwd:-.}"
