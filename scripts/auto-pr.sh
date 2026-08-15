#!/usr/bin/env bash
#
# auto-pr.sh — land a scheduled job's own output on a rolling bot branch and
# open, or refresh, one reviewable pull request for it.
#
# A job that produces committable artifacts on a schedule has no pull request to
# heal the way scripts/l10n-autofix.sh heals one in place, and pushing straight
# to the default branch puts unreviewed content on main with no CI. This is the
# third way: the run's output becomes a pull request a human reads.
#
# The branch is fixed per artifact stream and force-pushed on every run, so a
# stream has at most one open pull request at any time and an unmerged one is
# refreshed rather than duplicated. A run whose owned paths did not move pushes
# nothing, opens nothing, and exits 0 — repeated runs over a quiet stream leave
# no branches and no pull requests behind.
#
# Usage:
#     scripts/auto-pr.sh <branch> <title> -- <pathspec>...
#     scripts/auto-pr.sh --self-test
#
#   <branch>    the stream's fixed bot branch (e.g. bot/evals-model-prices).
#   <title>     commit subject and pull-request title.
#   <pathspec>  the paths this stream owns. ONLY these are staged, so an
#               unrelated working-tree change can never ride along. Pathspec
#               magic is honoured, so a caller can pass `:(glob)a/*/b.yaml`.
#
# AUTO_PR_NOTE is appended to the pull-request body — the caller's place to say
# what the run found.
#
# Requires: git, and for the push and the pull request, gh authenticated via
# GH_TOKEN/GITHUB_TOKEN with push rights. Pushes and pull requests made with
# GITHUB_TOKEN start no workflow runs, so the body says how to start the checks.
set -euo pipefail

readonly BOT_NAME="github-actions[bot]"
readonly BOT_EMAIL="41898282+github-actions[bot]@users.noreply.github.com"

usage() {
  awk '/^# Usage:/ { on = 1 } on && !/^#/ { exit } on { sub(/^# ?/, ""); print }' "$0"
}

# owned_status prints one NUL-terminated "XY path" record per change under the
# owned pathspecs — modified, deleted, and newly created together.
#
# `git status` answers rather than `git diff` for two reasons: it lists
# untracked files in the same pass, which is how a first sidecar or a first
# catalog for a new locale shows up at all; and it tolerates a pathspec that
# matches nothing, which a glob over per-locale artifacts does until some locale
# gets its first one. `git add` does not tolerate that, so the concrete paths
# read out of here are what gets staged.
owned_status() {
  git status --porcelain=v1 -z --untracked-files=all --no-renames -- "$@"
}

# pr_body renders the pull-request body: what produced it, the caller's note,
# and how to start the checks a GITHUB_TOKEN push cannot start itself.
pr_body() {
  cat <<EOF
Automated by the \`${GITHUB_WORKFLOW:-auto-pr}\` workflow (run ${GITHUB_RUN_ID:-local}).

${AUTO_PR_NOTE:-}

> Opened with \`GITHUB_TOKEN\`, so CI does not start automatically — close and
> reopen the pull request (or push any commit) to start the checks.
EOF
}

# base_branch is the ref the workflow ran on: the branch a dispatch selected, or
# the default branch for a scheduled run. A scheduled checkout is a detached
# HEAD, which names no branch, so main is the fallback.
base_branch() {
  local ref="${GITHUB_REF_NAME:-}"
  if [ -z "$ref" ]; then
    ref="$(git rev-parse --abbrev-ref HEAD)"
  fi
  if [ "$ref" = "HEAD" ] || [ -z "$ref" ]; then
    ref=main
  fi
  printf '%s\n' "$ref"
}

# open_new_pr opens the pull request for a freshly pushed bot branch, and says
# something usable when it cannot.
#
# The commit is already on the remote by this point, so a refused `gh pr create`
# means the content was delivered and only the announcement is missing — a
# distinction the raw `gh` error does not draw. The common refusal is the
# repository's own "Allow GitHub Actions to create and approve pull requests"
# setting, which is off here; `bot/release-downloads-kapi` sat at a stale release
# for a fortnight behind exactly that, with the install page promising links the
# branch already had. So: name the cause when it is that one, hand over the
# compare URL either way, and still fail — an unopened pull request is nobody's
# dashboard, and a green run would repeat the fortnight.
open_new_pr() {
  local branch="$1" title="$2" base err
  base="$(base_branch)"
  err="$(mktemp)"

  if gh pr create --head "$branch" --base "$base" \
    --title "$title" --body "$(pr_body)" 2>"$err"; then
    rm -f "$err"
    return 0
  fi
  cat "$err" >&2

  local compare="${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:-neokapi/neokapi}/compare/${base}...${branch}?expand=1"
  local cause="Opening it failed; the branch itself is pushed and current."
  if grep -qi 'not permitted to create.*pull request' "$err"; then
    cause="GitHub Actions is not permitted to open pull requests in this repository (Settings → Actions → General → Workflow permissions). The branch itself is pushed and current."
  fi
  rm -f "$err"

  echo "::error::${title}: the change is on ${branch} but has no pull request. ${cause} Open it at ${compare}"
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    {
      echo "### Delivered to \`${branch}\`, not yet opened"
      echo
      echo "${cause}"
      echo
      echo "[Open the pull request](${compare})"
    } >> "$GITHUB_STEP_SUMMARY"
  fi
  return 1
}

# ── the flow ─────────────────────────────────────────────────────────────────
#
# Nothing to deliver is a successful run, not a silent failure, so a quiet stream
# returns 0. A delivery that reached the branch but could not be opened as a pull
# request returns non-zero — see open_new_pr.
auto_pr() {
  local branch="$1" title="$2"
  shift 2
  local paths=("$@")

  local rec
  local entries=() changed=()
  while IFS= read -r -d '' rec; do
    entries+=("$rec")
    changed+=("${rec:3}")
  done < <(owned_status "${paths[@]}")

  if [ "${#changed[@]}" -eq 0 ]; then
    echo "auto-pr: the owned paths did not move — nothing to deliver."
    return 0
  fi

  echo "auto-pr: delivering ${#changed[@]} change(s)"
  printf '  %s\n' "${entries[@]}"

  # Stage the run's own changes and nothing else, by concrete path: an unrelated
  # edit elsewhere in the tree cannot ride along, and a removed artifact is
  # staged as the removal it is.
  git add -- "${changed[@]}"
  if git diff --cached --quiet; then
    echo "auto-pr: the drift under the owned paths was not stageable — nothing to deliver."
    return 0
  fi

  git -c user.name="$BOT_NAME" -c user.email="$BOT_EMAIL" \
    commit --quiet -m "$title" \
    -m "Automated by the ${GITHUB_WORKFLOW:-auto-pr} workflow (run ${GITHUB_RUN_ID:-local})."

  git push --force origin "HEAD:refs/heads/${branch}"

  local open_pr
  open_pr="$(gh pr list --head "$branch" --state open --json number --jq '.[0].number // empty' || true)"
  if [ -n "$open_pr" ]; then
    echo "auto-pr: refreshed pull request #${open_pr} on ${branch}."
    gh pr comment "$open_pr" --body "Refreshed by run ${GITHUB_RUN_ID:-local} (force-push)."
  else
    open_new_pr "$branch" "$title"
  fi
  # The commit stays on the local (usually detached) checkout; only the bot
  # branch carries it remotely.
}

# ── self-test ────────────────────────────────────────────────────────────────
#
# Every case runs the real flow against a real repository with a real remote, so
# the push, the staging and the pathspec matching are git's own. `gh` is the one
# stub: it records its argv, and answers `pr list` from a file the case sets.

SELFTEST_STATUS=0

# stub_gh installs a `gh` on PATH that logs every invocation to $GH_LOG and
# answers `gh pr list` with the contents of $GH_OPEN_PR. With $GH_CREATE_REFUSED
# non-empty, `gh pr create` refuses the way a repository that forbids Actions
# pull requests refuses — on stderr, non-zero.
stub_gh() {
  local bin="$1/bin"
  mkdir -p "$bin"
  cat >"$bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$GH_LOG"
if [ "${1:-}" = "pr" ] && [ "${2:-}" = "list" ]; then
  cat "$GH_OPEN_PR" 2>/dev/null || true
fi
if [ "${1:-}" = "pr" ] && [ "${2:-}" = "create" ] && [ -n "${GH_CREATE_REFUSED:-}" ]; then
  echo "pull request create failed: GraphQL: GitHub Actions is not permitted to create or approve pull requests (createPullRequest)" >&2
  exit 1
fi
exit 0
STUB
  chmod +x "$bin/gh"
  PATH="$bin:$PATH"
  export PATH
}

# planted_repo builds a checkout with an owned artifact and an unowned source
# file, wired to a bare remote so pushes are real.
planted_repo() {
  local dir="$1" remote="$2"
  git init -q --bare "$remote"
  mkdir -p "$dir/owned" "$dir/src"
  printf 'one\n' >"$dir/owned/a.json"
  printf 'package src\n' >"$dir/src/main.go"
  git -C "$dir" -c init.defaultBranch=main init -q
  git -C "$dir" remote add origin "$remote"
  git -C "$dir" -c user.email=bot@example.invalid -c user.name=bot add -A
  git -C "$dir" -c user.email=bot@example.invalid -c user.name=bot \
    -c commit.gpgsign=false commit -qm planted
  git -C "$dir" push -q origin main
}

# ok records a passing case; fail records a failing one with context.
ok() { echo "✓ self-test: $1"; }
fail() {
  echo "✖ self-test: $1"
  shift
  printf '%s\n' "$@" | sed 's/^/    /'
  SELFTEST_STATUS=1
}

# run_case runs the flow in the scratch repo and prints its combined output.
run_case() {
  local repo="$1"
  shift
  (cd "$repo" && auto_pr "$@" 2>&1)
}

self_test() {
  local start tmp
  start="$PWD"
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand $tmp now, not at trap time
  trap "cd '$start'; rm -rf '$tmp'" EXIT

  local repo="$tmp/repo" remote="$tmp/remote.git"
  planted_repo "$repo" "$remote"

  stub_gh "$tmp"
  export GH_LOG="$tmp/gh.log"
  export GH_OPEN_PR="$tmp/open_pr"
  : >"$GH_LOG"
  : >"$GH_OPEN_PR"

  local out
  local -a owned=(owned ':(glob)side/*/note.*.yaml')

  # A quiet stream delivers nothing: no commit, no push, no pull request. This
  # is the idempotence case — a second run over an unchanged tree must not open
  # a second pull request or leave a branch behind.
  out="$(run_case "$repo" bot/stream "chore: deliver" "${owned[@]}")"
  # Every assertion below matches over a here-string rather than through a pipe.
  # Under `set -o pipefail`, `printf … | grep -q` is a race: grep exits on the
  # first match, printf takes EPIPE, and the pipeline reports the WRITE failure
  # — so the louder the evidence, the likelier the test is to fail on it.
  if grep -q "did not move" <<<"$out" && [ ! -s "$GH_LOG" ] &&
    ! git -C "$remote" show-ref --verify --quiet refs/heads/bot/stream; then
    ok "a quiet stream opens nothing and leaves no branch"
  else
    fail "a quiet stream opened something" "$out" "gh: $(cat "$GH_LOG")"
  fi

  # Drift under the owned paths becomes a pull request.
  printf 'two\n' >"$repo/owned/a.json"
  out="$(run_case "$repo" bot/stream "chore: deliver" "${owned[@]}")"
  if grep -q "pr create" "$GH_LOG" &&
    git -C "$remote" show-ref --verify --quiet refs/heads/bot/stream; then
    ok "drift under the owned paths opens a pull request on the bot branch"
  else
    fail "drift did not open a pull request" "$out" "gh: $(cat "$GH_LOG")"
  fi

  # An unowned change is never staged, even while an owned one is delivered: the
  # commit that reaches the branch carries the owned path only.
  git -C "$repo" reset -q --hard HEAD~1
  printf 'three\n' >"$repo/owned/a.json"
  printf 'package src // edited\n' >"$repo/src/main.go"
  : >"$GH_LOG"
  run_case "$repo" bot/stream "chore: deliver" "${owned[@]}" >/dev/null
  local delivered
  delivered="$(git -C "$repo" show --name-only --format= HEAD)"
  if grep -qx "owned/a.json" <<<"$delivered" &&
    ! grep -qx "src/main.go" <<<"$delivered"; then
    ok "an unowned change is not staged alongside an owned one"
  else
    fail "the delivery carried an unowned change" "$delivered"
  fi

  # A second run while a pull request is open refreshes it rather than opening a
  # sibling — one rolling pull request per stream.
  git -C "$repo" reset -q --hard HEAD~1
  git -C "$repo" checkout -q -- .
  printf '7\n' >"$GH_OPEN_PR"
  printf 'four\n' >"$repo/owned/a.json"
  : >"$GH_LOG"
  out="$(run_case "$repo" bot/stream "chore: deliver" "${owned[@]}")"
  if grep -q "pr comment 7" "$GH_LOG" && ! grep -q "pr create" "$GH_LOG"; then
    ok "an open pull request is refreshed, not duplicated"
  else
    fail "a second run did not refresh the open pull request" "$out" "gh: $(cat "$GH_LOG")"
  fi
  : >"$GH_OPEN_PR"

  # Glob-magic pathspecs reach git intact, so a first untracked sidecar under a
  # wildcard directory is delivered rather than missed.
  git -C "$repo" reset -q --hard HEAD~1
  git -C "$repo" checkout -q -- .
  mkdir -p "$repo/side/one"
  printf 'note: hei\n' >"$repo/side/one/note.nb.yaml"
  : >"$GH_LOG"
  run_case "$repo" bot/stream "chore: deliver" "${owned[@]}" >/dev/null
  delivered="$(git -C "$repo" show --name-only --format= HEAD)"
  if grep -qx "side/one/note.nb.yaml" <<<"$delivered"; then
    ok "an untracked file under a glob pathspec is delivered"
  else
    fail "the glob pathspec did not reach git intact" "$delivered"
  fi

  # A deleted artifact is a delivery too: `git add` with a pathspec stages the
  # removal, so the pull request proposes it rather than silently keeping it.
  git -C "$repo" reset -q --hard HEAD~1
  git -C "$repo" clean -qfd
  rm -f "$repo/owned/a.json"
  : >"$GH_LOG"
  run_case "$repo" bot/stream "chore: deliver" "${owned[@]}" >/dev/null
  if grep -q "^D.*owned/a.json" <<<"$(git -C "$repo" show --name-status --format= HEAD)"; then
    ok "a removed artifact is delivered as a removal"
  else
    fail "a removal was not delivered" "$(git -C "$repo" show --name-status --format= HEAD)"
  fi

  # A repository that forbids Actions pull requests still gets the content: the
  # branch is pushed and current, and the run says where to open it instead of
  # re-raising `gh`'s message about a permission the reader cannot place.
  git -C "$repo" reset -q --hard HEAD~1
  git -C "$repo" clean -qfd
  printf 'five\n' >"$repo/owned/a.json"
  : >"$GH_LOG"
  local rc=0
  # Exported, not prefixed: the stub is an external script, so a shell variable
  # it cannot read would make this case pass for the wrong reason.
  export GH_CREATE_REFUSED=1 GITHUB_REPOSITORY=neokapi/neokapi
  out="$(run_case "$repo" bot/refused "chore: deliver" "${owned[@]}")" || rc=$?
  unset GH_CREATE_REFUSED GITHUB_REPOSITORY
  if [ "$rc" -ne 0 ] &&
    git -C "$remote" show-ref --verify --quiet refs/heads/bot/refused &&
    grep -q "compare/main...bot/refused" <<<"$out" &&
    grep -q "not permitted to open pull requests" <<<"$out"; then
    ok "a refused pull request still delivers the branch, and says where to open it"
  else
    fail "a refused pull request was not reported usefully (rc=$rc)" "$out" "gh: $(cat "$GH_LOG")"
  fi

  cd "$start"
  return "$SELFTEST_STATUS"
}

# ── entry point ──────────────────────────────────────────────────────────────

main() {
  if [ "${1:-}" = "--self-test" ]; then
    self_test
    return
  fi
  if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
    usage
    return 0
  fi

  if [ $# -lt 4 ]; then
    usage >&2
    return 2
  fi
  local branch="$1" title="$2"
  shift 2
  if [ "${1:-}" != "--" ]; then
    usage >&2
    return 2
  fi
  shift
  if [ $# -eq 0 ]; then
    echo "auto-pr: no owned pathspecs — refusing to stage an unbounded tree" >&2
    return 2
  fi

  cd "$(git rev-parse --show-toplevel)"
  auto_pr "$branch" "$title" "$@"
}

main "$@"
