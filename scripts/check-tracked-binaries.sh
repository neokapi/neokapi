#!/usr/bin/env bash
#
# Guard: no compiled executable may be tracked in git.
#
# This repo is a Go monorepo with many `main` packages, and the natural way to
# get one wrong is to run a bare `go build ./scripts/checkeval` from the root:
# `go build` writes the binary next to the cwd, named after the package, and
# `git add -A` then sweeps it in. The result is a platform-specific executable —
# in practice always macOS arm64, because that is the machine it was built on —
# that every clone, every fork and every CI checkout downloads forever, and that
# nothing in the build ever reads.
#
# It has now happened four times: #1125 (scripts/contexteval/contexteval, 35.8
# MB), #1238, #1258 and #1520 (checkeval, 23.5 MB). Three of those were noticed
# and deleted one at a time; the two above survived because nobody was looking.
# 59 MB of dead Mach-O had accumulated in a repo whose entire source tree is a
# fraction of that.
#
# .gitignore is not the fix, for two reasons this repo has hit in practice:
#
#   1. .gitignore does not untrack. Once a file is in the index, ignore rules
#      are not consulted for it at all, so adding a rule after the fact looks
#      like a fix and changes nothing.
#   2. A leading '/' in a NESTED .gitignore anchors to that file's directory,
#      not to the repo root. scripts/contexteval/.gitignore contained
#      '/scripts/contexteval/contexteval', which resolves to
#      scripts/contexteval/scripts/contexteval/contexteval and matched nothing.
#
# So the check has to be on the index itself. Detection is by magic bytes rather
# than by filename, size or the executable bit: a compiled binary is recognisable
# from its first four bytes, whatever it is called, and the class of file we are
# rejecting is exactly "the linker wrote this".
#
# Legitimately-tracked binary blobs — .docx/.png/.wasm/font fixtures, the sample
# corpora — are not executables and carry none of these signatures, so they do
# not need an allowlist.
#
# Usage:
#     ./scripts/check-tracked-binaries.sh
#
# Wired into `make lint`, `make pre-push`, and the repo-guards CI job.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

# One pass over the index reading four bytes per file. Python because the
# alternative — a shell loop with `head -c 4 | od` per file — is ~7000 forks.
#
# CAFEBABE is shared by Mach-O universal binaries and Java .class files. We
# report it either way: this repo builds Java only in the separate okapi-bridge
# repository, so a tracked .class here is also something nobody meant to commit.
found=$(python3 - <<'PY'
import os
import subprocess

MAGIC = {
    b"\x7fELF": "ELF (Linux/BSD executable)",
    b"\xcf\xfa\xed\xfe": "Mach-O 64-bit (macOS executable)",
    b"\xce\xfa\xed\xfe": "Mach-O 32-bit (macOS executable)",
    b"\xfe\xed\xfa\xcf": "Mach-O 64-bit big-endian (macOS executable)",
    b"\xfe\xed\xfa\xce": "Mach-O 32-bit big-endian (macOS executable)",
    b"\xca\xfe\xba\xbe": "Mach-O universal binary / Java .class",
    b"\xbe\xba\xfe\xca": "Mach-O universal binary (reversed)",
}

listing = subprocess.run(
    ["git", "ls-files", "-z"], capture_output=True, check=True
).stdout

for raw in listing.split(b"\0"):
    if not raw:
        continue
    path = raw.decode("utf-8", "surrogateescape")
    try:
        with open(path, "rb") as fh:
            head = fh.read(4)
    except OSError:
        continue  # deleted in the worktree, or unreadable; not our business
    kind = MAGIC.get(head)
    if kind is None and head[:2] == b"MZ":
        kind = "PE/COFF (Windows executable)"
    if kind:
        size = os.path.getsize(path)
        print(f"{path}\t{kind}\t{size}")
PY
)

if [ -n "$found" ]; then
  echo "✖ compiled executable(s) tracked in git:"
  printf '%s\n' "$found" | while IFS=$'\t' read -r path kind size; do
    printf '  %s — %s, %s bytes\n' "$path" "$kind" "$size"
  done
  cat <<'EOF'

A compiled binary must never be committed. It is built for one operating system
and one architecture, it is large, it is stale the moment the source changes,
and git stores every version of it forever — but a clone on any other platform
cannot even run it.

This is almost always the output of a bare `go build` run from the repo root.
Use the `make` targets instead: they place binaries under bin/ (which is
ignored), and the eval and codegen helpers are invoked with `go run ./scripts/…`
so they are never written to disk at all.

To remove one:

    git rm --cached <path>          # untrack, keep your local copy
    # then add the path to .gitignore

Note that adding it to .gitignore ALONE does nothing for a file that is already
tracked, and that a leading '/' in a nested .gitignore is relative to that
file's own directory — write '/checkeval' in scripts/checkeval/.gitignore, not
'/scripts/checkeval/checkeval'.
EOF
  exit 1
fi

echo "✓ no compiled executables tracked in git"
