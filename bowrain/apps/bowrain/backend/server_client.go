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
//
// The request/response editor surface (workspaces, projects, blocks, TM,
// terminology, providers) now travels over REST via bowrain/core/client
// (see remote_convert.go and the editorRemote() helper). What remains here is
// the part that has no clean REST home yet: the real-time WatchProject stream
// (watcher.go), presence reporting, block review status, and the current-user
// lookup used to enrich a presence session. Store demotion and moving presence
// to SSE are tracked as bowrain-desktop realignment follow-ups (plan 07,
// phases 4-5).
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
	return c.authCtx(context.Background())
}

// authCtx attaches the JWT token to the given context as gRPC metadata.
func (c *ServerClient) authCtx(ctx context.Context) context.Context {
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

// GetCurrentUser returns the authenticated user's info. Used by GetCollabSession
// to enrich the presence session with the server's authoritative user record
// (avatar URL + stable ID), which the REST /auth/me shape does not carry.
func (c *ServerClient) GetCurrentUser() (*pb.UserResponse, error) {
	ctx, cancel := c.ctxWithTimeout(10 * time.Second)
	defer cancel()
	return c.editor.GetCurrentUser(ctx, &pb.GetCurrentUserRequest{})
}

// ReviewBlock sets or clears the reviewed status on a block. The review-status
// workflow has no direct REST counterpart (SetBlockStatus is a distinct
// PostgreSQL-only draft/in_review/published lifecycle), so it stays on gRPC.
// It accepts a context so the offline-replay path can thread its cancellation.
func (c *ServerClient) ReviewBlock(ctx context.Context, wsSlug, projectID, itemName, blockID, targetLocale string, reviewed bool) error {
	ctx, cancel := context.WithTimeout(c.authCtx(ctx), 10*time.Second)
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

// UpdatePresence reports the user's current focus over the presence stream.
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
