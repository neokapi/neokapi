package flow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/schema"
)

func TestResolveToolConfig_URIPrefixes(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", tmpDir)

	// Create expected directories.
	os.MkdirAll(filepath.Join(tmpDir, "tm"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "termbases"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "srx"), 0755)

	config := map[string]any{
		"tmxPath":   "tm:project-memory",
		"termsPath": "termbase:glossary",
		"srxPath":   "srx:custom-rules",
	}

	ctx := ResourceContext{ProjectDir: "/project"}
	resolved, err := ResolveToolConfig(config, nil, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"tmxPath":   filepath.Join(tmpDir, "tm", "project-memory.db"),
		"termsPath": filepath.Join(tmpDir, "termbases", "glossary.db"),
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
		"outputPath": "reports/qa-report.html",
		"enabled":    true,
		"count":      42,
	}

	ctx := ResourceContext{ProjectDir: "/home/user/project"}
	resolved, err := ResolveToolConfig(config, nil, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := resolved["outputPath"].(string)
	want := filepath.Join("/home/user/project", "reports/qa-report.html")
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

func TestResolveToolConfig_AbsolutePaths(t *testing.T) {
	config := map[string]any{
		"outputPath": "/absolute/path/report.html",
	}

	ctx := ResourceContext{ProjectDir: "/project"}
	resolved, err := ResolveToolConfig(config, nil, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := resolved["outputPath"].(string)
	if got != "/absolute/path/report.html" {
		t.Errorf("absolute path should be unchanged, got %q", got)
	}
}

func TestResolveToolConfig_OkapiVariables(t *testing.T) {
	config := map[string]any{
		"outputPath": "${rootDir}/qa-report.html",
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

	if got := resolved["outputPath"].(string); got != "/home/user/project/qa-report.html" {
		t.Errorf("outputPath: got %q, want %q", got, "/home/user/project/qa-report.html")
	}
	if got := resolved["logPath"].(string); got != "/home/user/project/replacementsLog.txt" {
		t.Errorf("logPath: got %q, want %q", got, "/home/user/project/replacementsLog.txt")
	}
}

func TestResolveToolConfig_LanguageVariables(t *testing.T) {
	config := map[string]any{
		"tmxPath": "${rootDir}/${srcLang}-${trgLang}.tmx",
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

	got := resolved["tmxPath"].(string)
	if got != "/project/en-fr.tmx" {
		t.Errorf("tmxPath: got %q, want %q", got, "/project/en-fr.tmx")
	}
}

func TestResolveToolConfig_SchemaAnnotation(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, "termbases"), 0755)

	cs := &schema.ComponentSchema{
		Properties: map[string]schema.PropertySchema{
			"termsFile": {
				Type: "string",
				PathInfo: &schema.PathAnnotation{
					Type:         "file",
					Role:         "input",
					ResourceKind: "termbase",
				},
			},
			"regularString": {
				Type: "string",
			},
		},
	}

	config := map[string]any{
		"termsFile":     "termbase:glossary",
		"regularString": "tm:not-a-path", // should NOT be resolved (no x-path annotation, name doesn't match heuristic)
	}

	ctx := ResourceContext{ProjectDir: "/project"}
	resolved, err := ResolveToolConfig(config, cs, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// termsFile should be resolved via URI prefix.
	got := resolved["termsFile"].(string)
	want := filepath.Join(tmpDir, "termbases", "glossary.db")
	if got != want {
		t.Errorf("termsFile: got %q, want %q", got, want)
	}

	// regularString should be unchanged (no x-path, name doesn't match heuristic).
	if resolved["regularString"] != "tm:not-a-path" {
		t.Errorf("regularString should be unchanged, got %v", resolved["regularString"])
	}
}

func TestResolveToolConfig_OutputAutoPlacement(t *testing.T) {
	cs := &schema.ComponentSchema{
		Properties: map[string]schema.PropertySchema{
			"outputPath": {
				Type:    "string",
				Default: "${rootDir}/qa-report.html",
				PathInfo: &schema.PathAnnotation{
					Type: "file",
					Role: "output",
				},
			},
		},
	}

	config := map[string]any{
		"outputPath": "${rootDir}/qa-report.html",
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
	want := filepath.Join("/project/output/fr", "quality-check", "qa-report.html")
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
	_, ok, err := resolveURIPrefix("tm:")
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
