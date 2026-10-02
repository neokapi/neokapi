package pluginhost

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type processTestServer struct {
	pb.UnimplementedBridgeServiceServer
	process func(pb.BridgeService_ProcessServer) error
}

func (s *processTestServer) Process(stream pb.BridgeService_ProcessServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	return s.process(stream)
}

func processTestPool(t *testing.T, process func(pb.BridgeService_ProcessServer) error) (*DaemonPool, *Plugin) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	pb.RegisterBridgeServiceServer(server, &processTestServer{process: process})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		<-done
	})
	conn, err := grpc.NewClient("passthrough:///plugin-test",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
	)
	require.NoError(t, err)
	plugin := &Plugin{Manifest: &manifest.Manifest{
		Plugin: "test-format",
		Capabilities: manifest.Capabilities{
			Formats: []manifest.Format{{Name: "test-format", Capabilities: []string{"read", "write"}}},
		},
	}}
	pool := NewDaemonPool(DaemonPoolOptions{})
	pool.clients[plugin.Name()] = &DaemonClient{Plugin: plugin, Conn: conn}
	t.Cleanup(pool.Shutdown)
	return pool, plugin
}

func TestDaemonReaderRequiresCompletion(t *testing.T) {
	for _, readDone := range []bool{false, true} {
		name := "empty stream"
		if readDone {
			name = "read done without completion"
		}
		t.Run(name, func(t *testing.T) {
			pool, plugin := processTestPool(t, func(stream pb.BridgeService_ProcessServer) error {
				if readDone {
					return stream.Send(&pb.ProcessResponse{
						Response: &pb.ProcessResponse_ReadDone{ReadDone: &pb.ProcessReadDone{}},
					})
				}
				return nil
			})
			reader := newDaemonReader(
				pool,
				plugin,
				"test-format",
				format.FormatSignature{},
				"test",
			)
			require.NoError(t, reader.Open(t.Context(), &model.RawDocument{}))
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			results := []model.PartResult{}
			for result := range reader.Read(ctx) {
				results = append(results, result)
			}
			require.Len(t, results, 1)
			require.ErrorIs(t, results[0].Error, io.ErrUnexpectedEOF)
		})
	}
}

func TestDaemonWriterRejectsEarlyTermination(t *testing.T) {
	pool, plugin := processTestPool(t, func(pb.BridgeService_ProcessServer) error {
		return nil
	})
	writer := newDaemonWriter(pool, plugin, "test-format")
	writer.SetOriginalContent([]byte("original"))
	parts := make(chan *model.Part)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- writer.Write(ctx, parts) }()
	select {
	case err := <-done:
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("writer waited for input after the daemon ended its response without completion")
	}
}

// okapi-bridge sends ProcessComplete once its pipeline has written the
// document, which can be before the host closes its input. The writer accepts
// that completion, waits for the input to finish, and returns the output.
func TestDaemonWriterAcceptsCompletionBeforeInputCloses(t *testing.T) {
	completed := make(chan struct{})
	pool, plugin := processTestPool(t, func(stream pb.BridgeService_ProcessServer) error {
		if _, err := stream.Recv(); err != nil {
			return err
		}
		defer close(completed)
		return stream.Send(&pb.ProcessResponse{
			Response: &pb.ProcessResponse_Complete{Complete: &pb.ProcessComplete{Output: []byte("written")}},
		})
	})
	writer := newDaemonWriter(pool, plugin, "test-format")
	writer.SetOriginalContent([]byte("original"))
	var out bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&out))
	parts := make(chan *model.Part)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- writer.Write(ctx, parts) }()

	parts <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("b1", "text")}
	<-completed
	// Let the completion reach the writer while its input is still open.
	time.Sleep(50 * time.Millisecond)
	close(parts)

	require.NoError(t, <-done)
	require.Equal(t, "written", out.String())
}

// After completion the wait for the input is bounded by the caller's context.
func TestDaemonWriterCompletionWaitObservesCancellation(t *testing.T) {
	completed := make(chan struct{})
	pool, plugin := processTestPool(t, func(stream pb.BridgeService_ProcessServer) error {
		defer close(completed)
		return stream.Send(&pb.ProcessResponse{
			Response: &pb.ProcessResponse_Complete{Complete: &pb.ProcessComplete{}},
		})
	})
	writer := newDaemonWriter(pool, plugin, "test-format")
	writer.SetOriginalContent([]byte("original"))
	parts := make(chan *model.Part)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- writer.Write(ctx, parts) }()

	<-completed
	time.Sleep(50 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("writer returned before its input finished: %v", err)
	default:
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("writer ignored cancellation while waiting for its input")
	}
}

func TestDaemonReaderReleasesRPCOnCompletion(t *testing.T) {
	cancelled := make(chan struct{})
	pool, plugin := processTestPool(t, func(stream pb.BridgeService_ProcessServer) error {
		if err := stream.Send(&pb.ProcessResponse{
			Response: &pb.ProcessResponse_Complete{Complete: &pb.ProcessComplete{}},
		}); err != nil {
			return err
		}
		<-stream.Context().Done()
		close(cancelled)
		return nil
	})
	reader := newDaemonReader(
		pool,
		plugin,
		"test-format",
		format.FormatSignature{},
		"test",
	)
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{}))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	for result := range reader.Read(ctx) {
		require.NoError(t, result.Error)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("reader returned without releasing its RPC")
	}
}

func TestDaemonWriterRequiresCompletionWithClosedInput(t *testing.T) {
	pool, plugin := processTestPool(t, func(stream pb.BridgeService_ProcessServer) error {
		for {
			_, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
		}
	})
	writer := newDaemonWriter(pool, plugin, "test-format")
	writer.SetOriginalContent([]byte("original"))
	parts := make(chan *model.Part)
	close(parts)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.ErrorIs(t, writer.Write(ctx, parts), io.ErrUnexpectedEOF)
}

func TestDaemonReaderCancellationWithFullOutput(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, baseline) })
	for _, inBand := range []bool{false, true} {
		name := "transport error"
		if inBand {
			name = "completion error"
		}
		t.Run(name, func(t *testing.T) {
			pool, plugin := processTestPool(t, func(stream pb.BridgeService_ProcessServer) error {
				for range 64 {
					if err := stream.Send(&pb.ProcessResponse{
						Response: &pb.ProcessResponse_Part{Part: &pb.PartMessage{}},
					}); err != nil {
						return err
					}
				}
				if inBand {
					return stream.Send(&pb.ProcessResponse{
						Response: &pb.ProcessResponse_Complete{Complete: &pb.ProcessComplete{Error: "failed"}},
					})
				}
				return status.Error(codes.Internal, "failed")
			})
			reader := newDaemonReader(
				pool,
				plugin,
				"test-format",
				format.FormatSignature{},
				"test",
			)
			require.NoError(t, reader.Open(t.Context(), &model.RawDocument{}))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			results := reader.Read(ctx)
			require.Eventually(t, func() bool { return len(results) == cap(results) }, time.Second, time.Millisecond)
			// Stop consuming at the capacity boundary, as an executor does after
			// a downstream failure. Error delivery must also obey cancellation.
			cancel()
		})
	}
}
