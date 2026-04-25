#!/usr/bin/env bash
#
# scaffold-walkthrough.sh — emit a walkthrough scaffold (prompt + scene
# spec + draft MDX) for a bowrain web or desktop scenario.
#
# Repeatable, deterministic. Used by sibling scripts to crank out the
# 14 desktop + 9 web walkthroughs from the deleted recordings/screenshots
# specs in one pass.
#
# Usage
# -----
#   scripts/scaffold-walkthrough.sh \
#     --id bowrain-web-tm-explorer \
#     --kind web \
#     --title "Browse the translation memory" \
#     --label "TM explorer (web)" \
#     --story "Three sentences of context" \
#     --scene-id browse \
#     --duration 45 \
#     --description "What the recording shows" \
#     [--audience translator|developer|admin] \
#     [--seed-recipe '{"workspace":"fresh"}']

set -euo pipefail

ID=""
KIND=""
TITLE=""
LABEL=""
STORY=""
SCENE_ID=""
DURATION=60
DESCRIPTION=""
AUDIENCE="translator"
SEED='{ "workspace": "fresh" }'

while [ $# -gt 0 ]; do
  case "$1" in
    --id)          ID="$2"; shift ;;
    --kind)        KIND="$2"; shift ;;
    --title)       TITLE="$2"; shift ;;
    --label)       LABEL="$2"; shift ;;
    --story)       STORY="$2"; shift ;;
    --scene-id)    SCENE_ID="$2"; shift ;;
    --duration)    DURATION="$2"; shift ;;
    --description) DESCRIPTION="$2"; shift ;;
    --audience)    AUDIENCE="$2"; shift ;;
    --seed-recipe) SEED="$2"; shift ;;
    *)             echo "unknown flag: $1" >&2; exit 2 ;;
  esac
  shift
done

[ -n "$ID" ] && [ -n "$KIND" ] && [ -n "$TITLE" ] && [ -n "$STORY" ] && [ -n "$SCENE_ID" ] \
  || { echo "missing required flags" >&2; exit 2; }
[ -n "$LABEL" ] || LABEL="$TITLE"
[ -n "$DESCRIPTION" ] || DESCRIPTION="$STORY"

REPO_ROOT="$(git rev-parse --show-toplevel)"
WT_DIR="$REPO_ROOT/bowrain/website/walkthroughs"
SCENE_DIR="$REPO_ROOT/bowrain/website/scenes/$ID"
DOC_DIR="$REPO_ROOT/bowrain/website/docs/walkthroughs"

mkdir -p "$WT_DIR" "$SCENE_DIR/fixtures" "$DOC_DIR"

# ── prompt ──────────────────────────────────────────────────────────
cat > "$WT_DIR/$ID.md" <<EOF
---
id: $ID
audience: $AUDIENCE
target_doc: docs/walkthroughs/$ID.mdx
backend_url: \${BOWRAIN_BACKEND_URL}
scenes:
  - id: $SCENE_ID
    kind: $KIND
    duration_budget_seconds: $DURATION
    seed: $SEED
    smoke_contract:
      - GET \${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

$STORY

## Scene 1 — $SCENE_ID ($KIND)

$DESCRIPTION

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
EOF

# ── scene spec ──────────────────────────────────────────────────────
cat > "$SCENE_DIR/01-$SCENE_ID.spec.ts" <<EOF
/**
 * Walkthrough: $ID
 * Scene 1: $SCENE_ID ($KIND)
 *
 * Generated from bowrain/website/walkthroughs/$ID.md.
 * Do not edit by hand — change the prompt and regenerate via /walkthrough-scenes.
 *
 * Scaffold pending real-backend validation. Run against BOWRAIN_BACKEND_URL
 * with a seeded workspace via BowrainAPI; cleanup in afterAll.
 */

import { test, expect } from "@playwright/test";
import { TEST_IDS } from "@neokapi/ui/test-ids";

const BACKEND_URL = process.env.BOWRAIN_BACKEND_URL || "https://dev.bowrain.cloud";

test.describe("walkthrough: $ID", () => {
  test.use({ viewport: { width: 1280, height: 800 } });

  // TODO(#425): seed via BowrainAPI; cleanup in afterAll.

  test("$SCENE_ID [scene]", async ({ page }) => {
    test.skip(true, "scaffold — needs real backend validation per #425 followup");
    expect(BACKEND_URL).toBeTruthy();
    expect(TEST_IDS).toBeTruthy();
  });
});
EOF

# ── MDX ─────────────────────────────────────────────────────────────
cat > "$DOC_DIR/$ID.mdx" <<EOF
---
id: $ID
title: "$TITLE"
sidebar_label: "$LABEL"
description: "$DESCRIPTION"
draft: true
---

import { ThemedVideo } from "@neokapi/docs-shared";

# $TITLE

> Generated from [\`walkthroughs/$ID.md\`](https://github.com/neokapi/neokapi/blob/main/bowrain/website/walkthroughs/$ID.md). Do not edit by hand — change the prompt and regenerate.
>
> **Status: scaffold.** The scene \`.spec.ts\` exists but is currently \`test.skip\`'d — full Playwright validation against the real backend is a #425 followup.

$STORY

## Scene 1 — $SCENE_ID

{/* <ThemedVideo
  sources={{
    light: "/video/bowrain/$SCENE_ID.webm",
    dark: "/video/bowrain/$SCENE_ID.webm",
  }}
/> */}

$DESCRIPTION

## Next

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started) for the broader workflow.
EOF

echo "scaffolded $ID ($KIND) — prompt + scene + mdx"
