#!/usr/bin/env bash
# fake-translator.sh — simulates a CAT-tool fill-in pass for the
# bilingual-workflow VHS demo. For every XLIFF file passed as an
# argument, insert a <target>…</target> after each <source>…</source>
# that doesn't already have one, using a small French/German phrasebook
# with a locale-prefixed fallback for unseen strings.
#
# Usage:
#   ./fake-translator.sh out/*.xliff
#
# This is deliberately a tiny Python inline script (no dependencies)
# rather than a full bash parser — XLIFF is XML, and regex/bash is a
# sharp edge with XML. The Python here is intentionally obvious.
set -euo pipefail

if [ $# -eq 0 ]; then
  echo "Usage: $0 <xliff> [<xliff> ...]" >&2
  exit 1
fi

python3 - "$@" <<'PY'
import os, re, sys

# Minimal phrasebook. Everything else gets a locale-prefixed placeholder
# so the demo is still obviously translator output.
PHRASEBOOK = {
    "fr-FR": {
        "Welcome back!": "Content de vous revoir !",
        "This is your dashboard. Everything you need is here.":
            "Voici votre tableau de bord. Tout ce qu'il vous faut se trouve ici.",
        "Save":     "Enregistrer",
        "Cancel":   "Annuler",
        "Settings": "Paramètres",
    },
    "de-DE": {
        "Welcome back!": "Willkommen zurück!",
        "This is your dashboard. Everything you need is here.":
            "Dies ist Ihr Dashboard. Alles, was Sie brauchen, ist hier.",
        "Save":     "Speichern",
        "Cancel":   "Abbrechen",
        "Settings": "Einstellungen",
    },
}

LOCALE_RE = re.compile(r"en-US-to-([a-z]{2}-[A-Z]{2})\.")

# Matches <segment id="sN"> ... <source>TEXT</source>\n  </segment> where
# no <target> is present. Captures:
#   1: whitespace/indent preserving the opening tag
#   2: existing <source> block (we keep it verbatim)
#   3: source text
#   4: whitespace up to </segment>
SEGMENT_RE = re.compile(
    r"(<segment\b[^>]*>)(\s*<source>(.*?)</source>)(\s*)</segment>",
    re.DOTALL,
)

def translate(text, locale):
    return PHRASEBOOK.get(locale, {}).get(text, f"[{locale}] {text}")

def xml_escape(s):
    return (s.replace("&", "&amp;")
             .replace("<", "&lt;")
             .replace(">", "&gt;"))

def fill_targets(xml, locale):
    def sub(m):
        open_tag, source_block, source_text, trailing = m.group(1), m.group(2), m.group(3), m.group(4)
        translated = translate(source_text.strip(), locale)
        target_block = f"\n        <target>{xml_escape(translated)}</target>"
        return f"{open_tag}{source_block}{target_block}{trailing}</segment>"
    return SEGMENT_RE.sub(sub, xml)

changed = 0
for path in sys.argv[1:]:
    m = LOCALE_RE.search(os.path.basename(path))
    if not m:
        print(f"skip (cannot infer locale): {path}", file=sys.stderr)
        continue
    locale = m.group(1)
    with open(path, "r", encoding="utf-8") as f:
        original = f.read()
    # Only touch segments without an existing <target> — the kapi extract
    # path pre-fills TM-matched targets, and the "translator" should
    # leave those alone.
    if "<target>" in original and "<source>" in original:
        # Careful: skip pre-filled segments by only replacing segments
        # whose content does not already contain <target>.
        def guarded_sub(m):
            block = m.group(0)
            if "<target>" in block:
                return block
            return fill_targets(block, locale)
        new = re.sub(
            r"<segment\b[^>]*>.*?</segment>",
            guarded_sub,
            original,
            flags=re.DOTALL,
        )
    else:
        new = fill_targets(original, locale)
    if new != original:
        with open(path, "w", encoding="utf-8") as f:
            f.write(new)
        changed += 1
        print(f"translated: {path} ({locale})")
    else:
        print(f"no changes: {path}")

if changed == 0:
    # Not an error — maybe the file was already fully translated.
    print("note: no segments required translation", file=sys.stderr)
PY
