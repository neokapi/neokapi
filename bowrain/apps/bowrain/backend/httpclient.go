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

// startBackground runs fn, named name, on a goroutine ServiceShutdown waits
// for, under a context that the app's lifetime and cancel both end. It
// returns nil and starts nothing once shutdown has begun.
func (a *App) startBackground(name string, fn func(ctx context.Context)) context.CancelFunc {
	a.bgMu.Lock()
	defer a.bgMu.Unlock()
	parent := a.requestContext()
	if a.bgClosed || parent.Err() != nil {
		return nil
	}
	if a.bgRunning == nil {
		a.bgRunning = map[string]int{}
	}
	a.bgRunning[name]++
	ctx, cancel := context.WithCancel(parent)
	a.background.Go(func() {
		defer func() {
			a.bgMu.Lock()
			if a.bgRunning[name]--; a.bgRunning[name] <= 0 {
				delete(a.bgRunning, name)
			}
			a.bgMu.Unlock()
		}()
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
