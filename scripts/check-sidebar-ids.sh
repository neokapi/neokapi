#!/usr/bin/env bash
#
# Guard: every doc id a Docusaurus sidebar names resolves to a page.
#
# A sidebar entry is a string, so nothing in the toolchain relates it to the
# file it addresses. `vp check` type-checks sidebars.ts and sees a valid array
# of valid strings; `go build` never looks; the tests pass. The mismatch
# surfaces only when Docusaurus loads the version, and it does not warn — it
# refuses to build the locale:
#
#     [ERROR] Loading of version failed for version current
#     [cause]: Error: Invalid sidebar file at "sidebars.ts".
#     These sidebar document ids do not exist:
#     - kapi/recipes/keep-source-compliant
#
# Both sites broke this way in one change: a vocabulary rename rewrote doc ids
# in `web/sidebars.ts` and `bowrain/web/docs/sidebars.ts` without renaming the
# pages, because a sidebar is source code to a sweep and prose to a reader. The
# docs job is late in CI and builds two sites in two locales, so the round trip
# to find out is long — and having fixed one site it is easy to conclude the
# class is fixed, which is exactly what happened.
#
# This runs in a second, before any of that.
#
# A page is addressable by three names, and all three count as resolving:
#   - its path without the extension            docs/server/publish.mdx  → server/publish
#   - an explicit front-matter `id:`            id: publish-guide        → server/publish-guide
#   - the directory it indexes                  docs/server/index.mdx    → server
set -euo pipefail

cd "$(dirname "$0")/.."

exec python3 - "$@" <<'PY'
import os
import re
import sys

# (label, sidebar file, the docs root its ids are relative to)
SITES = [
    ("kapi", "web/sidebars.ts", "web/docs"),
    ("bowrain", "bowrain/web/docs/sidebars.ts", "bowrain/web/docs/docs"),
]

SKIP_DIRS = {"node_modules", ".docusaurus", "build", ".git"}


def addressable(docs_dir):
    """Every id that resolves to a page under docs_dir."""
    ids = set()
    for dirpath, dirnames, filenames in os.walk(docs_dir):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
        for name in filenames:
            if not name.endswith((".md", ".mdx")):
                continue
            rel = os.path.relpath(os.path.join(dirpath, name), docs_dir)
            base = re.sub(r"\.mdx?$", "", rel)
            ids.add(base)
            # An index page also answers to the directory that holds it.
            if base == "index":
                ids.add("")
            elif base.endswith("/index"):
                ids.add(base[: -len("/index")])
            # A front-matter id replaces the filename, keeping the directory.
            with open(os.path.join(dirpath, name), encoding="utf-8", errors="ignore") as fh:
                head = fh.read(2000)
            m = re.search(r"^id:\s*[\"']?([^\"'\s]+)", head, re.M)
            if m:
                parent = os.path.dirname(rel)
                ids.add(os.path.join(parent, m.group(1)) if parent else m.group(1))
    return ids


def referenced(src):
    """Every doc id the sidebar names, with the line it sits on."""
    refs = []
    for m in re.finditer(r'\bid:\s*"([^"]+)"', src):
        refs.append((m.group(1), src[: m.start()].count("\n") + 1))
    # Shorthand entries: a bare string on its own line inside an items array.
    for m in re.finditer(r'^\s*"([a-z0-9][a-zA-Z0-9/_.-]*)",?\s*$', src, re.M):
        refs.append((m.group(1), src[: m.start()].count("\n") + 1))
    return refs


missing = []
for label, sidebar, docs_dir in SITES:
    if not os.path.exists(sidebar):
        print(f"check-sidebar-ids: no sidebar at {sidebar}", file=sys.stderr)
        sys.exit(1)
    with open(sidebar, encoding="utf-8") as fh:
        src = fh.read()
    have = addressable(docs_dir)
    seen = set()
    for doc_id, line in referenced(src):
        if doc_id in seen:
            continue
        seen.add(doc_id)
        if doc_id not in have:
            missing.append((label, sidebar, line, doc_id, docs_dir))

if not missing:
    sys.exit(0)

print("Sidebar entries naming a page that does not exist:")
print()
for label, sidebar, line, doc_id, docs_dir in missing:
    print(f"  {sidebar}:{line}")
    print(f"    id {doc_id!r} has no page under {docs_dir}/")
print()
print("Docusaurus fails the whole locale on this, so it costs a full docs CI")
print("round trip to learn what this line already told you. Either the page was")
print("renamed and the sidebar was not, or the sidebar was renamed and the page")
print("was not — a rename sweep does the second, because a sidebar looks like")
print("code and a doc id looks like prose.")
print()
print("Rename the page to match, or point the entry at the page that exists.")
sys.exit(1)
PY
