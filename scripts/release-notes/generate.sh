#!/usr/bin/env bash
#
# generate.sh — build categorized release notes for a kapi or bowrain release
# tag and write them to scripts/release-notes/<tag>.md, ready to feed to
# action-gh-release's `body_path`.
#
# What it does
# ------------
#   1. Detects the release track from the tag family:
#        v<X.Y.Z>[...]          → kapi     (release.yml)
#        bowrain-v<X.Y.Z>[...]  → bowrain  (release-bowrain.yml)
#   2. Finds the *previous* tag in the SAME family (never crossing tracks) and
#      lists every squash-merge commit in `<prev>..<tag>`.
#        - a prerelease/rc tag counts from the previous tag of ANY kind (so the
#          notes are the delta since the last rc — they accumulate rc-by-rc);
#        - a final/stable tag counts from the previous STABLE tag (rc tags are
#          skipped), so a final's notes span the whole release, not just the
#          sliver since the last rc.
#   3. Categorizes each commit from its conventional-commit prefix on the
#      squash subject (feat / fix / perf / docs / chore; everything else →
#      Other), links the PR number, and emits grouped markdown.
#   4. Appends the track's static "Installation" footer (kept verbatim from the
#      release workflows, so the published body still carries install steps).
#
# Hand-edit rule
# --------------
#   The founder may hand-author or hand-edit scripts/release-notes/<tag>.md
#   between an rc and the final (the file is committed). To never clobber that:
#     - if <tag>.md does NOT exist  → it is generated in place;
#     - if <tag>.md ALREADY exists  → it is left untouched and the freshly
#       generated notes are written to <tag>.generated.md as a side file for
#       diffing. Pass --force to overwrite <tag>.md in place.
#   Either way <tag>.md exists afterwards, so `body_path` always resolves.
#
# Dependencies: git only (+ coreutils). No gh, no network — runs anywhere the
# repo is checked out with full history/tags (CI uses fetch-depth: 0).
#
# Usage:
#   scripts/release-notes/generate.sh v1.2.0
#   scripts/release-notes/generate.sh --force bowrain-v1.2.0-rc13
#   REPO=neokapi/neokapi scripts/release-notes/generate.sh v1.2.0
set -euo pipefail

FORCE=0
TAG=""
while [ $# -gt 0 ]; do
  case "$1" in
    --force) FORCE=1; shift ;;
    -h|--help)
      sed -n '2,45p' "$0"; exit 0 ;;
    -*) echo "unknown flag: $1" >&2; exit 2 ;;
    *) TAG="$1"; shift ;;
  esac
done
[ -n "$TAG" ] || { echo "usage: $0 [--force] <tag>" >&2; exit 2; }

REPO="${REPO:-${GITHUB_REPOSITORY:-neokapi/neokapi}}"
# This script lives at scripts/release-notes/, so repo root is two levels up.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NOTES_DIR="$REPO_ROOT/scripts/release-notes"
PR_BASE="https://github.com/${REPO}/pull"
COMMIT_BASE="https://github.com/${REPO}/commit"

# --- Track / family resolution ------------------------------------------------
case "$TAG" in
  bowrain-v[0-9]*)
    TRACK="bowrain"; PRODUCT="Bowrain"
    VERSION="${TAG#bowrain-v}"
    MATCH="bowrain-v[0-9]*"; EXCL_PRE="bowrain-v[0-9]*-*"
    ;;
  v[0-9]*)
    TRACK="kapi"; PRODUCT="kapi"
    VERSION="${TAG#v}"
    MATCH="v[0-9]*"; EXCL_PRE="v[0-9]*-*"
    ;;
  *)
    echo "tag '$TAG' is not a kapi (v*) or bowrain (bowrain-v*) release tag." >&2
    exit 2
    ;;
esac

# Prerelease iff the version part carries a suffix (e.g. 1.2.0-rc13).
case "$VERSION" in *-*) PRERELEASE=1 ;; *) PRERELEASE=0 ;; esac

# Target commit for the range. Normally the tag object (a tag push checks out
# refs/tags/<tag> with fetch-depth: 0). A COORDINATED release, though, dispatches
# the reusable workflow with a version input and no tag ref, so the tag isn't in
# the checkout yet — fall back to HEAD so a release is never blocked for lack of
# notes. The tag STRING still drives the title, version and family either way.
if git rev-parse -q --verify "${TAG}^{commit}" >/dev/null 2>&1; then
  TARGET="$TAG"
else
  TARGET="HEAD"
  echo "note: tag '$TAG' not in this checkout — computing notes from HEAD." >&2
fi

# --- Previous tag in the same family ------------------------------------------
# git describe from the target's parent finds the nearest preceding reachable
# tag, so rc-accumulation and cross-track isolation both fall out for free.
PREV=""
if [ "$PRERELEASE" -eq 1 ]; then
  PREV="$(git describe --tags --abbrev=0 --match "$MATCH" "${TARGET}^" 2>/dev/null || true)"
else
  PREV="$(git describe --tags --abbrev=0 --match "$MATCH" --exclude "$EXCL_PRE" "${TARGET}^" 2>/dev/null || true)"
fi

if [ -n "$PREV" ]; then
  RANGE="${PREV}..${TARGET}"
  SINCE="_Changes since [\`${PREV}\`](https://github.com/${REPO}/releases/tag/${PREV})._"
else
  RANGE="$TARGET"
  SINCE="_Initial release in this track._"
fi

# --- Collect + categorize commits ---------------------------------------------
# Six category buckets, emitted in this order. bash 3.2-safe: no associative
# arrays (macOS ships bash 3.2), so buckets are plain scalars appended via a
# case, mirroring publish-windows-signed.sh's 3.2 discipline.
b_feat=""; b_fix=""; b_perf=""; b_docs=""; b_chore=""; b_other=""

# %h<US>%s per commit; unit separator (0x1f) can't appear in a subject. tformat
# terminates every record with a newline (format: omits the last), so the read
# loop never drops the final commit.
US=$'\x1f'
count=0
# Regexes live in variables and are matched unquoted — bash 3.2's [[ =~ ]]
# mis-parses parenthesised regex written inline.
pr_re='\(#([0-9]+)\)[[:space:]]*$'
cc_re='^([a-zA-Z]+)(\(([^)]*)\))?(!)?:[[:space:]]+(.*)$'
while IFS="$US" read -r hash subject; do
  [ -n "$hash" ] || continue

  # Trailing "(#NNNN)" is the squash-merge PR number; a subject may also carry
  # inline "#NNN" refs, so anchor to the END of the string.
  pr=""
  if [[ "$subject" =~ $pr_re ]]; then
    pr="${BASH_REMATCH[1]}"
  fi
  # Strip that trailing PR token from the visible text.
  subject_clean="$(printf '%s' "$subject" | sed -E 's/[[:space:]]*\(#[0-9]+\)[[:space:]]*$//')"

  # Conventional-commit prefix: type(scope)!: description
  type=""; scope=""; bang=""; desc="$subject_clean"
  if [[ "$subject_clean" =~ $cc_re ]]; then
    type="$(printf '%s' "${BASH_REMATCH[1]}" | tr '[:upper:]' '[:lower:]')"
    scope="${BASH_REMATCH[3]}"
    bang="${BASH_REMATCH[4]}"
    desc="${BASH_REMATCH[5]}"
  fi

  # "**scope:** description", flag breaking changes, link the PR (or commit).
  entry="$desc"
  [ -n "$scope" ] && entry="**${scope}:** ${entry}"
  [ -n "$bang" ] && entry="**BREAKING** ${entry}"
  if [ -n "$pr" ]; then
    entry="- ${entry} ([#${pr}](${PR_BASE}/${pr}))"
  else
    entry="- ${entry} ([\`${hash}\`](${COMMIT_BASE}/${hash}))"
  fi

  case "$type" in
    feat)  b_feat="${b_feat}${entry}"$'\n' ;;
    fix)   b_fix="${b_fix}${entry}"$'\n' ;;
    perf)  b_perf="${b_perf}${entry}"$'\n' ;;
    docs)  b_docs="${b_docs}${entry}"$'\n' ;;
    chore) b_chore="${b_chore}${entry}"$'\n' ;;
    *)     b_other="${b_other}${entry}"$'\n' ;;
  esac
  count=$((count + 1))
done < <(git log --no-merges --pretty=tformat:"%h${US}%s" "$RANGE")

# --- Track-specific install footer (verbatim from the release workflows) ------
install_footer() {
  if [ "$TRACK" = "kapi" ]; then
    cat <<'EOF'
## Installation

**Homebrew (kapi CLI, Apache-2.0, no plugins):**
```
brew install neokapi/tap/kapi-cli
```

**Homebrew (Kapi desktop app, macOS):**
```
brew install --cask neokapi/tap/kapi
```

The bowrain plugin and Bowrain desktop app are released separately
(tag `bowrain-v*`):
```
brew install neokapi/tap/bowrain-cli      # kapi + bowrain plugin
brew install --cask neokapi/tap/bowrain   # Bowrain desktop app
```

Direct platform binaries (macOS, Linux, Windows) are attached as release
assets below. Windows binaries are Authenticode-signed and uploaded shortly
after the release. Prereleases: add the `-beta` brew track
(`brew install neokapi/tap/kapi-cli-beta`).
EOF
  else
    cat <<'EOF'
## Installation

**Homebrew (kapi + bowrain plugin):**
```
brew install neokapi/tap/bowrain-cli
```

**Homebrew (Bowrain desktop app, macOS):**
```
brew install --cask neokapi/tap/bowrain
```

The kapi CLI (Apache-2.0) and Kapi desktop app are released separately on
`v*` tags. Prereleases: add the `-beta` brew track
(`brew install neokapi/tap/bowrain-cli-beta`).
EOF
  fi
}

# --- Render -------------------------------------------------------------------
emit_section() { # <heading> <body>
  [ -n "$2" ] || return 0
  echo "### $1"
  echo
  printf '%s' "$2"
  echo
}

render() {
  echo "# ${PRODUCT} ${VERSION}"
  echo
  echo "$SINCE"
  echo
  if [ "$count" -eq 0 ]; then
    echo "_No notable changes in this range._"
    echo
  else
    emit_section "Features"      "$b_feat"
    emit_section "Bug Fixes"     "$b_fix"
    emit_section "Performance"   "$b_perf"
    emit_section "Documentation" "$b_docs"
    emit_section "Chores"        "$b_chore"
    emit_section "Other"         "$b_other"
  fi
  install_footer
}

mkdir -p "$NOTES_DIR"
OUT="$NOTES_DIR/${TAG}.md"

if [ -f "$OUT" ] && [ "$FORCE" -eq 0 ]; then
  SIDE="$NOTES_DIR/${TAG}.generated.md"
  render > "$SIDE"
  echo "note: ${OUT#"$REPO_ROOT"/} already exists — left untouched (hand-edit rule)." >&2
  echo "      regenerated notes written to ${SIDE#"$REPO_ROOT"/}; pass --force to overwrite." >&2
  exit 0
fi

render > "$OUT"
echo ">> Wrote ${OUT#"$REPO_ROOT"/} (${count} commit(s), range ${RANGE})."
