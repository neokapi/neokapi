// Package contextremote keeps a project's shared context on the Bowrain
// project the recipe's `bowrain:` block connects to.
//
// Each object of the context layout (core/workspace) is one context object of
// the project, read and written through the server's context object routes.
// The server creates an object once and refuses other bytes under a name it
// holds, which is all the layout asks of a store. The sign-in is the one
// `kapi auth login` keeps in the OS keychain, or BOWRAIN_AUTH_TOKEN in CI.
//
// The package links no host runtime, so the server's tests drive the remote
// against the real routes without taking on the host's dependencies.
// host/venue/contextbackend registers it as the "bowrain" backend.
package contextremote

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
)

// projectURLEnvVar redirects a connected recipe at another project without
// editing it, as it does for `kapi push` (host/venue/schema.ProjectURLEnvVar).
// The schema package registers the platform's recipe keys on import, which a
// binary linking this package must not take on, so the name is spelt here and
// a test holds the two equal.
const projectURLEnvVar = "BOWRAIN_PROJECT_URL"

// maxBatchBytes bounds the object bytes of one push request. The server reads
// a request body of at most 50 MB, and base64 grows the bytes by a third.
const maxBatchBytes = 24 << 20

// Options place the remote.
type Options struct {
	// URL is the recipe's compound project URL,
	// <server>/<workspace>/<project>.
	URL string
	// Token, when set, is the bearer token every request carries, in place of
	// the stored sign-in. Tests set it; a recipe never does.
	Token string
}

// Remote is a project's context objects on a Bowrain server.
type Remote struct {
	server, workspace, project string
	token                      string

	once      sync.Once
	client    *apiclient.BowrainClient
	clientErr error
}

// New places a remote on the project the URL names. It reaches nothing: the
// sign-in is read at the first request, so reading a project's sync state
// opens no keychain.
func New(o Options) (*Remote, error) {
	raw := o.URL
	if v := strings.TrimSpace(os.Getenv(projectURLEnvVar)); v != "" {
		raw = v
	}
	server, ws, pid, err := parseProjectURL(raw)
	if err != nil {
		return nil, err
	}
	return &Remote{server: server, workspace: ws, project: pid, token: o.Token}, nil
}

// parseProjectURL splits <server>/<workspace>/<project>. A project with no
// workspace has no context objects: the routes are workspace-scoped.
func parseProjectURL(raw string) (server, ws, pid string, err error) {
	u, perr := url.Parse(strings.TrimSpace(raw))
	if perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", "", fmt.Errorf("bowrain: %q is not a project URL (expected <server>/<workspace>/<project>)", raw)
	}
	// Every request carries a bearer token, so it travels over TLS unless it
	// never leaves this machine.
	if u.Scheme == "http" && !loopback(u.Hostname()) {
		return "", "", "", fmt.Errorf("bowrain: %s sends the sign-in unencrypted: give bowrain.url an https:// address (http:// is taken for localhost only)", raw)
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) != 2 || segments[0] == "" || segments[1] == "" {
		return "", "", "", fmt.Errorf("bowrain: %q does not name a project (expected <server>/<workspace>/<project>)", raw)
	}
	if segments[0] == "projects" {
		return "", "", "", fmt.Errorf("bowrain: %s is not in a workspace, and a project shares its context on Bowrain once it is: claim it into one with `kapi auth claim`", raw)
	}
	return config.NormalizeServerURL(u.Scheme + "://" + u.Host), segments[0], segments[1], nil
}

// loopback reports whether host names this machine.
func loopback(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// bowrainClient builds the client on first use, with the stored sign-in.
func (r *Remote) bowrainClient() (*apiclient.BowrainClient, error) {
	r.once.Do(func() {
		if r.token != "" {
			r.client = apiclient.NewWorkspaceBowrainClient(r.server, r.workspace, r.project, r.token)
			return
		}
		auth, err := config.LoadAuth()
		if err != nil {
			r.clientErr = fmt.Errorf("%w: bowrain: no sign-in for %s: run `kapi auth login` (or set BOWRAIN_AUTH_TOKEN in CI)", workspace.ErrRemoteUnreachable, r.server)
			return
		}
		// The recipe names the server, and anyone can write a recipe. The
		// stored tokens go only to the server the sign-in itself names; a
		// sign-in that names none is refused rather than trusted.
		if authServer := config.NormalizeServerURL(auth.ServerURL); authServer == "" || authServer != r.server {
			r.clientErr = fmt.Errorf("bowrain: the stored sign-in does not name %s: run `kapi auth login` against it (in CI, set BOWRAIN_SERVER_URL beside BOWRAIN_AUTH_TOKEN)", r.server)
			return
		}
		c := apiclient.NewWorkspaceBowrainClient(r.server, r.workspace, r.project, auth.AccessToken)
		if auth.RefreshToken != "" {
			c.SetRefreshToken(auth.RefreshToken, func(newAccess, newRefresh string) {
				auth.AccessToken, auth.RefreshToken = newAccess, newRefresh
				_ = config.SaveAuth(*auth)
			})
		}
		r.client = c
	})
	return r.client, r.clientErr
}

// Describe reports the project.
func (r *Remote) Describe() workspace.RemoteDescriptor {
	return workspace.RemoteDescriptor{Kind: project.ContextBackendBowrain, Location: r.server + "/" + r.workspace + "/" + r.project}
}

// List reads the names under one directory of the layout.
func (r *Remote) List(ctx context.Context, dir string) ([]string, error) {
	switch dir {
	case workspace.RemoteLogDir, workspace.RemoteBlobDir, workspace.RemoteCheckpointsDir:
	default:
		return nil, fmt.Errorf("bowrain: %q is not a directory of the context layout", dir)
	}
	c, err := r.bowrainClient()
	if err != nil {
		return nil, err
	}
	names, err := c.ListContextObjects(ctx, dir)
	if err != nil {
		return nil, r.refusal("list", dir, err)
	}
	out := names[:0]
	for _, name := range names {
		if workspace.ValidObjectName(name) && strings.HasPrefix(name, dir) {
			out = append(out, name)
		}
	}
	return out, nil
}

// Get reads one object, refusing one larger than a blob may be.
func (r *Remote) Get(ctx context.Context, name string) ([]byte, error) {
	if !workspace.ValidObjectName(name) {
		return nil, fmt.Errorf("bowrain: %q is not a name in the context layout", name)
	}
	c, err := r.bowrainClient()
	if err != nil {
		return nil, err
	}
	data, err := c.GetContextObject(ctx, name, workspace.MaxBlobSize)
	if err != nil {
		return nil, r.refusal("read", name, err)
	}
	if len(data) > workspace.MaxBlobSize {
		return nil, fmt.Errorf("bowrain: %s is larger than any object of the context layout", name)
	}
	return data, nil
}

// Put writes the objects in batches the server takes in one request each.
// The server writes a batch in one transaction and refuses all of it when it
// holds one of its names with other bytes.
func (r *Remote) Put(ctx context.Context, objs ...workspace.Object) error {
	for _, obj := range objs {
		if !workspace.ValidObjectName(obj.Name) {
			return fmt.Errorf("bowrain: %q is not a name in the context layout", obj.Name)
		}
	}
	if len(objs) == 0 {
		return nil
	}
	c, err := r.bowrainClient()
	if err != nil {
		return err
	}
	var batch []apiclient.ContextObjectWire
	size := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := c.PutContextObjects(ctx, batch); err != nil {
			return r.refusal("write", batch[0].Name, err)
		}
		batch, size = nil, 0
		return nil
	}
	for _, obj := range objs {
		if size > 0 && size+len(obj.Data) > maxBatchBytes {
			if err := flush(); err != nil {
				return err
			}
		}
		batch = append(batch, apiclient.ContextObjectWire{Name: obj.Name, Data: obj.Data})
		size += len(obj.Data)
	}
	return flush()
}

// Close holds nothing open.
func (r *Remote) Close() error { return nil }

// refusal maps what a route answered to the workspace error of the same
// meaning: a missing object, an object held with other bytes, or a server
// that could not be reached or would not take the sign-in.
func (r *Remote) refusal(verb, name string, err error) error {
	var e *apiclient.ContextObjectError
	if !errors.As(err, &e) {
		return fmt.Errorf("%w: bowrain %s %s: %w", workspace.ErrRemoteUnreachable, verb, name, err)
	}
	switch {
	case e.StatusCode == http.StatusNotFound && e.Code == apiclient.ContextObjectMissingCode:
		return fmt.Errorf("%w: %s", workspace.ErrNoObject, name)
	case e.StatusCode == http.StatusConflict && e.Code == apiclient.ContextObjectExistsCode:
		// The details carry the bare name of the refused object.
		return fmt.Errorf("%w: %s", workspace.ErrObjectExists, e.Details)
	case e.StatusCode == http.StatusNotFound && e.Code == apiclient.ContextObjectNoProjectCode:
		return fmt.Errorf("bowrain %s %s: %s holds no project %s/%s; check the url in the recipe's bowrain: block",
			verb, name, r.server, r.workspace, r.project)
	case e.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w: bowrain %s %s: the server did not accept the sign-in: run `kapi auth login`", workspace.ErrRemoteUnreachable, verb, name)
	case e.StatusCode == http.StatusForbidden:
		return fmt.Errorf("bowrain %s %s: the signed-in member may not %s this project's context: %w", verb, name, verb, err)
	case e.StatusCode >= 500 || e.StatusCode == http.StatusTooManyRequests || e.StatusCode == http.StatusRequestTimeout:
		return fmt.Errorf("%w: bowrain %s %s: %w", workspace.ErrRemoteUnreachable, verb, name, err)
	}
	return fmt.Errorf("bowrain %s %s: %w", verb, name, err)
}
