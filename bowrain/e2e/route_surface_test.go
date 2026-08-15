package e2e

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/bowrain/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The shared e2e client is a client of the API like any other, and a client
// drifts silently: a route it names can stop existing without anything failing
// to compile. The suite that would notice needs four images and a database, so
// it runs nightly at best, and route drift reaches main between runs.
//
// This test needs no stack. It builds the real Echo router in-process, reads
// the paths the TypeScript client constructs, and asserts each is registered —
// the guard TestClientServerRouteSurface gives the Go client, cheap enough to
// run on every pull request.
//
// It answers one question: could any route serve this path. A path that lands
// on a route meant for something else — /workspaces/:slug reaching the project
// handler at /:ws/:id — satisfies the router and still fails against a running
// server. Only the Playwright suite settles that, which is why both lanes exist.

// clientPathParam stands in for a `${…}` interpolation while a path is matched
// structurally: it may line up with a server route's `:param`, never with a
// literal segment.
const clientPathParam = "\x01"

var (
	// A `${…}` interpolation. Every one in the client is brace-free inside, so
	// the naive form is exact rather than merely close.
	tsInterpolation = regexp.MustCompile(`\$\{[^}]*\}`)
	// this.get / this.post / … , with an optional generic type argument.
	clientCall = regexp.MustCompile(`this\.(get|post|put|patch|del)\b`)
)

var httpVerb = map[string]string{
	"get":   "GET",
	"post":  "POST",
	"put":   "PUT",
	"patch": "PATCH",
	"del":   "DELETE",
}

// clientRoute is one call the TypeScript client makes, kept with the line it
// came from so a failure names the helper rather than a path fragment.
type clientRoute struct {
	method string
	path   string
	line   int
}

// TestSharedClientRouteSurface asserts that every route bowrain/e2e/shared's
// BowrainAPI constructs is registered on the server.
func TestSharedClientRouteSurface(t *testing.T) {
	src := readSharedClient(t)
	routes := extractClientRoutes(src)
	require.NotEmpty(t, routes, "no routes extracted — the client's shape changed and this guard went blind")
	// Well under the real count, but enough that a parser that silently stops
	// finding calls fails here rather than passing vacuously.
	require.Greater(t, len(routes), 40, "suspiciously few routes extracted")

	registered := registeredRoutes(t)

	for _, r := range routes {
		assert.Truef(t, matchesAnyRoute(r, registered),
			"api-client.ts:%d constructs %s %s, which no server route serves", r.line, r.method, r.path)
	}
}

func readSharedClient(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("shared", "api-client.ts"))
	require.NoError(t, err)
	return string(b)
}

// registeredRoutes returns the server's route table, each path split into
// segments and stripped of the /api/v1 prefix the client's paths omit.
func registeredRoutes(t *testing.T) map[string][][]string {
	t.Helper()
	// Authenticated mode registers the workspace-scoped families; without a JWT
	// secret the server serves only its public handful.
	cfg := server.DefaultConfig()
	cfg.JWTSecret = "route-surface-guard"
	srv := server.NewServer(cfg)
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	byMethod := map[string][][]string{}
	for _, r := range srv.GetEcho().Routes() {
		path, ok := strings.CutPrefix(r.Path, "/api/v1")
		if !ok {
			continue
		}
		byMethod[r.Method] = append(byMethod[r.Method], splitPath(path))
	}
	require.NotEmpty(t, byMethod["GET"], "no routes registered — the server did not build")
	return byMethod
}

// extractClientRoutes finds the calls BowrainAPI makes: the this.<verb> helpers,
// and the raw fetch() calls that build on this.apiUrl for multipart upload.
func extractClientRoutes(src string) []clientRoute {
	var out []clientRoute

	for _, m := range clientCall.FindAllStringSubmatchIndex(src, -1) {
		verb := httpVerb[src[m[2]:m[3]]]
		raw, ok := callPathArg(src, m[1])
		if !ok {
			continue
		}
		out = append(out, clientRoute{method: verb, path: normalizeClientPath(raw), line: lineOf(src, m[0])})
	}

	// `await fetch(`${this.apiUrl}/…`, { method: "POST", … })`, which is how the
	// multipart uploads go out — they cannot use the JSON helpers.
	const anchor = "${this.apiUrl}"
	for idx := 0; ; {
		i := strings.Index(src[idx:], anchor)
		if i < 0 {
			break
		}
		at := idx + i
		idx = at + len(anchor)
		raw, ok := templateLiteralAround(src, at)
		if !ok {
			continue
		}
		path := normalizeClientPath(strings.TrimPrefix(raw, anchor))
		// The private get/post/put/patch/del primitives interpolate their whole
		// path argument. They carry no route of their own; their callers do.
		if path == clientPathParam {
			continue
		}
		out = append(out, clientRoute{
			method: methodAfter(src, idx),
			path:   path,
			line:   lineOf(src, at),
		})
	}
	return out
}

// callPathArg reads the first argument of a this.<verb> call — always the path
// — starting from the index just past the method name. An explicit type
// argument may sit between the name and the parenthesis, and it is skipped by
// balancing angle brackets rather than by scanning for a delimiter: the
// generics in this file contain `;` and `[]` of their own.
func callPathArg(src string, from int) (string, bool) {
	i := skipSpace(src, from)
	if i < len(src) && src[i] == '<' {
		depth := 0
		for ; i < len(src); i++ {
			switch src[i] {
			case '<':
				depth++
			case '>':
				depth--
			}
			if depth == 0 {
				i++
				break
			}
		}
		i = skipSpace(src, i)
	}
	if i >= len(src) || src[i] != '(' {
		return "", false
	}
	i = skipSpace(src, i+1)
	if i >= len(src) {
		return "", false
	}
	quote := src[i]
	if quote != '`' && quote != '"' {
		return "", false // a variable, not a literal path
	}
	end := strings.IndexByte(src[i+1:], quote)
	if end < 0 {
		return "", false
	}
	return src[i+1 : i+1+end], true
}

func skipSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\t' || s[i] == '\r') {
		i++
	}
	return i
}

// templateLiteralAround returns the literal containing position at.
func templateLiteralAround(src string, at int) (string, bool) {
	start := strings.LastIndexByte(src[:at], '`')
	if start < 0 {
		return "", false
	}
	end := strings.IndexByte(src[at:], '`')
	if end < 0 {
		return "", false
	}
	return src[start+1 : at+end], true
}

// methodAfter reads the `method: "…"` of a fetch options object. Its absence
// means a GET, as it does in fetch itself.
func methodAfter(src string, from int) string {
	window := src[from:min(from+400, len(src))]
	_, rest, ok := strings.Cut(window, `method: "`)
	if !ok {
		return "GET"
	}
	method, _, ok := strings.Cut(rest, `"`)
	if !ok {
		return "GET"
	}
	return method
}

// normalizeClientPath reduces a template literal to comparable path segments:
// interpolations become the param sentinel, and a query string is dropped —
// routing never sees it.
func normalizeClientPath(raw string) string {
	p := tsInterpolation.ReplaceAllString(raw, clientPathParam)
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	return p
}

func splitPath(p string) []string {
	return strings.Split(strings.Trim(p, "/"), "/")
}

// matchesAnyRoute reports whether any registered route of the same method could
// serve this path.
func matchesAnyRoute(r clientRoute, registered map[string][][]string) bool {
	want := splitPath(r.path)
	for _, have := range registered[r.method] {
		if segmentsMatch(want, have) {
			return true
		}
	}
	return false
}

func segmentsMatch(want, have []string) bool {
	for i, h := range have {
		if strings.HasPrefix(h, "*") {
			return true
		}
		if i >= len(want) {
			return false
		}
		w := want[i]
		// A trailing interpolation glued to a literal (`blast-radius${…}`) is an
		// optional query suffix, not a segment of its own.
		w = strings.TrimSuffix(w, clientPathParam)
		if w == "" {
			// The whole segment is an interpolation: only a route param can take it.
			if !strings.HasPrefix(h, ":") {
				return false
			}
			continue
		}
		if strings.HasPrefix(h, ":") {
			continue
		}
		if w != h {
			return false
		}
	}
	return len(want) == len(have)
}

func lineOf(src string, pos int) int {
	return strings.Count(src[:pos], "\n") + 1
}
