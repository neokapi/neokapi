package main

import (
	"encoding/json"
	"fmt"
	"maps"
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
func checkDrift(bridgeDir, pluginsDir, metaPath, nativeDocsDir, outDir, coreCatalogs, cliCatalogs string) error {
	formatEntries, toolEntries, resolveExt, bridgePresent, err := buildEntries(bridgeDir, pluginsDir, metaPath, nativeDocsDir)
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
	if diff := compareFamilies(filepath.Join(outDir, "format-families.json"), buildFamilyDataset("", formatEntries, resolveExt)); diff != "" {
		problems = append(problems, "format-families.json: "+diff)
	}
	if diff := compareBuiltInGaps(filepath.Join(outDir, "reference-gaps.json"), wantGaps, wantSummary); diff != "" {
		problems = append(problems, "reference-gaps.json: "+diff)
	}
	wantPrompts := collectPromptDataset("")
	if diff := comparePrompts(filepath.Join(outDir, "prompts.json"), wantPrompts); diff != "" {
		problems = append(problems, "prompts.json: "+diff)
	}
	wantModels := collectModelDataset("")
	if diff := compareModels(filepath.Join(outDir, "models.json"), wantModels); diff != "" {
		problems = append(problems, "models.json: "+diff)
	}
	wantCommands := collectCommandDataset("")
	if diff := compareCommands(filepath.Join(outDir, "commands.json"), wantCommands); diff != "" {
		problems = append(problems, "commands.json: "+diff)
	}
	wantMCP, err := collectMCPDataset("")
	if err != nil {
		return err
	}
	if diff := compareMCPTools(filepath.Join(outDir, "mcp-tools.json"), wantMCP); diff != "" {
		problems = append(problems, "mcp-tools.json: "+diff)
	}
	// The locale variants derive from the committed English dataset and the
	// committed catalogs, so they are as much a function of the tree as the
	// English is; a catalog the loop wrote without the variants following is
	// drift of the same kind.
	problems = append(problems, localeVariantDrift(outDir, coreCatalogs, cliCatalogs, nativeDocsDir)...)

	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "  - %s\n", p)
		}
		return fmt.Errorf("committed reference dataset is stale; run `make generate-reference-docs` and commit the result")
	}

	fmt.Printf("reference dataset is fresh (built-in subset: %d formats, %d tools, %d gaps, %d prompts, %d models)\n",
		countBuiltIn(formatEntries), countBuiltIn(toolEntries), len(builtInGaps(wantGaps)), len(wantPrompts.Prompts), len(collectModelDataset("").Models))
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

// comparePrompts gates the committed prompt reference against the live catalog.
// Rewording any prompt kapi sends fails this until the reference is regenerated,
// so the published prompts cannot drift away from the ones the binary uses.
func comparePrompts(path string, want PromptDataset) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("cannot read %s: %v", path, err)
	}
	var got PromptDataset
	if err := json.Unmarshal(raw, &got); err != nil {
		return fmt.Sprintf("cannot parse %s: %v", path, err)
	}

	// generatedAt changes every run and is not part of the contract.
	got.GeneratedAt = ""
	want.GeneratedAt = ""

	gotJSON, err := json.Marshal(got)
	if err != nil {
		return fmt.Sprintf("cannot re-encode %s: %v", path, err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		return fmt.Sprintf("cannot encode catalog: %v", err)
	}
	if string(gotJSON) != string(wantJSON) {
		return "the committed prompt reference no longer matches the prompts kapi sends"
	}
	return ""
}

// compareCommands compares the committed command reference against the live
// cobra tree.
//
// Without this the gate was hollow for commands.json: every other dataset was
// gated, so a change to a command's help text — the flag list, the examples, the
// file extensions a command documents — regenerated cleanly but was never
// required to be committed, and web/docs/reference/commands/*.mdx kept
// publishing whatever the last regeneration happened to capture.
func compareCommands(path string, want CommandDataset) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("cannot read %s: %v", path, err)
	}
	var got CommandDataset
	if err := json.Unmarshal(raw, &got); err != nil {
		return fmt.Sprintf("cannot parse %s: %v", path, err)
	}

	// generatedAt changes every run and is not part of the contract.
	got.GeneratedAt = ""
	want.GeneratedAt = ""

	if len(got.Commands) != len(want.Commands) {
		return fmt.Sprintf("command count changed: committed %d, regenerated %d",
			len(got.Commands), len(want.Commands))
	}
	for i := range want.Commands {
		if !jsonEqual(want.Commands[i], got.Commands[i]) {
			return fmt.Sprintf("command %q is stale (help text, flags, or examples changed)",
				want.Commands[i].ID)
		}
	}
	return ""
}

// compareMCPTools gates the committed MCP reference against what a live
// `kapi mcp` server serves — its tools and the addresses it answers reads at.
// Registering, retiring, or rewording either fails this until the reference is
// regenerated, so /reference/mcp cannot teach an assistant a tool the server
// does not answer to, or an address it does not serve.
func compareMCPTools(path string, want MCPDataset) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("cannot read %s: %v", path, err)
	}
	var got MCPDataset
	if err := json.Unmarshal(raw, &got); err != nil {
		return fmt.Sprintf("cannot parse %s: %v", path, err)
	}

	// generatedAt changes every run and is not part of the contract.
	got.GeneratedAt = ""
	want.GeneratedAt = ""

	if len(got.Tools) != len(want.Tools) {
		return fmt.Sprintf("MCP tool count changed: committed %d, served %d",
			len(got.Tools), len(want.Tools))
	}
	for i := range want.Tools {
		if !jsonEqual(want.Tools[i], got.Tools[i]) {
			return fmt.Sprintf("MCP tool %q is stale (name, description, surface, or parameters changed)",
				want.Tools[i].Name)
		}
	}

	if len(got.Resources) != len(want.Resources) {
		return fmt.Sprintf("MCP resource count changed: committed %d, served %d",
			len(got.Resources), len(want.Resources))
	}
	for i := range want.Resources {
		if !jsonEqual(want.Resources[i], got.Resources[i]) {
			return fmt.Sprintf("MCP resource %q is stale (address, description, or mime type changed)",
				want.Resources[i].URITemplate)
		}
	}
	return ""
}

// compareModels gates the committed model reference against the live catalog.
// Editing providers/ai/models.json without regenerating fails this, so the
// published /models page cannot describe a catalog kapi no longer ships.
func compareModels(path string, want ModelDataset) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("cannot read %s: %v", path, err)
	}
	var got ModelDataset
	if err := json.Unmarshal(raw, &got); err != nil {
		return fmt.Sprintf("cannot parse %s: %v", path, err)
	}

	// generatedAt changes every run and is not part of the contract.
	got.GeneratedAt = ""
	want.GeneratedAt = ""

	gotJSON, err := json.Marshal(got)
	if err != nil {
		return fmt.Sprintf("cannot re-encode %s: %v", path, err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		return fmt.Sprintf("cannot encode catalog: %v", err)
	}
	if string(gotJSON) != string(wantJSON) {
		return "the committed model reference no longer matches providers/ai/models.json"
	}
	return ""
}

// compareFamilies holds the committed family map against a freshly built one.
// Only the maps are compared; generatedAt moves on every run.
func compareFamilies(path string, fresh FamilyDataset) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("cannot read committed file: %v", err)
	}
	var committed FamilyDataset
	if err := json.Unmarshal(data, &committed); err != nil {
		return fmt.Sprintf("cannot parse committed file: %v", err)
	}
	if !maps.Equal(committed.Formats, fresh.Formats) {
		return fmt.Sprintf("format families differ (committed %d, fresh %d)", len(committed.Formats), len(fresh.Formats))
	}
	if !maps.Equal(committed.Extensions, fresh.Extensions) {
		return fmt.Sprintf("extension families differ (committed %d, fresh %d)", len(committed.Extensions), len(fresh.Extensions))
	}
	return ""
}
