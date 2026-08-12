package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRecordProject writes a project whose one collection pairs src/en.<ext> with
// src/{lang}.<ext> — the shape every committed target artifact has: a document
// in the source's own shape, in another language. Nothing is compiled; the store
// is as empty as it is after `git clone`.
func newRecordProject(t *testing.T, ext string) (a *App, root, recipe string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "RecordTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Collections: []project.Collection{
			{Name: "app", Path: "src/en" + ext, Target: "src/{lang}" + ext},
		},
	}
	recipe = filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))

	a = &App{}
	a.InitRegistries()
	return a, root, recipe
}

func writeDoc(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// storeEntries returns the project content memory's entries, keyed by their
// source text.
func storeEntries(t *testing.T, a *App, root string) map[string]memory.Entry {
	t.Helper()
	db, err := a.ProjectDB(context.Background(), root)
	require.NoError(t, err)
	tm := db.Memory()
	require.NotNil(t, tm)
	all, err := tm.Entries(context.Background())
	require.NoError(t, err)
	out := map[string]memory.Entry{}
	for _, e := range all {
		out[e.VariantText("en")] = e
	}
	return out
}

// lookupFullScore asks the store the question a full-score fill policy asks:
// what does it say for this source, at score 1.0 and unambiguously? An empty
// result is the store declining to answer — which is what an unresolved
// disagreement looks like from the fill path.
func lookupFullScore(t *testing.T, a *App, root, source string) []memory.Match {
	t.Helper()
	db, err := a.ProjectDB(context.Background(), root)
	require.NoError(t, err)
	tm := db.Memory()
	require.NotNil(t, tm)
	block := &model.Block{ID: "q", Translatable: true}
	block.SetSourceRuns([]model.Run{{Text: &model.TextRun{Text: source}}})
	matches, err := tm.Lookup(context.Background(), block, "en", "nb", memory.LookupOptions{MinScore: 1.0, MaxResults: 20})
	require.NoError(t, err)
	return matches
}

func TestAbsorbCommittedRecord(t *testing.T) {
	tests := []struct {
		name string
		ext  string
		// source and target are the two committed documents; an empty target
		// means the locale has no committed artifact yet.
		source, target string
		wantPairs      int
		wantLearned    int
		wantRefused    int
		wantDocuments  int
		// wantMemory is the source→target pair the store must hold afterwards.
		wantMemory map[string]string
	}{
		{
			name:          "a committed target teaches every pair it carries",
			ext:           ".json",
			source:        `{"greeting":"Hello world","farewell":"Goodbye"}`,
			target:        `{"greeting":"Hei verden","farewell":"Ha det"}`,
			wantPairs:     2,
			wantLearned:   2,
			wantDocuments: 1,
			wantMemory:    map[string]string{"Hello world": "Hei verden", "Goodbye": "Ha det"},
		},
		{
			name:          "a target leaf identical to its source is not a pair",
			ext:           ".json",
			source:        `{"greeting":"Hello world","ok":"OK"}`,
			target:        `{"greeting":"Hei verden","ok":"OK"}`,
			wantPairs:     1,
			wantLearned:   1,
			wantDocuments: 1,
			wantMemory:    map[string]string{"Hello world": "Hei verden"},
		},
		{
			name:          "a target that dropped an inline code is refused",
			ext:           ".md",
			source:        "**Hello** world\n\nPlain line\n",
			target:        "Hei verden\n\nEnkel linje\n",
			wantPairs:     1,
			wantLearned:   1,
			wantRefused:   1,
			wantDocuments: 1,
			wantMemory:    map[string]string{"Plain line": "Enkel linje"},
		},
		{
			name:   "no committed target teaches nothing",
			ext:    ".json",
			source: `{"greeting":"Hello world"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, root, recipe := newRecordProject(t, tc.ext)
			writeDoc(t, root, "src/en"+tc.ext, tc.source)
			if tc.target != "" {
				writeDoc(t, root, "src/nb"+tc.ext, tc.target)
			}

			res, err := a.SeedProjectContext(context.Background(), recipe)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDocuments, res.Record.Documents, "committed target documents read")
			assert.Equal(t, tc.wantPairs, res.Record.Pairs, "pairs absorbed")
			assert.Equal(t, tc.wantLearned, res.Record.Learned, "entries learned")
			assert.Equal(t, tc.wantRefused, res.Record.Refused, "pairs refused for dropped inline codes")

			entries := storeEntries(t, a, root)
			assert.Len(t, entries, len(tc.wantMemory))
			for src, want := range tc.wantMemory {
				e, ok := entries[src]
				require.True(t, ok, "the store holds a pair for %q", src)
				assert.Equal(t, want, e.VariantText("nb"))
				require.Len(t, e.Origins, 1)
				assert.Equal(t, recordOriginSource, e.Origins[0].Source,
					"a record-derived entry says so, so a reader can tell it from a seed")
				assert.Equal(t, "src/nb"+tc.ext, e.Origins[0].Key)
			}
		})
	}
}

// TestAbsorbCommittedRecord_DigestKeyed: an unchanged pair of documents absorbs
// nothing on the next pass, and an edited target absorbs exactly itself. This is
// what keeps `kapi up` from re-reading the whole committed corpus every run.
func TestAbsorbCommittedRecord_DigestKeyed(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	writeDoc(t, root, "src/en.json", `{"greeting":"Hello world"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"Hei verden"}`)
	ctx := context.Background()

	first, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	require.Equal(t, 1, first.Record.Learned)
	require.Zero(t, first.Record.Skipped)

	second, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	assert.Zero(t, second.Record.Documents, "an unchanged target is not read back")
	assert.Zero(t, second.Record.Learned)
	assert.Zero(t, second.Record.Reconciled)
	assert.Equal(t, 1, second.Record.Skipped)
	assert.False(t, second.Compiled(), "a second run has nothing to report")

	writeDoc(t, root, "src/nb.json", `{"greeting":"Hallo verden"}`)
	third, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	assert.Equal(t, 1, third.Record.Documents, "an edited target is read again")
	assert.Equal(t, 1, third.Record.Reconciled, "its own entry is corrected, not duplicated")
	entries := storeEntries(t, a, root)
	require.Len(t, entries, 1)
	corrected := entries["Hello world"]
	assert.Equal(t, "Hallo verden", corrected.VariantText("nb"))
}

// TestAbsorbCommittedRecord_RecordSupersedesSeed: the seed and the committed
// translation disagree about the same string. Left competing, the ambiguity rule
// demotes both and a full-score fill takes neither — the disagreement would cost
// the translation. The record is the later, reviewed answer, so it wins and the
// store answers unambiguously again.
func TestAbsorbCommittedRecord_RecordSupersedesSeed(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	writeDoc(t, root, "src/en.json", `{"greeting":"Hello world"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"Hei verden"}`)
	writeMemoryBundle(t, root, "app-nb", map[string]string{"Hello world": "Hallo verden"})

	res, err := a.SeedProjectContext(context.Background(), recipe)
	require.NoError(t, err)
	assert.Equal(t, 1, res.MemoryFiles, "the seed compiled first")
	assert.Equal(t, 1, res.Record.Reconciled, "then the record corrected it")
	assert.Zero(t, res.Record.Learned, "the seed's entry carries the answer; no rival is added")

	matches := lookupFullScore(t, a, root, "Hello world")
	require.Len(t, matches, 1, "one full-score answer, not an unresolved pair")
	assert.False(t, matches[0].Ambiguous)
	entry := matches[0].Entry
	assert.Equal(t, "Hei verden", entry.VariantText("nb"))

	seed, err := os.ReadFile(filepath.Join(project.LayoutAt(root).MemoryDir(), "app-nb.memory.json"))
	require.NoError(t, err)
	assert.Contains(t, string(seed), "Hallo verden",
		"the committed seed is read-only — it is the store that moved, not the file")
}

// TestAbsorbCommittedRecord_LaterStatementWins: precedence between a seed and
// the record is not a fixed ranking, it is which one moved. On the pass that
// compiles both — a fresh clone — the record answers last. Afterwards each
// artifact has had its say, so an edit to either is the newer statement: a seed
// edited on its own reaches the store, because the unchanged target has already
// been absorbed and re-asserting it would overwrite the edit that just arrived.
func TestAbsorbCommittedRecord_LaterStatementWins(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	writeDoc(t, root, "src/en.json", `{"greeting":"Hello world"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"Hei verden"}`)
	writeMemoryBundle(t, root, "app-nb", map[string]string{"Hello world": "Hallo verden"})
	ctx := context.Background()

	_, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	first := lookupFullScore(t, a, root, "Hello world")
	require.Len(t, first, 1)
	fromRecord := first[0].Entry
	assert.Equal(t, "Hei verden", fromRecord.VariantText("nb"), "the record answered last")

	writeMemoryBundle(t, root, "app-nb", map[string]string{"Hello world": "God dag verden"})
	res, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	require.Equal(t, 1, res.MemoryFiles, "the edited bundle recompiled")
	assert.Equal(t, 1, res.Record.Skipped, "the target has not moved, so it is not re-asserted")

	second := lookupFullScore(t, a, root, "Hello world")
	require.Len(t, second, 1)
	fromSeed := second[0].Entry
	assert.Equal(t, "God dag verden", fromSeed.VariantText("nb"))

	writeDoc(t, root, "src/nb.json", `{"greeting":"Hei, verden"}`)
	_, err = a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	third := lookupFullScore(t, a, root, "Hello world")
	require.Len(t, third, 1)
	fromNewerRecord := third[0].Entry
	assert.Equal(t, "Hei, verden", fromNewerRecord.VariantText("nb"),
		"an edited target is the newer statement in its turn")
}

// TestAbsorbCommittedRecord_ContestedSourceResolvesToTheRepeatedAnswer: the
// record itself answers one string two ways. No text-keyed store can hold both
// without making every occurrence unfillable, so the answer the corpus repeats
// wins and the disagreement is counted for a reader.
func TestAbsorbCommittedRecord_ContestedSourceResolvesToTheRepeatedAnswer(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	writeDoc(t, root, "src/en.json", `{"a":"Name","b":"Name","c":"Name"}`)
	writeDoc(t, root, "src/nb.json", `{"a":"Navn","b":"Navn","c":"Betegnelse"}`)

	res, err := a.SeedProjectContext(context.Background(), recipe)
	require.NoError(t, err)
	assert.Equal(t, 3, res.Record.Pairs)
	assert.Equal(t, 1, res.Record.Learned, "one source string, one entry")
	assert.Equal(t, 1, res.Record.Contested)

	matches := lookupFullScore(t, a, root, "Name")
	require.Len(t, matches, 1)
	assert.False(t, matches[0].Ambiguous)
	entry := matches[0].Entry
	assert.Equal(t, "Navn", entry.VariantText("nb"))
}

// TestAbsorbCommittedRecord_FoldsEveryLocaleIntoOneEntry: an entry is
// multilingual, so every locale's answer for one source belongs in it. Two
// locales are two pairs and one entry — and, when they correct an entry already
// in the store, one staged copy: each read returns the store's pre-run state, so
// two copies would disagree on the locales they did not touch and the write that
// landed second would put the other locale back.
func TestAbsorbCommittedRecord_FoldsEveryLocaleIntoOneEntry(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.TargetLanguages = []model.LocaleID{"nb", "de"}
	require.NoError(t, project.Save(recipe, proj))

	writeDoc(t, root, "src/en.json", `{"greeting":"Hello world"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"Hei verden"}`)
	writeDoc(t, root, "src/de.json", `{"greeting":"Hallo Welt"}`)
	ctx := context.Background()

	res, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Record.Pairs, "one pair per locale")
	assert.Equal(t, 1, res.Record.Learned, "one source, one entry")

	entries := storeEntries(t, a, root)
	require.Len(t, entries, 1)
	e := entries["Hello world"]
	assert.Equal(t, "Hei verden", e.VariantText("nb"))
	assert.Equal(t, "Hallo Welt", e.VariantText("de"))

	// Both targets move at once: each locale's correction must survive the other.
	writeDoc(t, root, "src/nb.json", `{"greeting":"Hallo verden"}`)
	writeDoc(t, root, "src/de.json", `{"greeting":"Hallo, Welt"}`)
	res, err = a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Record.Pairs, "both locales answered again")
	assert.Equal(t, 1, res.Record.Reconciled, "one entry corrected, in two locales")

	entries = storeEntries(t, a, root)
	require.Len(t, entries, 1)
	e = entries["Hello world"]
	assert.Equal(t, "Hallo verden", e.VariantText("nb"))
	assert.Equal(t, "Hallo, Welt", e.VariantText("de"))
}

// TestAbsorbCommittedRecord_KeepsPulledApproval is the survival property: an
// approval a venue pull wrote into the store is not committed to git, so an
// absorbing pass must be an upsert of what the record answers and nothing more.
// A string the record does not answer keeps the pulled wording; a string it does
// answer keeps it too once absorbed, because the digest stamp makes the next
// pass read nothing.
func TestAbsorbCommittedRecord_KeepsPulledApproval(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	writeDoc(t, root, "src/en.json", `{"greeting":"Hello world"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"Hei verden"}`)
	ctx := context.Background()

	_, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)

	// What a venue pull leaves behind: a pair for a string git carries no
	// artifact for, and a newer approval for one it does.
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	tm := db.Memory()
	require.NotNil(t, tm)
	now := time.Now().UTC()
	require.NoError(t, tm.Add(ctx, memory.Entry{
		ID:          "pulled:unseen",
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Not in any document"}}},
			"nb": {{Text: &model.TextRun{Text: "Ikke i noe dokument"}}},
		},
		CreatedAt: now, UpdatedAt: now,
	}))
	held := storeEntries(t, a, root)["Hello world"]
	held.Variants["nb"] = []model.Run{{Text: &model.TextRun{Text: "Hei, verden"}}}
	require.NoError(t, tm.Add(ctx, held))

	res, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	require.Equal(t, 1, res.Record.Skipped, "an unchanged artifact is not read back")

	entries := storeEntries(t, a, root)
	unseen, seen := entries["Not in any document"], entries["Hello world"]
	assert.Equal(t, "Ikke i noe dokument", unseen.VariantText("nb"),
		"a pair the record does not answer is untouched")
	assert.Equal(t, "Hei, verden", seen.VariantText("nb"),
		"the pulled approval survives — the artifact it came from has not moved")
}
