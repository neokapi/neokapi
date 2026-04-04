#!/bin/bash
# gen-storybook-fixtures.sh — Generate Storybook fixtures from real data.
#
# Combines:
#   1. Built-in neokapi format schemas (via kapi CLI)
#   2. Built-in neokapi tool schemas (via kapi CLI)
#   3. Okapi bridge format schemas (from okapi-bridge/dist/plugin/)
#   4. Okapi bridge tool schemas (from okapi-bridge/dist/plugin/)
#   5. Plugin documentation (from okapi-bridge/dist/plugin/docs/)
#   6. Presets (from okapi-bridge/dist/plugin/formats/*/presets/)
#
# Usage:
#   ./scripts/gen-storybook-fixtures.sh [--bridge-dir PATH]
#
# Default bridge dir: ../okapi-bridge/dist/plugin

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
BRIDGE_DIR="${1:---bridge-dir}"

# Parse args
if [ "$BRIDGE_DIR" = "--bridge-dir" ]; then
    shift 2>/dev/null || true
    BRIDGE_DIR="${1:-$ROOT_DIR/../okapi-bridge/dist/plugin}"
fi

OUTPUT_DIR="$ROOT_DIR/apps/kapi-desktop/frontend/src/stories/fixtures"
KAPI_DIR="$ROOT_DIR"

GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'
info() { echo -e "${GREEN}▸${NC} $1"; }
warn() { echo -e "${YELLOW}▸${NC} $1"; }

mkdir -p "$OUTPUT_DIR"

# ── Built-in format list ──────────────────────────────────────────────────

info "Collecting built-in format list..."
BUILTIN_FORMATS=$(cd "$KAPI_DIR" && go run ./kapi/cmd/kapi formats --json --disable-plugins okapi 2>/dev/null)

# ── Built-in format schemas ───────────────────────────────────────────────

info "Dumping built-in format schemas..."
BUILTIN_FORMAT_SCHEMAS="[]"
for name in $(echo "$BUILTIN_FORMATS" | jq -r '.formats[].name'); do
    schema=$(cd "$KAPI_DIR" && go run ./kapi/cmd/kapi formats schema "$name" --disable-plugins okapi 2>/dev/null) || continue
    if [ -n "$schema" ] && [ "$schema" != "null" ]; then
        # Add source and name metadata
        schema=$(echo "$schema" | jq --arg n "$name" '. + {"x-source": "built-in", "x-name": $n}')
        BUILTIN_FORMAT_SCHEMAS=$(echo "$BUILTIN_FORMAT_SCHEMAS" | jq --argjson s "$schema" '. + [$s]')
    fi
done
builtin_fmt_count=$(echo "$BUILTIN_FORMAT_SCHEMAS" | jq 'length')
info "  $builtin_fmt_count built-in format schemas collected"

# ── Built-in tool list ────────────────────────────────────────────────────

info "Collecting built-in tool list..."
BUILTIN_TOOLS=$(cd "$KAPI_DIR" && go run ./kapi/cmd/kapi tools --json --disable-plugins okapi 2>/dev/null)

# ── Built-in tool schemas ─────────────────────────────────────────────────

info "Dumping built-in tool schemas..."
BUILTIN_TOOL_SCHEMAS="[]"
for name in $(echo "$BUILTIN_TOOLS" | jq -r '.tools[].name'); do
    schema=$(cd "$KAPI_DIR" && go run ./kapi/cmd/kapi tools schema "$name" --disable-plugins okapi 2>/dev/null) || continue
    if [ -n "$schema" ] && [ "$schema" != "null" ]; then
        # Find tool metadata from the tool list
        tool_meta=$(echo "$BUILTIN_TOOLS" | jq --arg n "$name" '.tools[] | select(.name == $n)')
        category=$(echo "$tool_meta" | jq -r '.category // "other"')
        desc=$(echo "$tool_meta" | jq -r '.description // ""')
        schema=$(echo "$schema" | jq --arg n "$name" --arg c "$category" --arg d "$desc" \
            '. + {"x-source": "built-in", "x-name": $n, "x-tool": {"id": $n, "displayName": .title, "description": $d, "category": $c}}')
        BUILTIN_TOOL_SCHEMAS=$(echo "$BUILTIN_TOOL_SCHEMAS" | jq --argjson s "$schema" '. + [$s]')
    fi
done
builtin_tool_count=$(echo "$BUILTIN_TOOL_SCHEMAS" | jq 'length')
info "  $builtin_tool_count built-in tool schemas collected"

# ── Bridge format schemas ─────────────────────────────────────────────────

BRIDGE_FORMAT_SCHEMAS="[]"
BRIDGE_PRESETS="{}"
if [ -d "$BRIDGE_DIR/formats" ]; then
    info "Reading bridge format schemas from $BRIDGE_DIR/formats/..."
    for format_dir in "$BRIDGE_DIR/formats"/*/; do
        format_id=$(basename "$format_dir")
        schema_file="$format_dir/schema.json"
        [ -f "$schema_file" ] || continue

        schema=$(jq --arg n "$format_id" '. + {"x-source": "okapi-bridge", "x-name": $n}' "$schema_file")
        BRIDGE_FORMAT_SCHEMAS=$(echo "$BRIDGE_FORMAT_SCHEMAS" | jq --argjson s "$schema" '. + [$s]')

        # Collect presets
        if [ -d "$format_dir/presets" ]; then
            for preset_file in "$format_dir/presets"/*.json; do
                [ -f "$preset_file" ] || continue
                preset_id=$(basename "$preset_file" .json)
                preset=$(jq '.' "$preset_file")
                BRIDGE_PRESETS=$(echo "$BRIDGE_PRESETS" | jq --arg fid "$format_id" --arg pid "$preset_id" --argjson p "$preset" \
                    '.[$fid] = ((.[$fid] // {}) + {($pid): $p})')
            done
        fi
    done
    bridge_fmt_count=$(echo "$BRIDGE_FORMAT_SCHEMAS" | jq 'length')
    info "  $bridge_fmt_count bridge format schemas collected"
else
    warn "No bridge formats found at $BRIDGE_DIR/formats/"
fi

# ── Bridge tool schemas ───────────────────────────────────────────────────

BRIDGE_TOOL_SCHEMAS="[]"
if [ -d "$BRIDGE_DIR/tools" ]; then
    info "Reading bridge tool schemas from $BRIDGE_DIR/tools/..."
    for tool_dir in "$BRIDGE_DIR/tools"/*/; do
        tool_id=$(basename "$tool_dir")
        schema_file="$tool_dir/schema.json"
        [ -f "$schema_file" ] || continue

        schema=$(jq --arg n "$tool_id" '. + {"x-source": "okapi-bridge", "x-name": $n}' "$schema_file")
        BRIDGE_TOOL_SCHEMAS=$(echo "$BRIDGE_TOOL_SCHEMAS" | jq --argjson s "$schema" '. + [$s]')
    done
    bridge_tool_count=$(echo "$BRIDGE_TOOL_SCHEMAS" | jq 'length')
    info "  $bridge_tool_count bridge tool schemas collected"
else
    warn "No bridge tools found at $BRIDGE_DIR/tools/"
fi

# ── Plugin docs ───────────────────────────────────────────────────────────

PLUGIN_DOCS="{}"
if [ -f "$BRIDGE_DIR/../plugin-docs.json" ]; then
    info "Reading plugin docs..."
    PLUGIN_DOCS=$(jq '.' "$BRIDGE_DIR/../plugin-docs.json")
elif [ -f "$BRIDGE_DIR/docs/plugin-docs.json" ]; then
    PLUGIN_DOCS=$(jq '.' "$BRIDGE_DIR/docs/plugin-docs.json")
fi

# Also check the current fixtures location as fallback
if [ "$PLUGIN_DOCS" = "{}" ] && [ -f "$OUTPUT_DIR/plugin-docs.json" ]; then
    info "Using existing plugin-docs.json from fixtures"
    PLUGIN_DOCS=$(jq '.' "$OUTPUT_DIR/plugin-docs.json")
fi

# ── Concepts ──────────────────────────────────────────────────────────────

CONCEPTS="{}"
if [ -f "$BRIDGE_DIR/docs/concepts.json" ]; then
    info "Reading concepts documentation..."
    CONCEPTS=$(jq '.' "$BRIDGE_DIR/docs/concepts.json")
fi

# ── Manifest ──────────────────────────────────────────────────────────────

MANIFEST="{}"
if [ -f "$BRIDGE_DIR/manifest.json" ]; then
    info "Reading plugin manifest..."
    MANIFEST=$(jq '.' "$BRIDGE_DIR/manifest.json")
fi

# ── Write fixture files ──────────────────────────────────────────────────

info "Writing fixture files..."

# 1. All format schemas (built-in + bridge)
jq -n --argjson builtin "$BUILTIN_FORMAT_SCHEMAS" --argjson bridge "$BRIDGE_FORMAT_SCHEMAS" \
    '{ builtIn: $builtin, bridge: $bridge, all: ($builtin + $bridge) }' \
    > "$OUTPUT_DIR/format-schemas.json"
total_fmts=$(jq '.all | length' "$OUTPUT_DIR/format-schemas.json")
info "  format-schemas.json ($total_fmts formats)"

# 2. All tool schemas (built-in + bridge)
jq -n --argjson builtin "$BUILTIN_TOOL_SCHEMAS" --argjson bridge "$BRIDGE_TOOL_SCHEMAS" \
    '{ builtIn: $builtin, bridge: $bridge, all: ($builtin + $bridge) }' \
    > "$OUTPUT_DIR/tool-schemas.json"
total_tools=$(jq '.all | length' "$OUTPUT_DIR/tool-schemas.json")
info "  tool-schemas.json ($total_tools tools)"

# 3. Format list (metadata only, no full schemas)
jq -n --argjson bf "$BUILTIN_FORMATS" --argjson bridge "$BRIDGE_FORMAT_SCHEMAS" \
    '{
        builtIn: $bf.formats,
        bridge: [$bridge[] | {
            name: ."x-name",
            display_name: .title,
            source: "okapi-bridge",
            extensions: (."x-format".extensions // []),
            mime_types: (."x-format".mimeTypes // [])
        }]
    }' > "$OUTPUT_DIR/format-list.json"
info "  format-list.json"

# 4. Tool list (metadata only)
jq -n --argjson bt "$BUILTIN_TOOLS" --argjson bridge "$BRIDGE_TOOL_SCHEMAS" \
    '{
        builtIn: $bt.tools,
        bridge: [$bridge[] | {
            name: ."x-name",
            display_name: (."x-tool".displayName // .title),
            description: (."x-tool".description // .description // ""),
            category: (."x-tool".category // "other"),
            source: "okapi-bridge",
            has_schema: true,
            inputs: (."x-tool".inputs // []),
            tags: (."x-tool".tags // [])
        }]
    }' > "$OUTPUT_DIR/tool-list.json"
info "  tool-list.json"

# 5. Presets
echo "$BRIDGE_PRESETS" | jq '.' > "$OUTPUT_DIR/presets.json"
preset_count=$(echo "$BRIDGE_PRESETS" | jq '[.[] | keys[]] | length')
info "  presets.json ($preset_count presets across formats)"

# 6. Plugin docs (pass-through if available)
echo "$PLUGIN_DOCS" | jq '.' > "$OUTPUT_DIR/plugin-docs.json"
info "  plugin-docs.json"

# 7. Concepts
echo "$CONCEPTS" | jq '.' > "$OUTPUT_DIR/concepts.json"
info "  concepts.json"

# 8. Manifest
echo "$MANIFEST" | jq '.' > "$OUTPUT_DIR/manifest.json"
info "  manifest.json"

echo ""
info "Done. Fixtures written to $OUTPUT_DIR/"
info "  Formats: $total_fmts ($builtin_fmt_count built-in + ${bridge_fmt_count:-0} bridge)"
info "  Tools:   $total_tools ($builtin_tool_count built-in + ${bridge_tool_count:-0} bridge)"
