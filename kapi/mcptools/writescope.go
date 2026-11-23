package mcptools

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/host"
)

// A tool call names its destination in an argument, which an agent writes.
// The call may therefore write only inside directories a person chose: the
// project in scope, or, outside one, the directory the server was started in
// and the workspace roots the client declares (host/mcp_roots.go).

// withWriteRoots returns ctx carrying the directories a call may reach.
func withWriteRoots(ctx context.Context, roots []string) context.Context {
	return host.WithMCPRoots(ctx, roots)
}

// writeRootsFrom returns the directories ctx carries, or the working
// directory for a handler called directly.
func writeRootsFrom(ctx context.Context) []string { return host.MCPRootsFrom(ctx) }

// sessionWriteRoots is what the server's roots middleware puts on a call's
// context. A server built without that middleware (a test, an embedder) gets
// the same answer here: startDir and the roots the client of req declares.
func sessionWriteRoots(ctx context.Context, req *mcp.CallToolRequest, startDir string) []string {
	if roots, ok := host.MCPRootsSet(ctx); ok {
		return roots
	}
	var ss *mcp.ServerSession
	if req != nil {
		ss = req.Session
	}
	return host.MCPSessionRoots(ctx, ss, startDir)
}

// fileRootPath is the local directory a file:// root URI names.
func fileRootPath(uri string) (string, bool) { return host.FileRootPath(uri) }

// underRoots returns the first of roots that p lies under once links are
// resolved, or the error of a link that leads out of one.
func underRoots(roots []string, p string) (string, error) { return host.MCPUnderRoots(roots, p) }

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
