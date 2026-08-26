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
			// The #1869 shape: a help string gained wording that names its
			// parameters, and the committed translation is of the sentence
			// before it. The two documents still pair by key, so nothing else
			// tells them apart; the placeholders the target does not carry do.
			name:          "a target that dropped a text placeholder is refused",
			ext:           ".json",
			source:        `{"target":"Write it to {path} in {lang}","ok":"OK, ready"}`,
			target:        `{"target":"Skriv den til stien","ok":"OK, klar"}`,
			wantPairs:     1,
			wantLearned:   1,
			wantRefused:   1,
			wantDocuments: 1,
			wantMemory:    map[string]string{"OK, ready": "OK, klar"},
		},
		{
			// Coverage is never the question: a plural translated with the
			// categories the target language needs carries what it owes.
			name:          "a plural translated with fewer categories is absorbed",
			ext:           ".json",
			source:        `{"n":"{count, plural, one {# file} other {# files}}"}`,
			target:        `{"n":"{count, plural, other {# filer}}"}`,
			wantPairs:     1,
			wantLearned:   1,
			wantDocuments: 1,
			wantMemory: map[string]string{
				"{count, plural, one {# file} other {# files}}": "{count, plural, other {# filer}}",
			},
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
			assert.Equal(t, tc.wantRefused, res.Record.Refused, "pairs refused for dropped placeholders")

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

// newTwoChannelRecordProject writes a project whose two collections sit at two
// points of one product's context space — the shape the repo's own recipe has,
// where the CLI's catalog and the engine's are reviewed apart and absorbed
// together.
func newTwoChannelRecordProject(t *testing.T) (a *App, root, recipe string) {
	t.Helper()
	root = t.TempDir()
	for _, dir := range []string{"cli", "engine"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o755))
	}

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "TwoChannelRecordTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Profiles: map[string]project.Profile{
			"neokapi": {Channels: []project.Channel{{ID: "cli"}, {ID: "engine"}}},
		},
		Collections: []project.Collection{
			{Name: "neokapi-cli", Channel: "neokapi/cli", Path: "cli/en.json", Target: "cli/{lang}.json"},
			{Name: "neokapi-engine", Channel: "neokapi/engine", Path: "engine/en.json", Target: "engine/{lang}.json"},
		},
	}
	recipe = filepath.Join(root, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))

	a = &App{}
	a.InitRegistries()
	return a, root, recipe
}

// lookupFrom asks the store the question a fill asks: for this source, at this
// point, what does the record answer? It is the full-score, unambiguous
// question a `fillTargetThreshold: 100` policy puts.
func lookupFrom(t *testing.T, a *App, root, source, at string) []memory.Match {
	t.Helper()
	db, err := a.ProjectDB(context.Background(), root)
	require.NoError(t, err)
	tm := db.Memory()
	require.NotNil(t, tm)
	block := &model.Block{ID: "q", Translatable: true}
	block.SetSourceRuns([]model.Run{{Text: &model.TextRun{Text: source}}})
	matches, err := tm.Lookup(context.Background(), block, "en", "nb", memory.LookupOptions{
		MinScore: 1.0, MaxResults: 20, Point: at,
	})
	require.NoError(t, err)
	return matches
}

// TestAbsorbCommittedRecord_ContestedSourceResolvesToTheNearestApproval is the
// defect this file was written around, in miniature: one English string, two
// Norwegian answers, each approved in a different collection of the same
// product. The record used to pick between them by how often the corpus
// repeated each spelling, so the CLI's own reviewed wording was replaced by the
// engine's for no reason other than the engine having three occurrences to the
// CLI's one — and adding or removing an unrelated string could flip it back.
//
// The engine's answer is BOTH the more repeated one and the one that sorts
// first, so neither the old rule nor the tie-break can produce this result: only
// asking from where the string sits does.
func TestAbsorbCommittedRecord_ContestedSourceResolvesToTheNearestApproval(t *testing.T) {
	a, root, recipe := newTwoChannelRecordProject(t)
	writeDoc(t, root, "cli/en.json", `{"recycle":"Recycle"}`)
	writeDoc(t, root, "cli/nb.json", `{"recycle":"Gjenbruk"}`)
	writeDoc(t, root, "engine/en.json", `{"a":"Recycle","b":"Recycle","c":"Recycle"}`)
	writeDoc(t, root, "engine/nb.json", `{"a":"Bruk om igjen","b":"Bruk om igjen","c":"Bruk om igjen"}`)

	res, err := a.SeedProjectContext(context.Background(), recipe)
	require.NoError(t, err)
	assert.Equal(t, 4, res.Record.Pairs)
	assert.Equal(t, 1, res.Record.Contested, "one source string, answered two ways")
	assert.Equal(t, 2, res.Record.Learned, "one entry per approval, not one winner")

	cli := memory.NewPoint("neokapi", "cli", "neokapi-cli")
	engine := memory.NewPoint("neokapi", "engine", "neokapi-engine")

	fromCLI := lookupFrom(t, a, root, "Recycle", cli)
	require.NotEmpty(t, fromCLI)
	assert.False(t, fromCLI[0].Ambiguous)
	assert.Equal(t, "Gjenbruk", fromCLI[0].Entry.VariantText("nb"),
		"the CLI is answered by the approval made in the CLI's own collection, "+
			"though the other answer is repeated three times over and sorts first")

	fromEngine := lookupFrom(t, a, root, "Recycle", engine)
	require.NotEmpty(t, fromEngine)
	assert.False(t, fromEngine[0].Ambiguous)
	assert.Equal(t, "Bruk om igjen", fromEngine[0].Entry.VariantText("nb"))
}

// TestAbsorbCommittedRecord_ContestedSourceIsNamed: a count says a disagreement
// exists and leaves nobody able to look at it. Both candidates are real
// translations a reader approved, so the only way to audit the resolver is to
// see both answers and where each was approved.
func TestAbsorbCommittedRecord_ContestedSourceIsNamed(t *testing.T) {
	a, root, recipe := newTwoChannelRecordProject(t)
	writeDoc(t, root, "cli/en.json", `{"recycle":"Recycle"}`)
	writeDoc(t, root, "cli/nb.json", `{"recycle":"Gjenbruk"}`)
	writeDoc(t, root, "engine/en.json", `{"a":"Recycle"}`)
	writeDoc(t, root, "engine/nb.json", `{"a":"Bruk om igjen"}`)

	res, err := a.SeedProjectContext(context.Background(), recipe)
	require.NoError(t, err)
	require.Len(t, res.Record.ContestedSources, 1)
	c := res.Record.ContestedSources[0]
	assert.Equal(t, "Recycle", c.Source)
	assert.Equal(t, model.LocaleID("nb"), c.Locale)
	assert.Equal(t, []ContestedAnswer{
		{Target: "Gjenbruk", Point: "neokapi/cli/neokapi-cli", Governs: true},
		{Target: "Bruk om igjen", Point: "neokapi/engine/neokapi-engine", Governs: true},
	}, c.Answers, "each answer, and the point that approved it")

	line := formatSeedLine(res)
	assert.Contains(t, line, "1 source string(s) the record answers more than one way")
	assert.Contains(t, line, `contested: "Recycle" in nb`)
	assert.Contains(t, line, `"Gjenbruk" approved at neokapi/cli/neokapi-cli`)
	assert.Contains(t, line, `"Bruk om igjen" approved at neokapi/engine/neokapi-engine`)
}

// TestAbsorbCommittedRecord_TwoApprovalsAtOnePointFallToTheirText: coordinates
// cannot separate two answers approved in the same collection, and the fallback
// must not be the repeat count the coordinates replaced — a winner that moves
// when unrelated content moves is exactly the defect. The answer's own text
// decides instead: arbitrary in meaning, and a function of the two answers
// alone, so it cannot move when anything else does.
func TestAbsorbCommittedRecord_TwoApprovalsAtOnePointFallToTheirText(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	writeDoc(t, root, "src/en.json", `{"a":"Name","b":"Name","c":"Name"}`)
	writeDoc(t, root, "src/nb.json", `{"a":"Navn","b":"Navn","c":"Betegnelse"}`)

	res, err := a.SeedProjectContext(context.Background(), recipe)
	require.NoError(t, err)
	assert.Equal(t, 3, res.Record.Pairs)
	assert.Equal(t, 1, res.Record.Contested)
	assert.Equal(t, 1, res.Record.Learned, "one point, so one entry")

	at := memory.NewPoint("", "", "app")
	matches := lookupFrom(t, a, root, "Name", at)
	require.Len(t, matches, 1)
	assert.False(t, matches[0].Ambiguous)
	assert.Equal(t, "Betegnelse", matches[0].Entry.VariantText("nb"),
		"the smaller text, not the twice-repeated one")

	// The corpus grows a third occurrence of the losing spelling. Under the
	// repeat count that flips the answer; under the tie-break nothing moves.
	writeDoc(t, root, "src/en.json", `{"a":"Name","b":"Name","c":"Name","d":"Name"}`)
	writeDoc(t, root, "src/nb.json", `{"a":"Navn","b":"Navn","c":"Betegnelse","d":"Navn"}`)
	_, err = a.SeedProjectContext(context.Background(), recipe)
	require.NoError(t, err)
	after := lookupFrom(t, a, root, "Name", at)
	require.Len(t, after, 1)
	assert.Equal(t, "Betegnelse", after[0].Entry.VariantText("nb"),
		"a rebuild reproduces the wording it started from")
}

// TestAbsorbCommittedRecord_ContestedSourceIsAmbiguousWithoutAPoint: a reader
// who cannot say where they are asking from has no way to prefer one approval
// over another, and the corpus says so rather than picking. That is the same
// answer it always gave; what changed is that a caller who CAN say where it is
// now gets a decision instead.
func TestAbsorbCommittedRecord_ContestedSourceIsAmbiguousWithoutAPoint(t *testing.T) {
	a, root, recipe := newTwoChannelRecordProject(t)
	writeDoc(t, root, "cli/en.json", `{"recycle":"Recycle"}`)
	writeDoc(t, root, "cli/nb.json", `{"recycle":"Gjenbruk"}`)
	writeDoc(t, root, "engine/en.json", `{"a":"Recycle"}`)
	writeDoc(t, root, "engine/nb.json", `{"a":"Bruk om igjen"}`)

	_, err := a.SeedProjectContext(context.Background(), recipe)
	require.NoError(t, err)

	assert.Empty(t, lookupFrom(t, a, root, "Recycle", ""),
		"a full-score policy takes neither approval when it cannot say which one governs here")

	db, err := a.ProjectDB(context.Background(), root)
	require.NoError(t, err)
	block := &model.Block{ID: "q", Translatable: true}
	block.SetSourceRuns([]model.Run{{Text: &model.TextRun{Text: "Recycle"}}})
	matches, err := db.Memory().Lookup(context.Background(), block, "en", "nb",
		memory.LookupOptions{MinScore: 0.7, MaxResults: 20})
	require.NoError(t, err)
	require.Len(t, matches, 2)
	for _, m := range matches {
		assert.True(t, m.Ambiguous, "both approvals are offered, and neither is filled unattended")
	}
}

// TestAbsorbCommittedRecord_RelearnsAStoreKeyedWithoutItsPoint: a store written
// before an answer carried the point that approved it holds one row per source,
// with whichever wording the corpus repeated most. Left alone, that row sits
// beside the point-keyed ones as a rival bound to no location — competing at
// full score with the approvals that actually govern.
//
// The absorber's own entries are re-learned rather than migrated, because
// everything it taught the corpus is reproducible from the committed
// translations it read. What the corpus learned elsewhere is not the record's to
// forget, and is left where it is.
func TestAbsorbCommittedRecord_RelearnsAStoreKeyedWithoutItsPoint(t *testing.T) {
	a, root, recipe := newTwoChannelRecordProject(t)
	ctx := context.Background()
	writeDoc(t, root, "cli/en.json", `{"recycle":"Recycle"}`)
	writeDoc(t, root, "cli/nb.json", `{"recycle":"Gjenbruk"}`)
	writeDoc(t, root, "engine/en.json", `{"a":"Recycle"}`)
	writeDoc(t, root, "engine/nb.json", `{"a":"Bruk om igjen"}`)

	_, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)

	// A store as an older kapi left it: one record-derived row for the source,
	// carrying no point, and the pass having stamped every artifact as read.
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	legacy := memory.Entry{
		ID:          "record:legacy",
		HintSrcLang: "en",
		Origins:     []memory.Origin{{Source: "record", Key: "cli/nb.json"}},
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Recycle"}}},
			"nb": {{Text: &model.TextRun{Text: "Attvinning"}}},
		},
	}
	require.NoError(t, db.Memory().Add(ctx, legacy))
	require.NoError(t, db.PutMeta(ctx, MetaRecordScheme, ""))

	res, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Record.Documents, "every committed target is read again")

	entries := storeEntries(t, a, root)
	require.NotEmpty(t, entries)
	all, err := db.Memory().Entries(ctx)
	require.NoError(t, err)
	for _, e := range all {
		assert.NotEqual(t, "record:legacy", e.ID, "the row bound to no location is gone")
	}

	fromCLI := lookupFrom(t, a, root, "Recycle", memory.NewPoint("neokapi", "cli", "neokapi-cli"))
	require.NotEmpty(t, fromCLI)
	assert.Equal(t, "Gjenbruk", fromCLI[0].Entry.VariantText("nb"))
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

// The superseded pairing. A catalog entry keeps its key across a source
// rewrite, so its old translation stays sitting beside the new sentence. The
// absorber reads documents, not decisions, so it read that adjacency as a pair
// and taught the memory an exact answer for wording nobody had ever translated
// — which is what left `kapi up` recycling a stale target back over itself
// every pass and reporting the drift it had just confirmed.

// approveRecordUnit approves the (key, nb) unit through the real review path, so
// the decision carries the basis a live approval would.
func approveRecordUnit(t *testing.T, a *App, recipe, key string) {
	t.Helper()
	approveRecordUnitIn(t, a, recipe, "nb", ".json", key)
}

// approveRecordUnitIn is the same for any locale of the fixture.
func approveRecordUnitIn(t *testing.T, a *App, recipe, locale, ext, key string) {
	t.Helper()
	changed, err := a.ApproveReviewUnit(context.Background(), recipe, "en", locale,
		"src/"+locale+ext, key, "reviewed")
	require.NoError(t, err)
	require.True(t, changed)
}

// extractRecordBlocks populates the project block store from the working tree —
// the pre-pass step every `kapi up` runs, and the reason the wording a decision
// blessed is still recoverable after the source file has moved on.
func extractRecordBlocks(t *testing.T, a *App, recipe string) {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	pctx := project.NewProjectContext(proj, recipe)
	resolved, rerr := pctx.ResolveContent(a.FormatReg)
	require.NoError(t, rerr)
	_, _, serr := a.syncProjectBlockStore(context.Background(), pctx, recipe, resolved)
	require.NoError(t, serr)
}

// TestAbsorbCommittedRecord_SupersededPairingKeepsItsOwnSource: the reviewer's
// wording is absorbed against the sentence it actually translates, recovered
// from the block store by the decision's own basis hash. Refusing the pair
// outright would be safe but lossy — a translation the loop produced and a
// person then approved has never been absorbed (the run stamps its own output),
// so the rewrite would throw it away instead of leaving it as leverage.
func TestAbsorbCommittedRecord_SupersededPairingKeepsItsOwnSource(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	writeDoc(t, root, "src/en.json", `{"greeting":"Hello world"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"Hei verden"}`)
	ctx := context.Background()

	extractRecordBlocks(t, a, recipe)
	approveRecordUnit(t, a, recipe, "greeting")

	// The source sentence is rewritten; its key survives, so the old
	// translation is still beside it.
	writeDoc(t, root, "src/en.json", `{"greeting":"Good evening, world"}`)

	res, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Record.Superseded, "the record's own basis contradicts the document's layout")
	assert.Equal(t, 1, res.Record.Learned)

	entries := storeEntries(t, a, root)
	require.Len(t, entries, 1)
	_, wrong := entries["Good evening, world"]
	assert.False(t, wrong, "the rewritten sentence has no translation, and the memory must not claim one")
	blessed := entries["Hello world"]
	assert.Equal(t, "Hei verden", blessed.VariantText("nb"),
		"the reviewer's wording is kept under the sentence it translates, as leverage for the rewrite")
}

// TestAbsorbCommittedRecord_UnrecoverableSupersededPairingIsRefused: with no
// extracted blocks there is nothing to recover the blessed wording from. The
// pair is then not written at all — a memory with one fewer entry costs a
// lookup, an entry asserting a translation of the wrong sentence costs the
// translation.
func TestAbsorbCommittedRecord_UnrecoverableSupersededPairingIsRefused(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	writeDoc(t, root, "src/en.json", `{"greeting":"Hello world"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"Hei verden"}`)

	approveRecordUnit(t, a, recipe, "greeting")
	writeDoc(t, root, "src/en.json", `{"greeting":"Good evening, world"}`)

	res, err := a.SeedProjectContext(context.Background(), recipe)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Record.Superseded)
	assert.Zero(t, res.Record.Learned, "an unrecoverable pairing is declined, not guessed at")
	assert.Empty(t, storeEntries(t, a, root))
	assert.True(t, res.Compiled(), "and the decline is reported, not silent")
}

// TestAbsorbCommittedRecord_SupersededOnlyWhileTheDecisionStillBlessesTheTarget:
// the refusal is scoped to the pairing the record actually describes. Once the
// translation has moved on too — the loop re-drafted it, or a person rewrote it
// — the decision says nothing about what is on disk, and the pair is absorbed
// like any undecided one. Without this the unit would be refused forever and
// every `kapi up` would pay to draft it again.
func TestAbsorbCommittedRecord_SupersededOnlyWhileTheDecisionStillBlessesTheTarget(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	writeDoc(t, root, "src/en.json", `{"greeting":"Hello world"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"Hei verden"}`)
	ctx := context.Background()

	extractRecordBlocks(t, a, recipe)
	approveRecordUnit(t, a, recipe, "greeting")

	// Both halves move: the source is rewritten and the translation with it.
	writeDoc(t, root, "src/en.json", `{"greeting":"Good evening, world"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"God kveld, verden"}`)

	res, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	assert.Zero(t, res.Record.Superseded, "neither half of the decision is on disk any more")
	assert.Equal(t, 1, res.Record.Learned)
	learned := storeEntries(t, a, root)["Good evening, world"]
	assert.Equal(t, "God kveld, verden", learned.VariantText("nb"))
}

// A source rewrite is a fact about the SOURCE, so it mispairs every locale of
// the unit. Only the decided locale had anything to contradict the adjacency
// with, so the others learned the mispairing and went on serving the translation
// of a deleted sentence — with the loop reporting them caught up.

// newRecordProjectLocales writes the record fixture with more than one target
// language, which is what makes "every locale" a question at all.
func newRecordProjectLocales(t *testing.T, locales ...model.LocaleID) (a *App, root, recipe string) {
	t.Helper()
	a, root, recipe = newRecordProject(t, ".json")
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.TargetLanguages = locales
	require.NoError(t, project.Save(recipe, proj))
	return a, root, recipe
}

// TestAbsorbCommittedRecord_RewrittenSourceMispairsEveryLocale: one locale holds
// a decision, two hold none. The block store still has the wording the committed
// targets were produced against, and the corpus still answers it with exactly
// those targets — which is what says they have not moved with the source. Every
// one of them is kept under the sentence it translates, and the rewritten
// sentence is left with no translation at all, because it has none.
func TestAbsorbCommittedRecord_RewrittenSourceMispairsEveryLocale(t *testing.T) {
	a, root, recipe := newRecordProjectLocales(t, "nb", "de", "nl")
	writeDoc(t, root, "src/en.json", `{"greeting":"Hello world"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"Hei verden"}`)
	writeDoc(t, root, "src/de.json", `{"greeting":"Hallo Welt"}`)
	writeDoc(t, root, "src/nl.json", `{"greeting":"Hallo wereld"}`)
	ctx := context.Background()

	extractRecordBlocks(t, a, recipe)
	warm, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	require.Equal(t, 1, warm.Record.Learned, "the corpus holds what the committed targets say")

	// Only de is reviewed. The other two locales have nothing on record.
	approveRecordUnitIn(t, a, recipe, "de", ".json", "greeting")

	writeDoc(t, root, "src/en.json", `{"greeting":"Good evening, world"}`)

	res, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	assert.Equal(t, 3, res.Record.Superseded,
		"the rewrite mispairs the undecided locales exactly as it mispairs the decided one")
	assert.Zero(t, res.Record.Learned, "each pair is re-asserted under the source it already had")

	entries := storeEntries(t, a, root)
	_, wrong := entries["Good evening, world"]
	assert.False(t, wrong, "no locale claims a translation of the rewritten sentence")
	blessed := entries["Hello world"]
	assert.Equal(t, "Hei verden", blessed.VariantText("nb"))
	assert.Equal(t, "Hallo Welt", blessed.VariantText("de"))
	assert.Equal(t, "Hallo wereld", blessed.VariantText("nl"),
		"a locale with no decision keeps its wording under the sentence it translates too")
}

// TestAbsorbCommittedRecord_SourceAndTargetRewrittenTogetherIsTheFreshPairing is
// the control that stops the rule above from becoming "refuse on any source
// edit". A person who rewrites a sentence and its translation in one commit has
// authored a pairing, not left a stale one — the corpus does not answer the old
// wording with what is on disk, so nothing contradicts it and it is absorbed.
// Without this the loop would take the new translation away from them and
// re-draft over it.
func TestAbsorbCommittedRecord_SourceAndTargetRewrittenTogetherIsTheFreshPairing(t *testing.T) {
	a, root, recipe := newRecordProjectLocales(t, "nb", "de")
	writeDoc(t, root, "src/en.json", `{"greeting":"Hello world"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"Hei verden"}`)
	writeDoc(t, root, "src/de.json", `{"greeting":"Hallo Welt"}`)
	ctx := context.Background()

	extractRecordBlocks(t, a, recipe)
	_, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)

	// nb moves with the source; de is left behind.
	writeDoc(t, root, "src/en.json", `{"greeting":"Good evening, world"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"God kveld, verden"}`)

	res, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Record.Superseded, "only the locale that stayed behind is mispaired")

	entries := storeEntries(t, a, root)
	fresh, held := entries["Good evening, world"], entries["Hello world"]
	assert.Equal(t, "God kveld, verden", fresh.VariantText("nb"),
		"the pairing a person authored is learned, not refused")
	assert.Empty(t, fresh.VariantText("de"), "and the locale that did not move claims nothing")
	assert.Equal(t, "Hallo Welt", held.VariantText("de"))
}

// TestAbsorbCommittedRecord_NoPriorSourceLeavesTheAdjacencyAlone: with nothing
// extracted there is no record of what the targets were produced against, so
// there is no rewrite to see. The absorber reads the documents as it always did
// — the honest posture for a first pass on a fresh clone, where the store has
// never described this project.
func TestAbsorbCommittedRecord_NoPriorSourceLeavesTheAdjacencyAlone(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	writeDoc(t, root, "src/en.json", `{"greeting":"Good evening, world"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"Hei verden"}`)

	res, err := a.SeedProjectContext(context.Background(), recipe)
	require.NoError(t, err)
	assert.Zero(t, res.Record.Superseded)
	assert.Equal(t, 1, res.Record.Learned)
	learned := storeEntries(t, a, root)["Good evening, world"]
	assert.Equal(t, "Hei verden", learned.VariantText("nb"))
}

// The identical pair. A translation legitimately equal to its source — a proper
// noun, a product name, a short label — was dropped with the untranslated leaves
// a catalog carries verbatim, so the loop never learned it, re-drafted it with
// the provider on every pass, and overwrote the approval bound to it.
func TestAbsorbCommittedRecord_IdenticalPair(t *testing.T) {
	tests := []struct {
		name        string
		approve     bool
		wantLearned int
		wantMemory  string
	}{
		{
			name:        "an approved identical translation is a decision, and is absorbed",
			approve:     true,
			wantLearned: 1,
			wantMemory:  "Terminal",
		},
		{
			name:        "an identical leaf nobody decided is an untranslated one",
			approve:     false,
			wantLearned: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, root, recipe := newRecordProject(t, ".json")
			writeDoc(t, root, "src/en.json", `{"terminal":"Terminal"}`)
			writeDoc(t, root, "src/nb.json", `{"terminal":"Terminal"}`)
			if tc.approve {
				approveRecordUnit(t, a, recipe, "terminal")
			}

			res, err := a.SeedProjectContext(context.Background(), recipe)
			require.NoError(t, err)
			assert.Equal(t, tc.wantLearned, res.Record.Learned)

			entries := storeEntries(t, a, root)
			if tc.wantMemory == "" {
				assert.Empty(t, entries)
				return
			}
			require.Len(t, entries, 1)
			learned := entries["Terminal"]
			assert.Equal(t, tc.wantMemory, learned.VariantText("nb"),
				"recycle can fill the unit, so the AI step never sees it again")
		})
	}
}

// TestAbsorbCommittedRecord_ApprovalDoesNotSurviveAnEditToWhatItApproved: the
// identical pair is admitted by the decision, not by the identity, so a decision
// that no longer describes the translation on disk admits nothing. Otherwise an
// approval of one wording would let any later identical target in behind it.
func TestAbsorbCommittedRecord_ApprovalDoesNotSurviveAnEditToWhatItApproved(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	writeDoc(t, root, "src/en.json", `{"terminal":"Terminal"}`)
	writeDoc(t, root, "src/nb.json", `{"terminal":"Terminalen"}`)
	approveRecordUnit(t, a, recipe, "terminal")

	// The translation is edited to the source wording after the approval, so
	// the decision on record judges wording that is no longer there.
	writeDoc(t, root, "src/nb.json", `{"terminal":"Terminal"}`)

	res, err := a.SeedProjectContext(context.Background(), recipe)
	require.NoError(t, err)
	assert.Zero(t, res.Record.Learned)
	assert.Empty(t, storeEntries(t, a, root))
}

// TestAbsorbCommittedRecord_CarriesTheBlockIdentity: an absorbed answer records
// which block it was approved for, so it joins that block's version chain
// rather than standing alone as a source string somebody once approved.
//
// Without this the field, the storage, the bundle and the query all exist and
// every chain is empty — a feature that looks built and answers nothing.
func TestAbsorbCommittedRecord_CarriesTheBlockIdentity(t *testing.T) {
	a, root, recipe := newRecordProject(t, ".json")
	writeDoc(t, root, "src/en.json", `{"greeting":"Hello world","farewell":"Goodbye"}`)
	writeDoc(t, root, "src/nb.json", `{"greeting":"Hei verden","farewell":"Ha det"}`)

	res, err := a.SeedProjectContext(context.Background(), recipe)
	require.NoError(t, err)
	require.Equal(t, 2, res.Record.Learned)

	entries := storeEntries(t, a, root)
	require.Len(t, entries, 2)
	for src, e := range entries {
		assert.NotEmpty(t, e.Unit,
			"the answer for %q must name the block it was approved for", src)
	}

	// Two blocks, two identities: a chain keyed on a shared unit would braid
	// unrelated answers together.
	assert.NotEqual(t, entries["Hello world"].Unit, entries["Goodbye"].Unit,
		"different blocks must not share a chain")
}
