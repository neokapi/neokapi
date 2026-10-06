package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// A checkout's records name a block by the key its reader gives it, and the
// venue files the block under the key a push resolves it to (Block.Key). The
// checkout's half, that a push sends each decision under Block.Key and a pull
// files the venue's decisions back under the reader's key, is tested in
// host/venue/source. These tests send what such a push sends through the real
// client to the real server, each push landing in PostgreSQL, and read the
// join where the server's projections, tallies and review read it.

const keyedItem = "locales/en.json"

var keyedItems = []apiclient.ItemMeta{{Name: keyedItem, Format: "json"}}

// keyedProject is a project the server holds nothing of yet, written in
// projectLanguage and translated into French.
func keyedProject(t *testing.T, projectLanguage string) (*Server, string, *apiclient.BowrainClient) {
	t.Helper()
	srv, token := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test/projects", strings.NewReader(
		`{"name":"Keys","default_source_language":"`+projectLanguage+`","target_languages":["fr"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	ts := httptest.NewServer(srv.GetEcho())
	t.Cleanup(ts.Close)
	return srv, created.ID, apiclient.NewProjectBearerClient(ts.URL, created.ID, token)
}

// keyedCatalog is the checkout's catalog as a push reads it, with a French
// translation of every entry, each block keyed as a push to a venue holding
// nothing resolves it: the venue offers no priors, so every block is minted a
// key of its own (reconcile, as host.ResolveIdentity runs it).
func keyedCatalog(t *testing.T) []*model.Block {
	t.Helper()
	var blocks []*model.Block
	for _, e := range []struct{ name, en, fr string }{
		{"greeting", "Hello there", "Bonjour"},
		{"farewell", "Goodbye now", "Au revoir"},
	} {
		b := &model.Block{ID: e.name, Name: e.name, Translatable: true, SourceLocale: "en-US"}
		b.SetSourceRuns([]model.Run{model.TextR(e.en)})
		b.SetEdition(model.Variant("fr"), model.Edition{
			Runs: []model.Run{model.TextR(e.fr)}, Status: model.Status(model.TargetStatusTranslated),
		})
		blocks = append(blocks, b)
	}
	docs := reconcile.Documents([]reconcile.Document{{Path: keyedItem, Blocks: blocks}}, nil)
	for _, r := range reconcile.Blocks(docs[0].Key, blocks, nil) {
		r.Block.Key = r.Key
	}
	for _, b := range blocks {
		require.NotEqual(t, b.Name, b.Key, "a venue holding nothing files %s under a key of its own", b.Name)
	}
	return blocks
}

// approvalOf is the record a push carries for a person's approval of b's
// French translation, under the key the push filed b by.
func approvalOf(b *model.Block) venue.UnitDecision {
	read := state.ReadTarget(b, "fr", "en-US")
	stamp := time.Now().UTC().Format(time.RFC3339)
	return venue.UnitDecision{
		ItemName: keyedItem, Unit: b.Key, Variant: "fr", Status: string(model.TargetStatusEstablished),
		Revision: read.Revision, Basis: read.Basis, ReviewState: venue.ReviewStateApproved,
		DecidedAt: stamp, Updated: stamp,
	}
}

// pushKeyed pushes the catalog and the records given, as a checkout's push
// does, and lands the push as a deployed worker does.
func pushKeyed(t *testing.T, srv *Server, client *apiclient.BowrainClient, blocks []*model.Block, decisions []venue.UnitDecision) {
	t.Helper()
	resp, err := client.Push(context.Background(), map[string][]*model.Block{keyedItem: blocks}, keyedItems, nil,
		decisions, apiclient.TransferUnder("en-US"))
	require.NoError(t, err)
	drainWithAuthority(t, srv)
	require.Nil(t, resp.Governance, "the server accepted every verdict the push carried")
}

// stored is the row the server holds for the block named name.
func stored(t *testing.T, srv *Server, pid, name string) *venue.StoredBlock {
	t.Helper()
	rows, err := srv.ContentStore.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: pid, Stream: "main", ItemName: keyedItem})
	require.NoError(t, err)
	for _, sb := range rows {
		if sb.Block.Name == name {
			return sb
		}
	}
	t.Fatalf("the server holds no %s", name)
	return nil
}

// A checkout translates its catalog and approves one translation before the
// project's first push. The push files each block under a key it minted, and
// sends the approval under the same key; the approval lands on that unit, the
// review reads it there, and the tallies read it current.
func TestCheckoutDecisionJoinsTheBlockItsFirstPushFiled(t *testing.T) {
	srv, pid, client := keyedProject(t, "en-US")
	blocks := keyedCatalog(t)
	greeting := blocks[0]
	approval := approvalOf(greeting)

	pushKeyed(t, srv, client, blocks, []venue.UnitDecision{approval})

	sb := stored(t, srv, pid, "greeting")
	require.Equal(t, greeting.Key, sb.Key, "the server files the block under the key the push resolved")

	ds, ok := srv.ContentStore.(platstore.DecisionStore)
	require.True(t, ok)
	records, err := ds.ListUnitDecisions(t.Context(), pid, "main")
	require.NoError(t, err)
	var units []string
	for _, d := range records {
		if d.Variant == "fr" && d.IsDecision() {
			units = append(units, d.ItemName+"|"+d.Unit)
		}
	}
	assert.Equal(t, []string{keyedItem + "|" + sb.Key}, units,
		"the approval is filed under the unit the server holds, and under no other key")

	d := srv.unitDecisionFor(t.Context(), pid, "main", sb, "fr")
	require.NotNil(t, d, "the review reads the approval on the block it judges")
	assert.Equal(t, venue.ReviewStateApproved, d.ReviewState)
	assert.Equal(t, approval.Revision, d.Revision)
	assert.False(t, reviewProvenanceOf(d, sb, "fr").Stale, "the approval's source is the one the server holds")

	fr, ok := sb.Block.Edition(model.Variant("fr"))
	require.True(t, ok)
	assert.Equal(t, model.TargetStatusEstablished, model.TargetStatus(fr.Status),
		"the translation projects the rung the approval establishes")

	tallies, err := ds.TallyDecisionBasis(t.Context(), pid, "main")
	require.NoError(t, err)
	var counted bool
	for _, tl := range tallies {
		if tl.Variant == "fr" {
			counted = true
			assert.Zero(t, tl.Stale, "nothing reads stale")
			assert.Zero(t, tl.BasisUnknown, "the approval names its source")
		}
	}
	assert.True(t, counted, "the tallies count the approval")
}

// A pull hands the checkout each decision under the key the server files its
// unit by, the key the checkout resolves its block to, and the checkout files
// it back under its reader's key.
func TestPullCarriesTheDecisionUnderTheServersKey(t *testing.T) {
	srv, pid, client := keyedProject(t, "en-US")
	blocks := keyedCatalog(t)
	pushKeyed(t, srv, client, blocks, []venue.UnitDecision{approvalOf(blocks[0])})
	sb := stored(t, srv, pid, "greeting")

	resp, err := client.Pull(context.Background(), 0, nil, 0)
	require.NoError(t, err)
	var units []string
	for _, d := range resp.Decisions {
		if d.Variant == "fr" && d.ReviewState != "" {
			units = append(units, d.ItemName+"|"+d.Unit)
		}
	}
	assert.Equal(t, []string{keyedItem + "|" + sb.Key}, units)
}

// A push taken under en-US to a project the server holds as written in en-GB
// is refused before anything is stored, with both languages named and the fix.
func TestPushIsRefusedWhenTheRecipesSourceLanguageIsNotTheProjects(t *testing.T) {
	srv, pid, client := keyedProject(t, "en-GB")
	_, err := client.Push(context.Background(), map[string][]*model.Block{keyedItem: keyedCatalog(t)}, keyedItems, nil,
		nil, apiclient.TransferUnder("en-US"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 409")
	assert.Contains(t, err.Error(), "the recipe's source language is en-US")
	assert.Contains(t, err.Error(), "written in en-GB")
	assert.Contains(t, err.Error(), "set defaults.source_language to en-GB in kapi.yaml")
	drainWithAuthority(t, srv)
	rows, err := srv.ContentStore.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: pid, Stream: "main", ItemName: keyedItem})
	require.NoError(t, err)
	assert.Empty(t, rows, "the refused push stored nothing")
}

// The comparison is of languages, not of spellings.
func TestPushLandsWhenTheRecipeSpellsTheProjectsLanguageAnotherWay(t *testing.T) {
	srv, pid, client := keyedProject(t, "en_us")
	pushKeyed(t, srv, client, keyedCatalog(t), nil)
	stored(t, srv, pid, "greeting")
}
