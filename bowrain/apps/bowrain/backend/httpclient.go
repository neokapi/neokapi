package backend

import (
	"context"
	"net/http"
	"time"
)

// remoteRequestTimeout bounds one request to the connected server, body
// included.
const remoteRequestTimeout = 30 * time.Second

// remoteClient sends the app's requests to the connected server. Its timeout
// bounds every exchange, and each request carries the app's lifetime
// context, so shutting the app down cancels one still in flight.
type remoteClient struct {
	http *http.Client
}

func newRemoteClient(timeout time.Duration) *remoteClient {
	return &remoteClient{http: &http.Client{Timeout: timeout}}
}

// do sends req under ctx.
func (c *remoteClient) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	return c.http.Do(req.WithContext(ctx))
}

// startBackground runs fn on a goroutine ServiceShutdown waits for, under a
// context that the app's lifetime and cancel both end. It returns nil and
// starts nothing once the app is shutting down. The caller holds a.mu.
func (a *App) startBackground(fn func(ctx context.Context)) context.CancelFunc {
	parent := a.requestContext()
	if parent.Err() != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	a.background.Go(func() {
		defer cancel()
		fn(ctx)
	})
	return cancel
}

// requestContext is the context a request to the server runs under: the
// app's lifetime, which ServiceShutdown cancels.
func (a *App) requestContext() context.Context {
	if a.lifetime == nil {
		return context.Background()
	}
	return a.lifetime
}

// client returns the app's remote client.
func (a *App) client() *remoteClient {
	if a.remote == nil {
		return newRemoteClient(remoteRequestTimeout)
	}
	return a.remote
}
