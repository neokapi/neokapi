package workhome_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changetest"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/workhome"
	"github.com/neokapi/neokapi/core/workspace"
)

// project is the key every machine of these tests keeps the project under.
const project workspace.ProjectKey = "prj_workhome"

// machine is one workspace: its log, the project's projector and the
// workspace home over them.
type machine struct {
	ws   *workspace.Workspace
	p    *projector.Projector
	home *workhome.Home
	st   projector.Stores
}

// newMachine opens the workspace kept in dir, as one machine (or one
// process) holds it.
func newMachine(t *testing.T, dir string) *machine {
	t.Helper()
	ctx := context.Background()
	ws, err := workspace.OpenLocal(ctx, dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })
	raw, err := ws.Context(ctx, project)
	require.NoError(t, err)
	st, err := projector.ContextStores(raw)
	require.NoError(t, err)
	p, err := projector.New(ws, project, st)
	require.NoError(t, err)
	return &machine{ws: ws, p: p, st: st, home: &workhome.Home{Store: st.Heads, Log: p}}
}

// keptLayout serves the JSON documents under root, each with a German
// edition the workspace home keeps until a delivery writes de/<name>.
type keptLayout struct {
	root string
	reg  *registry.FormatRegistry
	home filehome.Keeper
}

func (l keptLayout) Locate(ctx context.Context, doc string) (filehome.Doc, error) {
	var edition *model.EditionKey
	if rest, ok := strings.CutPrefix(doc, "de/"); ok {
		de := model.EditionKey{Locale: "de"}
		edition, doc = &de, rest
	}
	d, err := filehome.DirLayout{Root: l.root, Formats: l.reg, SourceLocale: "en"}.Locate(ctx, doc)
	if err != nil {
		return filehome.Doc{}, err
	}
	d.Edition = edition
	d.EditionFile = func(k model.EditionKey) (filehome.EditionFile, bool) {
		if k.Locale != "de" {
			return filehome.EditionFile{}, false
		}
		return filehome.EditionFile{Ref: "de/" + d.Ref, Path: filepath.Join(l.root, "de", filepath.FromSlash(d.Ref)), Kept: l.home}, true
	}
	return d, nil
}

// keptFixture is a directory of JSON documents whose German editions the
// workspace home of m keeps, and a change service over them.
type keptFixture struct {
	dir string
	m   *machine
	svc *change.Service
}

func newKeptFixture(t *testing.T, m *machine, files map[string]string, opts filehome.Options) *keptFixture {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	if opts.LockDir == "" {
		opts.LockDir = filepath.Join(t.TempDir(), "locks")
	}
	home := filehome.New(keptLayout{root: dir, reg: reg, home: m.home}, opts)
	svc := change.NewService(filehome.Formats{Registry: reg}, change.OneHome(home))
	return &keptFixture{dir: dir, m: m, svc: svc}
}

var person = change.Actor{Kind: change.ActorPerson, Name: "reviewer"}

// draft gives the German edition of each named block of doc its text, as one
// change set creating each.
func (f *keptFixture) draft(t *testing.T, doc string, texts map[string]string) *change.Result {
	t.Helper()
	var ops []change.Op
	for _, key := range sortedKeys(texts) {
		text := texts[key]
		ops = append(ops, change.Op{Kind: change.KindSetContent, IfMatch: model.AbsentRevision,
			At: change.Ref{Doc: doc, Block: key, Edition: model.EditionKey{Locale: "de"}}, Body: &change.SetContent{Text: &text}})
	}
	res, err := f.svc.Apply(context.Background(), change.Set{Ops: ops}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	return res
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// snapshot is what the workspace home keeps of the German edition of doc.
func (f *keptFixture) snapshot(t *testing.T, doc string) []byte {
	t.Helper()
	kept, err := f.m.home.Edition(context.Background(), doc, model.EditionKey{Locale: "de"})
	require.NoError(t, err)
	data, err := json.Marshal(kept)
	require.NoError(t, err)
	return data
}

// TestWorkspaceHome_Conformance runs the change service's conformance suite
// on editions the workspace home keeps: every document of the suite is the
// German edition of a JSON file, which no file holds.
func TestWorkspaceHome_Conformance(t *testing.T) {
	changetest.Run(t, func(t *testing.T) changetest.Env {
		var hook func(string)
		m := newMachine(t, t.TempDir())
		f := newKeptFixture(t, m, map[string]string{
			"a.json": `{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n",
			"b.json": `{"title": "Welcome"}` + "\n",
		}, filehome.Options{BeforeSettle: func(doc string) {
			if hook != nil {
				hook(doc)
			}
		}})
		f.draft(t, "a.json", map[string]string{"greeting": "Hallo", "farewell": "Auf Wiedersehen", "thanks": "Danke"})
		f.draft(t, "b.json", map[string]string{"title": "Willkommen"})
		return changetest.Env{
			SetBeforeSettle: func(fn func(string)) { hook = fn },
			Service:         f.svc,
			DocA:            "de/a.json",
			DocB:            "de/b.json",
			Snapshot: func(t *testing.T, doc string) []byte {
				return f.snapshot(t, strings.TrimPrefix(doc, "de/"))
			},
		}
	})
}

// TestWorkspaceHome_KeepsTheEditionInTheLog: a write to a kept edition lands
// in the workspace home and in no file, is recorded once with its result, and
// reads back with the status and origin the write left.
func TestWorkspaceHome_KeepsTheEditionInTheLog(t *testing.T) {
	ctx := context.Background()
	m := newMachine(t, t.TempDir())
	f := newKeptFixture(t, m, map[string]string{"a.json": `{"greeting": "Hello there"}` + "\n"}, filehome.Options{})

	res := f.draft(t, "a.json", map[string]string{"greeting": "Hallo"})
	require.NotNil(t, res.Record, "the write is recorded")
	var kept *change.DocResult
	for i, d := range res.Docs {
		if d.Edition == "de" {
			kept = &res.Docs[i]
		}
	}
	require.NotNil(t, kept, "the result names the German edition")
	assert.Equal(t, workhome.Name, kept.Home)
	assert.True(t, kept.Written)
	_, err := os.Stat(filepath.Join(f.dir, "de", "a.json"))
	assert.True(t, os.IsNotExist(err), "the workspace home writes no file")

	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "de/a.json"})
	require.NoError(t, err)
	assert.Equal(t, workhome.Name, page.Home, "a read of the kept edition names its home")
	require.Len(t, page.Blocks, 1)
	assert.Equal(t, "Hallo", page.Blocks[0].Text)

	rows, err := m.st.Heads.Rows(ctx, "a.json", "de")
	require.NoError(t, err)
	require.Contains(t, rows, "greeting")
	assert.Equal(t, model.Status(model.TargetStatusTranslated), rows["greeting"].Status, "a person's write leaves the edition translated")
	assert.Equal(t, model.OriginHuman, rows["greeting"].Origin.Kind)

	hist, err := m.st.History.Edition(ctx, "a.json", "greeting", "de", 0)
	require.NoError(t, err)
	require.Len(t, hist, 1, "the commit's record is the one record of the write")
	assert.Equal(t, *res.Record, hist[0].Op)
	assert.Equal(t, model.AbsentRevision, hist[0].Before)
	assert.Equal(t, rows["greeting"].Rev, hist[0].After)
}

// TestWorkspaceHome_RefusesAStaleHead: a commit staged on a head another
// writer has moved stores nothing and is refused doc_changed.
func TestWorkspaceHome_RefusesAStaleHead(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	first, second := newMachine(t, dir), newMachine(t, dir)
	f := newKeptFixture(t, first, map[string]string{"a.json": `{"greeting": "Hello there", "farewell": "Goodbye"}` + "\n"}, filehome.Options{})
	f.draft(t, "a.json", map[string]string{"greeting": "Hallo"})

	de := model.EditionKey{Locale: "de"}
	read, err := first.home.Edition(ctx, "a.json", de)
	require.NoError(t, err)

	// Another process, on the same workspace, writes the edition first.
	moved, err := second.home.Edition(ctx, "a.json", de)
	require.NoError(t, err)
	tschuess := model.Edition{Runs: []model.Run{model.TextR("Tschüss")}}
	_, err = second.home.Commit(ctx, filehome.KeptWrite{Doc: "a.json", Edition: de, Token: moved.Token,
		Changes: []filehome.KeptChange{{Block: "farewell", Before: model.AbsentRevision, Edition: &tschuess}},
		Record:  &change.Record{Actor: person, Origin: "apply"}})
	require.NoError(t, err)

	servus := model.Edition{Runs: []model.Run{model.TextR("Servus")}}
	_, err = first.home.Commit(ctx, filehome.KeptWrite{Doc: "a.json", Edition: de, Token: read.Token,
		Changes: []filehome.KeptChange{{Block: "greeting", Before: model.RunsRevision(de, read.Blocks["greeting"].Runs), Edition: &servus}},
		Record:  &change.Record{Actor: person, Origin: "apply"}})
	var ce *change.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, change.CodeDocChanged, ce.Code)

	now, err := first.home.Edition(ctx, "a.json", de)
	require.NoError(t, err)
	assert.Equal(t, "Hallo", model.RunsText(now.Blocks["greeting"].Runs), "the refused write stored nothing")
	assert.Equal(t, "Tschüss", model.RunsText(now.Blocks["farewell"].Runs), "and the other writer's write stands")
}

// TestWorkspaceHome_RebuildReproducesTheHeads: the workspace home's tables
// are projections of the log, so a rebuild writes them again exactly.
func TestWorkspaceHome_RebuildReproducesTheHeads(t *testing.T) {
	ctx := context.Background()
	m := newMachine(t, t.TempDir())
	f := newKeptFixture(t, m, map[string]string{"a.json": `{"greeting": "Hello there", "farewell": "Goodbye"}` + "\n"}, filehome.Options{})
	f.draft(t, "a.json", map[string]string{"greeting": "Hallo", "farewell": "Tschüss"})
	text := "Servus"
	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "de/a.json"})
	require.NoError(t, err)
	res, err := f.svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindSetContent, At: page.Blocks[0].Ref,
		IfMatch: page.Blocks[0].Rev, Body: &change.SetContent{Text: &text}}}}, person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	before := dumpLocal(t, m)
	_, err = m.p.Rebuild(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, dumpLocal(t, m), "a rebuild reproduces the workspace home")

	_, err = m.p.Checkpoint(ctx)
	require.NoError(t, err)
	report, err := m.p.Rebuild(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, report.Checkpoint, "the rebuild starts from the checkpoint")
	assert.Equal(t, before, dumpLocal(t, m), "and a checkpoint carries the workspace home")
}

// dumpHeads renders the workspace home's tables, for comparing the state two
// machines reach. The local position of a head's latest write differs from
// log to log, and is left out.
func dumpHeads(t *testing.T, m *machine) string {
	t.Helper()
	return dump(t, m, false)
}

// dumpLocal is dumpHeads with each head's local position, for comparing two
// states of one machine.
func dumpLocal(t *testing.T, m *machine) string {
	t.Helper()
	return dump(t, m, true)
}

func dump(t *testing.T, m *machine, local bool) string {
	t.Helper()
	ctx := context.Background()
	heads, err := m.st.Heads.Heads(ctx)
	require.NoError(t, err)
	var b strings.Builder
	for _, h := range heads {
		if !local {
			h.Seq = 0
		}
		data, err := json.Marshal(h)
		require.NoError(t, err)
		b.Write(data)
		b.WriteByte('\n')
		rows, err := m.st.Heads.Rows(ctx, h.Doc, h.Edition)
		require.NoError(t, err)
		for _, key := range sortedRowKeys(rows) {
			data, err := json.Marshal(rows[key])
			require.NoError(t, err)
			b.Write(data)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func sortedRowKeys(rows map[string]workhome.Row) []string {
	m := map[string]string{}
	for k := range rows {
		m[k] = k
	}
	return sortedKeys(m)
}
