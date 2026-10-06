package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host"
)

// An ARB catalog declares its own language (@@locale), and the recipe names
// another one. The change service and a flow keep the language the file
// declares, so the basis a checkout records is taken under it, while a venue
// takes every source revision under the project's language. These tests drive
// the real change service and a real flow, and read what the push sends.

const (
	arbSource = "l10n/app_en.arb"
	arbFrench = "l10n/app_fr.arb"
)

// declaredCheckout is a checkout written in en-US whose ARB catalog declares
// en, beside a JSON catalog that declares no language.
func declaredCheckout(t *testing.T, srv *refServer) *BowrainSourceConnector {
	t.Helper()
	return checkoutOf(t, srv, "en-US", map[string]string{
		arbSource:         `{"@@locale": "en", "greeting": "Read the guide", "farewell": "Goodbye now"}` + "\n",
		"locales/en.json": `{"title": "Welcome"}` + "\n",
	},
		coreproj.Collection{Name: "app", Path: arbSource, Target: "l10n/app_{lang}.arb"},
		coreproj.Collection{Name: "site", Path: "locales/en.json", Target: "locales/{lang}.json"})
}

// approveFrench approves the French translation of key in doc through the
// change service, as a person's review does, and returns the record the
// project's ledger holds for it.
func approveFrench(t *testing.T, c *BowrainSourceConnector, doc, key string) state.UnitState {
	t.Helper()
	ctx := context.Background()
	svc, err := c.app.ChangeService(ctx, host.ChangeServiceOptions{Project: c.project.RecipePath(), SourceLocale: "en-US"})
	require.NoError(t, err)
	fr := model.EditionKey{Locale: "fr"}
	var read change.BlockRead
	_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: doc, Editions: []model.EditionKey{fr}},
		func(_ *model.Block, r change.BlockRead) error {
			if r.Ref.Block == key {
				read = r
			}
			return nil
		})
	require.NoError(t, err)
	require.NotEmpty(t, read.Editions["fr"].Rev, "the flow translated %s", key)
	at := read.Ref
	at.Edition = fr
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindDecide, At: at, IfMatch: read.Editions["fr"].Rev,
		Body: &change.Decide{Outcome: change.OutcomeEstablish},
	}}}, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	st, err := c.workingStore(ctx)
	require.NoError(t, err)
	units, err := st.All(ctx)
	require.NoError(t, err)
	for _, u := range units {
		if u.Unit == key && u.Variant == fr && u.Decision.ReviewState != "" {
			return u
		}
	}
	t.Fatalf("the ledger holds no approval of %s", key)
	return state.UnitState{}
}

// scannedSource is the block keyed key in item as a push reads it.
func scannedSource(t *testing.T, c *BowrainSourceConnector, item, key string) *model.Block {
	t.Helper()
	scan, err := c.scanLocal(context.Background(), nil)
	require.NoError(t, err)
	for _, b := range scan.blocks[item] {
		if change.BlockKey(b) == key {
			return b
		}
	}
	t.Fatalf("the push reads no %s in %s", key, item)
	return nil
}

// filedAs is the key a venue holding nothing files the block keyed key in
// item under, as a push resolves it.
func filedAs(t *testing.T, c *BowrainSourceConnector, item, key string) string {
	t.Helper()
	scan, err := c.scanLocal(context.Background(), nil)
	require.NoError(t, err)
	local := localBlockKeys(scan.blocks)
	host.ResolveIdentity(scan.blocks, host.Priors{})
	for _, b := range scan.blocks[item] {
		if local[b] == key {
			return b.Key
		}
	}
	t.Fatalf("the push reads no %s in %s", key, item)
	return ""
}

// sentFor is the decision record a push sent for the block keyed key in
// item, in French. The push sends it under the key the venue files the block
// by, never the key the checkout's ledger names it by.
func sentFor(t *testing.T, srv *refServer, c *BowrainSourceConnector, item, key string) venue.UnitDecision {
	t.Helper()
	filed := filedAs(t, c, item, key)
	require.NotEqual(t, key, filed, "a venue holding nothing mints the block a key of its own")
	for _, d := range srv.decisions {
		if d.ItemName == item && d.Unit == filed && d.Variant == "fr" {
			return d
		}
	}
	t.Fatalf("the push sent no record of %s under %s", key, filed)
	return venue.UnitDecision{}
}

// A checkout approves a translation of an ARB string, and a flow wrote that
// translation. Both recorded the source under the language the file declares;
// the push sends each basis as the venue's revision of that same source, so
// the venue grades the approval current and the translation fresh.
func TestPush_SendsEachBasisAsTheVenueTakesIt(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 5})
	conn := declaredCheckout(t, srv)
	up(t, conn)
	record := approveFrench(t, conn, arbSource, "greeting")

	b := scannedSource(t, conn, arbSource, "greeting")
	require.Equal(t, model.LocaleID("en"), b.SourceLocale, "the push reads the language the file declares")
	want := venue.SourceRevision(b, "en-US")
	require.NotEqual(t, want, record.Basis,
		"the change service took the basis under the file's language, which a venue never stamps")
	require.True(t, state.ReadSource(b, "en-US").HoldsBasis(record.Basis), "the checkout reads its approval current")

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)

	d := sentFor(t, srv, conn, arbSource, "greeting")
	assert.Equal(t, venue.ReviewStateApproved, d.ReviewState)
	assert.Equal(t, want, d.Basis, "the approval names the source as the venue takes its revision")
	assert.Equal(t, record.Revision, d.Revision, "the translation half is unchanged")

	byUnit := map[string]string{}
	for _, key := range []string{"greeting", "farewell"} {
		byUnit[filedAs(t, conn, arbSource, key)] = key
	}
	var writes int
	for _, w := range srv.writes {
		if w.ItemName != arbSource {
			continue
		}
		writes++
		key, ok := byUnit[w.Unit]
		require.True(t, ok, "the write names %s by the key the venue files a block under", w.Unit)
		assert.Equal(t, venue.WriterTool, w.Writer, "the flow wrote %s", key)
		assert.Equal(t, venue.SourceRevision(scannedSource(t, conn, arbSource, key), "en-US"), w.Basis,
			"the flow's write of %s names its source as the venue takes it", key)
	}
	assert.Equal(t, 2, writes, "both translations the flow wrote go with the push")
}

// A push of named paths still carries every decision the checkout holds, and
// reads the source of each one it did not scan to send its basis.
func TestPush_OfNamedPathsSendsEveryBasisAsTheVenueTakesIt(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 5})
	conn := declaredCheckout(t, srv)
	up(t, conn)
	approveFrench(t, conn, arbSource, "greeting")

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{
		Paths: []string{filepath.Join(conn.project.Root, "locales", "en.json")},
	})
	require.NoError(t, err)

	d := sentFor(t, srv, conn, arbSource, "greeting")
	assert.Equal(t, venue.SourceRevision(scannedSource(t, conn, arbSource, "greeting"), "en-US"), d.Basis)
}

// A basis of another source than the one the checkout holds travels as it
// is: the venue grades it stale, as the checkout does.
func TestPush_SendsAStaleBasisAsItIs(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 5})
	conn := declaredCheckout(t, srv)
	up(t, conn)
	record := approveFrench(t, conn, arbSource, "greeting")

	require.NoError(t, os.WriteFile(filepath.Join(conn.project.Root, filepath.FromSlash(arbSource)),
		[]byte(`{"@@locale": "en", "greeting": "Read the whole guide", "farewell": "Goodbye now"}`+"\n"), 0o644))
	b := scannedSource(t, conn, arbSource, "greeting")
	require.False(t, state.ReadSource(b, "en-US").HoldsBasis(record.Basis), "the checkout reads its approval stale")

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	d := sentFor(t, srv, conn, arbSource, "greeting")
	assert.Equal(t, record.Basis, d.Basis)
	assert.NotEqual(t, venue.SourceRevision(b, "en-US"), d.Basis, "the venue grades it stale")
}

// A flow writes the French catalog of an ARB file that declares en, and a
// person then edits a French translation through the change service. The
// French file declares fr after each write.
func TestUp_WritesTheTargetsLocaleIntoAnARBCatalog(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 5})
	conn := declaredCheckout(t, srv)
	up(t, conn)
	french := filepath.Join(conn.project.Root, filepath.FromSlash(arbFrench))
	body, err := os.ReadFile(french)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"@@locale": "fr"`, "the flow wrote the French file")

	approveFrench(t, conn, arbSource, "greeting")
	ctx := context.Background()
	svc, err := conn.app.ChangeService(ctx, host.ChangeServiceOptions{Project: conn.project.RecipePath(), SourceLocale: "en-US"})
	require.NoError(t, err)
	fr := model.EditionKey{Locale: "fr"}
	var read change.BlockRead
	_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: arbSource, Editions: []model.EditionKey{fr}},
		func(_ *model.Block, r change.BlockRead) error {
			if r.Ref.Block == "farewell" {
				read = r
			}
			return nil
		})
	require.NoError(t, err)
	text := "Au revoir"
	at := read.Ref
	at.Edition = fr
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{{
		Kind: change.KindSetContent, At: at, IfMatch: read.Editions["fr"].Rev, Body: &change.SetContent{Text: &text},
	}}}, change.Actor{Kind: change.ActorPerson})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	body, err = os.ReadFile(french)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"@@locale": "fr"`, "the change service wrote the French file")
	assert.Contains(t, string(body), `"farewell": "Au revoir"`)
}
