package flow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/schema"
	"github.com/stretchr/testify/require"
)

// writeStore creates the named store file under the config dir, as the person
// would before a recipe names it.
func writeStore(t *testing.T, configDir string, rel ...string) {
	t.Helper()
	p := filepath.Join(append([]string{configDir}, rel...)...)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, nil, 0o644))
}

// realTempDir is t.TempDir with its links resolved (/var is /private/var on
// macOS), the spelling a resolved path carries.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

func TestResolveToolConfig_URIPrefixes(t *testing.T) {
	tmpDir := realTempDir(t)
	t.Setenv("KAPI_CONFIG_DIR", tmpDir)

	writeStore(t, tmpDir, "memory", "project-memory.db")
	writeStore(t, tmpDir, "terms", "product.db")
	writeStore(t, tmpDir, "srx", "custom-rules.srx")

	config := map[string]any{
		"tmx_path":  "memory:project-memory",
		"termsPath": "terms:product",
		"srxPath":   "srx:custom-rules",
	}

	ctx := ResourceContext{ProjectDir: "/project"}
	resolved, err := ResolveToolConfig(config, nil, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"tmx_path":  filepath.Join(tmpDir, "memory", "project-memory.db"),
		"termsPath": filepath.Join(tmpDir, "terms", "product.db"),
		"srxPath":   filepath.Join(tmpDir, "srx", "custom-rules.srx"),
	}

	for key, want := range expected {
		got, ok := resolved[key].(string)
		if !ok {
			t.Errorf("%s: expected string, got %T", key, resolved[key])
			continue
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", key, got, want)
		}
	}
}

func TestResolveToolConfig_RelativePaths(t *testing.T) {
	config := map[string]any{
		"outputPath": "reports/check-report.html",
		"enabled":    true,
		"count":      42,
	}

	ctx := ResourceContext{ProjectDir: "/home/user/project"}
	resolved, err := ResolveToolConfig(config, nil, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := resolved["outputPath"].(string)
	want := filepath.Join("/home/user/project", "reports/check-report.html")
	if got != want {
		t.Errorf("outputPath: got %q, want %q", got, want)
	}

	// Non-path properties should be unchanged.
	if resolved["enabled"] != true {
		t.Errorf("enabled should be unchanged")
	}
	if resolved["count"] != 42 {
		t.Errorf("count should be unchanged")
	}
}

// A project flow's step config is recipe content: a path property that is
// absolute outside the project, or climbs out of it, is refused with the key
// named. One inside the project or the output directory resolves, and without
// a project an absolute path is kept as given.
func TestResolveToolConfig_PathsStayInTheProject(t *testing.T) {
	project := filepath.FromSlash("/project")
	out := filepath.FromSlash("/out/fr")
	for _, tc := range []struct {
		name, value, want string
		ctx               ResourceContext
	}{
		{"absolute outside", "/absolute/path/report.html", "", ResourceContext{ProjectDir: project}},
		{"climbs out", "../../etc/passwd", "", ResourceContext{ProjectDir: project}},
		{"rootDir climbs out", "${rootDir}/../x.html", "", ResourceContext{ProjectDir: project}},
		{"absolute inside", "/project/reports/r.html", filepath.FromSlash("/project/reports/r.html"), ResourceContext{ProjectDir: project}},
		{"relative inside", "reports/r.html", filepath.Join(project, "reports", "r.html"), ResourceContext{ProjectDir: project}},
		{"output dir", "${outputDir}/r.html", filepath.Join(out, "r.html"), ResourceContext{ProjectDir: project, OutputDir: out}},
		{"no project", "/absolute/path/report.html", "/absolute/path/report.html", ResourceContext{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolved, err := ResolveToolConfig(map[string]any{"outputPath": tc.value}, nil, tc.ctx)
			if tc.want == "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), "outputPath")
				require.Contains(t, err.Error(), "lies outside the project")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, resolved["outputPath"])
		})
	}
}

func TestResolveToolConfig_OkapiVariables(t *testing.T) {
	config := map[string]any{
		"outputPath": "${rootDir}/check-report.html",
		"logPath":    "${rootDir}/replacementsLog.txt",
	}

	ctx := ResourceContext{
		ProjectDir:   "/home/user/project",
		SourceLocale: "en",
		TargetLocale: "fr-CA",
	}

	resolved, err := ResolveToolConfig(config, nil, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := resolved["outputPath"].(string); got != "/home/user/project/check-report.html" {
		t.Errorf("outputPath: got %q, want %q", got, "/home/user/project/check-report.html")
	}
	if got := resolved["logPath"].(string); got != "/home/user/project/replacementsLog.txt" {
		t.Errorf("logPath: got %q, want %q", got, "/home/user/project/replacementsLog.txt")
	}
}

func TestResolveToolConfig_LanguageVariables(t *testing.T) {
	config := map[string]any{
		"tmx_path": "${rootDir}/${srcLang}-${trgLang}.tmx",
	}

	ctx := ResourceContext{
		ProjectDir:   "/project",
		SourceLocale: "en-US",
		TargetLocale: "fr",
	}

	resolved, err := ResolveToolConfig(config, nil, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := resolved["tmx_path"].(string)
	if got != "/project/en-fr.tmx" {
		t.Errorf("tmx_path: got %q, want %q", got, "/project/en-fr.tmx")
	}
}

func TestResolveToolConfig_SchemaAnnotation(t *testing.T) {
	tmpDir := realTempDir(t)
	t.Setenv("KAPI_CONFIG_DIR", tmpDir)
	writeStore(t, tmpDir, "terms", "product.db")

	cs := &schema.ComponentSchema{
		Properties: map[string]schema.PropertySchema{
			"termsFile": {
				Type: "string",
				PathInfo: &schema.PathAnnotation{
					Type:         "file",
					Role:         "input",
					ResourceKind: "terms",
				},
			},
			"regularString": {
				Type: "string",
			},
		},
	}

	config := map[string]any{
		"termsFile":     "terms:product",
		"regularString": "memory:not-a-path", // should NOT be resolved (no x-path annotation, name doesn't match heuristic)
	}

	ctx := ResourceContext{ProjectDir: "/project"}
	resolved, err := ResolveToolConfig(config, cs, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// termsFile should be resolved via URI prefix.
	got := resolved["termsFile"].(string)
	want := filepath.Join(tmpDir, "terms", "product.db")
	if got != want {
		t.Errorf("termsFile: got %q, want %q", got, want)
	}

	// regularString should be unchanged (no x-path, name doesn't match heuristic).
	if resolved["regularString"] != "memory:not-a-path" {
		t.Errorf("regularString should be unchanged, got %v", resolved["regularString"])
	}
}

func TestResolveToolConfig_OutputAutoPlacement(t *testing.T) {
	cs := &schema.ComponentSchema{
		Properties: map[string]schema.PropertySchema{
			"outputPath": {
				Type:    "string",
				Default: "${rootDir}/check-report.html",
				PathInfo: &schema.PathAnnotation{
					Type: "file",
					Role: "output",
				},
			},
		},
	}

	config := map[string]any{
		"outputPath": "${rootDir}/check-report.html",
	}

	ctx := ResourceContext{
		ProjectDir: "/project",
		OutputDir:  "/project/output/fr",
		ToolName:   "quality-check",
	}

	resolved, err := ResolveToolConfig(config, cs, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := resolved["outputPath"].(string)
	want := filepath.Join("/project/output/fr", "quality-check", "check-report.html")
	if got != want {
		t.Errorf("outputPath: got %q, want %q", got, want)
	}
}

func TestResolveToolConfig_EmptyValues(t *testing.T) {
	config := map[string]any{
		"outputPath": "",
		"termsPath":  "",
	}

	ctx := ResourceContext{ProjectDir: "/project"}
	resolved, err := ResolveToolConfig(config, nil, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Empty strings should remain empty.
	if resolved["outputPath"] != "" {
		t.Errorf("empty outputPath should remain empty")
	}
	if resolved["termsPath"] != "" {
		t.Errorf("empty termsPath should remain empty")
	}
}

func TestResolveToolConfig_NilConfig(t *testing.T) {
	ctx := ResourceContext{ProjectDir: "/project"}
	resolved, err := ResolveToolConfig(nil, nil, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != nil {
		t.Errorf("nil config should return nil")
	}
}

func TestShortLang(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"en", "en"},
		{"en-US", "en"},
		{"de-CH", "de"},
		{"fr_CA", "fr"},
		{"zh-Hans-CN", "zh"},
		{"", ""},
	}
	for _, tt := range tests {
		got := shortLang(tt.input)
		if got != tt.want {
			t.Errorf("shortLang(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveURIPrefix_EmptyName(t *testing.T) {
	_, ok, err := resolveURIPrefix("memory:")
	if !ok {
		t.Error("expected ok=true for tm: prefix")
	}
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestResolveURIPrefix_NotPrefix(t *testing.T) {
	_, ok, _ := resolveURIPrefix("/absolute/path")
	if ok {
		t.Error("expected ok=false for non-prefix value")
	}
}
