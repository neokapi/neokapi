#!/usr/bin/env bash
# Filter go test -json output and print package-level progress to stderr.
# Stdin: go test -json stream. Stdout: passthrough (unmodified).
#
# Prints lines like:
#   PASS  core/plugin/bridge/filters/json (2.3s)
#   FAIL  core/plugin/bridge/filters/its (0.8s)
#   SKIP  core/plugin/bridge/filters/pdf (0.0s)
#
# Usage:
#   go test -json ./... | bash scripts/go-test-progress.sh > results.json

while IFS= read -r line; do
    # Pass through unmodified to stdout (the JSON results file).
    printf '%s\n' "$line"

    # Fast pattern match for package-level events (no "Test" key or "Test":"").
    # Package events: {"Action":"pass","Package":"...","Elapsed":N}
    # Test events also have "Test":"..." — skip those.
    case "$line" in
        *'"Test"'*) continue ;;
    esac

    # Extract action for pass/fail/skip.
    case "$line" in
        *'"Action":"pass"'*)  label="PASS" ;;
        *'"Action":"fail"'*)  label="FAIL" ;;
        *'"Action":"skip"'*)  label="SKIP" ;;
        *) continue ;;
    esac

    # Extract package name (between "Package":" and next ").
    pkg="${line#*\"Package\":\"}"
    pkg="${pkg%%\"*}"
    # Strip common prefix.
    short="${pkg#github.com/neokapi/neokapi/}"

    # Extract elapsed time (between "Elapsed": and }).
    elapsed="${line#*\"Elapsed\":}"
    elapsed="${elapsed%%[,\}]*}"

    printf '  %-4s  %s (%ss)\n' "$label" "$short" "$elapsed" >&2
done
