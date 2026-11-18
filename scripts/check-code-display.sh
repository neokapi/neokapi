#!/usr/bin/env bash
# Coded text is never displayed: a surface shows an inline code by its
# CodeDisplay (packages/kapi-format/src/code-display.ts), never by its native
# `data`, its `equiv` or its `disp`. Those fields hold source slices and
# extractor identifiers (`<span className="x">`, `{serverURL}`, `=m0`), and a
# review card once drew `<=m0><serverURL/></=m0>` from them.
#
# This fails when user-interface code outside the allowlist reads one of those
# fields off a run. The allowlist holds code that carries the fields as data
# (wire builders, the edit-text bridge, format writers and the KBF lab) or
# that already passes them through humanCodeName or an HTML-tag check. Adding
# a file to it needs the same: the file must not be able to put a code's
# native form on screen.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

allow=(
  'packages/kapi-format/src/'
  'packages/i18n-react/src/'
  'packages/contract-types/src/'
  'packages/bowrain-client/src/changes.ts'
  'packages/kapi-lab/src/KbfExplorer.tsx'
  'packages/ui/src/components/editor/codedText.ts'
  'packages/ui/src/components/editor/runsCodedBridge.ts'
  'packages/ui/src/components/editor/pluralCellPreview.ts'
  'packages/ui/src/components/editor/codeLabels.ts'
  'packages/ui/src/components/editor/editText.ts'
  'packages/ui/src/components/editor/tagSemantics.ts'
  'packages/ui/src/components/editor/IntellisensePlugin.tsx'
  'packages/ui/src/components/plural/runs-text.ts'
  'packages/ui/src/components/preview/projectionRender.ts'
  'packages/ui/src/components/preview/renderDoc.ts'
  'packages/ui/src/components/preview/RenderedDocument.tsx'
  'packages/ui/src/components/preview/RunSequence.tsx'
  'packages/ui/src/components/preview/runSummary.ts'
  'packages/ui/src/vocabularies/index.ts'
  'bowrain/packages/ui/src/api/contentChanges.ts'
  'bowrain/packages/ui/src/components/blockAdapter.ts'
  'bowrain/packages/ui/src/components/editor/DocumentPreview.tsx'
)

pattern='\b(ph|pcOpen|pcClose|sub|span|code)\??\.(data|equiv|disp|equiv_text|display_text)\b'
roots=(bowrain/mobile/src packages bowrain/packages apps/kapi-desktop/frontend/src bowrain/apps)

hits=$(git grep -nP "$pattern" -- "${roots[@]}" \
  | grep -E '\.(ts|tsx)\:' \
  | grep -vE '(\.test|\.spec|\.stories|\.gen)\.tsx?:|/__tests__/|/stories/|/demo/|/e2e/|node_modules' \
  || true)

bad=""
while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  file=${line%%:*}
  ok=0
  for a in "${allow[@]}"; do
    if [[ "$file" == "$a"* ]]; then ok=1; break; fi
  done
  [[ $ok -eq 0 ]] && bad+="$line"$'\n'
done <<<"$hits"

if [[ -n "$bad" ]]; then
  echo "check-code-display: user-interface code reads a code's native form:" >&2
  printf '%s' "$bad" >&2
  echo "Show a code through codeDisplays/CodeChip, or humanCodeName with a kind word." >&2
  exit 1
fi
echo "check-code-display: no surface reads a code's native form"
