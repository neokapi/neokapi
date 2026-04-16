package backend

import (
	"context"
	"time"

	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// ServerClient wraps a gRPC connection to the Bowrain server's EditorService.
type ServerClient struct {
	conn      *grpc.ClientConn
	editor    pb.EditorServiceClient
	token     string
	serverURL string
}

// NewServerClient creates a new gRPC client to the given server address.
// The address should be in host:port format (e.g. "localhost:9090").
func NewServerClient(grpcAddr, token string, useTLS bool) (*ServerClient, error) {
	var opts []grpc.DialOption

	if useTLS {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(grpcAddr, opts...)
	if err != nil {
		return nil, err
	}

	return &ServerClient{
		conn:      conn,
		editor:    pb.NewEditorServiceClient(conn),
		token:     token,
		serverURL: grpcAddr,
	}, nil
}

// Close closes the underlying gRPC connection.
func (c *ServerClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ctx returns a context with the JWT token in gRPC metadata.
func (c *ServerClient) ctx() context.Context {
	ctx := context.Background()
	if c.token != "" {
		md := metadata.Pairs("authorization", "Bearer "+c.token)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	return ctx
}

// ctxWithTimeout returns an authenticated context with a timeout.
func (c *ServerClient) ctxWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	ctx := c.ctx()
	return context.WithTimeout(ctx, d)
}

// SetToken updates the JWT token used for authentication.
func (c *ServerClient) SetToken(token string) {
	c.token = token
}

// --- Auth & workspace ---

// GetCurrentUser returns the authenticated user's info.
func (c *ServerClient) GetCurrentUser() (*pb.UserResponse, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	return c.editor.GetCurrentUser(ctx, &pb.GetCurrentUserRequest{})
}

// ListWorkspaces returns all workspaces the user is a member of.
func (c *ServerClient) ListWorkspaces() ([]WorkspaceInfo, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	resp, err := c.editor.ListWorkspaces(ctx, &pb.ListWorkspacesRequest{})
	if err != nil {
		return nil, err
	}
	workspaces := make([]WorkspaceInfo, len(resp.Workspaces))
	for i, ws := range resp.Workspaces {
		workspaces[i] = WorkspaceInfo{
			ID:          ws.Id,
			Name:        ws.Name,
			Slug:        ws.Slug,
			Description: ws.Description,
			LogoURL:     ws.LogoUrl,
			Role:        ws.Role,
		}
	}
	return workspaces, nil
}

// --- Projects ---

// ListProjects returns all editor projects in a workspace.
func (c *ServerClient) ListProjects(wsSlug string) ([]ProjectInfo, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	resp, err := c.editor.ListEditorProjects(ctx, &pb.ListEditorProjectsRequest{
		WorkspaceSlug: wsSlug,
	})
	if err != nil {
		return nil, err
	}
	return protoProjectsToInfos(resp.Projects), nil
}

// GetProject returns a single editor project.
func (c *ServerClient) GetProject(wsSlug, projectID string) (*ProjectInfo, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	resp, err := c.editor.GetEditorProject(ctx, &pb.GetEditorProjectRequest{
		WorkspaceSlug: wsSlug,
		ProjectId:     projectID,
	})
	if err != nil {
		return nil, err
	}
	info := protoProjectToInfo(resp.Project)
	return &info, nil
}

// --- Blocks ---

// GetBlocks returns all blocks for an item in a project.
func (c *ServerClient) GetBlocks(wsSlug, projectID, itemName string) ([]BlockInfo, error) {
	ctx, cancel := c.ctxWithTimeout(30 * time.Second)
	defer cancel()
	resp, err := c.editor.GetBlocks(ctx, &pb.GetBlocksRequest{
		WorkspaceSlug: wsSlug,
		ProjectId:     projectID,
		ItemName:      itemName,
	})
	if err != nil {
		return nil, err
	}
	blocks := make([]BlockInfo, len(resp.Blocks))
	for i, b := range resp.Blocks {
		blocks[i] = protoBlockToInfo(b)
	}
	return blocks, nil
}

// UpdateBlockTarget updates a block's target content on the server
// with a structured Run sequence.
func (c *ServerClient) UpdateBlockTarget(wsSlug, projectID, blockID, targetLocale string, runs []RunInfo) error {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	req := &pb.UpdateBlockTargetRequest{
		WorkspaceSlug: wsSlug,
		ProjectId:     projectID,
		BlockId:       blockID,
		TargetLocale:  targetLocale,
		Runs:          runInfosToEditorProto(runs),
	}
	_, err := c.editor.UpdateBlockTarget(ctx, req)
	return err
}

// runInfosToEditorProto converts the frontend-facing RunInfo
// representation to the gRPC EditorRun oneof shape.
func runInfosToEditorProto(infos []RunInfo) []*pb.EditorRun {
	if len(infos) == 0 {
		return nil
	}
	out := make([]*pb.EditorRun, len(infos))
	for i, ri := range infos {
		out[i] = runInfoToEditorProto(ri)
	}
	return out
}

// editorProtoToRunInfos reverses runInfosToEditorProto.
func editorProtoToRunInfos(msgs []*pb.EditorRun) []RunInfo {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]RunInfo, len(msgs))
	for i, m := range msgs {
		out[i] = editorProtoToRunInfo(m)
	}
	return out
}

func runInfoToEditorProto(ri RunInfo) *pb.EditorRun {
	switch {
	case ri.Text != nil:
		return &pb.EditorRun{Kind: &pb.EditorRun_Text{Text: &pb.EditorTextRun{Text: ri.Text.Text}}}
	case ri.Ph != nil:
		return &pb.EditorRun{Kind: &pb.EditorRun_Ph{Ph: &pb.EditorPlaceholderRun{
			Id: ri.Ph.ID, Type: ri.Ph.Type, SubType: ri.Ph.SubType,
			Data: ri.Ph.Data, Equiv: ri.Ph.Equiv, Disp: ri.Ph.Disp,
			Constraints: runConstraintsInfoToEditorProto(ri.Ph.Constraints),
		}}}
	case ri.PcOpen != nil:
		return &pb.EditorRun{Kind: &pb.EditorRun_PcOpen{PcOpen: &pb.EditorPcOpenRun{
			Id: ri.PcOpen.ID, Type: ri.PcOpen.Type, SubType: ri.PcOpen.SubType,
			Data: ri.PcOpen.Data, Equiv: ri.PcOpen.Equiv, Disp: ri.PcOpen.Disp,
			Constraints: runConstraintsInfoToEditorProto(ri.PcOpen.Constraints),
		}}}
	case ri.PcClose != nil:
		return &pb.EditorRun{Kind: &pb.EditorRun_PcClose{PcClose: &pb.EditorPcCloseRun{
			Id: ri.PcClose.ID, Type: ri.PcClose.Type, SubType: ri.PcClose.SubType,
			Data: ri.PcClose.Data, Equiv: ri.PcClose.Equiv,
		}}}
	case ri.Sub != nil:
		return &pb.EditorRun{Kind: &pb.EditorRun_Sub{Sub: &pb.EditorSubRun{
			Id: ri.Sub.ID, Ref: ri.Sub.Ref, Equiv: ri.Sub.Equiv,
		}}}
	case ri.Plural != nil:
		forms := make(map[string]*pb.EditorRunList, len(ri.Plural.Forms))
		for form, runs := range ri.Plural.Forms {
			forms[form] = &pb.EditorRunList{Runs: runInfosToEditorProto(runs)}
		}
		return &pb.EditorRun{Kind: &pb.EditorRun_Plural{Plural: &pb.EditorPluralRun{
			Pivot: ri.Plural.Pivot, Forms: forms,
		}}}
	case ri.Select != nil:
		cases := make(map[string]*pb.EditorRunList, len(ri.Select.Cases))
		for key, runs := range ri.Select.Cases {
			cases[key] = &pb.EditorRunList{Runs: runInfosToEditorProto(runs)}
		}
		return &pb.EditorRun{Kind: &pb.EditorRun_Select{Select: &pb.EditorSelectRun{
			Pivot: ri.Select.Pivot, Cases: cases,
		}}}
	}
	return nil
}

func editorProtoToRunInfo(msg *pb.EditorRun) RunInfo {
	if msg == nil {
		return RunInfo{}
	}
	switch k := msg.Kind.(type) {
	case *pb.EditorRun_Text:
		return RunInfo{Text: &TextRunInfo{Text: k.Text.GetText()}}
	case *pb.EditorRun_Ph:
		return RunInfo{Ph: &PlaceholderRunInfo{
			ID: k.Ph.GetId(), Type: k.Ph.GetType(), SubType: k.Ph.GetSubType(),
			Data: k.Ph.GetData(), Equiv: k.Ph.GetEquiv(), Disp: k.Ph.GetDisp(),
			Constraints: editorProtoToRunConstraintsInfo(k.Ph.GetConstraints()),
		}}
	case *pb.EditorRun_PcOpen:
		return RunInfo{PcOpen: &PcOpenRunInfo{
			ID: k.PcOpen.GetId(), Type: k.PcOpen.GetType(), SubType: k.PcOpen.GetSubType(),
			Data: k.PcOpen.GetData(), Equiv: k.PcOpen.GetEquiv(), Disp: k.PcOpen.GetDisp(),
			Constraints: editorProtoToRunConstraintsInfo(k.PcOpen.GetConstraints()),
		}}
	case *pb.EditorRun_PcClose:
		return RunInfo{PcClose: &PcCloseRunInfo{
			ID: k.PcClose.GetId(), Type: k.PcClose.GetType(), SubType: k.PcClose.GetSubType(),
			Data: k.PcClose.GetData(), Equiv: k.PcClose.GetEquiv(),
		}}
	case *pb.EditorRun_Sub:
		return RunInfo{Sub: &SubRunInfo{
			ID: k.Sub.GetId(), Ref: k.Sub.GetRef(), Equiv: k.Sub.GetEquiv(),
		}}
	case *pb.EditorRun_Plural:
		forms := make(map[string][]RunInfo, len(k.Plural.GetForms()))
		for form, runList := range k.Plural.GetForms() {
			forms[form] = editorProtoToRunInfos(runList.GetRuns())
		}
		return RunInfo{Plural: &PluralRunInfo{Pivot: k.Plural.GetPivot(), Forms: forms}}
	case *pb.EditorRun_Select:
		cases := make(map[string][]RunInfo, len(k.Select.GetCases()))
		for key, runList := range k.Select.GetCases() {
			cases[key] = editorProtoToRunInfos(runList.GetRuns())
		}
		return RunInfo{Select: &SelectRunInfo{Pivot: k.Select.GetPivot(), Cases: cases}}
	}
	return RunInfo{}
}

func runConstraintsInfoToEditorProto(ri *RunConstraintsInfo) *pb.EditorRunConstraints {
	if ri == nil {
		return nil
	}
	return &pb.EditorRunConstraints{Deletable: ri.Deletable, Cloneable: ri.Cloneable, Reorderable: ri.Reorderable}
}

func editorProtoToRunConstraintsInfo(msg *pb.EditorRunConstraints) *RunConstraintsInfo {
	if msg == nil {
		return nil
	}
	return &RunConstraintsInfo{Deletable: msg.GetDeletable(), Cloneable: msg.GetCloneable(), Reorderable: msg.GetReorderable()}
}

// ReviewBlock sets or clears the reviewed status on a block.
func (c *ServerClient) ReviewBlock(wsSlug, projectID, itemName, blockID, targetLocale string, reviewed bool) error {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	_, err := c.editor.ReviewBlock(ctx, &pb.ReviewBlockRequest{
		WorkspaceSlug: wsSlug,
		ProjectId:     projectID,
		ItemName:      itemName,
		BlockId:       blockID,
		TargetLocale:  targetLocale,
		Reviewed:      reviewed,
	})
	return err
}

// --- TM lookup ---

// LookupTMForBlock returns TM matches for a block.
func (c *ServerClient) LookupTMForBlock(wsSlug, projectID, blockID, targetLocale string) ([]TMMatchInfo, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	resp, err := c.editor.LookupTMForBlock(ctx, &pb.TMLookupRequest{
		WorkspaceSlug: wsSlug,
		ProjectId:     projectID,
		BlockId:       blockID,
		TargetLocale:  targetLocale,
	})
	if err != nil {
		return nil, err
	}
	matches := make([]TMMatchInfo, len(resp.Matches))
	for i, m := range resp.Matches {
		matches[i] = TMMatchInfo{
			Source:    m.Source,
			Target:    m.Target,
			Score:     m.Score,
			MatchType: m.MatchType,
		}
	}
	return matches, nil
}

// --- Term lookup ---

// LookupTermsForBlock returns term matches for a block.
func (c *ServerClient) LookupTermsForBlock(wsSlug, projectID, blockID, targetLocale string) ([]BlockTermMatch, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	resp, err := c.editor.LookupTermsForBlock(ctx, &pb.TermLookupRequest{
		WorkspaceSlug: wsSlug,
		ProjectId:     projectID,
		BlockId:       blockID,
		TargetLocale:  targetLocale,
	})
	if err != nil {
		return nil, err
	}
	matches := make([]BlockTermMatch, len(resp.Matches))
	for i, m := range resp.Matches {
		matches[i] = BlockTermMatch{
			SourceTerm:  m.SourceTerm,
			TargetTerms: m.TargetTerms,
			Domain:      m.Domain,
			Status:      m.Status,
			Start:       int(m.Start),
			End:         int(m.End),
		}
	}
	return matches, nil
}

// --- TM CRUD ---

// GetTMEntries searches TM entries on the server.
func (c *ServerClient) GetTMEntries(wsSlug, query, sourceLocale, targetLocale string, offset, limit int) (*TMSearchResult, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	resp, err := c.editor.GetTMEntries(ctx, &pb.TMEntriesRequest{
		WorkspaceSlug: wsSlug,
		Query:         query,
		SourceLocale:  sourceLocale,
		TargetLocale:  targetLocale,
		Offset:        int32(offset),
		Limit:         int32(limit),
	})
	if err != nil {
		return nil, err
	}
	entries := make([]TMEntryInfo, len(resp.Entries))
	for i, e := range resp.Entries {
		entries[i] = TMEntryInfo{
			ID:           e.Id,
			Source:       e.Source,
			Target:       e.Target,
			SourceLocale: e.SourceLocale,
			TargetLocale: e.TargetLocale,
			UpdatedAt:    e.UpdatedAt,
		}
	}
	return &TMSearchResult{Entries: entries, TotalCount: int(resp.TotalCount)}, nil
}

// GetTMCount returns the TM entry count.
func (c *ServerClient) GetTMCount(wsSlug string) (int, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	resp, err := c.editor.GetTMCount(ctx, &pb.TMCountRequest{WorkspaceSlug: wsSlug})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

// AddTMEntry adds a new TM entry.
func (c *ServerClient) AddTMEntry(wsSlug, source, target, sourceLocale, targetLocale string) (*TMEntryInfo, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	resp, err := c.editor.AddTMEntry(ctx, &pb.AddTMEntryRequest{
		WorkspaceSlug: wsSlug,
		Source:        source,
		Target:        target,
		SourceLocale:  sourceLocale,
		TargetLocale:  targetLocale,
	})
	if err != nil {
		return nil, err
	}
	info := TMEntryInfo{
		ID:           resp.Entry.Id,
		Source:       resp.Entry.Source,
		Target:       resp.Entry.Target,
		SourceLocale: resp.Entry.SourceLocale,
		TargetLocale: resp.Entry.TargetLocale,
		UpdatedAt:    resp.Entry.UpdatedAt,
	}
	return &info, nil
}

// UpdateTMEntry updates a TM entry.
func (c *ServerClient) UpdateTMEntry(wsSlug, entryID, source, target, sourceLocale, targetLocale string) error {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	_, err := c.editor.UpdateTMEntry(ctx, &pb.UpdateTMEntryRequest{
		WorkspaceSlug: wsSlug,
		EntryId:       entryID,
		Source:        source,
		Target:        target,
		SourceLocale:  sourceLocale,
		TargetLocale:  targetLocale,
	})
	return err
}

// DeleteTMEntry deletes a TM entry.
func (c *ServerClient) DeleteTMEntry(wsSlug, entryID string) error {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	_, err := c.editor.DeleteTMEntry(ctx, &pb.DeleteTMEntryRequest{
		WorkspaceSlug: wsSlug,
		EntryId:       entryID,
	})
	return err
}

// --- Terminology CRUD ---

// GetTerms searches terminology concepts.
func (c *ServerClient) GetTerms(wsSlug, query, sourceLocale, targetLocale string, offset, limit int) (*TermSearchResult, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	resp, err := c.editor.GetTerms(ctx, &pb.TermsRequest{
		WorkspaceSlug: wsSlug,
		Query:         query,
		SourceLocale:  sourceLocale,
		TargetLocale:  targetLocale,
		Offset:        int32(offset),
		Limit:         int32(limit),
	})
	if err != nil {
		return nil, err
	}
	concepts := make([]ConceptInfo, len(resp.Concepts))
	for i, c := range resp.Concepts {
		concepts[i] = protoConceptToInfo(c)
	}
	return &TermSearchResult{Concepts: concepts, TotalCount: int(resp.TotalCount)}, nil
}

// GetTermCount returns the concept count.
func (c *ServerClient) GetTermCount(wsSlug string) (int, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	resp, err := c.editor.GetTermCount(ctx, &pb.TermCountRequest{WorkspaceSlug: wsSlug})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

// AddConcept adds a new terminology concept.
func (c *ServerClient) AddConcept(wsSlug, domain, definition string, terms []TermInfo) (*ConceptInfo, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	resp, err := c.editor.AddConcept(ctx, &pb.AddConceptRequest{
		WorkspaceSlug: wsSlug,
		Domain:        domain,
		Definition:    definition,
		Terms:         infoTermsToProto(terms),
	})
	if err != nil {
		return nil, err
	}
	info := protoConceptToInfo(resp.Concept)
	return &info, nil
}

// UpdateConcept updates a concept.
func (c *ServerClient) UpdateConcept(wsSlug, conceptID, domain, definition string, terms []TermInfo) error {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	_, err := c.editor.UpdateConcept(ctx, &pb.UpdateConceptRequest{
		WorkspaceSlug: wsSlug,
		ConceptId:     conceptID,
		Domain:        domain,
		Definition:    definition,
		Terms:         infoTermsToProto(terms),
	})
	return err
}

// DeleteConcept deletes a concept.
func (c *ServerClient) DeleteConcept(wsSlug, conceptID string) error {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	_, err := c.editor.DeleteConcept(ctx, &pb.DeleteConceptRequest{
		WorkspaceSlug: wsSlug,
		ConceptId:     conceptID,
	})
	return err
}

// ImportTermsCSV imports terms from CSV.
func (c *ServerClient) ImportTermsCSV(wsSlug, csvContent, sourceLocale, targetLocale, domain string, hasHeader bool) (int, error) {
	ctx, cancel := c.ctxWithTimeout(30 * time.Second)
	defer cancel()
	resp, err := c.editor.ImportTermsCSV(ctx, &pb.ImportTermsCSVRequest{
		WorkspaceSlug: wsSlug,
		CsvContent:    csvContent,
		SourceLocale:  sourceLocale,
		TargetLocale:  targetLocale,
		Domain:        domain,
		HasHeader:     hasHeader,
	})
	if err != nil {
		return 0, err
	}
	return int(resp.Imported), nil
}

// ImportTermsJSON imports terms from JSON.
func (c *ServerClient) ImportTermsJSON(wsSlug, jsonContent string) (int, error) {
	ctx, cancel := c.ctxWithTimeout(30 * time.Second)
	defer cancel()
	resp, err := c.editor.ImportTermsJSON(ctx, &pb.ImportTermsJSONRequest{
		WorkspaceSlug: wsSlug,
		JsonContent:   jsonContent,
	})
	if err != nil {
		return 0, err
	}
	return int(resp.Imported), nil
}

// ExportTermsJSON exports terms as JSON.
func (c *ServerClient) ExportTermsJSON(wsSlug, name string) (string, error) {
	ctx, cancel := c.ctxWithTimeout(30 * time.Second)
	defer cancel()
	resp, err := c.editor.ExportTermsJSON(ctx, &pb.ExportTermsJSONRequest{
		WorkspaceSlug: wsSlug,
		Name:          name,
	})
	if err != nil {
		return "", err
	}
	return resp.JsonContent, nil
}

// --- AI provider configuration ---

// ListProviderConfigs returns all provider configurations from the server (no API keys).
func (c *ServerClient) ListProviderConfigs(wsSlug string) ([]ProviderConfigInfo, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	resp, err := c.editor.ListProviderConfigs(ctx, &pb.ListProviderConfigsRequest{
		WorkspaceSlug: wsSlug,
	})
	if err != nil {
		return nil, err
	}
	configs := make([]ProviderConfigInfo, len(resp.Configs))
	for i, cfg := range resp.Configs {
		configs[i] = ProviderConfigInfo{
			ID:           cfg.Id,
			Name:         cfg.Name,
			ProviderType: cfg.ProviderType,
			Model:        cfg.Model,
			BaseURL:      cfg.BaseUrl,
		}
	}
	return configs, nil
}

// SaveProviderConfig creates or updates a provider config on the server.
func (c *ServerClient) SaveProviderConfig(wsSlug string, req SaveProviderRequest) (*ProviderConfigInfo, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	resp, err := c.editor.SaveProviderConfig(ctx, &pb.SaveProviderConfigRPC{
		WorkspaceSlug: wsSlug,
		Id:            req.ID,
		Name:          req.Name,
		ProviderType:  req.ProviderType,
		Model:         req.Model,
		BaseUrl:       req.BaseURL,
		ApiKey:        req.APIKey,
	})
	if err != nil {
		return nil, err
	}
	info := ProviderConfigInfo{
		ID:           resp.Id,
		Name:         resp.Name,
		ProviderType: resp.ProviderType,
		Model:        resp.Model,
		BaseURL:      resp.BaseUrl,
	}
	return &info, nil
}

// DeleteProviderConfig removes a provider config on the server.
func (c *ServerClient) DeleteProviderConfig(wsSlug, id string) error {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	_, err := c.editor.DeleteProviderConfig(ctx, &pb.DeleteProviderConfigRequest{
		WorkspaceSlug: wsSlug,
		Id:            id,
	})
	return err
}

// TestProviderConfig tests a provider config on the server.
func (c *ServerClient) TestProviderConfig(wsSlug string, req SaveProviderRequest) error {
	ctx, cancel := c.ctxWithTimeout(30 * time.Second)
	defer cancel()
	_, err := c.editor.TestProviderConfig(ctx, &pb.TestProviderConfigRPC{
		WorkspaceSlug: wsSlug,
		Id:            req.ID,
		Name:          req.Name,
		ProviderType:  req.ProviderType,
		Model:         req.Model,
		BaseUrl:       req.BaseURL,
		ApiKey:        req.APIKey,
	})
	return err
}

// --- Presence ---

// UpdatePresence reports the user's current focus.
func (c *ServerClient) UpdatePresence(wsSlug, projectID, itemName, blockID string) error {
	ctx, cancel := c.ctxWithTimeout(5 * time.Second)
	defer cancel()
	_, err := c.editor.UpdatePresence(ctx, &pb.UpdatePresenceRequest{
		WorkspaceSlug: wsSlug,
		ProjectId:     projectID,
		ItemName:      itemName,
		BlockId:       blockID,
	})
	return err
}

// --- Proto conversion helpers ---

func protoProjectsToInfos(protos []*pb.EditorProjectInfo) []ProjectInfo {
	result := make([]ProjectInfo, len(protos))
	for i, p := range protos {
		result[i] = protoProjectToInfo(p)
	}
	return result
}

func protoProjectToInfo(p *pb.EditorProjectInfo) ProjectInfo {
	items := make([]ProjectItem, len(p.Items))
	for i, item := range p.Items {
		items[i] = ProjectItem{
			ID:         item.Id,
			Name:       item.Name,
			Format:     item.Format,
			Type:       item.Type,
			Size:       item.Size,
			BlockCount: int(item.BlockCount),
			WordCount:  int(item.WordCount),
		}
	}
	return ProjectInfo{
		ID:                    p.Id,
		Name:                  p.Name,
		DefaultSourceLanguage: p.SourceLocale,
		TargetLanguages:       p.TargetLocales,
		Items:                 items,
		CreatedAt:             p.CreatedAt,
		ModifiedAt:            p.ModifiedAt,
	}
}

func protoBlockToInfo(b *pb.BlockInfo) BlockInfo {
	targetRuns := make(map[string][]RunInfo, len(b.TargetRuns))
	for locale, rl := range b.TargetRuns {
		targetRuns[locale] = editorProtoToRunInfos(rl.GetRuns())
	}
	return BlockInfo{
		ID:           b.Id,
		SourceRuns:   editorProtoToRunInfos(b.SourceRuns),
		TargetRuns:   targetRuns,
		Translatable: b.Translatable,
		Properties:   b.Properties,
	}
}

func protoConceptToInfo(c *pb.ConceptInfo) ConceptInfo {
	terms := make([]TermInfo, len(c.Terms))
	for i, t := range c.Terms {
		terms[i] = TermInfo{
			Text:         t.Text,
			Locale:       t.Locale,
			Status:       t.Status,
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
		}
	}
	return ConceptInfo{
		ID:         c.Id,
		Domain:     c.Domain,
		Definition: c.Definition,
		Terms:      terms,
		Properties: c.Properties,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}
}

func infoTermsToProto(terms []TermInfo) []*pb.TermInfo {
	result := make([]*pb.TermInfo, len(terms))
	for i, t := range terms {
		result[i] = &pb.TermInfo{
			Text:         t.Text,
			Locale:       t.Locale,
			Status:       t.Status,
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
		}
	}
	return result
}
