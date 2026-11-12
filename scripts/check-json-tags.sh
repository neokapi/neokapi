#!/usr/bin/env bash
#
# Guard: machine-readable output and config keys use snake_case names.
#
# Every document a person or an agent reads from kapi is snake_case: `--json`
# results, NDJSON event streams, MCP tool inputs and outputs, and the recipe,
# including the tool and format config keys a recipe writes under `config:`.
# The one family that keeps lowerCamelCase is the content model on the wire,
# which follows the canonical protobuf JSON mapping (F-04). See "Field names"
# in web/docs/reference/cli-contract.md.
#
# What fails:
#
#   1. A Go struct tag `json:"someName"` (a lowercase start followed by an
#      uppercase letter) in a non-test file under SCAN_PATHS. The tool and
#      format packages are scanned because a config struct's json tag is the
#      config key a recipe writes.
#   2. A camelCase case label (`case "someName":`) in a native format config
#      (core/formats/*/*config*.go), whose ApplyMap switches on the config key.
#
# The Okapi transformers (core/formats/*/transform.go) are not scanned: their
# case labels are Okapi Framework parameter names. A recipe that still writes a
# camelCase config key is refused at load with the key to write; the
# registry-wide counterparts of this guard are
# TestBuiltInToolConfigKeysAreSnakeCase and TestNativeFormatConfigKeysAreSnakeCase.
#
# What passes, deliberately:
#
#   1. ALLOWLIST: files whose JSON is fixed by someone else (an external
#      protocol, a file format, a plugin wire) or is never read by a consumer.
#      Each entry is a repo-relative path, optionally narrowed to one tag with
#      `path:tagName`, followed by the reason.
#   2. Test files, which may decode foreign documents freely.
#
# To cover more packages, add them to SCAN_PATHS and list the exceptions the
# scan then finds in ALLOWLIST.
#
# Usage:
#     ./scripts/check-json-tags.sh              # scan tracked files
#     ./scripts/check-json-tags.sh --self-test  # prove the matcher both ways
#
# Wired into `make check-json-tags` (part of `make lint`), `make pre-push`, and
# the repo-guards job in .github/workflows/ci.yml.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

# The packages whose types reach a consumer as JSON.
SCAN_PATHS=(
  host
  cli
  kapi
  core/convergence
  core/project/storesync.go
  bowrain/plugin
  core/tools
  core/ai/tools
  core/mt/tools
  core/segment
  core/formats
  plugins
)

# The native format configs, whose ApplyMap case labels are config keys.
CONFIG_PATHS=(
  'core/formats/*config*.go'
)

# path[:tag]  reason
ALLOWLIST='
host/asr_plugin.go                 wire to the kapi-asr plugin (plugins/asr/asrproto)
host/vision_plugin.go              wire to the kapi-vision plugin
host/connect.go                    reads Claude Code settings.json (enabledPlugins)
host/hook.go                       Claude Code hook protocol
host/hookoutcomes.go               reads Claude Code transcripts
host/exectrust.go                  private trust file under the config dir
host/kpzcache.go                   private cache metadata
host/kpzdivergence.go              reads the private kpz cache metadata
host/flowdef/store.go              apiVersion envelope of a stored flow definition
host/pluginhost/registry/cosign.go cosign and Rekor bundle formats
host/venue/client/sync_types.go    mirrors the content-model run kinds (protojson)
kapi/cmd/kapi-wasm-cli/lab_annotate.go  browser lab options passed from JavaScript
kapi/cmd/kapi-wasm-cli/workspace.go     browser lab workspace export passed to JavaScript
core/project/storesync.go:mtimeNs  source stamp persisted in the project store
core/tools/annotations.go          block annotation values the engine stores, not a config
plugins/asr/asrproto/asrproto.go   wire to the kapi-asr plugin
plugins/vision/visionproto/visionproto.go  wire to the kapi-vision plugin
'

readonly TAG_RE='json:"[a-z][a-z0-9]*[A-Z][A-Za-z0-9]*'
readonly CASE_RE='case "[a-z][a-z0-9_]*[A-Z][A-Za-z0-9_]*"'

# allowed <file> <tag> → 0 when the allowlist covers the pair.
allowed() {
  local file="$1" tag="$2" entry
  while read -r entry _; do
    [[ -z "$entry" ]] && continue
    if [[ "$entry" == "$file" || "$entry" == "$file:$tag" ]]; then
      return 0
    fi
  done <<<"$ALLOWLIST"
  return 1
}

# scan <lines> prints each violation from `file:line:text` input.
scan() {
  local file line text tag
  while IFS=: read -r file line text; do
    [[ -z "$file" ]] && continue
    while read -r tag; do
      [[ -z "$tag" ]] && continue
      tag="${tag#json:\"}"
      if ! allowed "$file" "$tag"; then
        printf '%s:%s: json:"%s" is camelCase; use snake_case\n' "$file" "$line" "$tag"
      fi
    done < <(grep -oE "$TAG_RE" <<<"$text" || true)
  done
}

# scan_cases <lines> prints each camelCase format config key from `file:line:text`.
scan_cases() {
  local file line text key
  while IFS=: read -r file line text; do
    [[ -z "$file" ]] && continue
    while read -r key; do
      [[ -z "$key" ]] && continue
      key="${key#case \"}"
      key="${key%\"}"
      if ! allowed "$file" "$key"; then
        printf '%s:%s: config key "%s" is camelCase; use snake_case\n' "$file" "$line" "$key"
      fi
    done < <(grep -oE "$CASE_RE" <<<"$text" || true)
  done
}

if [[ "${1:-}" == "--self-test" ]]; then
  bad='host/example.go:3:	ShipState string `json:"shipState,omitempty"`'
  good='host/example.go:4:	ShipState string `json:"ship_state,omitempty"`'
  listed='host/hook.go:5:	HookEventName string `json:"hookEventName"`'
  narrowed='core/project/storesync.go:9:	X int `json:"storeMissing"`'
  [[ -n "$(scan <<<"$bad")" ]] || { echo "self-test: camelCase tag not caught"; exit 1; }
  [[ -z "$(scan <<<"$good")" ]] || { echo "self-test: snake_case tag flagged"; exit 1; }
  [[ -z "$(scan <<<"$listed")" ]] || { echo "self-test: allowlisted file flagged"; exit 1; }
  [[ -n "$(scan <<<"$narrowed")" ]] || { echo "self-test: tag allowlist covers the whole file"; exit 1; }
  config_bad='core/tools/example.go:7:	FillTargetThreshold int `json:"fillTargetThreshold,omitempty"`'
  config_good='core/tools/example.go:8:	FillTargetThreshold int `json:"fill_target_threshold,omitempty"`'
  case_bad='core/formats/json/config.go:9:		case "extractAllPairs":'
  case_good='core/formats/json/config.go:10:		case "extract_all_pairs", "INLINE":'
  [[ -n "$(scan <<<"$config_bad")" ]] || { echo "self-test: camelCase config tag not caught"; exit 1; }
  [[ -z "$(scan <<<"$config_good")" ]] || { echo "self-test: snake_case config tag flagged"; exit 1; }
  [[ -n "$(scan_cases <<<"$case_bad")" ]] || { echo "self-test: camelCase ApplyMap key not caught"; exit 1; }
  [[ -z "$(scan_cases <<<"$case_good")" ]] || { echo "self-test: snake_case ApplyMap key flagged"; exit 1; }
  echo "check-json-tags: self-test passed"
  exit 0
fi

cd "$root"
matches="$(git grep -nE "$TAG_RE" -- "${SCAN_PATHS[@]}" ':!*_test.go' ':!*.pb.go' ':!*/testdata/*' || true)"
violations="$(scan <<<"$matches")"
case_matches="$(git grep -nE "$CASE_RE" -- "${CONFIG_PATHS[@]}" ':!*_test.go' || true)"
violations="$violations$(scan_cases <<<"$case_matches")"
if [[ -n "$violations" ]]; then
  echo "$violations"
  echo
  echo "Machine-readable output uses snake_case field names (web/docs/reference/cli-contract.md#field-names)."
  echo "Rename the tag, or, when the JSON belongs to an external format or protocol, add the file to ALLOWLIST in $0."
  exit 1
fi
echo "check-json-tags: no camelCase json tag in the output, tool and format packages, no camelCase format config key"
