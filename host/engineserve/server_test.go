package engineserve

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"

	neokapiconfig "github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	contentv1 "github.com/neokapi/neokapi/core/proto/content/v1"
	enginev1 "github.com/neokapi/neokapi/core/proto/engine/v1"
	"github.com/neokapi/neokapi/core/registry"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const fixtureJSON = `{
  "greeting": "Hello, world",
  "farewell": "Goodbye"
}
`

// newTestClient serves the engine on a bufconn and returns a connected
// client (the standard golang-grpc in-memory test idiom).
func newTestClient(t *testing.T) enginev1.EngineServiceClient {
	t.Helper()

	formatReg := registry.NewFormatRegistry()
	schemaReg := schema.NewSchemaRegistry()
	formats.RegisterAll(formatReg, formats.RegisterOptions{
		SchemaReg: schemaReg,
		ConfigReg: neokapiconfig.DefaultRegistry,
	})
	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)

	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	enginev1.RegisterEngineServiceServer(gs, New(formatReg, toolReg))
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return enginev1.NewEngineServiceClient(conn)
}

// extractParts drives one Extract call over chunked bytes and collects the
// returned parts.
func extractParts(t *testing.T, client enginev1.EngineServiceClient, name, content string) []*contentv1.PartMessage {
	t.Helper()
	stream, err := client.Extract(t.Context())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&enginev1.ExtractRequest{Request: &enginev1.ExtractRequest_Header{
		Header: &enginev1.ExtractHeader{Name: name, SourceLocale: "en"},
	}}))
	// Deliberately split into small chunks to exercise reassembly.
	for i := 0; i < len(content); i += 16 {
		end := min(i+16, len(content))
		require.NoError(t, stream.Send(&enginev1.ExtractRequest{Request: &enginev1.ExtractRequest_Chunk{
			Chunk: &enginev1.DocumentChunk{Data: []byte(content[i:end])},
		}}))
	}
	require.NoError(t, stream.CloseSend())

	var parts []*contentv1.PartMessage
	var complete *enginev1.ExtractComplete
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		switch m := resp.GetResponse().(type) {
		case *enginev1.ExtractResponse_Parts:
			parts = append(parts, m.Parts.GetParts()...)
		case *enginev1.ExtractResponse_Complete:
			complete = m.Complete
		}
	}
	require.NotNil(t, complete, "extract must end with a Complete message")
	assert.Len(t, parts, int(complete.GetParts()))
	assert.NotEmpty(t, complete.GetFormat())
	return parts
}

// processParts drives one Process call with the given header over parts.
func processParts(t *testing.T, client enginev1.EngineServiceClient, header *enginev1.ProcessHeader, parts []*contentv1.PartMessage) []*contentv1.PartMessage {
	t.Helper()
	stream, err := client.Process(t.Context())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&enginev1.ProcessRequest{Request: &enginev1.ProcessRequest_Header{Header: header}}))
	require.NoError(t, stream.Send(&enginev1.ProcessRequest{Request: &enginev1.ProcessRequest_Parts{
		Parts: &enginev1.PartBatch{Parts: parts},
	}}))
	require.NoError(t, stream.CloseSend())

	var out []*contentv1.PartMessage
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if m, ok := resp.GetResponse().(*enginev1.ProcessResponse_Parts); ok {
			out = append(out, m.Parts.GetParts()...)
		}
	}
	return out
}

// mergeParts drives one Merge call and returns the output document bytes.
func mergeParts(t *testing.T, client enginev1.EngineServiceClient, header *enginev1.MergeHeader, parts []*contentv1.PartMessage) []byte {
	t.Helper()
	stream, err := client.Merge(t.Context())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&enginev1.MergeRequest{Request: &enginev1.MergeRequest_Header{Header: header}}))
	require.NoError(t, stream.Send(&enginev1.MergeRequest{Request: &enginev1.MergeRequest_Parts{
		Parts: &enginev1.PartBatch{Parts: parts},
	}}))
	require.NoError(t, stream.CloseSend())

	var out []byte
	var complete *enginev1.MergeComplete
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		switch m := resp.GetResponse().(type) {
		case *enginev1.MergeResponse_Chunk:
			out = append(out, m.Chunk.GetData()...)
		case *enginev1.MergeResponse_Complete:
			complete = m.Complete
		}
	}
	require.NotNil(t, complete, "merge must end with a Complete message")
	assert.Equal(t, int64(len(out)), complete.GetBytes())
	return out
}

func TestExtractMergeRoundTripByteExact(t *testing.T) {
	client := newTestClient(t)

	parts := extractParts(t, client, "messages.json", fixtureJSON)
	require.NotEmpty(t, parts)

	out := mergeParts(t, client, &enginev1.MergeHeader{
		Name:     "messages.json",
		Original: &contentv1.ContentRef{Location: &contentv1.ContentRef_Inline{Inline: []byte(fixtureJSON)}},
	}, parts)
	assert.Equal(t, fixtureJSON, string(out), "identity extract→merge must round-trip byte-exact")
}

func TestExtractProcessMergePseudoTranslate(t *testing.T) {
	client := newTestClient(t)

	parts := extractParts(t, client, "messages.json", fixtureJSON)
	processed := processParts(t, client, &enginev1.ProcessHeader{
		Tools:        []*enginev1.ToolSpec{{Tool: "pseudo-translate"}},
		SourceLocale: "en",
		TargetLocale: "qps",
	}, parts)
	require.NotEmpty(t, processed)

	out := mergeParts(t, client, &enginev1.MergeHeader{
		Name:         "messages.json",
		Original:     &contentv1.ContentRef{Location: &contentv1.ContentRef_Inline{Inline: []byte(fixtureJSON)}},
		TargetLocale: "qps",
	}, processed)
	assert.NotEqual(t, fixtureJSON, string(out), "pseudo-translated output must differ from the source")
	assert.NotContains(t, string(out), `"Hello, world"`, "source text must be replaced by the pseudo target")
	// Structure survives: still a JSON catalog with the same keys.
	assert.Contains(t, string(out), `"greeting"`)
	assert.Contains(t, string(out), `"farewell"`)
}

func TestProcessNamedFlow(t *testing.T) {
	client := newTestClient(t)

	parts := extractParts(t, client, "messages.json", fixtureJSON)
	processed := processParts(t, client, &enginev1.ProcessHeader{
		Flow:         "pseudo-translate",
		SourceLocale: "en",
		TargetLocale: "qps",
	}, parts)
	require.NotEmpty(t, processed)
}

func TestProcessErrors(t *testing.T) {
	client := newTestClient(t)

	t.Run("unknown tool", func(t *testing.T) {
		stream, err := client.Process(t.Context())
		require.NoError(t, err)
		require.NoError(t, stream.Send(&enginev1.ProcessRequest{Request: &enginev1.ProcessRequest_Header{
			Header: &enginev1.ProcessHeader{Tools: []*enginev1.ToolSpec{{Tool: "no-such-tool"}}},
		}}))
		_, err = stream.Recv()
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Contains(t, err.Error(), "no-such-tool")
	})

	t.Run("unknown flow", func(t *testing.T) {
		stream, err := client.Process(t.Context())
		require.NoError(t, err)
		require.NoError(t, stream.Send(&enginev1.ProcessRequest{Request: &enginev1.ProcessRequest_Header{
			Header: &enginev1.ProcessHeader{Flow: "no-such-flow"},
		}}))
		_, err = stream.Recv()
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("missing header", func(t *testing.T) {
		stream, err := client.Process(t.Context())
		require.NoError(t, err)
		require.NoError(t, stream.Send(&enginev1.ProcessRequest{Request: &enginev1.ProcessRequest_Parts{
			Parts: &enginev1.PartBatch{},
		}}))
		_, err = stream.Recv()
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("flow and tools are mutually exclusive", func(t *testing.T) {
		stream, err := client.Process(t.Context())
		require.NoError(t, err)
		require.NoError(t, stream.Send(&enginev1.ProcessRequest{Request: &enginev1.ProcessRequest_Header{
			Header: &enginev1.ProcessHeader{
				Flow:  "pseudo-translate",
				Tools: []*enginev1.ToolSpec{{Tool: "pseudo-translate"}},
			},
		}}))
		_, err = stream.Recv()
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

// TestProcessRefusesExecClassTools: a tool name and its config reach this
// server over the wire, so an exec-class tool would let the caller choose the
// argv the server runs. There is no user at a terminal to ask, so the answer is
// fixed — the same conclusion MCP reached for its agent surface (AD-037).
// The refusal is PermissionDenied, not InvalidArgument: the tool exists and the
// request is well-formed, it is simply not on offer here.
func TestProcessRefusesExecClassTools(t *testing.T) {
	client := newTestClient(t)

	for _, toolName := range []string{"external-command", "script"} {
		t.Run(toolName, func(t *testing.T) {
			stream, err := client.Process(t.Context())
			require.NoError(t, err)
			require.NoError(t, stream.Send(&enginev1.ProcessRequest{Request: &enginev1.ProcessRequest_Header{
				Header: &enginev1.ProcessHeader{
					TargetLocale: "nb",
					Tools: []*enginev1.ToolSpec{{
						Tool:   toolName,
						Config: []byte(`{"command":"/bin/sh","args":["-c","id"]}`),
					}},
				},
			}}))
			_, err = stream.Recv()
			require.Error(t, err)
			assert.Equal(t, codes.PermissionDenied, status.Code(err))
			assert.Contains(t, err.Error(), toolName)
		})
	}
}

func TestExtractErrors(t *testing.T) {
	client := newTestClient(t)

	t.Run("unknown format", func(t *testing.T) {
		stream, err := client.Extract(t.Context())
		require.NoError(t, err)
		require.NoError(t, stream.Send(&enginev1.ExtractRequest{Request: &enginev1.ExtractRequest_Header{
			Header: &enginev1.ExtractHeader{Format: "no-such-format", Name: "x.bin"},
		}}))
		require.NoError(t, stream.CloseSend())
		_, err = stream.Recv()
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("missing header", func(t *testing.T) {
		stream, err := client.Extract(t.Context())
		require.NoError(t, err)
		require.NoError(t, stream.Send(&enginev1.ExtractRequest{Request: &enginev1.ExtractRequest_Chunk{
			Chunk: &enginev1.DocumentChunk{Data: []byte("{}")},
		}}))
		_, err = stream.Recv()
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

func TestDetect(t *testing.T) {
	client := newTestClient(t)

	resp, err := client.Detect(t.Context(), &enginev1.DetectRequest{
		Name:   "messages.json",
		Sample: []byte(fixtureJSON),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetFormat())

	_, err = client.Detect(t.Context(), &enginev1.DetectRequest{})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestListings(t *testing.T) {
	client := newTestClient(t)

	formatsResp, err := client.ListFormats(t.Context(), &enginev1.ListFormatsRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, formatsResp.GetFormats())
	names := make([]string, 0, len(formatsResp.GetFormats()))
	for _, f := range formatsResp.GetFormats() {
		names = append(names, f.GetName())
	}
	assert.Contains(t, strings.Join(names, ","), "json")

	toolsResp, err := client.ListTools(t.Context(), &enginev1.ListToolsRequest{})
	require.NoError(t, err)
	toolNames := make([]string, 0, len(toolsResp.GetTools()))
	for _, ti := range toolsResp.GetTools() {
		toolNames = append(toolNames, ti.GetName())
	}
	assert.Contains(t, toolNames, "pseudo-translate")

	flowsResp, err := client.ListFlows(t.Context(), &enginev1.ListFlowsRequest{})
	require.NoError(t, err)
	flowIDs := make([]string, 0, len(flowsResp.GetFlows()))
	for _, fi := range flowsResp.GetFlows() {
		flowIDs = append(flowIDs, fi.GetId())
	}
	assert.Contains(t, flowIDs, "pseudo-translate")
}
