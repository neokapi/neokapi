package formats_test

// This file is a structural maturity guardrail for format packages. It does not
// test behavior — it enforces the conventions that make a format's maturity
// mechanically checkable (see docs/internals/format-maturity.md). The goal is to
// stop NEW formats from being added below the floor; existing debt is tracked in
// explicit ledgers that should shrink over time, never grow.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/model"
)

// nonFormats are directories under core/formats/ that are not real document
// formats (stubs / internal helpers) and are exempt from the rubric.
var nonFormats = map[string]bool{
	"exec":       true, // command-exec pseudo reader
	"jsx":        true, // klf-rename alias stub
	"memorytest": true, // in-memory test helper
}

// grandfatheredRoundtrip lists formats whose read->write fidelity coverage lives
// somewhere OTHER than a conventionally-named roundtrip_test.go / skeleton_test.go
// (e.g. inside reader_test.go or invariants_test.go), or — for `mo` — is genuine
// tracked debt. NEW formats MUST NOT be added here: put read->write fidelity
// tests in roundtrip_test.go or skeleton_test.go so the floor stays checkable.
// Removing an entry (by adding a conventionally-named test) is encouraged.
var grandfatheredRoundtrip = map[string]bool{
	"epub":     true,
	"idml":     true,
	"json":     true,
	"markdown": true,
	"mo":       true,
	"odf":      true,
}

// realFormatDirs returns the format ids that have a reader.go and are not
// exempted non-formats.
func realFormatDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read core/formats: %v", err)
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() || nonFormats[e.Name()] {
			continue
		}
		if fileExists(filepath.Join(e.Name(), "reader.go")) {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// readRegistryFile reads a multi-axis registry file from core/formats/ (the
// test's cwd). The seeds were authored by the format-ops bootstrap
// (docs/internals/format-ops.md §9) and are committed; a missing registry is a
// hard failure.
func readRegistryFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read core/formats/%s (seeded by the format-ops bootstrap, "+
			"docs/internals/format-ops.md §9): %v", name, err)
	}
	return data
}

// decodeFormatKeyedYAML decodes a YAML document whose payload is a map keyed
// by format id, either at the top level or nested under one of wrapperKeys.
func decodeFormatKeyedYAML[T any](data []byte, wrapperKeys ...string) (map[string]T, error) {
	var top map[string]yaml.Node
	if err := yaml.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	for _, key := range wrapperKeys {
		if node, ok := top[key]; ok && node.Kind == yaml.MappingNode {
			var m map[string]T
			if err := node.Decode(&m); err != nil {
				return nil, fmt.Errorf("decode %q block: %w", key, err)
			}
			return m, nil
		}
	}
	var m map[string]T
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func validISODate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// supportEntry is one format's entry in support.yaml (format-maturity.md §1).
type supportEntry struct {
	Tier          string   `yaml:"tier"`
	TierSince     string   `yaml:"tier_since"`
	LastCertified string   `yaml:"last_certified"`
	Gates         []string `yaml:"gates"`
	Grandfathered bool     `yaml:"grandfathered"`
	Notes         string   `yaml:"notes"`
}

// supportTiers is the tier enum from format-maturity.md §1. Casing is owned by
// scripts/format-ops/check-support-gates.mjs; this floor validates the value.
var supportTiers = []string{"Supported", "Maintained", "Available"}

func validSupportTier(tier string) bool {
	for _, t := range supportTiers {
		if strings.EqualFold(tier, t) {
			return true
		}
	}
	return false
}

// TestSupportYAML is a hard gate on core/formats/support.yaml (the tier
// promise, format-maturity.md §1): exactly one entry per real format dir, valid
// tier enums, parseable dates, and gates that are either `make test` or
// workflow files that exist under .github/workflows/.
func TestSupportYAML(t *testing.T) {
	data := readRegistryFile(t, "support.yaml")
	entries, err := decodeFormatKeyedYAML[supportEntry](data, "formats")
	if err != nil {
		t.Fatalf("parse support.yaml: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("support.yaml parsed but declares no formats")
	}

	ids := realFormatDirs(t)
	realSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		realSet[id] = true
	}
	for _, id := range ids {
		if _, ok := entries[id]; !ok {
			t.Errorf("support.yaml has no entry for format %q — every real format dir "+
				"declares a support tier (format-maturity.md §1)", id)
		}
	}

	workflowsDir := filepath.Join("..", "..", ".github", "workflows")
	checkGateFiles := dirExists(workflowsDir)
	if !checkGateFiles {
		t.Logf("%s not present (partial checkout) — skipping gate workflow-file existence checks", workflowsDir)
	}

	for id, e := range entries {
		if !realSet[id] {
			t.Errorf("support.yaml entry %q does not match any real format dir under "+
				"core/formats/ — remove the entry or add the format", id)
			continue
		}
		if !validSupportTier(e.Tier) {
			t.Errorf("support.yaml: format %q has tier %q; want one of %v", id, e.Tier, supportTiers)
		}
		if e.TierSince == "" {
			t.Errorf("support.yaml: format %q is missing tier_since", id)
		} else if !validISODate(e.TierSince) {
			t.Errorf("support.yaml: format %q has unparseable tier_since %q (want YYYY-MM-DD)", id, e.TierSince)
		}
		// last_certified may be empty at bootstrap (it is written only by a
		// passing triage-score run); when present it must parse.
		if e.LastCertified != "" && !validISODate(e.LastCertified) {
			t.Errorf("support.yaml: format %q has unparseable last_certified %q (want YYYY-MM-DD)", id, e.LastCertified)
		}
		for _, gate := range e.Gates {
			if gate == "make test" {
				continue
			}
			if !checkGateFiles {
				continue
			}
			if !fileExists(filepath.Join(workflowsDir, filepath.Base(gate))) {
				t.Errorf("support.yaml: format %q names gate %q, which is neither \"make test\" "+
					"nor a workflow file under .github/workflows/ — a tier not enforced by CI "+
					"is marketing (format-maturity.md §1)", id, gate)
			}
		}
	}
}

// constructEntry is one row of the repo-level construct registry
// core/formats/constructs.yaml (format-maturity.md §2.2).
type constructEntry struct {
	ID             string   `yaml:"id"`
	CanonicalTypes []string `yaml:"canonical_types"`
}

// Construct ids are dot-separated kebab segments, e.g. "inline.bold",
// "placeholder.line-break" (category.name, the stable-ID scheme documented in
// the constructs.yaml header).
var kebabIDRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*(?:\.[a-z0-9]+(?:-[a-z0-9]+)*)*$`)

// TestConstructsRegistry is a hard gate on core/formats/constructs.yaml: it
// parses, construct ids are unique dotted-kebab-case, and every canonical_types
// value resolves in the core/model vocabulary packs — the registry may not
// invent canonical run types the model does not know.
func TestConstructsRegistry(t *testing.T) {
	data := readRegistryFile(t, "constructs.yaml")

	var entries []constructEntry
	var wrapper struct {
		Constructs []constructEntry `yaml:"constructs"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err == nil && len(wrapper.Constructs) > 0 {
		entries = wrapper.Constructs
	} else if err := yaml.Unmarshal(data, &entries); err != nil {
		t.Fatalf("parse constructs.yaml (want a top-level constructs: list or a bare list): %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("constructs.yaml parsed but contains no constructs")
	}

	vocab := model.NewVocabularyRegistry()
	if err := vocab.LoadDefaults(); err != nil {
		t.Fatalf("load core/model default vocabularies: %v", err)
	}

	seen := make(map[string]bool, len(entries))
	for i, e := range entries {
		if e.ID == "" {
			t.Errorf("constructs.yaml: entry %d has no id", i)
			continue
		}
		if seen[e.ID] {
			t.Errorf("constructs.yaml: duplicate construct id %q — ids are stable and unique "+
				"(format-maturity.md §2.2)", e.ID)
		}
		seen[e.ID] = true
		if !kebabIDRe.MatchString(e.ID) {
			t.Errorf("constructs.yaml: construct id %q is not dotted-kebab-case (%s)", e.ID, kebabIDRe)
		}
		for _, ct := range e.CanonicalTypes {
			if vocab.Lookup(ct) == nil {
				t.Errorf("constructs.yaml: construct %q lists canonical_types value %q, which is "+
					"not in the core/model vocabulary packs (core/model/vocabularies/*.json; e.g. "+
					"fmt:bold, link:hyperlink, media:image, code:placeholder)", e.ID, ct)
			}
		}
	}
}

// integrationEntry is one declared surface in core/formats/integrations.yaml
// (format-maturity.md §2.3).
type integrationEntry struct {
	Surface  string   `yaml:"surface"`
	Depth    string   `yaml:"depth"`
	Evidence string   `yaml:"evidence"`
	Files    []string `yaml:"files"`
}

var editorDepths = map[string]bool{"E1": true, "E2": true, "E3": true, "E4": true}

// evidencePathExists resolves an integrations.yaml evidence path on HEAD.
// Evidence is "path" or "path:TestName", repo-root-relative (with a
// core/formats-relative fallback). Returns (exists, checked): checked is false
// when the path's top-level tree is absent (partial checkout) — the
// skip-if-absent pattern for out-of-dir references.
func evidencePathExists(evidence string) (exists, checked bool) {
	path := evidence
	if before, _, ok := strings.Cut(evidence, ":"); ok {
		path = before
	}
	if fileExists(path) || dirExists(path) {
		return true, true // core/formats-relative
	}
	root := filepath.Join("..", "..", filepath.FromSlash(path))
	if fileExists(root) || dirExists(root) {
		return true, true
	}
	top := strings.SplitN(path, "/", 2)[0]
	if !dirExists(filepath.Join("..", "..", top)) {
		return false, false
	}
	return false, true
}

// TestIntegrationsIndex is a hard gate on core/formats/integrations.yaml: every
// key is a real format id, declared depths are E1–E4, and every evidence path
// resolves on HEAD (declarations on unmerged branches do not count —
// format-maturity.md §2.3).
func TestIntegrationsIndex(t *testing.T) {
	data := readRegistryFile(t, "integrations.yaml")
	byFormat, err := decodeFormatKeyedYAML[yaml.Node](data, "integrations", "formats")
	if err != nil {
		t.Fatalf("parse integrations.yaml: %v", err)
	}
	if len(byFormat) == 0 {
		t.Fatal("integrations.yaml parsed but declares no formats")
	}

	ids := realFormatDirs(t)
	realSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		realSet[id] = true
	}

	for id, node := range byFormat {
		if !realSet[id] {
			t.Errorf("integrations.yaml entry %q does not match any real format dir under core/formats/", id)
			continue
		}
		var entries []integrationEntry
		if node.Kind == yaml.SequenceNode {
			if err := node.Decode(&entries); err != nil {
				t.Errorf("integrations.yaml: format %q: decode entries: %v", id, err)
				continue
			}
		} else {
			var single integrationEntry
			if err := node.Decode(&single); err != nil {
				t.Errorf("integrations.yaml: format %q: decode entry: %v", id, err)
				continue
			}
			entries = []integrationEntry{single}
		}
		for _, e := range entries {
			if !editorDepths[e.Depth] {
				t.Errorf("integrations.yaml: format %q surface %q declares depth %q; want E1–E4",
					id, e.Surface, e.Depth)
			}
			if e.Evidence == "" {
				// The E1 probe is PreviewBuilder presence in the package, not a
				// path; deeper claims need gate evidence the audit can resolve.
				if e.Depth != "E1" {
					t.Errorf("integrations.yaml: format %q surface %q declares depth %q with no "+
						"evidence — E2+ claims carry a resolvable gate-evidence path "+
						"(format-maturity.md §2.3)", id, e.Surface, e.Depth)
				}
				continue
			}
			exists, checked := evidencePathExists(e.Evidence)
			if !checked {
				t.Logf("integrations.yaml: format %q surface %q evidence %q points outside this "+
					"checkout — skipping existence check", id, e.Surface, e.Evidence)
				continue
			}
			if !exists {
				t.Errorf("integrations.yaml: format %q surface %q evidence %q does not resolve on "+
					"HEAD — integrations on unmerged branches do not count (format-maturity.md §2.3)",
					id, e.Surface, e.Evidence)
			}
		}
	}
}

// advisoryArtifactCoverage reports (never fails) how many formats still lack a
// per-format axis artifact. These are the backfill burndown counters of the
// format-ops bootstrap (format-ops.md §9): 49/49 missing is the expected
// starting state, driven down by the remediate ritual.
func advisoryArtifactCoverage(t *testing.T, artifact, axis string) {
	t.Helper()
	ids := realFormatDirs(t)
	var missing []string
	for _, id := range ids {
		if !fileExists(filepath.Join(id, artifact)) {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		t.Logf("advisory: all %d formats carry a %s", len(ids), artifact)
		return
	}
	t.Logf("advisory: %d/%d formats lack a %s (%s backfill burndown, format-ops.md §9): %v",
		len(missing), len(ids), artifact, axis, missing)
}

// TestVocabularyCoverage counts formats without a vocabulary.yaml (Vocabulary
// axis V1 floor). Advisory only.
func TestVocabularyCoverage(t *testing.T) {
	advisoryArtifactCoverage(t, "vocabulary.yaml", "Vocabulary V1")
}

// TestDossierCoverage counts formats without a dossier.yaml (Knowledge axis K1
// floor). Advisory only.
func TestDossierCoverage(t *testing.T) {
	advisoryArtifactCoverage(t, "dossier.yaml", "Knowledge K1")
}

// TestCorpusManifestCoverage counts formats without a corpus.yaml (Corpus axis
// C1 floor). Advisory only.
func TestCorpusManifestCoverage(t *testing.T) {
	advisoryArtifactCoverage(t, "corpus.yaml", "Corpus C1")
}

var buildPreviewRe = regexp.MustCompile(`func \([^)]*\) BuildPreview\(`)

// TestPreviewCoverage counts formats whose package implements
// format.PreviewBuilder (the Editor axis E1 probe). Advisory only.
func TestPreviewCoverage(t *testing.T) {
	ids := realFormatDirs(t)
	var with []string
	for _, id := range ids {
		files, err := filepath.Glob(filepath.Join(id, "*.go"))
		if err != nil {
			t.Fatalf("glob %s/*.go: %v", id, err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			if buildPreviewRe.Match(src) {
				with = append(with, id)
				break
			}
		}
	}
	t.Logf("advisory: %d/%d formats implement format.PreviewBuilder (Editor E1 probe): %v — "+
		"%d formats fall back to the generic BlockIndex preview",
		len(with), len(ids), with, len(ids)-len(with))
}

// TestFormatSpecIsGated enforces that every spec.yaml is exercised by a
// spec_test.go. An ungated spec rots silently (see format-engineering.md §8). This
// is a hard floor — there are currently zero violators, and new formats must keep
// it that way.
func TestFormatSpecIsGated(t *testing.T) {
	for _, id := range realFormatDirs(t) {
		if !fileExists(filepath.Join(id, "spec.yaml")) {
			continue
		}
		if !fileExists(filepath.Join(id, "spec_test.go")) {
			t.Errorf("format %q ships a spec.yaml but no spec_test.go — the spec is "+
				"not gated by any test. Add spec_test.go driving spec.NativeRunner "+
				"(see core/formats/properties/spec_test.go).", id)
		}
	}
}

// TestRoundTripTestNamingConvention enforces that a format with a writer carries
// its read->write fidelity test in a conventionally-named roundtrip_test.go or
// skeleton_test.go, so the maturity floor is mechanically checkable. Existing
// exceptions are grandfathered; new formats must follow the convention.
func TestRoundTripTestNamingConvention(t *testing.T) {
	for _, id := range realFormatDirs(t) {
		if !fileExists(filepath.Join(id, "writer.go")) {
			continue // read-only formats (e.g. pdf) have nothing to round-trip
		}
		conventional := fileExists(filepath.Join(id, "roundtrip_test.go")) ||
			fileExists(filepath.Join(id, "skeleton_test.go"))
		if conventional {
			if grandfatheredRoundtrip[id] {
				t.Logf("format %q now has a conventional round-trip test — remove it "+
					"from grandfatheredRoundtrip in maturity_test.go.", id)
			}
			continue
		}
		if grandfatheredRoundtrip[id] {
			continue // tracked debt / non-conventional coverage
		}
		t.Errorf("format %q has a writer.go but no roundtrip_test.go or "+
			"skeleton_test.go. Add a read->write fidelity test in one of those files "+
			"(see docs/internals/format-maturity.md, L1). Do not add it to the "+
			"grandfathered ledger.", id)
	}
}

// TestRobustnessCoverage is advisory: it reports formats lacking a malformed_test.go.
// Robustness against broken input is an L2 requirement (format-maturity.md), and
// today only a handful of formats have it. This does not fail the build — it
// surfaces the gap so it can be burned down.
func TestRobustnessCoverage(t *testing.T) {
	var missing []string
	for _, id := range realFormatDirs(t) {
		if !fileExists(filepath.Join(id, "malformed_test.go")) {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		t.Logf("advisory: %d/%d formats lack a malformed_test.go (L2 robustness gap): %v",
			len(missing), len(realFormatDirs(t)), missing)
	}
}
