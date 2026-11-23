package host

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/core/rootpath"
	"github.com/neokapi/neokapi/host/pluginhost"
)

// The directories an MCP call may reach.
//
// A tool argument is written by an agent, which may be acting on text it read
// in a repository or a document; it is not a person's choice. A call that
// names a project, a file to read, or a destination therefore reaches only
// directories a person chose: the directory the server was started in, and
// the workspace roots the client declares. The project the server resolved at
// start stays the default for a call that names none.

type mcpRootsKey struct{}

// mcpListRootsTimeout bounds how long a call waits for a client to list its
// roots.
const mcpListRootsTimeout = 5 * time.Second

// WithMCPRoots returns ctx carrying the directories an MCP call may reach.
// An empty list means there are none.
func WithMCPRoots(ctx context.Context, roots []string) context.Context {
	if roots == nil {
		roots = []string{}
	}
	return context.WithValue(ctx, mcpRootsKey{}, roots)
}

// MCPRootsFrom returns the directories ctx carries. A context that carries
// none, a handler called directly rather than through the server, falls back
// to the working directory. Inside a `go test` binary the system temporary
// directory is added, where every test keeps its fixtures (as DataDir does);
// a test of a refusal names its roots with WithMCPRoots.
func MCPRootsFrom(ctx context.Context) []string {
	if roots, ok := ctx.Value(mcpRootsKey{}).([]string); ok {
		return roots
	}
	if !rootpath.Supported {
		// The browser build's file system holds the page's own files and
		// nothing else, and the page, not an agent, names what it reads.
		return []string{"/"}
	}
	roots := []string{}
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, wd)
	}
	if underTest() {
		roots = append(roots, os.TempDir())
	}
	return roots
}

// MCPRootsSet returns the directories ctx carries and whether it carries
// any list at all.
func MCPRootsSet(ctx context.Context) ([]string, bool) {
	roots, ok := ctx.Value(mcpRootsKey{}).([]string)
	return roots, ok
}

// MCPRootsMiddleware puts on every call's context the directory the server
// was started in, the root of the project it was started for, and the roots
// the client declares, both as the directories the call may reach and as the
// roots a plugin daemon may be sent paths in (pluginhost.WithPathRoots).
func (a *App) MCPRootsMiddleware() mcp.Middleware {
	var (
		mu      sync.Mutex
		cached  = map[*mcp.ServerSession][]string{}
		schemas mcpToolSchemas
	)
	sessionRoots := func(ctx context.Context, ss *mcp.ServerSession) []string {
		mu.Lock()
		roots, ok := cached[ss]
		mu.Unlock()
		if ok {
			return roots
		}
		roots = MCPSessionRoots(ctx, ss, a.mcpStartDir)
		if a.mcpRecipePath != "" {
			// The project the server was started for, which a person named
			// with -p or by where they started it.
			roots = append(roots, filepath.Dir(a.mcpRecipePath))
		}
		mu.Lock()
		cached[ss] = roots
		mu.Unlock()
		return roots
	}
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			var ss *mcp.ServerSession
			if req != nil {
				ss, _ = req.GetSession().(*mcp.ServerSession)
			}
			switch method {
			case "notifications/roots/list_changed":
				// The client's roots changed: the next call asks again.
				mu.Lock()
				delete(cached, ss)
				mu.Unlock()
				return next(ctx, method, req)
			case "tools/call", "resources/read":
			default:
				// Only a call or a read names a location; nothing else waits
				// on the client's roots.
				return next(ctx, method, req)
			}
			roots := sessionRoots(ctx, ss)
			ctx = WithMCPRoots(ctx, roots)
			// The same directories are the ones a plugin daemon may be sent
			// paths in while the call runs.
			ctx = pluginhost.WithPathRoots(ctx, roots)
			switch r := req.(type) {
			case *mcp.CallToolRequest:
				schemas.load(ctx, next, ss)
				if schemas.err != nil {
					return nil, schemas.err
				}
				props, known := schemas.props[r.Params.Name]
				if known {
					if err := checkMCPCallArgs(ctx, r.Params.Name, props, r.Params.Arguments); err != nil {
						// A refused argument is the call's result, as a tool's own
						// error is, so the agent reads why.
						return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
					}
				}
			case *mcp.ReadResourceRequest:
				if err := checkMCPResourceURI(ctx, r.Params.URI); err != nil {
					return nil, err
				}
			}
			return next(ctx, method, req)
		}
	}
}

// MCPSessionRoots is startDir, when known, followed by every local root the
// client of ss declares.
func MCPSessionRoots(ctx context.Context, ss *mcp.ServerSession, startDir string) []string {
	roots := []string{}
	if startDir != "" {
		roots = append(roots, startDir)
	}
	// Only a client that declared the roots capability is asked: one that did
	// not may never answer, and every call would wait on it.
	if ss == nil {
		return roots
	}
	if ip := ss.InitializeParams(); ip == nil || ip.Capabilities == nil || ip.Capabilities.RootsV2 == nil { //nolint:staticcheck // see below
		return roots
	}
	// A client that declared roots and does not answer delays the call by
	// this much, and the call reaches startDir alone.
	ctx, cancel := context.WithTimeout(ctx, mcpListRootsTimeout)
	defer cancel()
	// The roots feature is deprecated (SEP-2577) and remains in the protocol
	// for its window. A client that does not support it, or a session on a
	// protocol version that forbids the request while a call is served
	// (2026-07-28 and later), yields an error, and the call reaches startDir
	// alone.
	res, err := ss.ListRoots(ctx, &mcp.ListRootsParams{}) //nolint:staticcheck // see above
	if err != nil || res == nil {
		return roots
	}
	for _, r := range res.Roots {
		if p, ok := FileRootPath(r.URI); ok {
			roots = append(roots, p)
		}
	}
	return roots
}

// FileRootPath is the local directory a file:// root URI names. Only an
// empty host or localhost names this machine: file://host/etc is not the
// local /etc, and is refused.
func FileRootPath(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || u.Path == "" {
		return "", false
	}
	if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
		return "", false
	}
	p := u.Path
	if runtime.GOOS == "windows" {
		p = strings.TrimPrefix(p, "/")
	}
	return filepath.FromSlash(p), true
}

// MCPUnderRoots returns the first of roots that p lies under once links are
// resolved. A root reached through a link (/var and /tmp on macOS) is the
// directory it resolves to, so p may be spelled either way. When p is beneath
// a root as named but a link in it leads out, MCPUnderRoots returns that
// error instead. When p is under none, it returns "" and nil.
func MCPUnderRoots(roots []string, p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	var escape error
	for _, root := range roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		_, cerr := rootpath.Contain(absRoot, abs)
		if cerr == nil {
			return absRoot, nil
		}
		if _, named := rootpath.Beneath(absRoot, abs); named && escape == nil {
			escape = cerr
		}
	}
	return "", escape
}

// mcpRootOf returns the directory among those ctx carries that p, named in
// arg, lies under, refusing as MCPReachable does.
func mcpRootOf(ctx context.Context, arg, p string) (string, error) {
	if err := MCPReachable(ctx, arg, p); err != nil {
		return "", err
	}
	root, escape := MCPUnderRoots(MCPRootsFrom(ctx), p)
	if root == "" {
		if escape != nil {
			return "", fmt.Errorf("%s: %w", arg, escape)
		}
		return "", fmt.Errorf("%s: %q lies outside the directories this server may reach", arg, p)
	}
	return root, nil
}

// openMCPPath opens the file p an MCP call named in arg, through the
// directory it lies under, so a link swapped in after the check is refused.
func openMCPPath(ctx context.Context, arg, p string) (*os.File, error) {
	root, err := mcpRootOf(ctx, arg, p)
	if err != nil {
		return nil, err
	}
	return rootpath.OpenRegular(root, p)
}

// statMCPPath stats what p, named in arg, resolves to beneath the directory
// it lies under.
func statMCPPath(ctx context.Context, arg, p string) (os.FileInfo, error) {
	root, err := mcpRootOf(ctx, arg, p)
	if err != nil {
		return nil, err
	}
	return rootpath.Stat(root, p)
}

// MCPReachable refuses a path an MCP call named in arg when it lies under
// none of the directories ctx carries.
func MCPReachable(ctx context.Context, arg, p string) error {
	if p == "" {
		return nil
	}
	roots := MCPRootsFrom(ctx)
	if len(roots) == 0 {
		return fmt.Errorf("%s: this server has no directory it may reach. Start it in a directory, or have the client declare a workspace root", arg)
	}
	root, escape := MCPUnderRoots(roots, p)
	switch {
	case root != "":
		return nil
	case escape != nil:
		return fmt.Errorf("%s: %w", arg, escape)
	default:
		return fmt.Errorf("%s: %q lies outside the directories this server may reach (%s). Name one under them", arg, p, strings.Join(roots, ", "))
	}
}
