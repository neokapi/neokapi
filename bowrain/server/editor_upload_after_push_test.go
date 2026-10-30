package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/venue"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// resolveAsPushDoes writes each block's durable identity onto model.Block.Key
// the way host.ResolveIdentity does for kapi push: the document first, against
// the venue's tree, then every block against the venue's priors under that
// document's scope. Called here through core/reconcile so this test adds no
// host root import to bowrain's module graph.
func resolveAsPushDoes(name string, blocks []*model.Block, tree venue.Tree) {
	docs := reconcile.Documents([]reconcile.Document{{Path: name, Blocks: blocks}}, tree.Units())
	for _, r := range reconcile.Blocks(docs[0].Key, blocks, tree.Priors()) {
		if r.Block != nil {
			r.Block.Key = r.Key
		}
	}
}

const kapimartEmailTemplates = "../../apps/kapi-desktop/backend/sample/kapimart/src/en/email-templates.html"

// readAsUploadDoes reads one file the way editorAddFilesInternal does: the
// registry's reader for the extension, the read skeleton wired, editor.ParseItem.
func readAsUploadDoes(t *testing.T, reg *registry.FormatRegistry, name string, data []byte, source model.LocaleID) []*model.Block {
	t.Helper()
	fmtName, err := reg.Detector().DetectByExtension(filepath.Ext(name))
	require.NoError(t, err)
	reader, err := reg.NewReader(registry.FormatID(fmtName))
	require.NoError(t, err)
	coreproj.WireReadSkeleton(reg, fmtName, reader)
	doc := &model.RawDocument{URI: name, SourceLocale: source, Encoding: "UTF-8", Reader: io.NopCloser(bytes.NewReader(data))}
	result, err := editor.ParseItem(t.Context(), reader, doc, string(source), fmtName, name)
	require.NoError(t, err)
	return result.Blocks
}

// fetchTree fetches the venue's tree for the stream, as kapi push does before
// it resolves identity.
func fetchVenueTree(t *testing.T, e *echo.Echo, authHeader, pid string) *apiclient.TreeResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/tree", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "tree should return 200")
	var tr apiclient.TreeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tr))
	return &tr
}

// pushResolvedBlocks pushes blocks that already carry their resolved keys
// (model.Block.Key) through the route, declaring the scope and the tree as
// host/venue/source does: init, chunk upload, commit, drain.
func pushResolvedBlocks(t *testing.T, srv *Server, e *echo.Echo, authHeader, pid string, byItem map[string][]*model.Block, format string) {
	t.Helper()

	itemHashes := map[string]string{}
	for itemName, blocks := range byItem {
		blockHashes := map[string]string{}
		for _, b := range blocks {
			blockHashes[convergence.BlockKey(b)] = venue.RecordHash(b, "en")
		}
		itemHashes[itemName] = venue.ComputeItemHash(blockHashes)
	}
	basePath := "/api/v1/projects/" + pid

	initBody, _ := json.Marshal(map[string]any{
		"project_id": pid, "item_hashes": itemHashes, "content_model_epoch": venue.ContentModelEpoch,
	})
	req := httptest.NewRequest(http.MethodPost, basePath+"/sync/main/push/init", bytes.NewReader(initBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "push init should return 200")
	var initResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &initResp))
	if initResp["status"] == "unchanged" {
		// The venue already holds every item at this content; a real push
		// stops here and writes nothing.
		t.Logf("push init: unchanged, nothing sent")
		return
	}
	uploadID := initResp["upload_id"].(string)

	held := fetchVenueTree(t, e, authHeader, pid).Tree().Records()

	type chunkRefJSON struct {
		Index       int    `json:"index"`
		ContentType string `json:"content_type"`
		Hash        string `json:"hash"`
		RecordCount int    `json:"record_count"`
		ByteSize    int    `json:"byte_size"`
	}
	var chunkRefs []chunkRefJSON
	var scope venue.Scope
	var itemMeta []map[string]string
	for itemName, blocks := range byItem {
		scope = append(scope, itemName)
		itemMeta = append(itemMeta, map[string]string{"name": itemName, "format": format})
		var syncBlocks []*pb.SyncBlock
		for _, b := range blocks {
			if _, have := held[venue.RecordHash(b, "en")]; have {
				continue
			}
			syncBlocks = append(syncBlocks, venue.BlockToProto(b, itemName))
		}
		if len(syncBlocks) == 0 {
			continue
		}
		chunk := &pb.SyncChunk{ContentType: "blocks", RecordCount: int32(len(syncBlocks)), Blocks: syncBlocks}
		chunkData, err := proto.Marshal(chunk)
		require.NoError(t, err)
		idx := len(chunkRefs)
		req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("%s/sync/main/push/chunks/%s/%d", basePath, uploadID, idx), bytes.NewReader(chunkData))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Authorization", authHeader)
		rec = httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "chunk upload should return 200")
		raw := sha256.Sum256(chunkData)
		chunkRefs = append(chunkRefs, chunkRefJSON{Index: idx, ContentType: "blocks", Hash: hex.EncodeToString(raw[:]), RecordCount: len(syncBlocks), ByteSize: len(chunkData)})
	}

	commitBody, _ := json.Marshal(map[string]any{
		"upload_id":           uploadID,
		"project_id":          pid,
		"stream":              "main",
		"chunks":              chunkRefs,
		"items":               itemMeta,
		"scope":               scope,
		"tree":                venue.TreeFromBlocks(byItem),
		"content_model_epoch": venue.ContentModelEpoch,
	})
	req = httptest.NewRequest(http.MethodPost, basePath+"/sync/main/push/commit", bytes.NewReader(commitBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code, "push commit should return 202: %s", rec.Body.String())
	drainPushQueue(t, srv)
}

func itemRows(t *testing.T, srv *Server, pid, name string) map[string]*venue.StoredBlock {
	t.Helper()
	stored, err := srv.ContentStore.GetBlocks(t.Context(), store.BlockQuery{ProjectID: pid, Stream: "main", ItemName: name})
	require.NoError(t, err)
	byID := make(map[string]*venue.StoredBlock, len(stored))
	for _, sb := range stored {
		byID[sb.Block.ID] = sb
	}
	return byID
}

// A file kapi push has already placed in the project is uploaded through the
// editor. The push keyed its rows by the identity it resolved (model.Block.Key,
// "u-..."), the upload keys the same blocks by their reader names; the rows
// the push created, and the translations on them, must survive the upload.
func TestEditorUpload_AfterAPushOfTheSameFileKeepsThePushedRowsAndTranslations(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)
	ctx := t.Context()
	const name = "email-templates.html"

	data, err := os.ReadFile(kapimartEmailTemplates)
	require.NoError(t, err)
	proj, err := srv.ContentStore.GetProject(ctx, pid)
	require.NoError(t, err)

	// kapi push: read, fetch the venue's tree, resolve identity against it,
	// declare the tree in the keys resolution gave, push.
	blocks := readAsUploadDoes(t, srv.FormatRegistry, name, data, proj.DefaultSourceLanguage)
	require.NotEmpty(t, blocks)
	byItem := map[string][]*model.Block{name: blocks}
	tree := fetchVenueTree(t, e, authHeader, pid).Tree()
	resolveAsPushDoes(name, blocks, tree)
	for _, b := range blocks {
		require.True(t, strings.HasPrefix(b.Key, "u-"), "push resolves a fresh key for %q, got %q", b.Name, b.Key)
	}
	pushResolvedBlocks(t, srv, e, authHeader, pid, byItem, "html")

	pushed := itemRows(t, srv, pid, name)
	require.Len(t, pushed, len(blocks), "the push stored one row per block")
	var translated *venue.StoredBlock
	for _, sb := range pushed {
		require.True(t, strings.HasPrefix(sb.SourceID, "u-"), "a pushed row is keyed by its resolved identity, got %q", sb.SourceID)
		if translated == nil || sb.Block.ID < translated.Block.ID {
			translated = sb
		}
	}
	_, err = srv.ContentStore.UpdateBlock(ctx, pid, "main", translated.Block.ID, func(sb *venue.StoredBlock) error {
		sb.Block.SetTargetText(model.LocaleID("fr"), "Bonjour")
		return nil
	})
	require.NoError(t, err)

	// The same file, uploaded through the editor.
	info, err := editorAddFiles(ctx, srv.ContentStore, srv.FormatRegistry, pid, "main", map[string][]byte{name: data})
	require.NoError(t, err)
	require.Empty(t, info.Skipped)

	after := itemRows(t, srv, pid, name)
	assert.Len(t, after, len(pushed), "the upload adds no row and removes none")
	for id := range pushed {
		assert.Contains(t, after, id, "the row the push created for %q is still there", pushed[id].SourceID)
	}
	kept := after[translated.Block.ID]
	if assert.NotNil(t, kept, "the translated block's row survives the upload") {
		assert.Equal(t, "Bonjour", kept.Block.TargetText(model.LocaleID("fr")), "and so does its translation")
	}
}

// The other order: a file uploaded through the editor, translated, and then
// pushed by kapi. The push resolves identity against the venue's tree, so the
// uploaded rows are the priors it matches and keeps.
func TestEditorUpload_APushAfterAnUploadOfTheSameFileKeepsTheUploadedRows(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)
	ctx := t.Context()
	const name = "email-templates.html"

	data, err := os.ReadFile(kapimartEmailTemplates)
	require.NoError(t, err)
	proj, err := srv.ContentStore.GetProject(ctx, pid)
	require.NoError(t, err)

	info, err := editorAddFiles(ctx, srv.ContentStore, srv.FormatRegistry, pid, "main", map[string][]byte{name: data})
	require.NoError(t, err)
	require.Empty(t, info.Skipped)
	uploaded := itemRows(t, srv, pid, name)
	require.NotEmpty(t, uploaded)
	var translated *venue.StoredBlock
	for _, sb := range uploaded {
		if translated == nil || sb.Block.ID < translated.Block.ID {
			translated = sb
		}
	}
	_, err = srv.ContentStore.UpdateBlock(ctx, pid, "main", translated.Block.ID, func(sb *venue.StoredBlock) error {
		sb.Block.SetTargetText(model.LocaleID("fr"), "Bonjour")
		return nil
	})
	require.NoError(t, err)

	blocks := readAsUploadDoes(t, srv.FormatRegistry, name, data, proj.DefaultSourceLanguage)
	byItem := map[string][]*model.Block{name: blocks}
	tree := fetchVenueTree(t, e, authHeader, pid).Tree()
	resolveAsPushDoes(name, blocks, tree)
	pushResolvedBlocks(t, srv, e, authHeader, pid, byItem, "html")

	after := itemRows(t, srv, pid, name)
	assert.Len(t, after, len(uploaded))
	for id := range uploaded {
		assert.Contains(t, after, id, "the uploaded row %q survives the push", uploaded[id].SourceID)
	}
	kept := after[translated.Block.ID]
	if assert.NotNil(t, kept) {
		assert.Equal(t, "Bonjour", kept.Block.TargetText(model.LocaleID("fr")))
	}
}
