package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// checkDrift is the drift gate. It regenerates the dataset in memory and fails
// if the committed JSON under outDir is stale relative to the current source —
// catching the case where a tool/format schema, description, or doc sidecar
// changed but `make generate-reference-docs` was not re-run.
//
// Two properties make it safe to run in CI:
//
//   - It compares only the built-in (`source: "built-in"`) subset of each
//     committed dataset against the freshly built built-in subset. The
//     okapi-bridge entries come from an external repo and are only present when
//     the bridge plugin dir is supplied, so gating on them would spuriously fail
//     whenever the bridge is absent. Built-in entries are wholly controlled by
//     this repo's source, so they are the right thing to gate on.
//   - It ignores the `generatedAt` timestamp, which changes on every run.
//
// The gate never writes; it only reads the committed files and compares.
func checkDrift(bridgeDir, metaPath, nativeDocsDir, outDir string) error {
	formatEntries, toolEntries, bridgePresent, err := buildEntries(bridgeDir, metaPath, nativeDocsDir)
	if err != nil {
		return err
	}
	if !bridgePresent {
		fmt.Fprintln(os.Stderr, "note: okapi-bridge plugin dir absent; gating on the built-in subset only (set BRIDGE_PLUGIN to also gate okapi entries)")
	}

	// Rebuild the gap report from the same entries the live generator would use,
	// so a stale reference-gaps.json (e.g. a sidecar added without regenerating)
	// is also caught.
	all := append(append([]Entry{}, formatEntries...), toolEntries...)
	wantGaps := detectGaps(all)
	wantSummary := summarize(wantGaps)

	var problems []string

	if diff := compareBuiltInDataset(filepath.Join(outDir, "formats.json"), KindFormat, formatEntries); diff != "" {
		problems = append(problems, "formats.json: "+diff)
	}
	if diff := compareBuiltInDataset(filepath.Join(outDir, "tools.json"), KindTool, toolEntries); diff != "" {
		problems = append(problems, "tools.json: "+diff)
	}
	if diff := compareBuiltInGaps(filepath.Join(outDir, "reference-gaps.json"), wantGaps, wantSummary); diff != "" {
		problems = append(problems, "reference-gaps.json: "+diff)
	}

	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "  - %s\n", p)
		}
		return fmt.Errorf("committed reference dataset is stale; run `make generate-reference-docs` and commit the result")
	}

	fmt.Printf("reference dataset is fresh (built-in subset: %d formats, %d tools, %d gaps)\n",
		countBuiltIn(formatEntries), countBuiltIn(toolEntries), len(builtInGaps(wantGaps)))
	return nil
}

// compareBuiltInDataset loads a committed dataset file and compares its built-in
// entries against the freshly generated built-in entries. Returns a short
// description of the first mismatch, or "" when fresh.
func compareBuiltInDataset(path, kind string, fresh []Entry) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("cannot read committed file: %v", err)
	}
	var committed Dataset
	if err := json.Unmarshal(data, &committed); err != nil {
		return fmt.Sprintf("cannot parse committed file: %v", err)
	}

	wantBI := filterBuiltIn(fresh)
	gotBI := filterBuiltIn(committed.Entries)

	if len(gotBI) != len(wantBI) {
		return fmt.Sprintf("built-in %s count changed: committed %d, regenerated %d", kind, len(gotBI), len(wantBI))
	}
	// Both are sorted identically by sortEntries, so compare position by position.
	for i := range wantBI {
		if id := wantBI[i].ID; id != gotBI[i].ID {
			return fmt.Sprintf("built-in entry ordering/ids differ at index %d: committed %q, regenerated %q", i, gotBI[i].ID, id)
		}
		if !jsonEqual(wantBI[i], gotBI[i]) {
			return fmt.Sprintf("built-in entry %q is stale (schema, description, or doc changed)", wantBI[i].ID)
		}
	}
	return ""
}

// compareBuiltInGaps compares the committed gap report against a freshly built
// one, restricted to built-in gaps so the bridge's presence does not matter.
func compareBuiltInGaps(path string, wantGaps []Gap, wantSummary map[string]int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("cannot read committed file: %v", err)
	}
	var committed GapReport
	if err := json.Unmarshal(data, &committed); err != nil {
		return fmt.Sprintf("cannot parse committed file: %v", err)
	}

	want := builtInGaps(wantGaps)
	got := builtInGaps(committed.Gaps)
	if !jsonEqual(want, got) {
		return fmt.Sprintf("built-in gaps changed: committed %d, regenerated %d (a sidecar was added/edited without regenerating?)", len(got), len(want))
	}
	// Verify the built-in slice of the summary is consistent too.
	wantBISummary := builtInSummary(wantSummary)
	gotBISummary := builtInSummary(committed.Summary)
	if !jsonEqual(wantBISummary, gotBISummary) {
		return "built-in gap summary changed"
	}
	return ""
}

// filterBuiltIn returns the built-in-sourced entries, preserving order.
func filterBuiltIn(entries []Entry) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Source == SourceBuiltIn {
			out = append(out, e)
		}
	}
	return out
}

// builtInGaps returns the gaps whose source is built-in, preserving order.
func builtInGaps(gaps []Gap) []Gap {
	out := make([]Gap, 0, len(gaps))
	for _, g := range gaps {
		if g.Source == SourceBuiltIn {
			out = append(out, g)
		}
	}
	return out
}

// builtInSummary returns the summary keys that describe built-in entries.
func builtInSummary(summary map[string]int) map[string]int {
	out := map[string]int{}
	for k, v := range summary {
		if len(k) >= len(SourceBuiltIn) && k[:len(SourceBuiltIn)] == SourceBuiltIn {
			out[k] = v
		}
	}
	return out
}

func countBuiltIn(entries []Entry) int { return len(filterBuiltIn(entries)) }

// jsonEqual reports whether two values marshal to identical JSON. Robust to map
// key ordering and avoids depending on reflect.DeepEqual over json.RawMessage.
func jsonEqual(a, b any) bool {
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(ab) == string(bb)
}
