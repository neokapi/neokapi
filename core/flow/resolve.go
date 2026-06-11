// Resource resolution for pipeline step configuration.
//
// Steps reference external resources (TMs, termbases, SRX files, output paths) via
// URI prefixes (tm:name, termbase:name, srx:name) or relative/absolute file paths.
// The resolver translates these references into absolute filesystem paths before
// the config is passed to the tool or bridge.
package flow

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/schema"
)

// ResourceContext carries the resolution context for a pipeline run.
type ResourceContext struct {
	// ProjectDir is the directory containing the .kapi file, or cwd for ad-hoc runs.
	ProjectDir string

	// OutputDir is the computed output directory for the current target locale.
	OutputDir string

	// SourceLocale is the BCP-47 source locale (e.g. "en").
	SourceLocale string

	// TargetLocale is the BCP-47 target locale (e.g. "fr").
	TargetLocale string

	// ToolName is the tool name, used for auto-placing side-effect outputs.
	ToolName string
}

// URI prefix schemes for named resources.
const (
	PrefixTM       = "tm:"
	PrefixTermbase = "termbase:"
	PrefixSRX      = "srx:"
)

// Resource kind → KAPI_HOME subdirectory and file extension.
var resourceKinds = map[string]struct {
	Dir string
	Ext string
}{
	"tm":       {Dir: "tm", Ext: ".db"},
	"termbase": {Dir: "termbases", Ext: ".db"},
	"srx":      {Dir: "srx", Ext: ".srx"},
}

// ResolveToolConfig resolves resource references and path variables in a step config map.
// It returns a new map with all path-typed values resolved to absolute filesystem paths.
//
// Resolution uses schema annotations (x-path) when available, falling back to heuristic
// detection for properties whose names end with Path, Directory, Dir, URI, or Url.
func ResolveToolConfig(config map[string]any, cs *schema.ComponentSchema, ctx ResourceContext) (map[string]any, error) {
	if len(config) == 0 {
		return config, nil
	}

	resolved := make(map[string]any, len(config))
	maps.Copy(resolved, config)

	for key, val := range resolved {
		s, ok := val.(string)
		if !ok || s == "" {
			continue
		}

		if !isPathProperty(key, cs) {
			continue
		}

		r, err := resolvePathValue(s, key, cs, ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", key, err)
		}
		resolved[key] = r
	}

	return resolved, nil
}

// isPathProperty returns true if the property should be treated as a path reference.
func isPathProperty(key string, cs *schema.ComponentSchema) bool {
	// Check schema annotation first.
	if cs != nil {
		if prop, ok := cs.Properties[key]; ok && prop.PathInfo != nil {
			return true
		}
	}

	// Heuristic: property name suggests a path.
	lower := strings.ToLower(key)
	return strings.HasSuffix(lower, "path") ||
		strings.HasSuffix(lower, "directory") ||
		strings.HasSuffix(lower, "dir") ||
		strings.HasSuffix(lower, "uri") ||
		strings.HasSuffix(lower, "url")
}

// resolvePathValue resolves a single path value string.
func resolvePathValue(value, key string, cs *schema.ComponentSchema, ctx ResourceContext) (string, error) {
	// 1. Check for URI prefix (named resource reference).
	if resolved, ok, err := resolveURIPrefix(value); ok {
		return resolved, err
	}

	// 2. Check if this is an output-role value that should be auto-placed.
	//    Must check BEFORE variable substitution, since ${rootDir} defaults indicate
	//    the user hasn't explicitly set the output path.
	if isOutputRole(key, cs) && isRootDirDefault(value) && ctx.OutputDir != "" && ctx.ToolName != "" {
		// Extract the filename portion after ${rootDir}/ and place into output dir.
		filename := extractFilenameFromDefault(value)
		return filepath.Join(ctx.OutputDir, ctx.ToolName, filename), nil
	}

	// 3. Substitute Okapi variables.
	value = substituteVariables(value, ctx)

	// 4. Resolve relative paths against ProjectDir.
	if !filepath.IsAbs(value) && ctx.ProjectDir != "" {
		value = filepath.Join(ctx.ProjectDir, value)
	}

	return value, nil
}

// resolveURIPrefix checks for tm:, termbase:, or srx: prefixes and resolves them.
func resolveURIPrefix(value string) (string, bool, error) {
	for prefix, info := range map[string]struct {
		kind string
	}{
		PrefixTM:       {kind: "tm"},
		PrefixTermbase: {kind: "termbase"},
		PrefixSRX:      {kind: "srx"},
	} {
		if after, ok := strings.CutPrefix(value, prefix); ok {
			name := after
			if name == "" {
				return "", true, fmt.Errorf("empty resource name after %s prefix", prefix)
			}
			resolved, err := resolveNamedResource(info.kind, name)
			if err != nil {
				return "", true, err
			}
			return resolved, true, nil
		}
	}
	return "", false, nil
}

// resolveNamedResource returns the absolute path for a named resource in KAPI_HOME.
func resolveNamedResource(kind, name string) (string, error) {
	if strings.ContainsAny(name, "/\\") {
		return "", fmt.Errorf("resource name must not contain path separators: %q", name)
	}

	rk, ok := resourceKinds[kind]
	if !ok {
		return "", fmt.Errorf("unknown resource kind: %q", kind)
	}

	kapiHome, err := kapiHomeDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(kapiHome, rk.Dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create directory %s: %w", dir, err)
	}

	return filepath.Join(dir, name+rk.Ext), nil
}

// kapiHomeDir returns the KAPI_HOME directory (~/.config/kapi/).
func kapiHomeDir() (string, error) {
	if dir := os.Getenv("KAPI_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "kapi"), nil
}

// substituteVariables replaces Okapi-style ${var} placeholders with resolved values.
func substituteVariables(value string, ctx ResourceContext) string {
	if !strings.Contains(value, "${") {
		return value
	}

	replacements := map[string]string{
		"${rootDir}":      ctx.ProjectDir,
		"${inputRootDir}": ctx.ProjectDir,
		"${srcLang}":      shortLang(ctx.SourceLocale),
		"${trgLang}":      shortLang(ctx.TargetLocale),
		"${srcLoc}":       ctx.SourceLocale,
		"${trgLoc}":       ctx.TargetLocale,
		"${srcBCP47}":     ctx.SourceLocale,
		"${trgBCP47}":     ctx.TargetLocale,
		"${outputDir}":    ctx.OutputDir,
	}

	for placeholder, replacement := range replacements {
		if replacement != "" {
			value = strings.ReplaceAll(value, placeholder, replacement)
		}
	}
	return value
}

// shortLang extracts the language subtag from a BCP-47 locale (e.g. "de" from "de-CH").
func shortLang(locale string) string {
	if i := strings.IndexAny(locale, "-_"); i > 0 {
		return locale[:i]
	}
	return locale
}

// isOutputRole checks if a property has role "output" via schema annotation.
func isOutputRole(key string, cs *schema.ComponentSchema) bool {
	if cs == nil {
		return false
	}
	prop, ok := cs.Properties[key]
	if !ok || prop.PathInfo == nil {
		return false
	}
	return prop.PathInfo.Role == "output"
}

// isRootDirDefault returns true if the value is an Okapi-style ${rootDir} default.
func isRootDirDefault(value string) bool {
	return strings.HasPrefix(value, "${rootDir}/") || strings.HasPrefix(value, "${rootDir}\\")
}

// extractFilenameFromDefault extracts the filename from a ${rootDir}/filename default.
func extractFilenameFromDefault(value string) string {
	// Strip the ${rootDir}/ prefix and return the remainder as the filename.
	after := strings.TrimPrefix(value, "${rootDir}/")
	after = strings.TrimPrefix(after, "${rootDir}\\")
	return filepath.Base(after)
}
