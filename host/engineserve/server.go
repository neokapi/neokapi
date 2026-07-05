// Package engineserve implements the EngineService gRPC server: the neokapi
// content engine (format registry + tool registry + flow executor) served to
// foreign-language callers over a local socket. It is the plugin protocol
// flipped outbound — where cli/pluginhost drives a plugin daemon's
// BridgeService as a client, this package serves the symmetric surface with
// kapi as the engine.
//
// The package holds no listener code: it implements the service against any
// grpc.Server (Unix socket in `kapi engine serve`, bufconn in tests). Errors
// are returned as gRPC status errors, not in-band strings.
package engineserve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
	contentv1 "github.com/neokapi/neokapi/core/proto/content/v1"
	enginev1 "github.com/neokapi/neokapi/core/proto/engine/v1"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/safeio"
	"github.com/neokapi/neokapi/core/tool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// partBatchSize is how many parts are framed into one PartBatch message —
// the same amortization the bridge protocol uses.
const partBatchSize = 64

// Server implements enginev1.EngineServiceServer on top of the shared
// registries. Both registries must be fully populated (InitRegistries — and,
// when plugin formats should be reachable, plugin discovery) before serving.
type Server struct {
	enginev1.UnimplementedEngineServiceServer

	FormatReg *registry.FormatRegistry
	ToolReg   *registry.ToolRegistry
}

// New returns a Server bound to the given registries.
func New(formatReg *registry.FormatRegistry, toolReg *registry.ToolRegistry) *Server {
	return &Server{FormatReg: formatReg, ToolReg: toolReg}
}

// ────────────────────────────────────────────────────────────────────────────
// Extract
// ────────────────────────────────────────────────────────────────────────────

// Extract parses one document into a Part stream. Header first, then chunks
// (unless the header's input reference locates the bytes), then CloseSend;
// PartBatch messages stream back, closed by an ExtractComplete.
func (s *Server) Extract(stream enginev1.EngineService_ExtractServer) error {
	ctx := stream.Context()

	header, err := recvExtractHeader(stream)
	if err != nil {
		return err
	}

	content, srcPath, err := gatherDocument(header.GetInput(), func() ([]byte, error) {
		return recvExtractChunks(stream)
	})
	if err != nil {
		return err
	}

	formatID, err := s.resolveFormat(header.GetFormat(), header.GetName(), srcPath, content)
	if err != nil {
		return err
	}

	reader, err := s.FormatReg.NewReader(formatID)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "format %q: %v", formatID, err)
	}
	defer reader.Close()
	if err := applyConfigMap(reader.Config(), header.GetConfig()); err != nil {
		return status.Errorf(codes.InvalidArgument, "apply format config: %v", err)
	}

	doc := &model.RawDocument{
		URI:          docURI(header.GetName(), srcPath),
		Encoding:     header.GetEncoding(),
		SourceLocale: model.LocaleID(header.GetSourceLocale()),
		TargetLocale: model.LocaleID(header.GetTargetLocale()),
		FormatID:     string(formatID),
	}
	if srcPath != "" {
		f, err := os.Open(srcPath)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "open input %s: %v", srcPath, err)
		}
		defer f.Close()
		doc.Reader = f
	} else {
		doc.Reader = io.NopCloser(bytes.NewReader(content))
	}

	if err := reader.Open(ctx, doc); err != nil {
		return status.Errorf(codes.Internal, "open document: %v", err)
	}

	sent := 0
	batch := make([]*contentv1.PartMessage, 0, partBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := stream.Send(&enginev1.ExtractResponse{
			Response: &enginev1.ExtractResponse_Parts{Parts: &enginev1.PartBatch{Parts: batch}},
		})
		sent += len(batch)
		batch = make([]*contentv1.PartMessage, 0, partBatchSize)
		return err
	}
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			return status.Errorf(codes.Internal, "read: %v", res.Error)
		}
		batch = append(batch, protoconvert.PartToProto(res.Part))
		if len(batch) >= partBatchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return status.FromContextError(err).Err()
	}
	if err := flush(); err != nil {
		return err
	}
	return stream.Send(&enginev1.ExtractResponse{
		Response: &enginev1.ExtractResponse_Complete{
			Complete: &enginev1.ExtractComplete{Format: string(formatID), Parts: int32(sent)}, //nolint:gosec // part counts don't overflow int32
		},
	})
}

func recvExtractHeader(stream enginev1.EngineService_ExtractServer) (*enginev1.ExtractHeader, error) {
	req, err := stream.Recv()
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "expected header: %v", err)
	}
	header := req.GetHeader()
	if header == nil {
		return nil, status.Error(codes.InvalidArgument, "first message must be a header")
	}
	return header, nil
}

func recvExtractChunks(stream enginev1.EngineService_ExtractServer) ([]byte, error) {
	var buf bytes.Buffer
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return buf.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
		if c := req.GetChunk(); c != nil {
			// gRPC's per-message limit bounds each chunk, not the total; cap
			// the accumulated document at the same budget the readers apply
			// (safeio, AD-005) so a runaway stream cannot exhaust memory.
			if int64(buf.Len())+int64(len(c.GetData())) > safeio.DefaultMaxBytes {
				return nil, status.Errorf(codes.ResourceExhausted,
					"document exceeds the %d-byte budget", safeio.DefaultMaxBytes)
			}
			buf.Write(c.GetData())
		}
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Process
// ────────────────────────────────────────────────────────────────────────────

// Process runs a tool chain (or named built-in flow) over the incoming Part
// stream through flow.Executor — each tool in its own concurrent stage,
// exactly like the CLI pipeline.
func (s *Server) Process(stream enginev1.EngineService_ProcessServer) error {
	ctx := stream.Context()

	req, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "expected header: %v", err)
	}
	header := req.GetHeader()
	if header == nil {
		return status.Error(codes.InvalidArgument, "first message must be a header")
	}

	tools, err := s.buildTools(header)
	if err != nil {
		return err
	}

	executor := flow.NewExecutor(flow.WithChannelSize(partBatchSize))
	flowName := header.GetFlow()
	if flowName == "" {
		flowName = "process"
	}
	in, out, wait := executor.ExecuteWithChannels(ctx, &flow.Flow{Name: flowName, Tools: tools})

	// Feed incoming parts on a separate goroutine while draining the
	// executor's output here — the standard concurrent pipeline shape.
	var received int
	feedErr := make(chan error, 1)
	go func() {
		defer close(in)
		for {
			req, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				feedErr <- nil
				return
			}
			if err != nil {
				feedErr <- err
				return
			}
			if pb := req.GetParts(); pb != nil {
				for _, p := range pb.GetParts() {
					received++
					select {
					case in <- protoconvert.ProtoToPart(p):
					case <-ctx.Done():
						feedErr <- ctx.Err()
						return
					}
				}
			}
		}
	}()

	sent := 0
	batch := make([]*contentv1.PartMessage, 0, partBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := stream.Send(&enginev1.ProcessResponse{
			Response: &enginev1.ProcessResponse_Parts{Parts: &enginev1.PartBatch{Parts: batch}},
		})
		sent += len(batch)
		batch = make([]*contentv1.PartMessage, 0, partBatchSize)
		return err
	}
	for p := range out {
		batch = append(batch, protoconvert.PartToProto(p))
		if len(batch) >= partBatchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := wait(); err != nil {
		return status.Errorf(codes.Internal, "process: %v", err)
	}
	if err := <-feedErr; err != nil {
		return status.Errorf(codes.Internal, "receive parts: %v", err)
	}
	if err := flush(); err != nil {
		return err
	}
	return stream.Send(&enginev1.ProcessResponse{
		Response: &enginev1.ProcessResponse_Complete{
			Complete: &enginev1.ProcessComplete{PartsIn: int32(received), PartsOut: int32(sent)}, //nolint:gosec // part counts don't overflow int32
		},
	})
}

// buildTools assembles the ordered tool chain from a ProcessHeader: either an
// explicit ToolSpec list or a named built-in flow's tool nodes in topological
// order. Tool configs go through the registry's schema-driven factory
// (NewToolWithConfig), so credential preprocessing and defaults behave as in
// the CLI.
func (s *Server) buildTools(header *enginev1.ProcessHeader) ([]tool.Tool, error) {
	type spec struct {
		name   string
		config map[string]any
	}
	var specs []spec

	switch {
	case header.GetFlow() != "" && len(header.GetTools()) > 0:
		return nil, status.Error(codes.InvalidArgument, "set either flow or tools, not both")
	case header.GetFlow() != "":
		def := builtinFlow(header.GetFlow())
		if def == nil {
			return nil, status.Errorf(codes.InvalidArgument, "unknown flow %q", header.GetFlow())
		}
		nodes, err := toolNodesInOrder(def)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "flow %q: %v", header.GetFlow(), err)
		}
		for _, n := range nodes {
			specs = append(specs, spec{name: n.Name, config: n.Config})
		}
	case len(header.GetTools()) > 0:
		for _, t := range header.GetTools() {
			var config map[string]any
			if len(t.GetConfig()) > 0 {
				if err := json.Unmarshal(t.GetConfig(), &config); err != nil {
					return nil, status.Errorf(codes.InvalidArgument, "tool %q: config is not a JSON object: %v", t.GetTool(), err)
				}
			}
			specs = append(specs, spec{name: t.GetTool(), config: config})
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "header names no flow and no tools")
	}

	tools := make([]tool.Tool, 0, len(specs))
	for _, sp := range specs {
		if !s.ToolReg.Has(registry.ToolID(sp.name)) {
			return nil, status.Errorf(codes.InvalidArgument, "unknown tool %q", sp.name)
		}
		t, err := s.ToolReg.NewToolWithConfig(registry.ToolID(sp.name), sp.config, header.GetTargetLocale())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "tool %q: %v", sp.name, err)
		}
		tools = append(tools, t)
	}
	return tools, nil
}

func builtinFlow(id string) *flow.FlowDefinition {
	for _, def := range flow.BuiltInFlows() {
		if def.ID == id {
			return &def
		}
	}
	return nil
}

// toolNodesInOrder returns a flow definition's tool nodes in execution
// (topological) order, with each node's config. Only linear chains are
// supported: the tools run as one sequential pipeline stage list, so a
// fan-out or merge-join shape would be silently serialized — reject it
// instead so the failure mode is explicit.
func toolNodesInOrder(def *flow.FlowDefinition) ([]flow.FlowNode, error) {
	out := make(map[string]int, len(def.Nodes))
	in := make(map[string]int, len(def.Nodes))
	for _, e := range def.Edges {
		out[e.Source]++
		in[e.Target]++
		if out[e.Source] > 1 || in[e.Target] > 1 {
			return nil, status.Errorf(codes.InvalidArgument,
				"flow %s has parallel branches; the engine service runs linear tool chains only", def.ID)
		}
	}
	order, err := def.TopologicalOrder()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]flow.FlowNode, len(def.Nodes))
	for _, n := range def.Nodes {
		byID[n.ID] = n
	}
	var nodes []flow.FlowNode
	for _, id := range order {
		if n, ok := byID[id]; ok && n.Type == flow.NodeTool {
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Merge
// ────────────────────────────────────────────────────────────────────────────

// mergeChunkSize is how output document bytes are framed on the way back.
const mergeChunkSize = 256 * 1024

// Merge writes the incoming Part stream back to document bytes via the
// format's writer (skeleton round-trip). The header's original document is
// the skeleton reference for writers that rebuild from the source bytes.
func (s *Server) Merge(stream enginev1.EngineService_MergeServer) error {
	ctx := stream.Context()

	req, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "expected header: %v", err)
	}
	header := req.GetHeader()
	if header == nil {
		return status.Error(codes.InvalidArgument, "first message must be a header")
	}

	original, srcPath, err := gatherDocument(header.GetOriginal(), func() ([]byte, error) { return nil, nil })
	if err != nil {
		return err
	}
	if srcPath != "" && len(original) == 0 {
		if original, err = os.ReadFile(srcPath); err != nil {
			return status.Errorf(codes.InvalidArgument, "read original %s: %v", srcPath, err)
		}
	}

	formatID, err := s.resolveFormat(header.GetFormat(), header.GetName(), srcPath, original)
	if err != nil {
		return err
	}

	writer, err := s.FormatReg.NewWriter(formatID)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "format %q: %v", formatID, err)
	}
	if wc, ok := writer.(format.WriterConfigurable); ok {
		if err := applyConfigMap(wc.WriterConfig(), header.GetConfig()); err != nil {
			return status.Errorf(codes.InvalidArgument, "apply format config: %v", err)
		}
	} else if len(header.GetConfig()) > 0 {
		return status.Errorf(codes.InvalidArgument, "format %q writer accepts no config", formatID)
	}

	var out bytes.Buffer
	if err := writer.SetOutputWriter(&out); err != nil {
		return status.Errorf(codes.Internal, "set output: %v", err)
	}
	// Skeleton reference: prefer the on-disk path (the writer resolves
	// companion files itself), fall back to the original bytes — the same
	// precedence the file runner uses.
	if sps, ok := writer.(format.SourcePathSetter); ok && srcPath != "" {
		sps.SetSourcePath(srcPath)
	} else if ocs, ok := writer.(format.OriginalContentSetter); ok && len(original) > 0 {
		ocs.SetOriginalContent(original)
	}
	writer.SetLocale(model.LocaleID(header.GetTargetLocale()))

	parts := make(chan *model.Part, partBatchSize)
	feedErr := make(chan error, 1)
	go func() {
		defer close(parts)
		for {
			req, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				feedErr <- nil
				return
			}
			if err != nil {
				feedErr <- err
				return
			}
			if pb := req.GetParts(); pb != nil {
				for _, p := range pb.GetParts() {
					select {
					case parts <- protoconvert.ProtoToPart(p):
					case <-ctx.Done():
						feedErr <- ctx.Err()
						return
					}
				}
			}
		}
	}()

	if err := writer.Write(ctx, parts); err != nil {
		return status.Errorf(codes.Internal, "write: %v", err)
	}
	if err := <-feedErr; err != nil {
		return status.Errorf(codes.Internal, "receive parts: %v", err)
	}

	data := out.Bytes()
	for off := 0; off < len(data); off += mergeChunkSize {
		end := min(off+mergeChunkSize, len(data))
		if err := stream.Send(&enginev1.MergeResponse{
			Response: &enginev1.MergeResponse_Chunk{Chunk: &enginev1.DocumentChunk{Data: data[off:end]}},
		}); err != nil {
			return err
		}
	}
	return stream.Send(&enginev1.MergeResponse{
		Response: &enginev1.MergeResponse_Complete{
			Complete: &enginev1.MergeComplete{Bytes: int64(len(data))},
		},
	})
}

// ────────────────────────────────────────────────────────────────────────────
// Detect & listings
// ────────────────────────────────────────────────────────────────────────────

// Detect identifies a document's format from its name and an optional
// content sample.
func (s *Server) Detect(_ context.Context, req *enginev1.DetectRequest) (*enginev1.DetectResponse, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	id, err := s.detect(req.GetName(), req.GetSample())
	if err != nil {
		return &enginev1.DetectResponse{Format: ""}, nil //nolint:nilerr // undetected is a result, not an RPC failure
	}
	return &enginev1.DetectResponse{Format: string(id)}, nil
}

// ListFormats returns the registered formats.
func (s *Server) ListFormats(context.Context, *enginev1.ListFormatsRequest) (*enginev1.ListFormatsResponse, error) {
	infos := s.FormatReg.FormatInfos()
	resp := &enginev1.ListFormatsResponse{Formats: make([]*enginev1.FormatInfo, 0, len(infos))}
	for _, info := range infos {
		resp.Formats = append(resp.Formats, &enginev1.FormatInfo{
			Name:        string(info.Name),
			DisplayName: info.DisplayName,
			Extensions:  info.Extensions,
			MimeTypes:   info.MimeTypes,
			Source:      info.Source,
			HasReader:   info.HasReader,
			HasWriter:   info.HasWriter,
		})
	}
	return resp, nil
}

// ListTools returns the registered processing tools.
func (s *Server) ListTools(context.Context, *enginev1.ListToolsRequest) (*enginev1.ListToolsResponse, error) {
	infos := s.ToolReg.ListWithSchemas()
	resp := &enginev1.ListToolsResponse{Tools: make([]*enginev1.ToolInfo, 0, len(infos))}
	for _, info := range infos {
		resp.Tools = append(resp.Tools, &enginev1.ToolInfo{
			Name:        string(info.Name),
			DisplayName: info.DisplayName,
			Description: info.Description,
			Category:    info.Category,
		})
	}
	return resp, nil
}

// ListFlows returns the built-in composed flows.
func (s *Server) ListFlows(context.Context, *enginev1.ListFlowsRequest) (*enginev1.ListFlowsResponse, error) {
	defs := flow.BuiltInFlows()
	resp := &enginev1.ListFlowsResponse{Flows: make([]*enginev1.FlowInfo, 0, len(defs))}
	for _, def := range defs {
		tools, err := def.ToolNodeNames()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "flow %q: %v", def.ID, err)
		}
		resp.Flows = append(resp.Flows, &enginev1.FlowInfo{
			Id:          def.ID,
			Name:        def.Name,
			Description: def.Description,
			Tools:       tools,
		})
	}
	return resp, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────

// gatherDocument resolves a ContentRef into (inline bytes, on-disk path). A
// nil ref falls back to fromChunks (the client streamed the bytes).
func gatherDocument(ref *contentv1.ContentRef, fromChunks func() ([]byte, error)) (content []byte, path string, err error) {
	if ref == nil {
		content, err = fromChunks()
		if err != nil {
			return nil, "", status.Errorf(codes.Internal, "receive chunks: %v", err)
		}
		return content, "", nil
	}
	switch loc := ref.GetLocation().(type) {
	case *contentv1.ContentRef_Inline:
		if int64(len(loc.Inline)) > safeio.DefaultMaxBytes {
			return nil, "", status.Errorf(codes.ResourceExhausted,
				"inline document exceeds the %d-byte budget", safeio.DefaultMaxBytes)
		}
		return loc.Inline, "", nil
	case *contentv1.ContentRef_Path:
		if !filepath.IsAbs(loc.Path) {
			return nil, "", status.Errorf(codes.InvalidArgument, "input path must be absolute: %s", loc.Path)
		}
		if _, err := os.Stat(loc.Path); err != nil {
			return nil, "", status.Errorf(codes.InvalidArgument, "input path: %v", err)
		}
		return nil, loc.Path, nil
	default:
		return nil, "", status.Error(codes.InvalidArgument, "unsupported input reference (use inline bytes or an absolute path)")
	}
}

// resolveFormat returns the explicit format id, or detects one from the
// document name (extension) and bytes.
func (s *Server) resolveFormat(explicit, name, srcPath string, content []byte) (registry.FormatID, error) {
	if explicit != "" {
		return registry.FormatID(explicit), nil
	}
	if name == "" && srcPath != "" {
		name = srcPath
	}
	if name == "" {
		return "", status.Error(codes.InvalidArgument, "format or name is required for detection")
	}
	id, err := s.detect(name, content)
	if err != nil {
		return "", status.Errorf(codes.InvalidArgument, "detect format of %q: %v", name, err)
	}
	return id, nil
}

// detect runs registry detection. Content-based detection needs a real file,
// so inline samples are staged into a temp file carrying the name's extension.
func (s *Server) detect(name string, sample []byte) (registry.FormatID, error) {
	path := name
	if len(sample) > 0 {
		tmp, err := os.CreateTemp("", "kapi-engine-detect-*"+filepath.Ext(name))
		if err != nil {
			return "", err
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.Write(sample); err != nil {
			tmp.Close()
			return "", err
		}
		tmp.Close()
		path = tmp.Name()
	} else if !filepath.IsAbs(path) {
		// A bare name with no sample: extension-only detection.
		return s.FormatReg.Detect(path, registry.DetectOptions{ExtensionOnly: true})
	}
	return s.FormatReg.Detect(path, registry.DetectOptions{})
}

// docURI picks the Part stream's document URI: the real path when we have
// one, else the client-supplied name.
func docURI(name, srcPath string) string {
	if srcPath != "" {
		return srcPath
	}
	return name
}

// applyConfigMap applies string-keyed config onto a format config, coercing
// JSON-ish scalars ("true", "3") so typed ApplyMap implementations accept
// them — the same convenience the CLI's flag parsing provides.
func applyConfigMap(cfg format.DataFormatConfig, values map[string]string) error {
	if len(values) == 0 || cfg == nil {
		return nil
	}
	coerced := make(map[string]any, len(values))
	for k, v := range values {
		coerced[k] = coerceScalar(v)
	}
	return cfg.ApplyMap(coerced)
}

func coerceScalar(v string) any {
	switch v {
	case "true":
		return true
	case "false":
		return false
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	return v
}
