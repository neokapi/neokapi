package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/sievepen"
)

// The trio's contract is a round trip: `pack` writes exactly the parts `info`
// lists, and `unpack` restores exactly those parts. These tests hold that —
// pack → unpack must reproduce the state, and `info` must describe the same
// archive from either side.

// writeArchiveProject builds a project with content, a memory store and a terms
// store, so a snapshot has every part to carry.
func writeArchiveProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()

	recipe := `version: v1
name: archive-demo
defaults:
  source_language: en
  target_languages: [nb, de]
content:
  - name: app
    items:
      - path: src/en.json
        target: "src/{lang}.json"
`
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "en.json"),
		[]byte(`{"greeting":"Hello","farewell":"Goodbye"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "nb.json"),
		[]byte(`{"greeting":"Hei","farewell":"Farvel"}`), 0o644))

	// A snapshot carries authoritative state, so the project needs some: seed
	// the content memory the way a translated run would leave it. Without this
	// `pack` correctly refuses ("nothing to pack"), which is its own contract
	// and is asserted separately.
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
	tm, err := sievepen.NewSQLiteTM(filepath.Join(root, project.StateDirName, "tm.db"))
	require.NoError(t, err)
	require.NoError(t, tm.Add(context.Background(), sievepen.TMEntry{
		ID:          "rt-1",
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Hello"}}},
			"nb": {{Text: &model.TextRun{Text: "Hei"}}},
		},
	}))
	require.NoError(t, tm.Close())
	return root
}

// seedExtraction records an extraction batch for the project's source file, the
// way `kapi extract` leaves one: a manifest naming the source, its content hash
// and its round-trip skeleton, plus the skeleton file itself. `pack` takes its
// source list from these manifests, so a project that has never been extracted
// packs no sources however many files its recipe tracks.
func seedExtraction(t *testing.T, root string) {
	t.Helper()
	layout, err := project.LayoutFor(filepath.Join(root, "kapi.yaml"))
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	const batch = "batch-1"
	dir, err := project.EnsureExtractionDir(layout, batch)
	require.NoError(t, err)

	raw := mustRead(t, filepath.Join(root, "src", "en.json"))
	sum := sha256.Sum256(raw)
	hash := "sha256:" + hex.EncodeToString(sum[:])

	skeleton := project.SkeletonFilename(strings.TrimPrefix(hash, "sha256:"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, skeleton), []byte("SKELETON"), 0o644))

	require.NoError(t, project.SaveExtractionManifest(layout, &project.ExtractionManifest{
		SchemaVersion: project.ExtractionSchemaVersion,
		Kind:          project.ExtractionManifestKind,
		BatchID:       batch,
		Generator:     project.ExtractionGenerator{ID: "kapi", Version: "test"},
		SourceLocale:  "en",
		Pairs: []project.ExtractionPair{{
			TargetLocale: "nb",
			Output:       "out/nb",
			Files: []project.ExtractionFile{{
				Source:     "src/en.json",
				SourceHash: hash,
				Format:     "json",
				Blocks:     2,
				Skeleton:   skeleton,
			}},
		}},
	}))
}

// TestPackRefusesAnEmptyProject: a snapshot of nothing is not a hand-off, and
// saying so beats writing an archive that silently carries no state.
func TestPackRefusesAnEmptyProject(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"),
		[]byte("version: v1\nname: empty\ndefaults:\n  source_language: en\n"), 0o644))
	t.Chdir(root)

	cmd := NewEnvCommand(context.Background(), "pack")
	AddProjectFlag(cmd)
	cmd.Flags().String("output", filepath.Join(t.TempDir(), "s.kpz"), "")
	cmd.Flags().Bool("log", false, "")
	cmd.Flags().Bool("with-source", false, "")

	err := (&App{SourceLang: "en"}).RunPack(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to pack")
}

// infoCmd builds a Command with the info flag surface and captured IO.
func infoCmd(t *testing.T, jsonOut bool) *EnvCommand {
	t.Helper()
	cmd := NewEnvCommand(context.Background(), "info")
	AddProjectFlag(cmd)
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Bool("text", false, "")
	cmd.Flags().String("output-format", "", "")
	cmd.Flags().String("jq", "", "")
	cmd.Flags().String("color", "never", "")
	if jsonOut {
		require.NoError(t, cmd.Flags().Set("json", "true"))
	}
	return cmd
}

// TestProjectInfoDescribesTheArchiveParts: `kapi info` in a project must name
// every part a snapshot carries, and say what it omits — that is the whole point
// of being able to ask before packing.
func TestProjectInfoDescribesTheArchiveParts(t *testing.T) {
	t.Chdir(writeArchiveProject(t))

	a := &App{}
	cmd := infoCmd(t, true)
	out, err := captureStdout(t, func() error { return a.RunProjectInfo(cmd) })
	require.NoError(t, err)

	var got ProjectInfoOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got), "info must emit valid JSON: %s", out)

	assert.Equal(t, "archive-demo", got.Project)
	assert.Equal(t, "en", got.SourceLang)
	assert.Equal(t, []string{"nb", "de"}, got.TargetLangs)
	assert.Equal(t, 1, got.Content, "one tracked source file")

	byName := map[string]ProjectArchivePart{}
	for _, p := range got.Parts {
		byName[p.Name] = p
	}
	for _, want := range []string{"recipe", "sources", "blocks", "overlays", "memory", "terms"} {
		part, ok := byName[want]
		require.Truef(t, ok, "info must account for the %q part", want)
		assert.NotEmptyf(t, part.Note, "%q must say what it is", want)
	}
	assert.True(t, byName["recipe"].Present, "the recipe is always packed")
	assert.Equal(t, 1, byName["memory"].Count, "the seeded memory entry is counted")

	// Nothing has been extracted, so no sources are packable — and the report
	// must say so rather than counting recipe-tracked files it would not carry.
	assert.False(t, byName["sources"].Present,
		"a project that has never been extracted packs no sources")
	assert.Contains(t, byName["sources"].Note, "kapi extract",
		"an absent part must say what would produce it")

	require.NotEmpty(t, got.Excluded, "what a snapshot omits must be stated, not implied")
	assert.Contains(t, got.Excluded[0], "cache")
}

// TestProjectInfoCountsWhatPackWouldCarry: after an extraction the sources part
// flips to present with a count, because `info` measures the same manifests
// `pack` walks.
func TestProjectInfoCountsWhatPackWouldCarry(t *testing.T) {
	root := writeArchiveProject(t)
	seedExtraction(t, root)
	t.Chdir(root)

	out, err := captureStdout(t, func() error { return (&App{}).RunProjectInfo(infoCmd(t, true)) })
	require.NoError(t, err)
	var got ProjectInfoOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got))

	sources := partByName(t, got.Parts, "sources")
	assert.True(t, sources.Present)
	assert.Equal(t, 1, sources.Count, "one extracted source")
	assert.Equal(t, archivePartNotes["sources"], sources.Note,
		"a present part carries its role, not the absence hint")
}

// TestProjectInfoPartNotesAreShared keeps one description per part, so the
// project view and the archive view cannot describe the same thing differently.
func TestProjectInfoPartNotesAreShared(t *testing.T) {
	for _, name := range []string{"recipe", "sources", "blocks", "overlays", "memory", "terms", "history"} {
		assert.NotEmptyf(t, ArchivePartNote(name), "%q needs a documented role", name)
	}
	assert.Empty(t, ArchivePartNote("not-a-part"))
}

// TestPackUnpackRoundTrip is the fidelity contract: pack a project, unpack it
// into a fresh directory, and the restored state must match.
func TestPackUnpackRoundTrip(t *testing.T) {
	src := writeArchiveProject(t)
	seedExtraction(t, src)
	t.Chdir(src)

	snapshot := filepath.Join(t.TempDir(), "snapshot.kpz")

	packCmd := NewEnvCommand(context.Background(), "pack")
	AddProjectFlag(packCmd)
	packCmd.Flags().String("output", snapshot, "")
	packCmd.Flags().Bool("log", false, "")
	packCmd.Flags().Bool("with-source", true, "")
	require.NoError(t, packCmd.Flags().Set("with-source", "true"))
	packCmd.Flags().Bool("json", false, "")
	packCmd.Flags().Bool("text", false, "")
	packCmd.Flags().String("output-format", "", "")
	packCmd.Flags().String("jq", "", "")
	packCmd.Flags().String("color", "never", "")

	a := &App{SourceLang: "en"}
	_, err := captureStdout(t, func() error { return a.RunPack(packCmd) })
	require.NoError(t, err, "pack must succeed")
	require.FileExists(t, snapshot)
	assert.Greater(t, fileSize(snapshot), int64(0), "a snapshot must not be empty")

	// Unpack into a fresh project directory holding only the recipe, as a
	// hand-off recipient would have.
	dst := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dst, "kapi.yaml"),
		mustRead(t, filepath.Join(src, "kapi.yaml")), 0o644))
	t.Chdir(dst)

	unpackCmd := NewEnvCommand(context.Background(), "unpack")
	AddProjectFlag(unpackCmd)
	unpackCmd.Flags().Bool("json", false, "")
	unpackCmd.Flags().Bool("text", false, "")
	unpackCmd.Flags().String("output-format", "", "")
	unpackCmd.Flags().String("jq", "", "")
	unpackCmd.Flags().String("color", "never", "")

	b := &App{SourceLang: "en"}
	_, err = captureStdout(t, func() error { return b.RunUnpack(unpackCmd, snapshot) })
	require.NoError(t, err, "unpack must succeed")

	// The restored project must report the same shape as the original.
	restored := infoCmd(t, true)
	rawRestored, err := captureStdout(t, func() error { return (&App{}).RunProjectInfo(restored) })
	require.NoError(t, err)
	var after ProjectInfoOutput
	require.NoError(t, json.Unmarshal([]byte(rawRestored), &after))

	assert.Equal(t, "archive-demo", after.Project, "the recipe survived the round trip")
	assert.Equal(t, "en", after.SourceLang)
	assert.Equal(t, []string{"nb", "de"}, after.TargetLangs)

	// Shape is not enough: the authoritative state must actually be there. The
	// content memory is the part a hand-off exists to carry, so compare entries
	// rather than merely asserting the file appeared.
	assert.True(t, partByName(t, after.Parts, "memory").Present,
		"the restored project must report a content memory")
	assert.Equal(t, tmEntries(t, src), tmEntries(t, dst),
		"pack → unpack must reproduce the content memory, not just its presence")

	// And the source file restored from --with-source must match byte for byte.
	assert.Equal(t,
		mustRead(t, filepath.Join(src, "src", "en.json")),
		mustRead(t, filepath.Join(dst, "src", "en.json")),
		"--with-source must restore the source bytes unchanged")
}

// TestUnpackWithoutSourceLeavesSourcesAlone: `pack` without --with-source
// carries identity + skeleton only, so unpacking must not fabricate source
// files. The omission is documented in `kapi info`; this holds it true.
func TestUnpackWithoutSourceLeavesSourcesAlone(t *testing.T) {
	src := writeArchiveProject(t)
	seedExtraction(t, src)
	t.Chdir(src)
	snapshot := filepath.Join(t.TempDir(), "lean.kpz")

	packCmd := NewEnvCommand(context.Background(), "pack")
	AddProjectFlag(packCmd)
	packCmd.Flags().String("output", snapshot, "")
	packCmd.Flags().Bool("log", false, "")
	packCmd.Flags().Bool("with-source", false, "")
	addOutputFlags(packCmd)

	_, err := captureStdout(t, func() error { return (&App{SourceLang: "en"}).RunPack(packCmd) })
	require.NoError(t, err)

	dst := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dst, "kapi.yaml"),
		mustRead(t, filepath.Join(src, "kapi.yaml")), 0o644))
	t.Chdir(dst)

	unpackCmd := NewEnvCommand(context.Background(), "unpack")
	AddProjectFlag(unpackCmd)
	addOutputFlags(unpackCmd)
	_, err = captureStdout(t, func() error { return (&App{SourceLang: "en"}).RunUnpack(unpackCmd, snapshot) })
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(dst, "src", "en.json"),
		"a lean snapshot carries no source bytes, so none may appear")
	// The state it does carry still lands.
	assert.Equal(t, tmEntries(t, src), tmEntries(t, dst),
		"the content memory travels with or without --with-source")
}

// addOutputFlags registers the --json/--output-format surface output.Print reads.
func addOutputFlags(cmd *EnvCommand) {
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Bool("text", false, "")
	cmd.Flags().String("output-format", "", "")
	cmd.Flags().String("jq", "", "")
	cmd.Flags().String("color", "never", "")
}

// partByName finds one archive part in an info result.
func partByName(t *testing.T, parts []ProjectArchivePart, name string) ProjectArchivePart {
	t.Helper()
	for _, p := range parts {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("info did not account for the %q part", name)
	return ProjectArchivePart{}
}

// tmEntries reads a project's content memory as a comparable snapshot: source
// text → per-locale variant text, so two projects can be compared without
// depending on row ids or insertion order.
func tmEntries(t *testing.T, root string) map[string]map[string]string {
	t.Helper()
	tm, err := sievepen.NewSQLiteTM(filepath.Join(root, project.StateDirName, "tm.db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, tm.Close()) }()

	entries, err := tm.Entries(context.Background())
	require.NoError(t, err)

	got := map[string]map[string]string{}
	for _, e := range entries {
		byLocale := map[string]string{}
		for locale, runs := range e.Variants {
			byLocale[string(locale)] = model.RunsPlainText(runs)
		}
		// Key on the source-language text: stable across a round trip in a way
		// entry ids need not be.
		src := byLocale[string(e.HintSrcLang)]
		got[src] = byLocale
	}
	return got
}

// TestProjectAndArchiveInfoAgree is the round-trip check a user can run: the
// parts `kapi info` lists for a project and the parts it lists for that
// project's snapshot must be the same parts, present in the same places, with
// the same counts and the same words. Two reports that disagree would mean pack
// did not carry what info promised.
func TestProjectAndArchiveInfoAgree(t *testing.T) {
	root := writeArchiveProject(t)
	seedExtraction(t, root)
	t.Chdir(root)

	before, err := captureStdout(t, func() error { return (&App{}).RunProjectInfo(infoCmd(t, true)) })
	require.NoError(t, err)
	var proj ProjectInfoOutput
	require.NoError(t, json.Unmarshal([]byte(before), &proj))

	snapshot := filepath.Join(t.TempDir(), "agree.kpz")
	packCmd := NewEnvCommand(context.Background(), "pack")
	AddProjectFlag(packCmd)
	packCmd.Flags().String("output", snapshot, "")
	packCmd.Flags().Bool("log", false, "")
	packCmd.Flags().Bool("with-source", true, "")
	require.NoError(t, packCmd.Flags().Set("with-source", "true"))
	addOutputFlags(packCmd)
	_, err = captureStdout(t, func() error { return (&App{SourceLang: "en"}).RunPack(packCmd) })
	require.NoError(t, err)

	raw, err := captureStdout(t, func() error {
		return (&App{}).RunArchiveInfo(infoCmd(t, true), snapshot, nil)
	})
	require.NoError(t, err)
	var arch ArchiveInfoOutput
	require.NoError(t, json.Unmarshal([]byte(raw), &arch))

	assert.Equal(t, proj.Project, arch.Project)
	assert.Equal(t, proj.SourceLang, arch.SourceLang)
	assert.Equal(t, proj.TargetLangs, arch.TargetLangs)

	// Same parts, same presence, same counts, same words — for every part the
	// project reported. (The archive adds `history`, which a project has no
	// on-disk equivalent of.)
	for _, want := range proj.Parts {
		got := partByName(t, arch.Parts, want.Name)
		assert.Equalf(t, want.Present, got.Present, "part %q: presence must survive the pack", want.Name)
		assert.Equalf(t, want.Count, got.Count, "part %q: count must survive the pack", want.Name)
		assert.Equalf(t, want.Note, got.Note, "part %q: described in the same words on both sides", want.Name)
	}
	partByName(t, arch.Parts, "history")
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}
