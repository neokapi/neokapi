//go:build e2e

package e2e

// The MCP conformance suite: the agent surface driven the way an agent drives
// it, and held to the answer the CLI gives for the same input.
//
// Everything here runs against the built `kapi` binary over stdio, through the
// protocol a client speaks: initialize, tools/list, tools/call, resources/read.
// The fixtures are real projects under t.TempDir(), seeded with the CLI, and
// the server carries the isolation environment the whole suite runs under.
//
// Two properties are under test.
//
// PARITY. Every check-type tool gets a fixture that must pass and a fixture
// that must fail, and the findings it reports over MCP are compared with what
// `kapi check` or `kapi exec` reports for the same content. A tool that reports
// a clean result over MCP while the CLI reports a violation is the defect this
// suite exists to catch: #1490 was exactly that, a bilingual check running on a
// source-only block, and the CLI tests could not see it because the CLI path
// was correct the whole time.
//
// PER-CALL PROJECT. One server process answers for two projects. The project
// is an argument of the call, the server's own project is the default, and a
// path that holds no project is refused naming the path.

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Fixtures ───────────────────────────────────────────────────────────────

// conformanceProject is one project on disk, with the paths the suite asks
// about. Each project forbids a term no other project forbids, so an answer
// says which project governed the call that produced it.
type conformanceProject struct {
	Name string
	Root string
	// Recipe is the project's kapi.yaml.
	Recipe string
	// Forbidden is the term this project's voice retires, and Replacement what
	// it asks for instead.
	Forbidden   string
	Replacement string
	// Violating holds Forbidden; Clean holds Replacement. Both are declared
	// content, so `kapi check` and check_file read them the same way.
	Violating string
	Clean     string
	// TargetLang is the project's single target language, and TargetTerm the
	// approved rendering of Replacement in it. A concept with no target term
	// yields a rule with no replacement, which the bilingual term check skips,
	// so a fixture meant to trip it declares one.
	TargetLang string
	TargetTerm string
}

// writeConformanceProject builds a project and seeds its terms store through
// the CLI, so the stores the suite reads are the ones kapi itself writes.
func writeConformanceProject(t *testing.T, name, forbidden, replacement, targetLang, targetTerm string) conformanceProject {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		file := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
		require.NoError(t, os.WriteFile(file, []byte(body), 0o644))
	}

	write("kapi.yaml", "version: v1\nname: "+name+`
defaults:
  source_language: en
  target_languages: [`+targetLang+`]
  source_gate: none
  voice: .kapi/voice.yaml
  terms_source: .kapi/terms.json
collections:
  - name: Docs
    content:
      - path: "docs/**/*.md"
`)
	write(".kapi/voice.yaml", "name: "+name+`
description: The voice of one project and no other.
tone:
  personality: [clear, restrained]
  formality: neutral
vocabulary:
  forbidden_terms:
    - term: `+forbidden+`
      replacement: `+replacement+`
      severity: major
`)
	write(".kapi/terms.json", `{
  "schemaVersion": "1.0",
  "kind": "kapi-terms",
  "concepts": [
    {
      "id": "c-`+name+`",
      "definition": "The store of approved prior wording.",
      "terms": [
        {"text": "`+replacement+`", "locale": "en", "status": "preferred"},
        {"text": "`+forbidden+`", "locale": "en", "status": "deprecated"},
        {"text": "`+targetTerm+`", "locale": "`+targetLang+`", "status": "preferred"}
      ],
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ]
}
`)
	write("docs/violating.md", "# Violating\n\nWe rely on the "+forbidden+" here.\n")
	write("docs/clean.md", "# Clean\n\nWe rely on the "+replacement+" here.\n")

	p := conformanceProject{
		Name:        name,
		Root:        root,
		Recipe:      filepath.Join(root, "kapi.yaml"),
		Forbidden:   forbidden,
		Replacement: replacement,
		Violating:   filepath.Join(root, "docs", "violating.md"),
		Clean:       filepath.Join(root, "docs", "clean.md"),
		TargetLang:  targetLang,
		TargetTerm:  targetTerm,
	}
	// Retrieval and the gates read the project's own store rather than the
	// committed files, so the fixture runs the import that puts the terms
	// bundle and the voice profile there.
	kapi(t, "context", "import", "-p", p.Recipe)
	return p
}

// sourceLangProject is one project written to exercise the source language a
// call reads in. Two of them differ in `defaults.source_language` and in
// nothing else, so the language is the only thing an answer can be coming from.
type sourceLangProject struct {
	Root   string
	Recipe string
	// Doc holds both deprecated terms, one in each language.
	Doc string
	// Lang is the project's source language, Deprecated the term its store
	// retires in that language, and Foreign the term the store retires in the
	// other one. A call reading this project's content reports Deprecated and
	// never Foreign; reading it in the other language inverts both.
	Lang       string
	Deprecated string
	Foreign    string
}

// sourceLangTerms is the terms store both projects import. One concept, with a
// deprecated and a preferred term in each of two languages, so a lookup in
// either language has something to find and the two answers are distinct.
const sourceLangTerms = `{
  "schemaVersion": "1.0",
  "kind": "kapi-terms",
  "concepts": [
    {
      "id": "c-handover",
      "definition": "Moving a piece of work from one person to the next.",
      "terms": [
        {"text": "handover", "locale": "en", "status": "deprecated"},
        {"text": "transfer", "locale": "en", "status": "preferred"},
        {"text": "overlevering", "locale": "nb", "status": "deprecated"},
        {"text": "overføring", "locale": "nb", "status": "preferred"}
      ],
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ]
}
`

// writeSourceLangProject builds a project declaring lang as its source
// language, with the shared terms store and a document holding a deprecated
// term in each language. It binds no voice profile: the vocabulary rules a
// profile declares match whatever the language, and the store lookup is the
// part that keys on it.
func writeSourceLangProject(t *testing.T, name, lang, deprecated, foreign, targetLang string) sourceLangProject {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		file := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
		require.NoError(t, os.WriteFile(file, []byte(body), 0o644))
	}
	write("kapi.yaml", "version: v1\nname: "+name+`
defaults:
  source_language: `+lang+`
  target_languages: [`+targetLang+`]
  source_gate: none
  terms_source: .kapi/terms.json
collections:
  - name: Docs
    content:
      - path: "docs/**/*.md"
`)
	write(".kapi/terms.json", sourceLangTerms)
	write("docs/note.md", "# Note\n\nThe handover and the overlevering are recorded here.\n")

	p := sourceLangProject{
		Root:       root,
		Recipe:     filepath.Join(root, "kapi.yaml"),
		Doc:        filepath.Join(root, "docs", "note.md"),
		Lang:       lang,
		Deprecated: deprecated,
		Foreign:    foreign,
	}
	kapi(t, "context", "import", "-p", p.Recipe)
	return p
}

// writeBilingual writes an XLIFF holding one source and one target, which is
// what `kapi exec` reads for a bilingual tool. Over MCP the same pair arrives
// as `text` and `target`.
func writeBilingual(t *testing.T, dir, name, source, target, targetLang string) string {
	t.Helper()
	file := filepath.Join(dir, name)
	body := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="` + targetLang + `">
  <file id="f1">
    <unit id="u1">
      <segment>
        <source>` + source + `</source>
        <target>` + target + `</target>
      </segment>
    </unit>
  </file>
</xliff>
`
	require.NoError(t, os.WriteFile(file, []byte(body), 0o644))
	return file
}

// ─── Driving the server and the CLI ─────────────────────────────────────────

// mcpServer starts a real `kapi mcp` over stdio and connects a client to it.
//
// The working directory is a fresh temporary directory rather than a project,
// so nothing answers by accident: what a call reaches is the project it named,
// or the one the server was started with.
func mcpServer(t *testing.T, args ...string) (*mcp.ClientSession, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	cmd := exec.CommandContext(ctx, kapiBin, append([]string{"mcp"}, args...)...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), isoEnv...)
	cmd.Stderr = os.Stderr

	client := mcp.NewClient(&mcp.Implementation{Name: "kapi-mcp-conformance", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: 5 * time.Second}, nil)
	require.NoError(t, err, "connect to `kapi mcp`")
	t.Cleanup(func() { _ = session.Close() })
	return session, ctx
}

// callTool issues one tools/call and requires it to succeed.
func callTool(t *testing.T, ctx context.Context, s *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	res := rawCallTool(t, ctx, s, name, args)
	require.False(t, res.IsError, "%s reported an error: %s", name, resultText(res))
	require.NotNil(t, res.StructuredContent, "%s returned no structured content", name)
	return asMap(t, res.StructuredContent)
}

// rawCallTool issues one tools/call and returns the result, error or not.
func rawCallTool(t *testing.T, ctx context.Context, s *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	raw, err := json.Marshal(args)
	require.NoError(t, err)
	res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: json.RawMessage(raw)})
	require.NoError(t, err, "tools/call %s", name)
	return res
}

// readResource issues one resources/read and returns the single content it
// carries, parsed when it is JSON.
func readResource(t *testing.T, ctx context.Context, s *mcp.ClientSession, uri string) (string, string) {
	t.Helper()
	res, err := s.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	require.NoError(t, err, "resources/read %s", uri)
	require.Len(t, res.Contents, 1)
	return res.Contents[0].Text, res.Contents[0].MIMEType
}

// resultText joins the text content of a result, which is where a refused call
// explains itself.
func resultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// asMap re-encodes a value as a generic JSON map, so a report from MCP and one
// from the CLI are compared as the same shape.
func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// kapiJSON runs a kapi command that prints JSON on stdout and returns the
// first JSON value it wrote.
//
// Only the first: a command whose gate failed prints its report and then the
// gate error, two documents on one stream, and a whole-stream decode of that
// fails on the second. A non-zero exit is a result here rather than a harness
// failure, which is exactly the case that prints two.
func kapiJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	cmd := exec.Command(kapiBin, args...)
	cmd.Env = append(os.Environ(), isoEnv...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	var out map[string]any
	decodeErr := json.NewDecoder(strings.NewReader(string(stdout))).Decode(&out)
	require.NoError(t, decodeErr,
		"kapi %s produced no JSON (exit %v)\nstdout:\n%s\nstderr:\n%s",
		strings.Join(args, " "), err, string(stdout), stderr.String())
	return out
}

// findings reads the findings array out of a kapi.check/v1 report.
func findings(report map[string]any) []any {
	got, _ := report["findings"].([]any)
	return got
}

// toolFindings reads the findings a framework tool's result carries, which it
// writes as a quality annotation on the block rather than as a report.
func toolFindings(result map[string]any) []any {
	anns, ok := result["annotations"].(map[string]any)
	if !ok {
		return nil
	}
	quality, ok := anns["quality.findings"].(map[string]any)
	if !ok {
		return nil
	}
	got, _ := quality["findings"].([]any)
	return got
}

// messages reduces findings to the messages they carry, which is what an MCP
// result and a CLI report have in common: the CLI names the file each finding
// sits in, and a snippet sent over MCP has no file.
func messages(items []any) []string {
	var out []string
	for _, item := range items {
		f, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if msg, ok := f["message"].(string); ok {
			out = append(out, msg)
		}
	}
	return out
}

// ─── The surface a client discovers ─────────────────────────────────────────

func TestMCPConformanceToolSurface(t *testing.T) {
	session, ctx := mcpServer(t)

	var tools []*mcp.Tool
	params := &mcp.ListToolsParams{}
	for {
		res, err := session.ListTools(ctx, params)
		require.NoError(t, err)
		tools = append(tools, res.Tools...)
		if res.NextCursor == "" {
			break
		}
		params.Cursor = res.NextCursor
	}
	require.NotEmpty(t, tools)

	byName := map[string]*mcp.Tool{}
	for _, tool := range tools {
		byName[tool.Name] = tool
	}

	// Every project-scoped tool takes the project as an argument. A tool added
	// to this surface without one can only serve the directory the server
	// happened to start in.
	projectScoped := []string{
		"check_file", "check_text", "context_search", "apply_edits",
		"up", "up_plan", "extract_content",
		"review_queue", "review_unit", "approve_unit", "reject_unit", "sign_off_unit",
		"term-check", "translate", "redact",
	}
	for _, name := range projectScoped {
		t.Run(name, func(t *testing.T) {
			tool := byName[name]
			require.NotNil(t, tool, "%s is not on the curated surface", name)
			props := schemaProperties(t, tool)
			assert.Contains(t, props, "project", "%s must accept a per-call project", name)
		})
	}
}

// schemaProperties reads a tool's declared input properties.
func schemaProperties(t *testing.T, tool *mcp.Tool) map[string]any {
	t.Helper()
	raw, err := json.Marshal(tool.InputSchema)
	require.NoError(t, err)
	var schema struct {
		Properties map[string]any `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(raw, &schema))
	return schema.Properties
}

// ─── Per-call project ───────────────────────────────────────────────────────

func TestMCPConformanceTwoProjectsOneServer(t *testing.T) {
	alpha := writeConformanceProject(t, "alpha", "translation memory", "content memory", "nb", "innholdsminne")
	beta := writeConformanceProject(t, "beta", "termbase", "terms store", "fr", "magasin de termes")
	// Bound to alpha, exactly as a client configured for one project starts it.
	session, ctx := mcpServer(t, "-p", alpha.Recipe)

	t.Run("the server's project is the default", func(t *testing.T) {
		report := callTool(t, ctx, session, "check_file", map[string]any{"file": alpha.Violating})
		assert.NotEmpty(t, findings(report), "alpha's voice must govern a call that named no project")
	})

	t.Run("a second project is served by the same process", func(t *testing.T) {
		report := callTool(t, ctx, session, "check_file", map[string]any{
			"file": beta.Violating, "project": beta.Recipe,
		})
		require.NotEmpty(t, findings(report))
		assert.Contains(t, strings.Join(messages(findings(report)), "\n"), beta.Forbidden)
	})

	for name, spelling := range map[string]string{
		"a project root":            "",
		"a file inside the project": "docs/clean.md",
		"a directory inside":        "docs",
	} {
		t.Run("named as "+name, func(t *testing.T) {
			named := beta.Root
			if spelling != "" {
				named = filepath.Join(beta.Root, filepath.FromSlash(spelling))
			}
			report := callTool(t, ctx, session, "check_file", map[string]any{
				"file": beta.Violating, "project": named,
			})
			assert.NotEmpty(t, findings(report), "%s must resolve beta", named)
		})
	}

	t.Run("one project's voice does not reach the other", func(t *testing.T) {
		report := callTool(t, ctx, session, "check_file", map[string]any{
			"file": beta.Violating, "project": alpha.Recipe,
		})
		assert.Empty(t, findings(report),
			"must fail: alpha's voice reported a term only beta retires")
	})

	t.Run("a path that holds no project is refused by name", func(t *testing.T) {
		outside := t.TempDir()
		res := rawCallTool(t, ctx, session, "check_file", map[string]any{
			"file": beta.Violating, "project": outside,
		})
		require.True(t, res.IsError, "must fail: a project path resolving to nothing was accepted")
		assert.Contains(t, resultText(res), outside, "the error names the path the call sent")
	})

	t.Run("context_search reads the named project's store", func(t *testing.T) {
		got := callTool(t, ctx, session, "context_search", map[string]any{
			"query": beta.Forbidden, "project": beta.Root,
		})
		want := kapiJSON(t, "context", "search", beta.Forbidden, "-p", beta.Recipe, "--json")
		assert.Equal(t, want, got, "context_search must answer what `kapi context search` answers")

		other := callTool(t, ctx, session, "context_search", map[string]any{
			"query": beta.Forbidden, "project": alpha.Root,
		})
		assert.NotEqual(t, want, other, "must fail: alpha's store answered beta's query")
	})

	t.Run("the context resource reads the named project", func(t *testing.T) {
		body, mime := readResource(t, ctx, session,
			"context://docs/clean.md?format=json&project="+beta.Root)
		assert.Equal(t, "application/json", mime)
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &got))
		want := kapiJSON(t, "context", "docs/clean.md", "-p", beta.Recipe, "--json")
		assert.Equal(t, want, got, "the resource must answer what `kapi context <path>` answers")
	})

	t.Run("concurrent calls keep their own project", func(t *testing.T) {
		type want struct {
			project  string
			file     string
			findings bool
		}
		cases := []want{
			{project: "", file: alpha.Violating, findings: true},
			{project: beta.Recipe, file: beta.Violating, findings: true},
			{project: alpha.Root, file: alpha.Clean},
			{project: filepath.Join(beta.Root, "docs"), file: beta.Clean},
		}
		var wg sync.WaitGroup
		for range 4 {
			for _, c := range cases {
				wg.Add(1)
				go func(c want) {
					defer wg.Done()
					args := map[string]any{"file": c.file}
					if c.project != "" {
						args["project"] = c.project
					}
					res := rawCallTool(t, ctx, session, "check_file", args)
					if !assert.False(t, res.IsError, resultText(res)) {
						return
					}
					report := asMap(t, res.StructuredContent)
					if c.findings {
						assert.NotEmpty(t, findings(report), "a concurrent call lost its project")
					} else {
						assert.Empty(t, findings(report), "a concurrent call took another project's voice")
					}
				}(c)
			}
		}
		wg.Wait()
	})
}

// TestMCPConformanceSourceLanguagePerCall holds one server process to two
// projects whose source languages differ. A term lookup matches the locale
// exactly, so content read in the other project's language is held to no
// vocabulary at all, and a server that settled the language once at startup
// answers for whichever project it started in.
//
// Each project's document holds a term its store retires in English and a term
// it retires in Norwegian. The answer names one of them, and which one says
// what language the call read in: swap the two and every assertion below
// inverts, so neither direction can pass by reporting nothing.
func TestMCPConformanceSourceLanguagePerCall(t *testing.T) {
	const en, nb = "handover", "overlevering"
	english := writeSourceLangProject(t, "english", "en", en, nb, "nb")
	norwegian := writeSourceLangProject(t, "norwegian", "nb", nb, en, "en")
	// Bound to the English project, so the Norwegian calls are the ones a
	// start-time resolution gets wrong.
	session, ctx := mcpServer(t, "-p", english.Recipe)

	for name, proj := range map[string]sourceLangProject{
		"the server's own project": english,
		"the project on the call":  norwegian,
	} {
		t.Run(name, func(t *testing.T) {
			args := map[string]any{"file": proj.Doc}
			if proj.Recipe != english.Recipe {
				args["project"] = proj.Recipe
			}
			got := messages(findings(callTool(t, ctx, session, "check_file", args)))
			joined := strings.Join(got, "\n")
			require.NotEmpty(t, got,
				"must fail: %s reported nothing, so its content was read in another language", proj.Lang)
			assert.Contains(t, joined, proj.Deprecated,
				"the term retired in %s must be reported", proj.Lang)
			assert.NotContains(t, joined, proj.Foreign,
				"must fail: the term retired in the other language was reported, so the call read in it")

			// The CLI resolves the same project's language for itself, which is
			// the answer the MCP call has to match.
			want := kapiJSON(t, "check", proj.Doc, "-p", proj.Recipe, "--json")
			assert.Equal(t, messages(findings(want)), got,
				"check_file must report what `kapi check -p %s` reports", proj.Recipe)
		})

		t.Run("a draft for "+name, func(t *testing.T) {
			args := map[string]any{
				"text":         "The " + en + " and the " + nb + " are recorded here.",
				"context_path": "docs/draft.md",
			}
			if proj.Recipe != english.Recipe {
				args["project"] = proj.Recipe
			}
			got := strings.Join(messages(findings(callTool(t, ctx, session, "check_text", args))), "\n")
			assert.Contains(t, got, proj.Deprecated,
				"a draft is checked in the language its destination's project writes")
			assert.NotContains(t, got, proj.Foreign,
				"must fail: the draft was read in the other project's language")
		})
	}
}

// ─── Parity: the check tools ────────────────────────────────────────────────

func TestMCPConformanceCheckFileParity(t *testing.T) {
	proj := writeConformanceProject(t, "parity", "translation memory", "content memory", "nb", "innholdsminne")
	session, ctx := mcpServer(t, "-p", proj.Recipe)

	for name, tc := range map[string]struct {
		file     string
		mustFail bool
	}{
		"the negative fixture": {file: proj.Violating, mustFail: true},
		"the positive fixture": {file: proj.Clean},
	} {
		t.Run(name, func(t *testing.T) {
			got := callTool(t, ctx, session, "check_file", map[string]any{"file": tc.file})
			want := kapiJSON(t, "check", tc.file, "-p", proj.Recipe, "--json")
			assert.Equal(t, findings(want), findings(got),
				"check_file must report what `kapi check` reports")
			assert.Equal(t, want["summary"], got["summary"])
			if tc.mustFail {
				require.NotEmpty(t, findings(got), "must fail: the violating document reported nothing")
				assert.Contains(t, strings.Join(messages(findings(got)), "\n"), proj.Forbidden)
			} else {
				assert.Empty(t, findings(got))
			}
		})
	}
}

func TestMCPConformanceCheckTextParity(t *testing.T) {
	proj := writeConformanceProject(t, "drafts", "translation memory", "content memory", "nb", "innholdsminne")
	session, ctx := mcpServer(t, "-p", proj.Recipe)

	for name, tc := range map[string]struct {
		body     string
		mustFail bool
	}{
		"the negative fixture": {body: "We rely on the " + proj.Forbidden + " here.", mustFail: true},
		"the positive fixture": {body: "We rely on the " + proj.Replacement + " here."},
	} {
		t.Run(name, func(t *testing.T) {
			got := callTool(t, ctx, session, "check_text", map[string]any{
				"text":         tc.body,
				"context_path": "docs/draft.md",
			})
			// The CLI has no snippet mode, so the same text is written at the
			// destination the draft was checked for and the file is checked.
			file := filepath.Join(proj.Root, "docs", "draft.md")
			require.NoError(t, os.WriteFile(file, []byte("# Draft\n\n"+tc.body+"\n"), 0o644))
			t.Cleanup(func() { _ = os.Remove(file) })
			want := kapiJSON(t, "check", file, "-p", proj.Recipe, "--json")
			assert.Equal(t, messages(findings(want)), messages(findings(got)),
				"a draft for a destination must be checked as that destination's content is")
			if tc.mustFail {
				require.NotEmpty(t, findings(got), "must fail: the violating draft reported nothing")
			} else {
				assert.Empty(t, findings(got))
			}
		})
	}
}

func TestMCPConformanceVoiceCheckParity(t *testing.T) {
	proj := writeConformanceProject(t, "voice", "translation memory", "content memory", "nb", "innholdsminne")
	session, ctx := mcpServer(t, "-p", proj.Recipe)
	profile := filepath.Join(proj.Root, ".kapi", "voice.yaml")

	for name, tc := range map[string]struct {
		body     string
		mustFail bool
	}{
		"the negative fixture": {body: "We rely on the " + proj.Forbidden + " here.", mustFail: true},
		"the positive fixture": {body: "We rely on the " + proj.Replacement + " here."},
	} {
		t.Run(name, func(t *testing.T) {
			got := callTool(t, ctx, session, "voice_check", map[string]any{
				"text": tc.body, "profile_file": profile,
			})
			want := kapiJSON(t, "voice", "check", "--profile-file", profile, "--input-text", tc.body, "--json")
			assert.Equal(t, want["score"], got["score"], "voice_check must score what `kapi voice check` scores")
			assert.Equal(t, messages(findings(want)), messages(findings(got)))
			if tc.mustFail {
				require.NotEmpty(t, findings(got), "must fail: the violating text scored clean")
			} else {
				assert.Empty(t, findings(got))
			}
		})
	}
}

// ─── Parity: the bilingual checks ───────────────────────────────────────────

// bilingualCase is one bilingual check's pair of fixtures and the arguments
// that drive it on both surfaces.
type bilingualCase struct {
	// tool is the registry tool's name, which is also its MCP tool name and its
	// `kapi exec` subcommand.
	tool string
	// source and the two targets: one that must be reported, one that must not.
	source     string
	badTarget  string
	goodTarget string
	// mcpArgs and cliArgs carry the tool's own configuration on each surface.
	mcpArgs map[string]any
	cliArgs []string
	// cliDriven is false for a tool the CLI cannot be given the same
	// configuration as the MCP call, where the fixtures are still held to
	// must-fail and must-pass. term-check is the one: `kapi exec term-check`
	// resolves its rules from a project's terms store and has no flag for the
	// ad-hoc rules an MCP call sends, so the two surfaces cannot be handed the
	// same input. The project-resolved path is compared end to end in
	// TestMCPConformanceCheckFileBilingualParity, where both surfaces read the
	// same terms store.
	cliDriven bool
}

func TestMCPConformanceBilingualChecksParity(t *testing.T) {
	// Only term-check is on the curated surface; the rest of the bilingual
	// checks are reachable with --all-tools, which is what this drives.
	session, ctx := mcpServer(t, "--all-tools")
	dir := t.TempDir()
	const targetLang = "nb"

	cases := []bilingualCase{
		{
			tool:       "dnt-check",
			source:     "Open the Kapi dashboard.",
			badTarget:  "Åpne Kapi-oversikten.",
			goodTarget: "Åpne Kapi dashboard.",
			mcpArgs:    map[string]any{"terms": []string{"dashboard"}},
			cliArgs:    []string{"--terms", "dashboard"},
			cliDriven:  true,
		},
		{
			tool:       "placeholder-check",
			source:     "Hello {name}",
			badTarget:  "Hei {navn}",
			goodTarget: "Hei {name}",
			cliDriven:  true,
		},
		{
			tool:       "qa",
			source:     "Save the document.",
			badTarget:  "Lagre lagre dokumentet.",
			goodTarget: "Lagre dokumentet.",
			cliDriven:  true,
		},
		{
			tool:       "term-check",
			source:     "Reuse comes from the content memory.",
			badTarget:  "Gjenbruk kommer fra oversettelsesminnet.",
			goodTarget: "Gjenbruk kommer fra innholdsminnet.",
			mcpArgs: map[string]any{"term_rules": []map[string]string{
				{"term": "content memory", "replacement": "innholdsminnet"},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			for name, target := range map[string]string{"negative": tc.badTarget, "positive": tc.goodTarget} {
				t.Run(name, func(t *testing.T) {
					args := map[string]any{
						"text": tc.source, "target": target, "target_lang": targetLang,
					}
					for k, v := range tc.mcpArgs {
						args[k] = v
					}
					got := callTool(t, ctx, session, tc.tool, args)
					reported := messages(toolFindings(got))
					// A bilingual tool writes its result either as a quality
					// annotation or, as term-check does, into block properties.
					// Both are read, so a tool that reported is never mistaken
					// for one that did not.
					if props, ok := got["properties"].(map[string]any); ok {
						if errs, ok := props[tc.tool+"-errors"].(string); ok && errs != "" {
							reported = append(reported, errs)
						}
					}

					if name == "negative" {
						require.NotEmpty(t, reported,
							"must fail: %s reported nothing over MCP for a translation that violates it", tc.tool)
					} else {
						require.Empty(t, reported,
							"must fail: %s reported a translation that satisfies it", tc.tool)
					}
					if !tc.cliDriven {
						return
					}

					file := writeBilingual(t, dir, tc.tool+"-"+name+".xlf", tc.source, target, targetLang)
					cli := kapiJSON(t, append([]string{"exec", tc.tool, file,
						"--target-lang", targetLang, "--json"}, tc.cliArgs...)...)
					assert.Equal(t, messages(findings(cli)), reported,
						"%s must report over MCP what `kapi exec %s` reports", tc.tool, tc.tool)
				})
			}
		})
	}
}

// check_file runs the bilingual checks too, over a pair of files, with the
// do-not-translate terms the call names and the term rules the project's terms
// store gives. Its findings are the ones `kapi check --target` reports for the
// same pair, which is where the two surfaces read the same terms store.
func TestMCPConformanceCheckFileBilingualParity(t *testing.T) {
	proj := writeConformanceProject(t, "bilingual", "translation memory", "content memory", "nb", "innholdsminne")
	session, ctx := mcpServer(t, "-p", proj.Recipe)
	dir := t.TempDir()

	source := filepath.Join(dir, "en.json")
	require.NoError(t, os.WriteFile(source,
		[]byte(`{"cta":"Open the Kapi dashboard with {count} alerts from the content memory"}`), 0o644))

	for name, tc := range map[string]struct {
		body     string
		mustFail bool
	}{
		// Drops a placeholder, translates a do-not-translate term, and renders
		// a concept with something other than its approved target term.
		"the negative fixture": {
			body:     `{"cta":"Åpne Kapi-oversikten med {antall} varsler fra oversettelsesminnet"}`,
			mustFail: true,
		},
		"the positive fixture": {
			body: `{"cta":"Åpne Kapi dashboard med {count} varsler fra innholdsminne"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".nb.json")
			require.NoError(t, os.WriteFile(target, []byte(tc.body), 0o644))

			got := callTool(t, ctx, session, "check_file", map[string]any{
				"file": source, "target": target, "target_lang": "nb", "dnt": []string{"dashboard"},
			})
			want := kapiJSON(t, "check", source, "--target", target,
				"--target-lang", "nb", "--dnt", "dashboard", "-p", proj.Recipe, "--json")
			assert.Equal(t, findings(want), findings(got),
				"check_file must report what `kapi check --target` reports")
			if tc.mustFail {
				require.NotEmpty(t, findings(got),
					"must fail: a translation dropping a placeholder, a do-not-translate term and an approved term reported clean")
				assert.Subset(t, ruleIDs(findings(got)), []string{"placeholder.placeholder", "dnt.do-not-translate", "terms.terminology"},
					"every bilingual check reachable here must report on the fixture built to trip it")
			} else {
				assert.Empty(t, findings(got))
			}
		})
	}
}

// ─── The first hour: an empty context, and what every read says about itself ──

// writeBareProject is the negative half of the empty-context fixtures: a
// project with a recipe, content, and nothing recorded about either. It is
// what most projects look like in the hour after `kapi init`.
func writeBareProject(t *testing.T, name string) (root, recipe string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	recipe = filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: "+name+`
defaults:
  source_language: en
  source_gate: none
collections:
  - name: Docs
    content:
      - path: "docs/**/*.md"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "guide.md"),
		[]byte("# Guide\n\nPlace a gadget on the dashboard.\n"), 0o644))
	return root, recipe
}

// TestMCPConformanceServerIntroducesItself: the instructions reach a client at
// initialize, ahead of the tool list and whether or not the host loads a
// skill. A client that loads nothing else still learns the two things that
// change what it does.
func TestMCPConformanceServerIntroducesItself(t *testing.T) {
	session, _ := mcpServer(t)

	init := session.InitializeResult()
	require.NotNil(t, init)
	instructions := init.Instructions
	require.NotEmpty(t, instructions, "the server introduces itself on initialize")

	assert.Contains(t, instructions, "context://", "ask what applies before writing")
	assert.Contains(t, instructions, "check_file", "run the check before reporting the work done")
	assert.Contains(t, instructions, "context_search")
	assert.Contains(t, instructions, "empty answer",
		"an empty answer read as nothing to do is the failure the instructions exist to prevent")
}

// TestMCPConformanceEmptyContextTeaches drives both fixtures through both
// surfaces: a project that records nothing, and one that records a voice and a
// vocabulary. An answer for the first says it is empty and what is worth
// noticing; an answer for the second says neither.
func TestMCPConformanceEmptyContextTeaches(t *testing.T) {
	bare, bareRecipe := writeBareProject(t, "bare")
	seeded := writeConformanceProject(t, "seeded", "translation memory", "content memory", "nb", "innholdsminne")
	session, ctx := mcpServer(t)

	const teaches = "who the text addresses"

	t.Run("by location, on a project that records nothing", func(t *testing.T) {
		body, _ := readResource(t, ctx, session, "context://docs/guide.md?format=json&project="+bare)
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &got))

		assert.Equal(t, "empty", got["coverage"], "nothing is bound here, and the answer says which")
		notes := strings.Join(noteStrings(got), "\n")
		assert.Contains(t, notes, "records nothing for this location")
		assert.Contains(t, notes, teaches)
		assert.NotContains(t, got, "voice", "an empty answer invents no rule")
		assert.NotContains(t, got, "terms")

		// The same answer through the CLI, to the byte.
		want := kapiJSON(t, "context", "docs/guide.md", "-p", bareRecipe, "--json")
		assert.Equal(t, want["coverage"], got["coverage"], "both surfaces grade the answer the same way")
		assert.Equal(t, want["notes"], got["notes"])

		// And the prose rendering carries it too, for a client that reads
		// markdown rather than JSON.
		text, mime := readResource(t, ctx, session, "context://docs/guide.md?project="+bare)
		assert.Equal(t, "text/markdown", mime)
		assert.Contains(t, text, "records nothing for this location")
	})

	t.Run("by location, on a project that records something", func(t *testing.T) {
		body, _ := readResource(t, ctx, session, "context://docs/clean.md?format=json&project="+seeded.Root)
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &got))

		assert.Equal(t, "covered", got["coverage"], "a voice and a vocabulary are both in force here")
		assert.NotContains(t, strings.Join(noteStrings(got), "\n"), teaches,
			"an answer with context behind it does not lecture the caller about collecting some")
	})

	t.Run("by content, on a query the project has never written about", func(t *testing.T) {
		got := callTool(t, ctx, session, "context_search", map[string]any{
			"query": "gadget", "project": bare,
		})
		assert.Equal(t, "empty", got["coverage"])
		notes := strings.Join(noteStrings(got), "\n")
		assert.Contains(t, notes, `records nothing about "gadget"`)
		assert.Contains(t, notes, teaches)
	})

	t.Run("a candidate lifts an empty answer to thin", func(t *testing.T) {
		// A proposal nobody has decided on holds no content to anything, so it
		// never makes an answer covered. It does say that someone looked here,
		// which is what keeps `empty` meaning nothing at all.
		proposed, proposedRecipe := writeBareProject(t, "proposed")
		kapi(t, "context", "propose", "utilise", "--use", "use",
			"--seen-in", "docs/guide.md", "-p", proposedRecipe)

		body, _ := readResource(t, ctx, session, "context://docs/guide.md?format=json&project="+proposed)
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &got))

		assert.Equal(t, "thin", got["coverage"])
		notes := strings.Join(noteStrings(got), "\n")
		assert.Contains(t, notes, "1 candidate rule")
		assert.Contains(t, notes, "confirmed or discarded",
			"a candidate is named as a candidate, never as a rule in force")
		assert.NotContains(t, notes, "records nothing for this location")

		want := kapiJSON(t, "context", "docs/guide.md", "-p", proposedRecipe, "--json")
		assert.Equal(t, want["coverage"], got["coverage"], "both surfaces grade it the same way")
		assert.Equal(t, want["notes"], got["notes"])
	})

	t.Run("by content, on a query the project has written about", func(t *testing.T) {
		got := callTool(t, ctx, session, "context_search", map[string]any{
			"query": seeded.Forbidden, "project": seeded.Root,
		})
		assert.NotEqual(t, "empty", got["coverage"])
		assert.NotContains(t, strings.Join(noteStrings(got), "\n"), teaches)
	})
}

// TestMCPConformanceEveryReadSaysWhatItRead: both primitives, on both
// surfaces, report the project that answered, the workspace revision it was
// read at, and whether the content kapi holds still matches the files on disk.
//
// Parity is the point. A field one surface reports and the other does not
// teaches an assistant a kapi that half of it does not have.
func TestMCPConformanceEveryReadSaysWhatItRead(t *testing.T) {
	bare, bareRecipe := writeBareProject(t, "provenance")
	session, ctx := mcpServer(t)

	t.Run("by location", func(t *testing.T) {
		body, _ := readResource(t, ctx, session, "context://docs/guide.md?format=json&project="+bare)
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &got))
		want := kapiJSON(t, "context", "docs/guide.md", "-p", bareRecipe, "--json")
		assertProvenanceParity(t, want, got, "provenance")
	})

	t.Run("by content", func(t *testing.T) {
		got := callTool(t, ctx, session, "context_search", map[string]any{
			"query": "gadget", "project": bare,
		})
		want := kapiJSON(t, "context", "search", "gadget", "-p", bareRecipe, "--json")
		assertProvenanceParity(t, want, got, "provenance")
	})

	t.Run("the prose rendering says it too", func(t *testing.T) {
		text, _ := readResource(t, ctx, session, "context://docs/guide.md?project="+bare)
		assert.Contains(t, text, "at workspace revision",
			"a client reading markdown is told the same three things as one reading JSON")
	})
}

// assertProvenanceParity holds the two surfaces to the same provenance.
func assertProvenanceParity(t *testing.T, cli, mcp map[string]any, key string) {
	t.Helper()
	want, ok := cli[key].(map[string]any)
	require.True(t, ok, "the CLI answer carries %s", key)
	got, ok := mcp[key].(map[string]any)
	require.True(t, ok, "the MCP answer carries %s", key)

	assert.NotEmpty(t, want["project"], "the answer names the project that produced it")
	assert.Equal(t, want["project"], got["project"], "both surfaces name one project")
	assert.Equal(t, want["name"], got["name"])
	assert.Equal(t, want["stale"], got["stale"], "both surfaces judge the projection the same way")
	// The revision is a position, so two reads of one unchanged workspace
	// report one number. Re-opening a project records nothing.
	assert.Equal(t, want["revision"], got["revision"],
		"reading the same unchanged workspace twice reports one revision")
}

// noteStrings reads the notes an answer carries.
func noteStrings(answer map[string]any) []string {
	items, _ := answer["notes"].([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ruleIDs reduces findings to the stable rule ids they carry.
func ruleIDs(items []any) []string {
	var out []string
	for _, item := range items {
		if f, ok := item.(map[string]any); ok {
			if rule, ok := f["rule"].(string); ok {
				out = append(out, rule)
			}
		}
	}
	return out
}

// ─── Growing the context: the write tools an agent keeps its habits with ────

// TestMCPConformanceContextGrowthTools drives the three write tools and the
// session read, each with a fixture that must be accepted and one that must be
// refused.
//
// Every one of them records an operation whose actor is an AGENT under this
// server's session. The caller never says who it is, so the refusal cases
// include an attempt to claim otherwise: an MCP client that could record as a
// person would be claiming the rights the policy reserves for one.
func TestMCPConformanceContextGrowthTools(t *testing.T) {
	proj := writeConformanceProject(t, "growth", "translation memory", "content memory", "nb", "innholdsminne")
	session, ctx := mcpServer(t, "-p", proj.Recipe)

	t.Run("observe records a fact", func(t *testing.T) {
		got := callTool(t, ctx, session, "context_observe", map[string]any{
			"text":  "the guides address the reader as you",
			"path":  "docs/clean.md",
			"quote": "We rely on the content memory here.",
		})
		assert.Equal(t, "observe", got["kind"])
		assert.Equal(t, "candidate", got["status"], "an observation is undecided until somebody acts on it")
		assert.NotEmpty(t, got["operation"], "the answer names the operation a person acts on")
		assert.NotEmpty(t, got["session"], "everything one run records is grouped under its session")
		assert.Contains(t, got["review"], "kapi context log --session ")
	})

	t.Run("observe with nothing to say is refused", func(t *testing.T) {
		res := rawCallTool(t, ctx, session, "context_observe", map[string]any{"text": "  "})
		require.True(t, res.IsError, "must fail: an observation with no text was recorded")
		assert.Contains(t, resultText(res), "text")
	})

	t.Run("propose records a candidate", func(t *testing.T) {
		got := callTool(t, ctx, session, "context_propose", map[string]any{
			"term": "utilise", "use": "use",
			"path": "docs/clean.md", "quote": "Utilise the editor.",
		})
		assert.Equal(t, "propose", got["kind"])
		assert.Equal(t, "candidate", got["status"])
		next, _ := got["next"].(string)
		assert.Contains(t, next, "kapi context confirm",
			"the answer says a person is what makes the rule bind")
		assert.Contains(t, next, "candidate")
	})

	t.Run("propose with no evidence is refused", func(t *testing.T) {
		// Twice over: the schema requires the argument, and the handler
		// refuses an empty one, so neither a client that omits it nor one that
		// sends a blank gets a rule nobody can check.
		omitted := rawCallTool(t, ctx, session, "context_propose", map[string]any{
			"term": "leverage", "use": "use",
		})
		require.True(t, omitted.IsError, "must fail: a rule with nothing behind it was recorded")
		assert.Contains(t, resultText(omitted), "path")

		blank := rawCallTool(t, ctx, session, "context_propose", map[string]any{
			"term": "leverage", "use": "use", "path": "  ",
		})
		require.True(t, blank.IsError, "must fail: a rule with a blank location was recorded")
		assert.Contains(t, resultText(blank), "evidence")
	})

	t.Run("correct records both wordings", func(t *testing.T) {
		got := callTool(t, ctx, session, "context_correct", map[string]any{
			"from": "sign in", "to": "log in",
			"path": "docs/clean.md", "propose": true,
		})
		assert.Equal(t, "correct", got["kind"])
		recorded, _ := got["recorded"].(string)
		assert.Contains(t, recorded, "sign in")
		assert.Contains(t, recorded, "log in")
	})

	for name, args := range map[string]map[string]any{
		"with one wording missing": {"to": "log in", "path": "docs/clean.md"},
		"with no evidence":         {"from": "sign in", "to": "log in"},
	} {
		t.Run("correct "+name+" is refused", func(t *testing.T) {
			res := rawCallTool(t, ctx, session, "context_correct", args)
			assert.True(t, res.IsError, "must fail: an unusable correction was recorded")
		})
	}

	t.Run("a call naming a project that holds none is refused by name", func(t *testing.T) {
		outside := t.TempDir()
		for tool, args := range map[string]map[string]any{
			"context_observe":         {"text": "a fact", "path": "docs/clean.md"},
			"context_propose":         {"term": "utilise", "use": "use", "path": "docs/clean.md"},
			"context_correct":         {"from": "sign in", "to": "log in", "path": "docs/clean.md"},
			"context_session_summary": {},
		} {
			args["project"] = outside
			res := rawCallTool(t, ctx, session, tool, args)
			require.Truef(t, res.IsError, "must fail: %s accepted a path holding no project", tool)
			assert.Contains(t, resultText(res), outside, "%s names the path the call sent", tool)
		}
	})

	t.Run("the caller is not allowed to say who it is", func(t *testing.T) {
		// An MCP client that could record as a person would be claiming the
		// rights the policy reserves for one. The tools declare no actor
		// argument, and an argument they do not declare is refused.
		res := rawCallTool(t, ctx, session, "context_observe", map[string]any{
			"text": "a fact", "path": "docs/clean.md",
			"actor": "person", "session": "somebody-elses",
		})
		require.True(t, res.IsError, "must fail: a caller chose its own actor")
		assert.Contains(t, resultText(res), "actor")
	})

	t.Run("every operation is recorded as an agent", func(t *testing.T) {
		got := callTool(t, ctx, session, "context_observe", map[string]any{
			"text": "the release notes are written in the past tense",
			"path": "docs/clean.md",
		})
		id, _ := got["operation"].(string)
		require.NotEmpty(t, id)

		log := kapiJSON(t, "context", "log", "-p", proj.Recipe, "--json")
		ops, _ := log["operations"].([]any)
		require.NotEmpty(t, ops)
		var found bool
		for _, item := range ops {
			op, ok := item.(map[string]any)
			if !ok || op["id"] != id {
				continue
			}
			found = true
			actor, ok := op["actor"].(map[string]any)
			require.True(t, ok, "every operation records who did it")
			assert.Equal(t, "agent", actor["kind"],
				"must fail: a call recorded itself as something other than an agent")
			assert.NotEmpty(t, actor["session"], "the server mints one session per process")
			assert.Equal(t, got["session"], actor["session"],
				"the session the tool reported is the session the log holds")
		}
		assert.True(t, found, "the operation the tool reported is in `kapi context log`")
	})

	t.Run("the session summary reports what this run recorded", func(t *testing.T) {
		got := callTool(t, ctx, session, "context_session_summary", map[string]any{})
		assert.Positive(t, got["observed"])
		assert.Positive(t, got["proposed"])
		assert.Positive(t, got["corrected"])
		assert.Positive(t, got["candidates"],
			"nobody has decided on any of them, which is what the report has to say")
		report, _ := got["report"].(string)
		assert.Contains(t, report, "kapi context log --session ",
			"the sentence an agent ends its report with says how to review the session")

		// The CLI reads the same session back, which is what makes the
		// sentence actionable.
		named, _ := got["session"].(string)
		require.NotEmpty(t, named)
		cli := kapiJSON(t, "context", "log", "--session", named, "-p", proj.Recipe, "--json")
		ops, _ := cli["operations"].([]any)
		assert.NotEmpty(t, ops, "`kapi context log --session` finds what the MCP session recorded")
	})
}

// TestMCPConformanceAgentCannotDecide: the policy reserves confirming,
// discarding another actor's work, reverting and widening for a person, and the
// agent surface carries no tool for any of them.
//
// Both halves are asserted. A tool absent from the listing but reachable by
// name is a surface a client can still find, and a tool present but refused at
// the policy is one an assistant will keep trying.
func TestMCPConformanceAgentCannotDecide(t *testing.T) {
	proj := writeConformanceProject(t, "decide", "translation memory", "content memory", "nb", "innholdsminne")
	session, ctx := mcpServer(t, "-p", proj.Recipe, "--all")

	listed := map[string]bool{}
	params := &mcp.ListToolsParams{}
	for {
		res, err := session.ListTools(ctx, params)
		require.NoError(t, err)
		for _, tool := range res.Tools {
			listed[tool.Name] = true
		}
		if res.NextCursor == "" {
			break
		}
		params.Cursor = res.NextCursor
	}
	require.True(t, listed["context_propose"], "the agent surface records")

	for _, name := range []string{
		"context_confirm", "context_discard", "context_revert", "context_widen",
		"confirm_context", "discard_context", "revert_context", "widen_context",
	} {
		t.Run(name+" is not on the surface", func(t *testing.T) {
			assert.False(t, listed[name],
				"must fail: deciding is a person's, and %s is listed for an agent", name)
			assert.False(t, toolAnswers(ctx, session, name), "must fail: %s answered a call", name)
		})
	}
}

// toolAnswers reports whether a tool answered a call at all. A name the server
// does not serve is refused by the protocol rather than by a handler, so the
// two refusals read differently on the wire and mean the same thing here.
func toolAnswers(ctx context.Context, s *mcp.ClientSession, name string) bool {
	res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: json.RawMessage(`{"id":"1"}`)})
	if err != nil {
		return false
	}
	return !res.IsError
}

// TestMCPConformanceCandidateCrossesProcesses: a candidate one agent recorded
// answers for the next one.
//
// The point of recording anything is that the next session does not work it out
// again, and the next session is a different process. Both servers run over the
// same workspace (the suite's isolation environment), and the second one is
// started after the first has recorded, so nothing it sees can have come from
// its own memory.
func TestMCPConformanceCandidateCrossesProcesses(t *testing.T) {
	proj := writeConformanceProject(t, "crossing", "translation memory", "content memory", "nb", "innholdsminne")

	first, firstCtx := mcpServer(t, "-p", proj.Recipe)
	recorded := callTool(t, firstCtx, first, "context_propose", map[string]any{
		"term": "utilise", "use": "use",
		"path": "docs/clean.md", "quote": "Utilise the editor.",
		"note": "the guides say use",
	})
	id, _ := recorded["operation"].(string)
	require.NotEmpty(t, id)
	require.NoError(t, first.Close())

	second, ctx := mcpServer(t, "-p", proj.Recipe)

	t.Run("a second process reads it, marked a candidate", func(t *testing.T) {
		body, _ := readResource(t, ctx, second, "context://docs/clean.md?format=json")
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &got))

		candidates, _ := got["candidates"].([]any)
		require.NotEmpty(t, candidates, "must fail: the candidate the first session recorded was not reported")
		var entry map[string]any
		for _, item := range candidates {
			if c, ok := item.(map[string]any); ok && c["term"] == "utilise" {
				entry = c
			}
		}
		require.NotNil(t, entry, "the candidate is reported by the word it is about")
		assert.Equal(t, "candidate", entry["status"],
			"must fail: a candidate was reported without saying it is one")
		assert.Equal(t, "use", entry["replacement"])
		assert.Equal(t, id, entry["operation"], "the id a person confirms it by")
		assert.NotEmpty(t, entry["session"], "the session it came from, so a person can revert the run")

		evidence, _ := entry["evidence"].([]any)
		require.NotEmpty(t, evidence, "must fail: a candidate was reported with no evidence behind it")
		seen, _ := evidence[0].(map[string]any)
		assert.Equal(t, "docs/clean.md", seen["path"])
		assert.Contains(t, seen["quote"], "Utilise")

		// Apart from the rules in force: a caller must not have to tell them
		// apart by reading the wording.
		terms, _ := got["terms"].([]any)
		for _, item := range terms {
			if hit, ok := item.(map[string]any); ok {
				assert.NotEqual(t, "utilise", hit["term"], "a candidate is not a term in force")
			}
		}
	})

	t.Run("both surfaces report it the same way", func(t *testing.T) {
		body, _ := readResource(t, ctx, second, "context://docs/clean.md?format=json")
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &got))
		want := kapiJSON(t, "context", "docs/clean.md", "-p", proj.Recipe, "--json")
		assert.Equal(t, want["candidates"], got["candidates"],
			"`kapi context <path>` and the context:// resource report one list")
	})

	t.Run("the prose rendering says it is not a rule", func(t *testing.T) {
		text, mime := readResource(t, ctx, second, "context://docs/clean.md")
		assert.Equal(t, "text/markdown", mime)
		assert.Contains(t, text, "Candidates, not yet decided")
		assert.Contains(t, text, "kapi context confirm")
	})

	t.Run("a candidate is reported and fails nothing", func(t *testing.T) {
		// The document uses the word the candidate retires, so a candidate
		// that could fail a check would fail this one.
		violating := filepath.Join(proj.Root, "docs", "candidate.md")
		require.NoError(t, os.WriteFile(violating,
			[]byte("# Candidate\n\nUtilise the "+proj.Replacement+" here.\n"), 0o644))
		t.Cleanup(func() { _ = os.Remove(violating) })

		report := callTool(t, ctx, second, "check_file", map[string]any{"file": violating})
		reported := findings(report)
		require.NotEmpty(t, reported, "a candidate is reported wherever a rule would be")
		for _, item := range reported {
			f, ok := item.(map[string]any)
			require.True(t, ok)
			assert.Equal(t, true, f["advisory"],
				"must fail: a rule nobody has confirmed was reported as a verdict")
			assert.Equal(t, "neutral", f["severity"],
				"must fail: a candidate was raised above the severity every gate ignores")
			assert.Contains(t, f["message"], "not yet confirmed")
		}

		want := kapiJSON(t, "check", violating, "-p", proj.Recipe, "--json")
		assert.Equal(t, findings(want), reported,
			"check_file must report what `kapi check` reports")
		assert.Equal(t, want["summary"], report["summary"])

		// And the check itself passes, which is the property the advisory
		// severity exists to guarantee.
		cmd := exec.CommandContext(ctx, kapiBin, "check", violating, "-p", proj.Recipe, "--json")
		cmd.Env = append(os.Environ(), isoEnv...)
		assert.NoError(t, cmd.Run(), "must fail: a check failed on a rule nobody had confirmed")
	})
}

// TestMCPConformanceGrowthParity holds the write tools to the command line.
// The skill drives the CLI and an MCP client drives the tools, so a habit that
// records one thing on one surface and another on the other teaches half the
// assistants a kapi the rest do not have.
func TestMCPConformanceGrowthParity(t *testing.T) {
	overMCP := writeConformanceProject(t, "parity-mcp", "translation memory", "content memory", "nb", "innholdsminne")
	overCLI := writeConformanceProject(t, "parity-cli", "translation memory", "content memory", "nb", "innholdsminne")
	session, ctx := mcpServer(t)

	callTool(t, ctx, session, "context_propose", map[string]any{
		"project": overMCP.Root,
		"term":    "utilise", "use": "use",
		"path": "docs/clean.md", "quote": "Utilise the editor.",
	})
	kapi(t, "context", "propose", "utilise", "--use", "use",
		"--seen-in", "docs/clean.md", "--quote", "Utilise the editor.", "-p", overCLI.Recipe)

	fromMCP := kapiJSON(t, "context", "docs/clean.md", "-p", overMCP.Recipe, "--json")
	fromCLI := kapiJSON(t, "context", "docs/clean.md", "-p", overCLI.Recipe, "--json")

	strip := func(answer map[string]any) []map[string]any {
		items, _ := answer["candidates"].([]any)
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			c, ok := item.(map[string]any)
			if !ok {
				continue
			}
			// The actor and the session differ by construction: one was
			// recorded by an agent and one by the person at the command line.
			delete(c, "proposed_by")
			delete(c, "session")
			delete(c, "at")
			delete(c, "operation")
			out = append(out, c)
		}
		return out
	}
	require.NotEmpty(t, strip(fromMCP))
	assert.Equal(t, strip(fromCLI), strip(fromMCP),
		"one proposal is one candidate, whichever surface recorded it")
}
