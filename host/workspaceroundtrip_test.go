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
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/kpz"
	"github.com/neokapi/neokapi/memory"
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
collections:
  - name: app
    content:
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
	db := openProjectStore(t, root)
	require.NoError(t, db.Memory().Add(context.Background(), memory.Entry{
		ID:          "rt-1",
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Hello"}}},
			"nb": {{Text: &model.TextRun{Text: "Hei"}}},
		},
	}))
	require.NoError(t, db.Close())
	return root
}

// openProjectStore opens a project's store directly, for tests that seed or
// inspect it outside an App.
func openProjectStore(t *testing.T, root string) *projectdb.DB {
	t.Helper()
	db, err := projectdb.Open(t.Context(), project.Layout{
		Root: root, StateDir: filepath.Join(root, project.StateDirName),
	})
	require.NoError(t, err)
	return db
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

// TestPackSkipsEmptyPartsOfAnOpenStore is the same contract asked of the state
// the merged store made ambiguous.
//
// Pack used to gate each part on a FILE existing: no `memory.db`, no memory
// part. Every subsystem's schema now exists from the store's first open, so
// file presence stopped distinguishing "this project has a content memory" from
// "this project has a state directory" — and a stat-based gate would pack an
// empty part for every project that ever ran a command.
//
// So the gate is rows. A project whose store is open but empty must pack exactly
// as one with no store at all: not at all.
func TestPackSkipsEmptyPartsOfAnOpenStore(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"),
		[]byte("version: v1\nname: empty\ndefaults:\n  source_language: en\n"), 0o644))

	// Open the store and write nothing: every table exists, every one is empty.
	db := openProjectStore(t, root)
	for _, probe := range []struct {
		name string
		has  func(context.Context) (bool, error)
	}{
		{"memory", db.HasMemory},
		{"terms", db.HasTerms},
		{"blocks", db.HasBlocks},
	} {
		has, err := probe.has(t.Context())
		require.NoError(t, err)
		require.False(t, has, "%s must start empty", probe.name)
	}
	require.NoError(t, db.Close())
	require.FileExists(t, project.LayoutAt(root).StorePath(),
		"the store file exists — that is precisely why presence cannot be a stat")

	t.Chdir(root)
	cmd := NewEnvCommand(context.Background(), "pack")
	AddProjectFlag(cmd)
	cmd.Flags().String("output", filepath.Join(t.TempDir(), "s.kpz"), "")
	cmd.Flags().Bool("log", false, "")
	cmd.Flags().Bool("with-source", false, "")

	a := &App{SourceLang: "en"}
	defer a.Shutdown()
	err := a.RunPack(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to pack",
		"an open-but-empty store carries nothing, exactly like no store at all")
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

// TestRestoreSourcesRefusesToEscapeTheProject: an archive arrives from somewhere
// else, so a source member must never be able to name a path outside the
// directory it is unpacked into. Refused, not silently skipped — a snapshot that
// asks for this is not one to half-apply.
func TestRestoreSourcesRefusesToEscapeTheProject(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim.txt")

	for _, tc := range []struct {
		name, member, wantErr string
	}{
		{"parent traversal", kpz.SourceDir + "../../victim.txt", "escapes the project root"},
		{"absolute path", kpz.SourceDir + "/etc/victim.txt", "escapes the project root"},
		{"empty path", kpz.SourceDir, "no usable relative path"},
		{"traversal mid-path", kpz.SourceDir + "src/../../victim.txt", "escapes the project root"},
		{"bare dot-dot", kpz.SourceDir + "..", "escapes the project root"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkg := &kpz.Package{Source: []kpz.SourceDoc{{
				Path: tc.member, Content: kpz.BytesContent([]byte("PWNED")),
			}}}
			err := restoreSources(pkg, root)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.NoFileExists(t, outside)
		})
	}
}

// TestRestoreSourcesKeepsAnExistingFile: the working tree is authoritative on its
// own sources, so unpack must not overwrite one that is already there.
func TestRestoreSourcesKeepsAnExistingFile(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	existing := filepath.Join(root, "src", "en.json")
	require.NoError(t, os.WriteFile(existing, []byte(`{"local":"wins"}`), 0o644))

	pkg := &kpz.Package{Source: []kpz.SourceDoc{
		{Path: kpz.SourceDir + "src/en.json", Content: kpz.BytesContent([]byte(`{"archive":"loses"}`))},
		{Path: kpz.SourceDir + "src/nested/de.json", Content: kpz.BytesContent([]byte(`{"new":"lands"}`))},
	}}
	require.NoError(t, restoreSources(pkg, root))

	assert.JSONEq(t, `{"local":"wins"}`, string(mustRead(t, existing)),
		"an existing source must not be overwritten")
	assert.JSONEq(t, `{"new":"lands"}`, string(mustRead(t, filepath.Join(root, "src", "nested", "de.json"))),
		"a missing source must be restored, creating its directory")
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
	db := openProjectStore(t, root)
	defer func() { require.NoError(t, db.Close()) }()

	entries, err := db.Memory().Entries(context.Background())
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
