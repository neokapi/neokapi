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

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/model"
)

// nonFormats are directories under core/formats/ that are not real document
// formats (stubs / internal helpers) and are exempt from the rubric.
var nonFormats = map[string]bool{
	"exec":       true, // command-exec pseudo reader
	"jsx":        true, // Kapi Bundle Format — kapi's own serialization, not a document format
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

// realFormatDirs returns the format ids in the maturity universe: a dir that
// ships a reader.go (an in-core format) OR a structure.yaml (the dashboard
// allowlist seat for a PLUGIN-PROVIDED, out-of-core format such as pdf — whose
// native reader is the kapi-pdfium plugin and whose browser path is a
// PDFium-wasm bridge, so it has no in-core reader.go). Exempted non-formats
// (exec/jsx/memorytest) are excluded and never ship a structure.yaml.
//
// This mirrors the JS definition in scripts/format-ops/lib.mjs (realFormatDirs)
// so the Go maturity gates and the JS format-ops gates agree on the universe.
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
		if fileExists(filepath.Join(e.Name(), "reader.go")) ||
			fileExists(filepath.Join(e.Name(), "structure.yaml")) {
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
	top, _, _ := strings.Cut(path, "/")
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

// structureAuthorities is the AD-028 provenance-tier enum a structure.yaml may
// declare (format-maturity.md §2.7; SHARPEN §2 "Authority qualifier"). Orthogonal
// to the G rung (depth): the same payload depth can arrive native, tagged,
// geometrically inferred, or ML-guessed.
var structureAuthorities = map[string]bool{
	"native": true, "tagged": true, "geometric": true, "ml": true,
}

var structureAuthorityNames = []string{"native", "tagged", "geometric", "ml"}

// authorityValues flattens a structure.yaml `authority` node into its declared
// tiers: a scalar (one tier) or a sequence (capability-conditional formats that
// reach more than one tier, e.g. image: [geometric, ml]). Absent => nil.
func authorityValues(n yaml.Node) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Value == "" {
			return nil
		}
		return []string{n.Value}
	case yaml.SequenceNode:
		var out []string
		for _, c := range n.Content {
			if c.Value != "" {
				out = append(out, c.Value)
			}
		}
		return out
	}
	return nil
}

// structureYAML is the shape TestStructureCoverage lightly validates. Only the
// two fields the audit consumes are typed; everything else is free documentary
// prose. `authority` and `geometry` are yaml.Node because each may be a scalar
// or a richer node (a list of tiers; a countersigned `na` mapping).
type structureYAML struct {
	Authority yaml.Node `yaml:"authority"`
	Geometry  yaml.Node `yaml:"geometry"`
}

// geometryNA reports whether the geometry cell is the `na` ceiling cap (a scalar
// "na" or a mapping with status: na) and, for the mapping form, its reviewed_by.
func (s structureYAML) geometryNA() (na bool, reviewedBy string) {
	switch s.Geometry.Kind {
	case yaml.ScalarNode:
		return strings.EqualFold(s.Geometry.Value, "na"), ""
	case yaml.MappingNode:
		var g struct {
			Status     string `yaml:"status"`
			ReviewedBy string `yaml:"reviewed_by"`
		}
		_ = s.Geometry.Decode(&g)
		return strings.EqualFold(g.Status, "na"), g.ReviewedBy
	}
	return false, ""
}

// TestStructureCoverage counts formats without a structure.yaml (Structure &
// Geometry axis — the AD-028 authority/ceiling declaration; the G-rung floor
// itself is grepped from the package by the audit, not declared here, so most
// formats correctly have none). Advisory only.
//
// When a structure.yaml IS present its shape is validated (a hard gate, like the
// other artifact registries): authority is from the fixed tier enum, and an `na`
// geometry cell — the ceiling cap on this cumulative depth ladder — must be
// countersigned with reviewed_by (format-maturity.md §2.7; SHARPEN §5 decision 6).
func TestStructureCoverage(t *testing.T) {
	advisoryArtifactCoverage(t, "structure.yaml", "Structure G")

	for _, id := range realFormatDirs(t) {
		p := filepath.Join(id, "structure.yaml")
		if !fileExists(p) {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("read %s: %v", p, err)
			continue
		}
		var doc structureYAML
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Errorf("structure.yaml: format %q: not valid YAML: %v", id, err)
			continue
		}
		for _, a := range authorityValues(doc.Authority) {
			if !structureAuthorities[a] {
				t.Errorf("structure.yaml: format %q declares authority %q; want one of %v "+
					"(AD-028 provenance tier, format-maturity.md §2.7)", id, a, structureAuthorityNames)
			}
		}
		if na, reviewedBy := doc.geometryNA(); na && reviewedBy == "" {
			t.Errorf("structure.yaml: format %q has geometry status \"na\" (the G4 ceiling cap) "+
				"without reviewed_by — `na` on this cumulative depth ladder is a COUNTERSIGNED "+
				"state (format-maturity.md §2.7; SHARPEN §5 decision 6)", id)
		}
	}
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

// proseLanguage is one entry of core/formats/prose.yaml, the languages the
// Prose axis tracks that have no format directory (format-maturity.md §2.8).
type proseLanguage struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
}

// proseCanaryDir is the one place the probe's canary subjects may be named.
// Every subject starting with proseCanaryPrefix is reserved for them.
const proseCanaryDir = "scripts/proseprobe/canary"

const proseCanaryPrefix = "canary"

var (
	proseSubjectRe = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	// proseTestNameRe is the naming contract for a rung test; scripts/proseprobe
	// applies the same pattern when it scores.
	proseTestNameRe = regexp.MustCompile(`^TestProseP([0-4])_([a-z][a-z0-9]*)$`)
	// proseClaimRe finds every test that claims the axis, well formed or not.
	proseClaimRe = regexp.MustCompile(`(?m)^func\s+(TestProseP[0-9]\w*)\s*\(`)
)

func readProseLanguages(t *testing.T) map[string]proseLanguage {
	t.Helper()
	var doc struct {
		Languages map[string]proseLanguage `yaml:"languages"`
	}
	if err := yaml.Unmarshal(readRegistryFile(t, "prose.yaml"), &doc); err != nil {
		t.Fatalf("parse prose.yaml: %v", err)
	}
	return doc.Languages
}

func realFormatSet(t *testing.T) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	for _, id := range realFormatDirs(t) {
		set[id] = true
	}
	return set
}

// proseLanguageProblem explains why a registry entry is refused, or returns "".
func proseLanguageProblem(id string, l proseLanguage, formats map[string]bool) string {
	switch {
	case !proseSubjectRe.MatchString(id):
		return fmt.Sprintf("prose.yaml: language %q is not lowercase letters and digits, so no rung test name can carry it", id)
	case strings.HasPrefix(id, proseCanaryPrefix):
		return fmt.Sprintf("prose.yaml: %q starts with %q, which the probe reserves for its canaries", id, proseCanaryPrefix)
	case formats[id]:
		return fmt.Sprintf("prose.yaml: %q is a format under core/formats; a format is scored on its own row and is not listed as a language", id)
	case strings.TrimSpace(l.Name) == "":
		return fmt.Sprintf("prose.yaml: language %q has no name", id)
	case l.Provider != "" && !formats[l.Provider]:
		return fmt.Sprintf("prose.yaml: language %q names provider %q, which is not a format under core/formats", id, l.Provider)
	}
	return ""
}

// proseTestNameProblem explains why a test claiming the Prose axis cannot be
// placed, or returns "". dir is the test's directory, repository-relative.
func proseTestNameProblem(name, dir string, formats map[string]bool, langs map[string]proseLanguage) string {
	m := proseTestNameRe.FindStringSubmatch(name)
	if m == nil {
		return fmt.Sprintf("%s in %s claims the Prose axis but does not match TestProseP<0-4>_<subject>", name, dir)
	}
	subject := m[2]
	if strings.HasPrefix(subject, proseCanaryPrefix) {
		if dir != proseCanaryDir {
			return fmt.Sprintf("%s in %s names the probe's canary, which lives only in %s", name, dir, proseCanaryDir)
		}
		return ""
	}
	if _, ok := langs[subject]; !ok && !formats[subject] {
		return fmt.Sprintf("%s in %s names %q, which is neither a format under core/formats nor a language in prose.yaml", name, dir, subject)
	}
	return ""
}

// TestProseRegistry is a hard gate on core/formats/prose.yaml: language ids
// are usable in a rung test name, never collide with a format or the canary,
// and name a real format when they name a provider.
func TestProseRegistry(t *testing.T) {
	langs := readProseLanguages(t)
	if len(langs) == 0 {
		t.Fatal("prose.yaml parsed but lists no languages")
	}
	formats := realFormatSet(t)
	for id, l := range langs {
		if msg := proseLanguageProblem(id, l, formats); msg != "" {
			t.Error(msg)
		}
	}
}

func TestProseRegistryRefuses(t *testing.T) {
	formats := map[string]bool{"yaml": true, "sourcecode": true}
	assert.Empty(t, proseLanguageProblem("go", proseLanguage{Name: "Go"}, formats))
	assert.Empty(t, proseLanguageProblem("ruby", proseLanguage{Name: "Ruby", Provider: "sourcecode"}, formats))
	for id, l := range map[string]proseLanguage{
		"Go":      {Name: "Go"},
		"go-lang": {Name: "Go"},
		"canary":  {Name: "Canary"},
		"canaryx": {Name: "X"},
		"yaml":    {Name: "YAML"},
		"rust":    {},
		"python":  {Name: "Python", Provider: "treesitter"},
	} {
		assert.NotEmpty(t, proseLanguageProblem(id, l, formats), id)
	}
}

// TestProseRungTestNames holds every test in the repository that claims the
// Prose axis to the naming contract, so a misspelt rung test fails here rather
// than scoring nothing. The walk must find the canary; a walk that finds no
// claim at all has read nothing and fails.
func TestProseRungTestNames(t *testing.T) {
	root := filepath.Join("..", "..")
	if !fileExists(filepath.Join(root, "go.work")) {
		t.Skip("no go.work above core/formats (partial checkout): the repository cannot be walked")
	}
	formats := realFormatSet(t)
	langs := readProseLanguages(t)

	claims := 0
	canary := 0
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
				name == "node_modules" || name == "testdata" || name == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, filepath.Dir(p))
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(rel)
		for _, m := range proseClaimRe.FindAllStringSubmatch(string(src), -1) {
			claims++
			if dir == proseCanaryDir {
				canary++
			}
			if msg := proseTestNameProblem(m[1], dir, formats, langs); msg != "" {
				t.Error(msg)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the repository: %v", err)
	}
	if canary == 0 {
		t.Fatalf("found %d Prose rung tests and none in %s: the walk did not reach the probe's canary", claims, proseCanaryDir)
	}
	t.Logf("%d Prose rung tests follow the naming contract", claims)
}

func TestProseRungTestNamesRefuse(t *testing.T) {
	formats := map[string]bool{"yaml": true}
	langs := map[string]proseLanguage{"go": {Name: "Go"}}
	for _, ok := range []struct{ name, dir string }{
		{"TestProseP1_yaml", "core/formats/yaml"},
		{"TestProseP2_go", "host/check"},
		{"TestProseP0_go", "plugins/gocomments"},
		{"TestProseP4_canary", proseCanaryDir},
		{"TestProseP1_canaryunlinked", proseCanaryDir},
	} {
		assert.Empty(t, proseTestNameProblem(ok.name, ok.dir, formats, langs), ok.name)
	}
	for _, bad := range []struct{ name, dir string }{
		{"TestProseP5_go", "host/check"},
		{"TestProseP1_Go", "host/check"},
		{"TestProseP1_golang", "host/check"},
		{"TestProseP1_go_edges", "host/check"},
		{"TestProseP1go", "host/check"},
		{"TestProseP3_canary", "core/formats/yaml"},
		{"TestProseP1_canarypresent", "core/formats/yaml"},
	} {
		assert.NotEmpty(t, proseTestNameProblem(bad.name, bad.dir, formats, langs), bad.name)
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
