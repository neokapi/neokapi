package mcptools

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/core/rootpath"
)

// A tool call names its destination in an argument, which an agent writes.
// The call may therefore write only inside directories a person chose: the
// project in scope, or, outside one, the directory the server was started in
// and the workspace roots the client declares. writeRoots carries the latter
// on the call's context.
type writeRootsKey struct{}

// withWriteRoots returns ctx carrying the directories a call outside a
// project may write under. An empty list means there are none.
func withWriteRoots(ctx context.Context, roots []string) context.Context {
	if roots == nil {
		roots = []string{}
	}
	return context.WithValue(ctx, writeRootsKey{}, roots)
}

// writeRootsFrom returns the directories ctx carries. A context that carries
// none, a handler called directly rather than through the server, falls back
// to the working directory.
func writeRootsFrom(ctx context.Context) []string {
	if roots, ok := ctx.Value(writeRootsKey{}).([]string); ok {
		return roots
	}
	if wd, err := os.Getwd(); err == nil {
		return []string{wd}
	}
	return []string{}
}

// sessionWriteRoots is startDir, when known, followed by every file root the
// client of req declares.
func sessionWriteRoots(ctx context.Context, req *mcp.CallToolRequest, startDir string) []string {
	roots := []string{}
	if startDir != "" {
		roots = append(roots, startDir)
	}
	if req == nil || req.Session == nil {
		return roots
	}
	// The roots feature is deprecated (SEP-2577) and remains in the protocol
	// for its window. A client that does not support it, or a session on a
	// protocol version that forbids the request while a call is served
	// (2026-07-28 and later), yields an error, and the call writes under
	// startDir alone.
	res, err := req.Session.ListRoots(ctx, &mcp.ListRootsParams{}) //nolint:staticcheck // see above
	if err != nil || res == nil {
		return roots
	}
	for _, r := range res.Roots {
		if p, ok := fileRootPath(r.URI); ok {
			roots = append(roots, p)
		}
	}
	return roots
}

// fileRootPath is the local directory a file:// root URI names. Only an
// empty host or localhost names this machine: file://host/etc is not the
// local /etc, and is refused.
func fileRootPath(uri string) (string, bool) {
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

// writeRootFor returns the root among roots that outputPath lies under once
// links are resolved. named says the path came from output_path; otherwise
// it is the default beside inputPath, and the error says so.
func writeRootFor(roots []string, outputPath, inputPath string, named bool) (string, error) {
	if len(roots) == 0 {
		return "", errors.New("output_path: this server has no directory it may write. Start it in a directory, or have the client declare a workspace root")
	}
	abs, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}
	root, escape := underRoots(roots, abs)
	if root != "" {
		return root, nil
	}
	if escape != nil {
		return "", fmt.Errorf("output_path: %w", escape)
	}
	where := strings.Join(roots, ", ")
	if !named {
		return "", fmt.Errorf("path: the output beside %q would lie outside the directories this server may write (%s). Pass an output_path under one of them", inputPath, where)
	}
	return "", fmt.Errorf("output_path: %q lies outside the directories this server may write (%s). Name an output_path under one of them", outputPath, where)
}

// underRoots returns the first of roots that p lies under once links are
// resolved. A root reached through a link (/var and /tmp on macOS) is the
// directory it resolves to, so p may be spelled either way. When p is beneath
// a root as named but a link in it leads out, underRoots returns that error
// instead. When p is under none, it returns "" and nil.
func underRoots(roots []string, p string) (string, error) {
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
