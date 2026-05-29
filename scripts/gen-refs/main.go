// gen-refs generates the unified reference dataset (built-in + okapi-bridge
// formats and tools, with schemas, docs, and presets) consumed by the website
// reference pages and the kapi-desktop Storybook via @neokapi/reference-data.
//
// Run from the repo root:
//
//	go run ./scripts/gen-refs
//	go run ./scripts/gen-refs -bridge /path/to/okapi-bridge/dist/plugin
package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func main() {
	var (
		bridgeDir  = flag.String("bridge", "../okapi-bridge/dist/plugin", "okapi-bridge plugin dir (manifest.json + formats/ + tools/)")
		metaPath   = flag.String("meta", "core/i18n/builtins/metadata.json", "i18n builtins metadata.json (native display names/descriptions)")
		nativeDocs = flag.String("nativedocs", "scripts/gen-refs/nativedocs", "dir of authored native doc sidecars ({formats,tools}/<id>.yaml)")
		outDir     = flag.String("out", "packages/reference-data/data", "output dir for formats.json, tools.json, reference-gaps.json")
		check      = flag.Bool("check", false, "drift gate: regenerate in memory and fail if the committed built-in subset under -out is stale (does not write)")
	)
	flag.Parse()

	if *check {
		if err := checkDrift(*bridgeDir, *metaPath, *nativeDocs, *outDir); err != nil {
			fmt.Fprintf(os.Stderr, "gen-refs -check: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := run(*bridgeDir, *metaPath, *nativeDocs, *outDir); err != nil {
		fmt.Fprintf(os.Stderr, "gen-refs: %v\n", err)
		os.Exit(1)
	}
}

// buildEntries collects the native formats and tools, overlays the authored doc
// sidecars, and appends the okapi-bridge entries when the plugin dir is present.
// It is the shared core of `run` (which writes the dataset) and `checkDrift`
// (which compares it against the committed files).
func buildEntries(bridgeDir, metaPath, nativeDocsDir string) (formats, tools []Entry, bridgePresent bool, err error) {
	meta, err := loadNativeMeta(metaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: native metadata unavailable (%v); descriptions may be sparse\n", err)
		meta = &nativeMeta{}
	}

	freg, treg := nativeRegistries()
	formatEntries := collectNativeFormats(freg, meta)
	toolEntries := collectNativeTools(treg, meta)

	// Overlay authored native doc sidecars.
	if err := overlayNativeDocs(nativeDocsDir, KindFormat, formatEntries); err != nil {
		return nil, nil, false, err
	}
	if err := overlayNativeDocs(nativeDocsDir, KindTool, toolEntries); err != nil {
		return nil, nil, false, err
	}

	// Append bridge entries (non-fatal if the plugin dir is absent).
	bf, bt, berr := collectBridge(bridgeDir)
	switch {
	case berr == nil:
		formatEntries = append(formatEntries, bf...)
		toolEntries = append(toolEntries, bt...)
		bridgePresent = true
		fmt.Printf("bridge: %d formats, %d tools from %s\n", len(bf), len(bt), bridgeDir)
	case errors.Is(berr, os.ErrNotExist):
		fmt.Fprintf(os.Stderr, "warning: okapi-bridge plugin dir not found at %s; emitting built-in entries only\n", bridgeDir)
	default:
		return nil, nil, false, fmt.Errorf("read bridge: %w", berr)
	}

	sortEntries(formatEntries)
	sortEntries(toolEntries)
	return formatEntries, toolEntries, bridgePresent, nil
}

func run(bridgeDir, metaPath, nativeDocsDir, outDir string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	formatEntries, toolEntries, _, err := buildEntries(bridgeDir, metaPath, nativeDocsDir)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, "formats.json"), Dataset{GeneratedAt: now, Kind: KindFormat, Entries: formatEntries}); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, "tools.json"), Dataset{GeneratedAt: now, Kind: KindTool, Entries: toolEntries}); err != nil {
		return err
	}

	all := append(append([]Entry{}, formatEntries...), toolEntries...)
	gaps := detectGaps(all)
	report := GapReport{GeneratedAt: now, Summary: summarize(gaps), Gaps: gaps}
	if err := writeJSON(filepath.Join(outDir, "reference-gaps.json"), report); err != nil {
		return err
	}

	// Generate the command reference dataset from the kapi cobra tree.
	cmdDataset := collectCommandDataset(now)
	if err := writeJSON(filepath.Join(outDir, "commands.json"), cmdDataset); err != nil {
		return err
	}

	fmt.Printf("wrote %s/{formats,tools,reference-gaps,commands}.json — %d formats, %d tools, %d commands\n",
		outDir, len(formatEntries), len(toolEntries), len(cmdDataset.Commands))
	printGapSummary(report)
	return nil
}

// overlayNativeDocs loads sidecars for one kind and merges them into the
// matching built-in entries.
func overlayNativeDocs(dir, kind string, entries []Entry) error {
	docs, err := loadNativeDocs(dir, kind)
	if err != nil {
		return fmt.Errorf("load native %s docs: %w", kind, err)
	}
	for i := range entries {
		if ndf, ok := docs[entries[i].ID]; ok {
			applyNativeDoc(&entries[i], ndf)
		}
	}
	return nil
}

// sortEntries orders entries by display name (case-insensitive), then id, then
// source, so a single source-filtered list reads alphabetically. The source
// tie-break is essential: a few names collide across sources (e.g. a built-in
// and an okapi-bridge "Word Count"), and without it their relative order is
// unstable across regenerations, producing spurious dataset diffs.
func sortEntries(entries []Entry) {
	slices.SortFunc(entries, func(a, b Entry) int {
		if c := cmp.Compare(strings.ToLower(a.DisplayName), strings.ToLower(b.DisplayName)); c != 0 {
			return c
		}
		if c := cmp.Compare(a.ID, b.ID); c != 0 {
			return c
		}
		return cmp.Compare(a.Source, b.Source)
	})
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func printGapSummary(r GapReport) {
	keys := make([]string, 0, len(r.Summary))
	for k := range r.Summary {
		if k != "total" {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	fmt.Printf("metadata gaps: %d total\n", r.Summary["total"])
	for _, k := range keys {
		fmt.Printf("  %-40s %d\n", k, r.Summary[k])
	}
}
