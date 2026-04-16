package server

import (
	"context"
	"net"
	"strings"
	"testing"

	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func setupEditorGRPC(t *testing.T) pb.EditorServiceClient {
	t.Helper()

	cfg := DefaultConfig()

	srv := NewServer(cfg)
	initTestStores(t, srv)

	lis := bufconn.Listen(bufSize)
	grpcSrv := grpc.NewServer()
	pb.RegisterEditorServiceServer(grpcSrv, NewEditorGRPCServer(srv))
	// Also register NeokapiService for project setup.
	pb.RegisterNeokapiServiceServer(grpcSrv, NewGRPCServer(srv))

	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			t.Logf("gRPC server error: %v", err)
		}
	}()
	t.Cleanup(func() { grpcSrv.GracefulStop() })

	conn, err := grpc.NewClient("passthrough://bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	return pb.NewEditorServiceClient(conn)
}

// neokapiClient returns a NeokapiServiceClient on the same connection for test setup.
func setupBothClients(t *testing.T) (pb.EditorServiceClient, pb.NeokapiServiceClient) {
	t.Helper()

	cfg := DefaultConfig()

	srv := NewServer(cfg)
	initTestStores(t, srv)

	lis := bufconn.Listen(bufSize)
	grpcSrv := grpc.NewServer()
	pb.RegisterEditorServiceServer(grpcSrv, NewEditorGRPCServer(srv))
	pb.RegisterNeokapiServiceServer(grpcSrv, NewGRPCServer(srv))

	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			t.Logf("gRPC server error: %v", err)
		}
	}()
	t.Cleanup(func() { grpcSrv.GracefulStop() })

	conn, err := grpc.NewClient("passthrough://bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	return pb.NewEditorServiceClient(conn), pb.NewNeokapiServiceClient(conn)
}

func TestEditorGRPCGetBlocks(t *testing.T) {
	editor, neokapi := setupBothClients(t)
	ctx := t.Context()

	// Create a project with blocks via NeokapiService.
	proj, err := neokapi.CreateProject(ctx, &pb.CreateProjectRequest{
		Name:          "Editor Test",
		SourceLocale:  "en",
		TargetLocales: []string{"fr", "de"},
	})
	require.NoError(t, err)

	_, err = neokapi.StoreBlocks(ctx, &pb.StoreBlocksRequest{
		ProjectId: proj.Id,
		Blocks: []*pb.BlockMessage{
			{Id: "b1", Source: "Hello", Targets: map[string]string{"fr": "Bonjour"}},
			{Id: "b2", Source: "World"},
		},
	})
	require.NoError(t, err)

	// Get blocks via EditorService (no item_name = all blocks).
	resp, err := editor.GetBlocks(ctx, &pb.GetBlocksRequest{
		ProjectId: proj.Id,
	})
	require.NoError(t, err)
	assert.Len(t, resp.Blocks, 2)

	// Verify first block has source and target.
	var hello *pb.BlockInfo
	for _, b := range resp.Blocks {
		if b.Id == "b1" {
			hello = b
			break
		}
	}
	require.NotNil(t, hello)
	assert.Equal(t, "Hello", flattenEditorRuns(hello.SourceRuns))
	require.NotNil(t, hello.TargetRuns["fr"])
	assert.Equal(t, "Bonjour", flattenEditorRuns(hello.TargetRuns["fr"].Runs))
}

// flattenEditorRuns returns the plain-text flattening of EditorRun
// sequence — used in tests that assert on block content.
func flattenEditorRuns(runs []*pb.EditorRun) string {
	var b strings.Builder
	for _, r := range runs {
		if t, ok := r.GetKind().(*pb.EditorRun_Text); ok {
			b.WriteString(t.Text.GetText())
		}
	}
	return b.String()
}

func TestEditorGRPCUpdateBlockTarget(t *testing.T) {
	editor, neokapi := setupBothClients(t)
	ctx := t.Context()

	// Setup project with block.
	proj, err := neokapi.CreateProject(ctx, &pb.CreateProjectRequest{
		Name:          "Update Test",
		SourceLocale:  "en",
		TargetLocales: []string{"fr"},
	})
	require.NoError(t, err)

	_, err = neokapi.StoreBlocks(ctx, &pb.StoreBlocksRequest{
		ProjectId: proj.Id,
		Blocks:    []*pb.BlockMessage{{Id: "b1", Source: "Hello"}},
	})
	require.NoError(t, err)

	// Update block target via EditorService.
	_, err = editor.UpdateBlockTarget(ctx, &pb.UpdateBlockTargetRequest{
		ProjectId:    proj.Id,
		BlockId:      "b1",
		TargetLocale: "fr",
		Runs: []*pb.EditorRun{{
			Kind: &pb.EditorRun_Text{Text: &pb.EditorTextRun{Text: "Bonjour"}},
		}},
	})
	require.NoError(t, err)

	// Verify via GetBlocks.
	resp, err := editor.GetBlocks(ctx, &pb.GetBlocksRequest{
		ProjectId: proj.Id,
	})
	require.NoError(t, err)
	require.Len(t, resp.Blocks, 1)
	require.NotNil(t, resp.Blocks[0].TargetRuns["fr"])
	assert.Equal(t, "Bonjour", flattenEditorRuns(resp.Blocks[0].TargetRuns["fr"].Runs))
	assert.Equal(t, "human", resp.Blocks[0].Properties["translation-origin"])
}

func TestEditorGRPCReviewBlock(t *testing.T) {
	editor, neokapi := setupBothClients(t)
	ctx := t.Context()

	proj, err := neokapi.CreateProject(ctx, &pb.CreateProjectRequest{
		Name:          "Review Test",
		SourceLocale:  "en",
		TargetLocales: []string{"fr"},
	})
	require.NoError(t, err)

	_, err = neokapi.StoreBlocks(ctx, &pb.StoreBlocksRequest{
		ProjectId: proj.Id,
		Blocks:    []*pb.BlockMessage{{Id: "b1", Source: "Hello", Targets: map[string]string{"fr": "Bonjour"}}},
	})
	require.NoError(t, err)

	// Mark as reviewed.
	_, err = editor.ReviewBlock(ctx, &pb.ReviewBlockRequest{
		ProjectId:    proj.Id,
		BlockId:      "b1",
		TargetLocale: "fr",
		Reviewed:     true,
	})
	require.NoError(t, err)

	// Verify status.
	resp, err := editor.GetBlocks(ctx, &pb.GetBlocksRequest{ProjectId: proj.Id})
	require.NoError(t, err)
	require.Len(t, resp.Blocks, 1)
	assert.Equal(t, "reviewed", resp.Blocks[0].Properties["translation-status"])

	// Unmark reviewed.
	_, err = editor.ReviewBlock(ctx, &pb.ReviewBlockRequest{
		ProjectId: proj.Id,
		BlockId:   "b1",
		Reviewed:  false,
	})
	require.NoError(t, err)

	resp, err = editor.GetBlocks(ctx, &pb.GetBlocksRequest{ProjectId: proj.Id})
	require.NoError(t, err)
	assert.Equal(t, "translated", resp.Blocks[0].Properties["translation-status"])
}

func TestEditorGRPCTMCRUD(t *testing.T) {
	editor := setupEditorGRPC(t)
	ctx := t.Context()

	ws := "test-ws"

	// Initial count should be 0.
	countResp, err := editor.GetTMCount(ctx, &pb.TMCountRequest{WorkspaceSlug: ws})
	require.NoError(t, err)
	assert.Equal(t, int32(0), countResp.Count)

	// Add entry.
	addResp, err := editor.AddTMEntry(ctx, &pb.AddTMEntryRequest{
		WorkspaceSlug: ws,
		Source:        "Hello",
		Target:        "Bonjour",
		SourceLocale:  "en",
		TargetLocale:  "fr",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, addResp.Entry.Id)
	assert.Equal(t, "Hello", addResp.Entry.Source)

	entryID := addResp.Entry.Id

	// Count should be 1.
	countResp, err = editor.GetTMCount(ctx, &pb.TMCountRequest{WorkspaceSlug: ws})
	require.NoError(t, err)
	assert.Equal(t, int32(1), countResp.Count)

	// Search.
	searchResp, err := editor.GetTMEntries(ctx, &pb.TMEntriesRequest{
		WorkspaceSlug: ws,
		Limit:         10,
	})
	require.NoError(t, err)
	assert.Len(t, searchResp.Entries, 1)

	// Update entry.
	_, err = editor.UpdateTMEntry(ctx, &pb.UpdateTMEntryRequest{
		WorkspaceSlug: ws,
		EntryId:       entryID,
		Source:        "Hello World",
		Target:        "Bonjour le Monde",
		SourceLocale:  "en",
		TargetLocale:  "fr",
	})
	require.NoError(t, err)

	// Delete entry.
	_, err = editor.DeleteTMEntry(ctx, &pb.DeleteTMEntryRequest{
		WorkspaceSlug: ws,
		EntryId:       entryID,
	})
	require.NoError(t, err)

	countResp, err = editor.GetTMCount(ctx, &pb.TMCountRequest{WorkspaceSlug: ws})
	require.NoError(t, err)
	assert.Equal(t, int32(0), countResp.Count)
}

func TestEditorGRPCTermCRUD(t *testing.T) {
	editor := setupEditorGRPC(t)
	ctx := t.Context()

	ws := "test-ws"

	// Initial count.
	countResp, err := editor.GetTermCount(ctx, &pb.TermCountRequest{WorkspaceSlug: ws})
	require.NoError(t, err)
	assert.Equal(t, int32(0), countResp.Count)

	// Add concept.
	addResp, err := editor.AddConcept(ctx, &pb.AddConceptRequest{
		WorkspaceSlug: ws,
		Domain:        "IT",
		Definition:    "A software program",
		Terms: []*pb.TermInfo{
			{Text: "software", Locale: "en", Status: "approved"},
			{Text: "logiciel", Locale: "fr", Status: "approved"},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, addResp.Concept.Id)
	assert.Equal(t, "IT", addResp.Concept.Domain)
	assert.Len(t, addResp.Concept.Terms, 2)

	conceptID := addResp.Concept.Id

	// Count.
	countResp, err = editor.GetTermCount(ctx, &pb.TermCountRequest{WorkspaceSlug: ws})
	require.NoError(t, err)
	assert.Equal(t, int32(1), countResp.Count)

	// Search.
	searchResp, err := editor.GetTerms(ctx, &pb.TermsRequest{
		WorkspaceSlug: ws,
		Limit:         10,
	})
	require.NoError(t, err)
	assert.Len(t, searchResp.Concepts, 1)

	// Update.
	_, err = editor.UpdateConcept(ctx, &pb.UpdateConceptRequest{
		WorkspaceSlug: ws,
		ConceptId:     conceptID,
		Domain:        "Software",
		Definition:    "Updated definition",
		Terms: []*pb.TermInfo{
			{Text: "software", Locale: "en", Status: "approved"},
			{Text: "logiciel", Locale: "fr", Status: "approved"},
		},
	})
	require.NoError(t, err)

	// Delete.
	_, err = editor.DeleteConcept(ctx, &pb.DeleteConceptRequest{
		WorkspaceSlug: ws,
		ConceptId:     conceptID,
	})
	require.NoError(t, err)

	countResp, err = editor.GetTermCount(ctx, &pb.TermCountRequest{WorkspaceSlug: ws})
	require.NoError(t, err)
	assert.Equal(t, int32(0), countResp.Count)
}

func TestEditorGRPCTermImportExport(t *testing.T) {
	editor := setupEditorGRPC(t)
	ctx := t.Context()

	ws := "test-ws"

	// Import CSV.
	csvResp, err := editor.ImportTermsCSV(ctx, &pb.ImportTermsCSVRequest{
		WorkspaceSlug: ws,
		CsvContent:    "software,logiciel\ncomputer,ordinateur",
		SourceLocale:  "en",
		TargetLocale:  "fr",
		Domain:        "IT",
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), csvResp.Imported)

	// Export JSON.
	exportResp, err := editor.ExportTermsJSON(ctx, &pb.ExportTermsJSONRequest{
		WorkspaceSlug: ws,
		Name:          "test-export",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, exportResp.JsonContent)

	// Import JSON into a new workspace.
	ws2 := "test-ws-2"
	jsonResp, err := editor.ImportTermsJSON(ctx, &pb.ImportTermsJSONRequest{
		WorkspaceSlug: ws2,
		JsonContent:   exportResp.JsonContent,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), jsonResp.Imported)
}
