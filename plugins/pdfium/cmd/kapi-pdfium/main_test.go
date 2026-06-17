package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
)

// startServer spins up the BridgeService over a Unix socket (the same setup as
// serve(), minus the stdio handshake) and returns a connected client.
func startServer(t *testing.T) pb.BridgeServiceClient {
	t.Helper()
	dir := t.TempDir()
	sock := filepath.Join(dir, "s.sock")
	lis, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := grpc.NewServer()
	pb.RegisterBridgeServiceServer(srv, &server{stop: srv.GracefulStop})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("unix:"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewBridgeServiceClient(conn)
}

// readDoc drives a read-only Process call with inline PDF bytes and returns the
// extracted blocks.
func readDoc(t *testing.T, client pb.BridgeServiceClient, path string, geometry bool) []*model.Block {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stream, err := client.Process(ctx)
	require.NoError(t, err)

	header := &pb.ProcessHeader{
		FilterClass:  "pdf",
		SourceLocale: "en",
		Input:        &pb.ContentRef{Location: &pb.ContentRef_Inline{Inline: data}},
	}
	if geometry {
		header.FilterParams = map[string]string{"geometry": "true"}
	}
	require.NoError(t, stream.Send(&pb.ProcessRequest{Request: &pb.ProcessRequest_Header{Header: header}}))
	require.NoError(t, stream.CloseSend())

	var blocks []*model.Block
	for {
		resp, err := stream.Recv()
		require.NoError(t, err)
		if c := resp.GetComplete(); c != nil {
			require.Empty(t, c.Error, "daemon reported error")
			break
		}
		if pm := resp.GetPart(); pm != nil {
			p := protoconvert.ProtoToPart(pm)
			if p.Type == model.PartBlock {
				if b, ok := p.Resource.(*model.Block); ok {
					blocks = append(blocks, b)
				}
			}
		}
	}
	return blocks
}

// End-to-end over real gRPC: the CID/Type0 Japanese PDF extracts correctly —
// the win over the hand-rolled in-core reader, now isolated in the plugin.
func TestProcess_CIDFontCJK(t *testing.T) {
	client := startServer(t)
	blocks := readDoc(t, client, "testdata/cjk.pdf", false)
	require.NotEmpty(t, blocks)
	var joined string
	for _, b := range blocks {
		joined += b.SourceText()
	}
	for _, want := range []string{"こんにちは世界", "日本語", "Hello"} {
		require.Contains(t, joined, want, "CID/CJK extraction over the plugin")
	}
}

// Fast path (default) emits one block per page; geometry mode emits positioned
// blocks with a GeometryAnnotation.
func TestProcess_Modes(t *testing.T) {
	client := startServer(t)

	textBlocks := readDoc(t, client, "testdata/hello.pdf", false)
	require.Len(t, textBlocks, 1, "fast path: one block per page")
	require.Contains(t, textBlocks[0].SourceText(), "Hello World")
	_, hasGeo := textBlocks[0].Geometry()
	require.False(t, hasGeo, "fast path carries no geometry")

	geoBlocks := readDoc(t, client, "testdata/hello.pdf", true)
	require.NotEmpty(t, geoBlocks)
	var withGeo int
	for _, b := range geoBlocks {
		if g, ok := b.Geometry(); ok && g.BBox.W > 0 {
			withGeo++
		}
	}
	require.Positive(t, withGeo, "geometry mode carries positioned blocks")
	require.Contains(t, strings.Join(blockTexts(geoBlocks), " "), "Hello World")
}

func blockTexts(bs []*model.Block) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.SourceText()
	}
	return out
}
