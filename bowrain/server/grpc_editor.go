package server

import (
	"context"
	"strings"
	"time"

	"github.com/gokapi/gokapi/bowrain/event"
	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/bowrain/store"
	"github.com/gokapi/gokapi/sievepen"
	"github.com/gokapi/gokapi/termbase"
	pb "github.com/gokapi/gokapi/bowrain/proto/v1"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// EditorGRPCServer implements the EditorServiceServer interface.
type EditorGRPCServer struct {
	pb.UnimplementedEditorServiceServer
	srv      *Server
	presence *presenceStore
}

// NewEditorGRPCServer creates a new EditorGRPCServer.
func NewEditorGRPCServer(srv *Server) *EditorGRPCServer {
	return &EditorGRPCServer{
		srv:      srv,
		presence: newPresenceStore(),
	}
}

// --- Auth & workspace ---

func (g *EditorGRPCServer) GetCurrentUser(ctx context.Context, _ *pb.GetCurrentUserRequest) (*pb.UserResponse, error) {
	claims, ok := GRPCUserFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	// Look up full user from auth store if available.
	if g.srv.AuthStore != nil {
		user, err := g.srv.AuthStore.GetUser(ctx, claims.Subject)
		if err == nil {
			return &pb.UserResponse{
				Id:        user.ID,
				Email:     user.Email,
				Name:      user.Name,
				AvatarUrl: user.AvatarURL,
			}, nil
		}
	}

	return &pb.UserResponse{
		Id:    claims.Subject,
		Email: claims.Email,
		Name:  claims.Name,
	}, nil
}

func (g *EditorGRPCServer) ListWorkspaces(ctx context.Context, _ *pb.ListWorkspacesRequest) (*pb.ListWorkspacesResponse, error) {
	claims, ok := GRPCUserFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	if g.srv.AuthStore == nil {
		return nil, status.Error(codes.Unavailable, "auth not configured")
	}

	workspaces, err := g.srv.AuthStore.ListWorkspaces(ctx, claims.Subject)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list workspaces: %v", err)
	}

	resp := &pb.ListWorkspacesResponse{}
	for _, ws := range workspaces {
		info := &pb.WorkspaceInfo{
			Id:          ws.ID,
			Name:        ws.Name,
			Slug:        ws.Slug,
			Description: ws.Description,
			LogoUrl:     ws.LogoURL,
		}
		// Get user's role in this workspace.
		if m, err := g.srv.AuthStore.GetMembership(ctx, ws.ID, claims.Subject); err == nil {
			info.Role = string(m.Role)
		}
		resp.Workspaces = append(resp.Workspaces, info)
	}
	return resp, nil
}

// --- Projects ---

func (g *EditorGRPCServer) ListEditorProjects(ctx context.Context, req *pb.ListEditorProjectsRequest) (*pb.ListEditorProjectsResponse, error) {
	if g.srv.ContentStore == nil {
		return nil, status.Error(codes.Unavailable, "content store not configured")
	}

	projects, err := g.srv.ContentStore.ListProjects(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list projects: %v", err)
	}

	resp := &pb.ListEditorProjectsResponse{}
	for _, p := range projects {
		if p.WorkspaceID != req.WorkspaceSlug {
			continue
		}
		info, err := g.buildEditorProjectInfo(ctx, p)
		if err != nil {
			continue
		}
		resp.Projects = append(resp.Projects, info)
	}
	return resp, nil
}

func (g *EditorGRPCServer) GetEditorProject(ctx context.Context, req *pb.GetEditorProjectRequest) (*pb.EditorProjectResponse, error) {
	if g.srv.ContentStore == nil {
		return nil, status.Error(codes.Unavailable, "content store not configured")
	}

	proj, err := g.srv.ContentStore.GetProject(ctx, req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found: %v", err)
	}

	info, err := g.buildEditorProjectInfo(ctx, proj)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "build project info: %v", err)
	}

	return &pb.EditorProjectResponse{Project: info}, nil
}

// --- Blocks ---

func (g *EditorGRPCServer) GetBlocks(ctx context.Context, req *pb.GetBlocksRequest) (*pb.GetBlocksResponse, error) {
	if g.srv.ContentStore == nil {
		return nil, status.Error(codes.Unavailable, "content store not configured")
	}

	proj, err := g.srv.ContentStore.GetProject(ctx, req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found: %v", err)
	}

	targetLocales := make([]string, len(proj.TargetLocales))
	for i, l := range proj.TargetLocales {
		targetLocales[i] = string(l)
	}

	storedBlocks, err := g.srv.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: req.ProjectId,
		ItemName:  req.ItemName,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get blocks: %v", err)
	}

	resp := &pb.GetBlocksResponse{}
	for _, sb := range storedBlocks {
		resp.Blocks = append(resp.Blocks, storedBlockToProto(sb, targetLocales))
	}
	return resp, nil
}

func (g *EditorGRPCServer) UpdateBlockTarget(ctx context.Context, req *pb.UpdateBlockTargetRequest) (*emptypb.Empty, error) {
	if g.srv.ContentStore == nil {
		return nil, status.Error(codes.Unavailable, "content store not configured")
	}

	sb, err := g.srv.ContentStore.GetBlock(ctx, req.ProjectId, req.BlockId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "block not found: %v", err)
	}

	locale := model.LocaleID(req.TargetLocale)

	if req.CodedText != "" {
		// Coded text update with spans.
		frag := &model.Fragment{CodedText: req.CodedText}
		for _, si := range req.Spans {
			frag.Spans = append(frag.Spans, protoToSpan(si))
		}
		sb.Block.SetTargetFragment(locale, frag)
	} else {
		// Plain text update.
		sb.Block.SetTargetText(locale, req.Text)
	}

	// Mark as human translation.
	if sb.Block.Properties == nil {
		sb.Block.Properties = make(map[string]string)
	}
	sb.Block.Properties["translation-origin"] = "human"

	if err := g.srv.ContentStore.StoreBlocks(ctx, req.ProjectId, []*model.Block{sb.Block}); err != nil {
		return nil, status.Errorf(codes.Internal, "store block: %v", err)
	}

	// Emit event for watchers.
	g.emitBlockChange(req.ProjectId, sb.Block.ID, "", "updated", ctx)

	return &emptypb.Empty{}, nil
}

func (g *EditorGRPCServer) ReviewBlock(ctx context.Context, req *pb.ReviewBlockRequest) (*emptypb.Empty, error) {
	if g.srv.ContentStore == nil {
		return nil, status.Error(codes.Unavailable, "content store not configured")
	}

	sb, err := g.srv.ContentStore.GetBlock(ctx, req.ProjectId, req.BlockId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "block not found: %v", err)
	}

	if sb.Block.Properties == nil {
		sb.Block.Properties = make(map[string]string)
	}
	if req.Reviewed {
		sb.Block.Properties["translation-status"] = "reviewed"
	} else {
		sb.Block.Properties["translation-status"] = "translated"
	}

	if err := g.srv.ContentStore.StoreBlocks(ctx, req.ProjectId, []*model.Block{sb.Block}); err != nil {
		return nil, status.Errorf(codes.Internal, "store block: %v", err)
	}

	g.emitBlockChange(req.ProjectId, sb.Block.ID, req.ItemName, "updated", ctx)
	return &emptypb.Empty{}, nil
}

// --- TM lookup ---

func (g *EditorGRPCServer) LookupTMForBlock(ctx context.Context, req *pb.TMLookupRequest) (*pb.TMLookupResponse, error) {
	if g.srv.ContentStore == nil || g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	matches, err := editorLookupTMForBlock(ctx, g.srv.ContentStore, g.srv.wsStores, req.WorkspaceSlug, req.ProjectId, req.BlockId, req.TargetLocale)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "TM lookup: %v", err)
	}

	resp := &pb.TMLookupResponse{}
	for _, m := range matches {
		resp.Matches = append(resp.Matches, &pb.TMLookupMatch{
			Source:    m.Source,
			Target:    m.Target,
			Score:     m.Score,
			MatchType: m.MatchType,
		})
	}
	return resp, nil
}

// --- Term lookup ---

func (g *EditorGRPCServer) LookupTermsForBlock(ctx context.Context, req *pb.TermLookupRequest) (*pb.TermLookupResponse, error) {
	if g.srv.ContentStore == nil || g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	matches, err := editorLookupTermsForBlock(ctx, g.srv.ContentStore, g.srv.wsStores, req.WorkspaceSlug, req.ProjectId, req.BlockId, req.TargetLocale)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "term lookup: %v", err)
	}

	resp := &pb.TermLookupResponse{}
	for _, m := range matches {
		resp.Matches = append(resp.Matches, &pb.BlockTermMatch{
			SourceTerm:  m.SourceTerm,
			TargetTerms: m.TargetTerms,
			Domain:      m.Domain,
			Status:      m.Status,
			Start:       int32(m.Start),
			End:         int32(m.End),
		})
	}
	return resp, nil
}

// --- TM CRUD ---

func (g *EditorGRPCServer) GetTMEntries(ctx context.Context, req *pb.TMEntriesRequest) (*pb.TMEntriesResponse, error) {
	if g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	tm, err := g.srv.wsStores.getTM(req.WorkspaceSlug)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "init TM: %v", err)
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}

	entries, total := tm.SearchEntries(req.Query, req.SourceLocale, req.TargetLocale, int(req.Offset), limit)

	resp := &pb.TMEntriesResponse{TotalCount: int32(total)}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, tmEntryToProto(e))
	}
	return resp, nil
}

func (g *EditorGRPCServer) GetTMCount(ctx context.Context, req *pb.TMCountRequest) (*pb.TMCountResponse, error) {
	if g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	tm, err := g.srv.wsStores.getTM(req.WorkspaceSlug)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "init TM: %v", err)
	}

	return &pb.TMCountResponse{Count: int32(tm.Count())}, nil
}

func (g *EditorGRPCServer) AddTMEntry(ctx context.Context, req *pb.AddTMEntryRequest) (*pb.TMEntryResponse, error) {
	if g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	tm, err := g.srv.wsStores.getTM(req.WorkspaceSlug)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "init TM: %v", err)
	}

	entry := sievepen.TMEntry{
		ID:           uuid.New().String(),
		Source:       &model.Fragment{CodedText: req.Source},
		Target:       &model.Fragment{CodedText: req.Target},
		SourceLocale: model.LocaleID(req.SourceLocale),
		TargetLocale: model.LocaleID(req.TargetLocale),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := tm.Add(entry); err != nil {
		return nil, status.Errorf(codes.Internal, "add TM entry: %v", err)
	}

	return &pb.TMEntryResponse{Entry: tmEntryToProto(entry)}, nil
}

func (g *EditorGRPCServer) UpdateTMEntry(ctx context.Context, req *pb.UpdateTMEntryRequest) (*emptypb.Empty, error) {
	if g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	tm, err := g.srv.wsStores.getTM(req.WorkspaceSlug)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "init TM: %v", err)
	}

	existing, ok := tm.GetEntry(req.EntryId)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "TM entry %q not found", req.EntryId)
	}

	if err := tm.Delete(req.EntryId); err != nil {
		return nil, status.Errorf(codes.Internal, "delete old TM entry: %v", err)
	}

	existing.Source = &model.Fragment{CodedText: req.Source}
	existing.Target = &model.Fragment{CodedText: req.Target}
	existing.SourceLocale = model.LocaleID(req.SourceLocale)
	existing.TargetLocale = model.LocaleID(req.TargetLocale)
	existing.UpdatedAt = time.Now()

	if err := tm.Add(existing); err != nil {
		return nil, status.Errorf(codes.Internal, "re-add TM entry: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (g *EditorGRPCServer) DeleteTMEntry(ctx context.Context, req *pb.DeleteTMEntryRequest) (*emptypb.Empty, error) {
	if g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	tm, err := g.srv.wsStores.getTM(req.WorkspaceSlug)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "init TM: %v", err)
	}

	if err := tm.Delete(req.EntryId); err != nil {
		return nil, status.Errorf(codes.NotFound, "TM entry not found: %v", err)
	}

	return &emptypb.Empty{}, nil
}

// --- Terminology CRUD ---

func (g *EditorGRPCServer) GetTerms(ctx context.Context, req *pb.TermsRequest) (*pb.TermsResponse, error) {
	if g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	tb := g.srv.wsStores.getTB(req.WorkspaceSlug)

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}

	concepts, total := tb.Search(req.Query, req.SourceLocale, req.TargetLocale, int(req.Offset), limit)

	resp := &pb.TermsResponse{TotalCount: int32(total)}
	for _, c := range concepts {
		resp.Concepts = append(resp.Concepts, conceptToProto(c))
	}
	return resp, nil
}

func (g *EditorGRPCServer) GetTermCount(ctx context.Context, req *pb.TermCountRequest) (*pb.TermCountResponse, error) {
	if g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	tb := g.srv.wsStores.getTB(req.WorkspaceSlug)
	return &pb.TermCountResponse{Count: int32(tb.Count())}, nil
}

func (g *EditorGRPCServer) AddConcept(ctx context.Context, req *pb.AddConceptRequest) (*pb.ConceptResponse, error) {
	if g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	tb := g.srv.wsStores.getTB(req.WorkspaceSlug)

	concept := termbase.Concept{
		ID:         uuid.New().String(),
		Domain:     req.Domain,
		Definition: req.Definition,
		Terms:      protoToTerms(req.Terms),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := tb.AddConcept(concept); err != nil {
		return nil, status.Errorf(codes.Internal, "add concept: %v", err)
	}

	return &pb.ConceptResponse{Concept: conceptToProto(concept)}, nil
}

func (g *EditorGRPCServer) UpdateConcept(ctx context.Context, req *pb.UpdateConceptRequest) (*emptypb.Empty, error) {
	if g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	tb := g.srv.wsStores.getTB(req.WorkspaceSlug)

	existing, ok := tb.GetConcept(req.ConceptId)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "concept %q not found", req.ConceptId)
	}

	existing.Domain = req.Domain
	existing.Definition = req.Definition
	existing.Terms = protoToTerms(req.Terms)
	existing.UpdatedAt = time.Now()

	if err := tb.AddConcept(existing); err != nil {
		return nil, status.Errorf(codes.Internal, "update concept: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (g *EditorGRPCServer) DeleteConcept(ctx context.Context, req *pb.DeleteConceptRequest) (*emptypb.Empty, error) {
	if g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	tb := g.srv.wsStores.getTB(req.WorkspaceSlug)
	if err := tb.DeleteConcept(req.ConceptId); err != nil {
		return nil, status.Errorf(codes.NotFound, "concept not found: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (g *EditorGRPCServer) ImportTermsCSV(ctx context.Context, req *pb.ImportTermsCSVRequest) (*pb.ImportCountResponse, error) {
	if g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	tb := g.srv.wsStores.getTB(req.WorkspaceSlug)
	count, err := termbase.ImportCSV(tb, strings.NewReader(req.CsvContent), termbase.CSVImportOptions{
		HasHeader:    req.HasHeader,
		SourceLocale: model.LocaleID(req.SourceLocale),
		TargetLocale: model.LocaleID(req.TargetLocale),
		Domain:       req.Domain,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "import CSV: %v", err)
	}

	return &pb.ImportCountResponse{Imported: int32(count)}, nil
}

func (g *EditorGRPCServer) ImportTermsJSON(ctx context.Context, req *pb.ImportTermsJSONRequest) (*pb.ImportCountResponse, error) {
	if g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	tb := g.srv.wsStores.getTB(req.WorkspaceSlug)
	count, err := termbase.ImportJSON(tb, strings.NewReader(req.JsonContent))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "import JSON: %v", err)
	}

	return &pb.ImportCountResponse{Imported: int32(count)}, nil
}

func (g *EditorGRPCServer) ExportTermsJSON(ctx context.Context, req *pb.ExportTermsJSONRequest) (*pb.ExportTermsJSONResponse, error) {
	if g.srv.wsStores == nil {
		return nil, status.Error(codes.Unavailable, "editor not configured")
	}

	tb := g.srv.wsStores.getTB(req.WorkspaceSlug)
	var buf strings.Builder
	if err := termbase.ExportJSON(tb, &buf, req.Name); err != nil {
		return nil, status.Errorf(codes.Internal, "export JSON: %v", err)
	}

	return &pb.ExportTermsJSONResponse{JsonContent: buf.String()}, nil
}

// --- Presence ---

func (g *EditorGRPCServer) UpdatePresence(ctx context.Context, req *pb.UpdatePresenceRequest) (*emptypb.Empty, error) {
	claims, ok := GRPCUserFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}

	entry := &presenceEntry{
		UserID:   claims.Subject,
		UserName: claims.Name,
		ItemName: req.ItemName,
		BlockID:  req.BlockId,
	}

	// Populate avatar from auth store if available.
	if g.srv.AuthStore != nil {
		if user, err := g.srv.AuthStore.GetUser(ctx, claims.Subject); err == nil {
			entry.AvatarURL = user.AvatarURL
		}
	}

	g.presence.Update(req.ProjectId, entry)
	return &emptypb.Empty{}, nil
}

// --- Real-time collaboration ---

func (g *EditorGRPCServer) WatchProject(req *pb.WatchProjectRequest, stream grpc.ServerStreamingServer[pb.ProjectEvent]) error {
	claims, ok := GRPCUserFromContext(stream.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "not authenticated")
	}

	projectID := req.ProjectId

	// Register presence on stream open.
	entry := &presenceEntry{
		UserID:   claims.Subject,
		UserName: claims.Name,
	}
	if g.srv.AuthStore != nil {
		if user, err := g.srv.AuthStore.GetUser(stream.Context(), claims.Subject); err == nil {
			entry.AvatarURL = user.AvatarURL
		}
	}
	g.presence.Update(projectID, entry)

	// Broadcast "joined" event to other watchers via event bus.
	if g.srv.EventBus != nil {
		g.srv.EventBus.Publish(presenceJoinedEvent(projectID, entry))
	}

	// Subscribe to events for this project.
	if g.srv.EventBus != nil {
		handler := func(e event.Event) {
			// Filter events by project ID.
			if e.ProjectID != projectID {
				return
			}
			evt := busEventToProjectEvent(e)
			if evt != nil {
				_ = stream.Send(evt)
			}
		}
		sub := g.srv.EventBus.SubscribeAll(handler)
		defer g.srv.EventBus.Unsubscribe(sub)
	}

	// Block until client disconnects.
	<-stream.Context().Done()

	// Clean up presence on disconnect.
	g.presence.Remove(projectID, claims.Subject)
	if g.srv.EventBus != nil {
		g.srv.EventBus.Publish(presenceLeftEvent(projectID, entry))
	}

	return nil
}

// --- Conversion helpers ---

func (g *EditorGRPCServer) buildEditorProjectInfo(ctx context.Context, proj *store.Project) (*pb.EditorProjectInfo, error) {
	locales := make([]string, len(proj.TargetLocales))
	for i, l := range proj.TargetLocales {
		locales[i] = string(l)
	}
	info := &pb.EditorProjectInfo{
		Id:            proj.ID,
		Name:          proj.Name,
		SourceLocale:  string(proj.SourceLocale),
		TargetLocales: locales,
		CreatedAt:     proj.CreatedAt.Format(time.RFC3339),
		ModifiedAt:    proj.UpdatedAt.Format(time.RFC3339),
	}

	items, err := g.srv.ContentStore.ListItems(ctx, proj.ID)
	if err != nil {
		return nil, err
	}

	for _, item := range items {
		blocks, err := g.srv.ContentStore.GetBlocks(ctx, store.BlockQuery{
			ProjectID: proj.ID,
			ItemName:  item.Name,
		})
		if err != nil {
			continue
		}

		wordCount := 0
		for _, sb := range blocks {
			if sb.Block.Translatable {
				wordCount += editorCountWords(sb.Block.SourceText())
			}
		}

		info.Items = append(info.Items, &pb.EditorProjectItem{
			Name:       item.Name,
			Format:     item.Format,
			Type:       item.ItemType,
			Size:       int64(len(item.SourceBytes)),
			BlockCount: int32(len(blocks)),
			WordCount:  int32(wordCount),
		})
	}

	return info, nil
}

func storedBlockToProto(sb *store.StoredBlock, targetLocales []string) *pb.BlockInfo {
	targets := make(map[string]string)
	targetsCoded := make(map[string]string)
	for _, locale := range targetLocales {
		if t := sb.Block.TargetText(model.LocaleID(locale)); t != "" {
			targets[locale] = t
		}
		// Include coded text for targets with spans.
		segs, ok := sb.Block.Targets[model.LocaleID(locale)]
		if ok && len(segs) > 0 && segs[0].Content != nil && segs[0].Content.HasSpans() {
			targetsCoded[locale] = segs[0].Content.CodedText
		}
	}

	props := make(map[string]string)
	for k, v := range sb.Block.Properties {
		props[k] = v
	}

	bi := &pb.BlockInfo{
		Id:           sb.Block.ID,
		Source:       sb.Block.SourceText(),
		Targets:      targets,
		TargetsCoded: targetsCoded,
		Translatable: sb.Block.Translatable,
		Properties:   props,
	}

	// Enrich with coded text and span info if available.
	if len(sb.Block.Source) > 0 && sb.Block.Source[0].Content != nil {
		frag := sb.Block.Source[0].Content
		if frag.HasSpans() {
			bi.HasSpans = true
			bi.SourceCoded = frag.CodedText
			for _, s := range frag.Spans {
				bi.SourceSpans = append(bi.SourceSpans, spanToProto(s))
			}
		}
	}

	return bi
}

func spanToProto(s *model.Span) *pb.SpanInfo {
	var spanType string
	switch s.SpanType {
	case model.SpanOpening:
		spanType = "opening"
	case model.SpanClosing:
		spanType = "closing"
	case model.SpanPlaceholder:
		spanType = "placeholder"
	}
	return &pb.SpanInfo{
		SpanType: spanType,
		Type:     s.Type,
		Id:       s.ID,
		Data:     s.Data,
	}
}

func protoToSpan(si *pb.SpanInfo) *model.Span {
	var st model.SpanType
	switch si.SpanType {
	case "opening":
		st = model.SpanOpening
	case "closing":
		st = model.SpanClosing
	case "placeholder":
		st = model.SpanPlaceholder
	}
	return &model.Span{
		SpanType: st,
		Type:     si.Type,
		ID:       si.Id,
		Data:     si.Data,
	}
}

func tmEntryToProto(e sievepen.TMEntry) *pb.TMEntryInfo {
	return &pb.TMEntryInfo{
		Id:           e.ID,
		Source:       e.SourceText(),
		Target:       e.TargetText(),
		SourceLocale: string(e.SourceLocale),
		TargetLocale: string(e.TargetLocale),
		UpdatedAt:    e.UpdatedAt.Format(time.RFC3339),
	}
}

func conceptToProto(c termbase.Concept) *pb.ConceptInfo {
	terms := make([]*pb.TermInfo, len(c.Terms))
	for i, t := range c.Terms {
		terms[i] = &pb.TermInfo{
			Text:         t.Text,
			Locale:       string(t.Locale),
			Status:       string(t.Status),
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
		}
	}
	return &pb.ConceptInfo{
		Id:         c.ID,
		Domain:     c.Domain,
		Definition: c.Definition,
		Terms:      terms,
		Properties: c.Properties,
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  c.UpdatedAt.Format(time.RFC3339),
	}
}

func protoToTerms(terms []*pb.TermInfo) []termbase.Term {
	result := make([]termbase.Term, len(terms))
	for i, t := range terms {
		result[i] = termbase.Term{
			Text:         t.Text,
			Locale:       model.LocaleID(t.Locale),
			Status:       model.TermStatus(t.Status),
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
		}
		if result[i].Status == "" {
			result[i].Status = model.TermApproved
		}
	}
	return result
}

// emitBlockChange publishes a block change event through the event bus.
func (g *EditorGRPCServer) emitBlockChange(projectID, blockID, itemName, changeType string, ctx context.Context) {
	if g.srv.EventBus == nil {
		return
	}
	userName := ""
	if claims, ok := GRPCUserFromContext(ctx); ok {
		userName = claims.Name
	}
	g.srv.EventBus.Publish(event.Event{
		ID:        uuid.New().String(),
		Type:      event.EventType("editor.block." + changeType),
		Source:    "editor-grpc",
		ProjectID: projectID,
		Data: map[string]string{
			"block_id":    blockID,
			"item_name":   itemName,
			"change_type": changeType,
			"changed_by":  userName,
		},
		Timestamp: time.Now(),
	})
}

// presenceJoinedEvent creates an event for a user joining a project.
func presenceJoinedEvent(projectID string, entry *presenceEntry) event.Event {
	return event.Event{
		ID:        uuid.New().String(),
		Type:      "editor.presence.joined",
		Source:    "editor-grpc",
		ProjectID: projectID,
		Data: map[string]string{
			"user_id":    entry.UserID,
			"user_name":  entry.UserName,
			"avatar_url": entry.AvatarURL,
			"event_kind": "presence",
		},
		Timestamp: time.Now(),
	}
}

// presenceLeftEvent creates an event for a user leaving a project.
func presenceLeftEvent(projectID string, entry *presenceEntry) event.Event {
	return event.Event{
		ID:        uuid.New().String(),
		Type:      "editor.presence.left",
		Source:    "editor-grpc",
		ProjectID: projectID,
		Data: map[string]string{
			"user_id":    entry.UserID,
			"user_name":  entry.UserName,
			"avatar_url": entry.AvatarURL,
			"event_kind": "presence",
		},
		Timestamp: time.Now(),
	}
}

// busEventToProjectEvent converts an event bus Event to a gRPC ProjectEvent.
// Returns nil if the event is not relevant to project watchers.
func busEventToProjectEvent(e event.Event) *pb.ProjectEvent {
	kind := e.Data["event_kind"]

	if kind == "presence" {
		changeType := "joined"
		if strings.Contains(string(e.Type), "left") {
			changeType = "left"
		} else if strings.Contains(string(e.Type), "moved") {
			changeType = "moved"
		}
		return &pb.ProjectEvent{
			Event: &pb.ProjectEvent_PresenceChange{
				PresenceChange: &pb.PresenceChangeEvent{
					ChangeType: changeType,
					User: &pb.PresenceInfo{
						UserId:    e.Data["user_id"],
						UserName:  e.Data["user_name"],
						AvatarUrl: e.Data["avatar_url"],
						ItemName:  e.Data["item_name"],
						BlockId:   e.Data["block_id"],
					},
				},
			},
		}
	}

	// Block change events.
	if strings.HasPrefix(string(e.Type), "editor.block.") || strings.HasPrefix(string(e.Type), "block.") {
		return &pb.ProjectEvent{
			Event: &pb.ProjectEvent_BlockChange{
				BlockChange: &pb.BlockChangeEvent{
					BlockId:    e.Data["block_id"],
					ItemName:   e.Data["item_name"],
					ChangeType: e.Data["change_type"],
					ChangedBy:  e.Data["changed_by"],
				},
			},
		}
	}

	return nil
}

// Ensure EditorGRPCServer implements the interface at compile time.
func init() {
	var _ pb.EditorServiceServer = (*EditorGRPCServer)(nil)
}
