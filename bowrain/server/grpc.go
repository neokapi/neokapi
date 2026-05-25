package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/connector"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

// GRPCServer implements the NeokapiServiceServer interface.
type GRPCServer struct {
	pb.UnimplementedNeokapiServiceServer
	srv *Server
}

func NewGRPCServer(srv *Server) *GRPCServer {
	return &GRPCServer{srv: srv}
}

func (g *GRPCServer) CreateProject(ctx context.Context, req *pb.CreateProjectRequest) (*pb.ProjectResponse, error) {
	if g.srv.Services == nil {
		return nil, status.Error(codes.Unavailable, "content store not configured")
	}

	locales := make([]model.LocaleID, len(req.TargetLocales))
	for i, l := range req.TargetLocales {
		locales[i] = model.LocaleID(l)
	}

	p := &store.Project{
		Name:                  req.Name,
		DefaultSourceLanguage: model.LocaleID(req.SourceLocale),
		TargetLanguages:       locales,
		Properties:            req.Properties,
	}
	if err := g.srv.Services.Project.CreateProject(ctx, p); err != nil {
		return nil, status.Errorf(codes.Internal, "create project: %v", err)
	}
	return projectToProto(p), nil
}

func (g *GRPCServer) GetProject(ctx context.Context, req *pb.GetProjectRequest) (*pb.ProjectResponse, error) {
	if g.srv.Services == nil {
		return nil, status.Error(codes.Unavailable, "content store not configured")
	}
	p, err := g.srv.Services.Project.GetProject(ctx, req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found: %v", err)
	}
	return projectToProto(p), nil
}

func (g *GRPCServer) ListProjects(ctx context.Context, _ *pb.ListProjectsRequest) (*pb.ListProjectsResponse, error) {
	if g.srv.Services == nil {
		return nil, status.Error(codes.Unavailable, "content store not configured")
	}
	projects, err := g.srv.Services.Project.ListProjects(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list projects: %v", err)
	}
	resp := &pb.ListProjectsResponse{}
	for _, p := range projects {
		resp.Projects = append(resp.Projects, projectToProto(p))
	}
	return resp, nil
}

func (g *GRPCServer) StoreBlocks(ctx context.Context, req *pb.StoreBlocksRequest) (*pb.StoreBlocksResponse, error) {
	if g.srv.Services == nil {
		return nil, status.Error(codes.Unavailable, "content store not configured")
	}

	var blocks []*model.Block
	for _, bm := range req.Blocks {
		b := model.NewBlock(bm.Id, bm.Source)
		b.Name = bm.Name
		b.Type = bm.Type
		for locale, text := range bm.Targets {
			b.SetTargetText(model.LocaleID(locale), text)
		}
		b.Properties = bm.Properties
		blocks = append(blocks, b)
	}

	if err := g.srv.Services.Project.StoreBlocks(ctx, req.ProjectId, blocks); err != nil {
		return nil, status.Errorf(codes.Internal, "store blocks: %v", err)
	}
	return &pb.StoreBlocksResponse{StoredCount: int32(len(blocks))}, nil
}

func (g *GRPCServer) StreamBlocks(req *pb.StreamBlocksRequest, stream pb.NeokapiService_StreamBlocksServer) error {
	if g.srv.Services == nil {
		return status.Error(codes.Unavailable, "content store not configured")
	}

	query := store.BlockQuery{ProjectID: req.ProjectId}
	if len(req.BlockIds) > 0 {
		query.IDs = req.BlockIds
	}

	blocks, err := g.srv.Services.Project.GetBlocks(grpcCtx(stream), query)
	if err != nil {
		return status.Errorf(codes.Internal, "get blocks: %v", err)
	}

	for _, sb := range blocks {
		resp := &pb.BlockResponse{
			Block:     blockToProto(sb.Block),
			ProjectId: sb.ProjectID,
			StoredAt:  sb.StoredAt.Format(time.RFC3339),
			UpdatedAt: sb.UpdatedAt.Format(time.RFC3339),
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
	return nil
}

func (g *GRPCServer) CreateVersion(ctx context.Context, req *pb.CreateVersionRequest) (*pb.VersionResponse, error) {
	if g.srv.Services == nil {
		return nil, status.Error(codes.Unavailable, "content store not configured")
	}
	v, err := g.srv.Services.Project.CreateVersion(ctx, req.ProjectId, req.Label, req.Description)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create version: %v", err)
	}
	return versionToProto(v), nil
}

func (g *GRPCServer) ListVersions(ctx context.Context, req *pb.ListVersionsRequest) (*pb.ListVersionsResponse, error) {
	if g.srv.Services == nil {
		return nil, status.Error(codes.Unavailable, "content store not configured")
	}
	versions, err := g.srv.Services.Project.ListVersions(ctx, req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list versions: %v", err)
	}
	resp := &pb.ListVersionsResponse{}
	for _, v := range versions {
		resp.Versions = append(resp.Versions, versionToProto(v))
	}
	return resp, nil
}

func (g *GRPCServer) PullContent(ctx context.Context, req *pb.PullContentRequest) (*pb.PullContentResponse, error) {
	if g.srv.Services == nil {
		return nil, status.Error(codes.Unavailable, "content store not configured")
	}
	opts := connector.FetchOptions{}
	items, err := g.srv.Services.Connector.Fetch(ctx, req.ConnectorId, req.ProjectId, opts)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "pull content: %v", err)
	}
	var count int
	for _, item := range items {
		count += len(item.Blocks)
	}
	return &pb.PullContentResponse{BlockCount: int32(count)}, nil
}

func (g *GRPCServer) PushContent(ctx context.Context, req *pb.PushContentRequest) (*pb.PushContentResponse, error) {
	if g.srv.Services == nil {
		return nil, status.Error(codes.Unavailable, "content store not configured")
	}

	// Count blocks before pushing so we can report the count.
	blocks, err := g.srv.Services.Project.GetBlocks(ctx, store.BlockQuery{ProjectID: req.ProjectId})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "count blocks: %v", err)
	}

	opts := connector.PublishOptions{}
	if err := g.srv.Services.Connector.Publish(ctx, req.ConnectorId, req.ProjectId, opts); err != nil {
		return nil, status.Errorf(codes.Internal, "push content: %v", err)
	}
	return &pb.PushContentResponse{PushedCount: int32(len(blocks))}, nil
}

// flowConfig is the YAML structure for flow definitions sent via gRPC.
type flowConfig struct {
	Name  string   `yaml:"name"`
	Tools []string `yaml:"tools"`
}

func (g *GRPCServer) ExecuteFlow(req *pb.ExecuteFlowRequest, stream pb.NeokapiService_ExecuteFlowServer) error {
	if g.srv.Services == nil {
		return status.Error(codes.Unavailable, "content store not configured")
	}

	// Parse the YAML flow config.
	var cfg flowConfig
	if err := yaml.Unmarshal([]byte(req.FlowConfig), &cfg); err != nil {
		return status.Errorf(codes.InvalidArgument, "parse flow config: %v", err)
	}
	if len(cfg.Tools) == 0 {
		return status.Error(codes.InvalidArgument, "flow config must specify at least one tool")
	}

	// Send setup progress.
	if err := stream.Send(&pb.FlowProgressResponse{
		Stage:   "setup",
		Message: fmt.Sprintf("building flow %q with %d tools", cfg.Name, len(cfg.Tools)),
	}); err != nil {
		return err
	}

	// Build tools from registry.
	tools := make([]tool.Tool, 0, len(cfg.Tools))
	for _, name := range cfg.Tools {
		t, err := g.srv.ToolRegistry.NewTool(registry.ToolID(name))
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "unknown tool %q: %v", name, err)
		}
		tools = append(tools, t)
	}

	// Load blocks from the store project.
	if req.ProjectId == "" {
		return status.Error(codes.InvalidArgument, "project_id is required for flow execution")
	}

	blocks, err := g.srv.Services.Project.GetBlocks(stream.Context(), store.BlockQuery{ProjectID: req.ProjectId})
	if err != nil {
		return status.Errorf(codes.Internal, "load blocks: %v", err)
	}
	if len(blocks) == 0 {
		return status.Error(codes.NotFound, "no blocks found in project")
	}

	// Send execution progress.
	if err := stream.Send(&pb.FlowProgressResponse{
		Stage:   "executing",
		Message: fmt.Sprintf("processing %d blocks through %d tools", len(blocks), len(tools)),
	}); err != nil {
		return err
	}

	// Process blocks through the tool chain.
	ctx := stream.Context()
	in := make(chan *model.Part, len(blocks)+1)
	for _, sb := range blocks {
		in <- &model.Part{Type: model.PartBlock, Resource: sb.Block}
	}
	close(in)

	out := in
	for _, t := range tools {
		nextOut := make(chan *model.Part, cap(in))
		currentIn := out
		currentTool := t
		go func() {
			defer close(nextOut)
			defer func() {
				if r := recover(); r != nil {
					slog.Error("recovered panic in gRPC tool goroutine", "panic", r)
				}
			}()
			_ = currentTool.Process(ctx, currentIn, nextOut)
		}()
		out = nextOut
	}

	// Collect processed blocks.
	var processed int
	for part := range out {
		if part.Type == model.PartBlock {
			processed++
		}
	}

	// Send completion.
	return stream.Send(&pb.FlowProgressResponse{
		Stage:   "complete",
		Done:    true,
		Message: fmt.Sprintf("flow %q completed: processed %d blocks", cfg.Name, processed),
	})
}

func (g *GRPCServer) Subscribe(req *pb.SubscribeRequest, stream pb.NeokapiService_SubscribeServer) error {
	if g.srv.EventBus == nil {
		return status.Error(codes.Unavailable, "event bus not configured")
	}

	// Determine which event types to listen for.
	types := make([]platev.EventType, len(req.EventTypes))
	for i, t := range req.EventTypes {
		types[i] = platev.EventType(t)
	}

	// Subscribe to events.
	handler := func(e platev.Event) {
		resp := &pb.EventResponse{
			Id:        e.ID,
			Type:      string(e.Type),
			Source:    e.Source,
			ProjectId: e.ProjectID,
			BlockId:   e.Data["block_id"],
			Timestamp: e.Timestamp.Format(time.RFC3339),
			Metadata:  e.Data,
		}
		_ = stream.Send(resp)
	}

	var sub *platev.Subscription
	if len(types) == 0 {
		sub = g.srv.EventBus.SubscribeAll(handler)
	} else {
		sub = g.srv.EventBus.Subscribe(types[0], handler)
		for _, t := range types[1:] {
			g.srv.EventBus.Subscribe(t, handler)
		}
	}

	// Block until client disconnects.
	<-stream.Context().Done()
	g.srv.EventBus.Unsubscribe(sub)
	return nil
}

// --- Conversion helpers ---

func projectToProto(p *store.Project) *pb.ProjectResponse {
	locales := make([]string, len(p.TargetLanguages))
	for i, l := range p.TargetLanguages {
		locales[i] = string(l)
	}
	return &pb.ProjectResponse{
		Id:            p.ID,
		Name:          p.Name,
		SourceLocale:  string(p.DefaultSourceLanguage),
		TargetLocales: locales,
		Properties:    p.Properties,
		CreatedAt:     p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     p.UpdatedAt.Format(time.RFC3339),
	}
}

func blockToProto(b *model.Block) *pb.BlockMessage {
	targets := make(map[string]string, len(b.Targets))
	for _, locale := range b.TargetLocales() {
		targets[string(locale)] = b.TargetText(locale)
	}

	bm := &pb.BlockMessage{
		Id:         b.ID,
		Name:       b.Name,
		Type:       b.Type,
		Source:     b.SourceText(),
		Targets:    targets,
		Properties: b.Properties,
	}
	if b.Identity != nil {
		bm.ContentHash = b.Identity.ContentHash
		bm.ContextHash = b.Identity.ContextHash
	}
	return bm
}

func versionToProto(v *store.Version) *pb.VersionResponse {
	return &pb.VersionResponse{
		Id:          v.ID,
		ProjectId:   v.ProjectID,
		Label:       v.Label,
		Description: v.Description,
		BlockCount:  int32(v.BlockCount),
		CreatedAt:   v.CreatedAt.Format(time.RFC3339),
	}
}

// grpcCtx extracts context from a gRPC server stream.
func grpcCtx(stream interface{ Context() context.Context }) context.Context {
	return stream.Context()
}

// Ensure GRPCServer implements the interface.
func init() {
	var _ pb.NeokapiServiceServer = (*GRPCServer)(nil)
	_ = fmt.Sprintf // suppress unused import
}
