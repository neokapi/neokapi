// Resource resolution for pipeline step configuration.
//
// Steps reference external resources (content memories, terms stores, SRX
// files, output paths) via URI prefixes (memory:name, terms:name, srx:name) or
// relative/absolute file paths. The resolver translates these references into
// absolute filesystem paths before the config is passed to the tool or bridge.
//
// A project flow's step config is recipe content, which a repository supplies.
// So a file path must resolve inside the project, or the run's output
// directory, once symbolic links are resolved, and the resolved path is what
// the tool gets. A named resource is one of the person's own stores, named by
// name only, and must already exist (NamedStorePath).
package flow

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/dataroot"

	"github.com/neokapi/neokapi/core/rootpath"
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
	PrefixMemory = "memory:"
	PrefixTerms  = "terms:"
	PrefixSRX    = "srx:"
)

// Resource kind → KAPI_HOME subdirectory and file extension.
var resourceKinds = map[string]struct {
	Dir string
	Ext string
}{
	"memory": {Dir: "memory", Ext: ".db"},
	"terms":  {Dir: "terms", Ext: ".db"},
	"srx":    {Dir: "srx", Ext: ".srx"},
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
	given := value
	value = substituteVariables(value, ctx)

	// 4. Resolve relative paths against ProjectDir. A project flow's step
	// config is recipe content, so the path must stay inside the project (or
	// the run's output directory) once links are resolved.
	if ctx.ProjectDir == "" {
		return value, nil
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(ctx.ProjectDir, value)
	}
	for _, dir := range []string{ctx.ProjectDir, ctx.OutputDir} {
		if dir == "" || !rootpath.IsBeneath(dir, value) {
			continue
		}
		real, err := containIn(dir, value)
		if err != nil {
			return "", fmt.Errorf("%s: %q leads outside the project: %w. Name a path inside the project, or a named resource (%s, %s, %s)",
				key, given, err, PrefixMemory, PrefixTerms, PrefixSRX)
		}
		return real, nil
	}
	return "", fmt.Errorf("%s: %q lies outside the project. Name a path inside the project, or a named resource (%s, %s, %s)",
		key, given, PrefixMemory, PrefixTerms, PrefixSRX)
}

// containIn returns the path p, which lies beneath dir as written, resolves to,
// with every link on it resolved, and an error when that leads outside dir.
// The tool that opens the path, a SQLite opener among them, follows links, so
// it gets the resolved path. A dir that does not exist yet holds nothing that
// could lead elsewhere, so p is kept as written.
func containIn(dir, p string) (string, error) {
	real, err := rootpath.Contain(dir, p)
	if err == nil {
		return real, nil
	}
	var esc *rootpath.EscapeError
	if !errors.As(err, &esc) && errors.Is(err, fs.ErrNotExist) {
		return filepath.Clean(p), nil
	}
	return "", err
}

// resolveURIPrefix checks for tm:, terms:, or srx: prefixes and resolves them.
func resolveURIPrefix(value string) (string, bool, error) {
	for prefix, info := range map[string]struct {
		kind string
	}{
		PrefixMemory: {kind: "memory"},
		PrefixTerms:  {kind: "terms"},
		PrefixSRX:    {kind: "srx"},
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

// resolveNamedResource returns the file of a named resource a step config
// names with a memory:, terms: or srx: prefix: <KAPI_CONFIG_DIR>/<kind>/<name><ext>.
//
// A named resource is one of the person's own stores, and a recipe names it by
// name only. The name is a plain name, the store already exists as a regular
// file that resolves inside that kind's directory, and the returned path is
// the resolved one (NamedStorePath). A recipe never creates a store, nor the
// directory that holds one: the person creates it, for example with
// `kapi terms import --name <name>`.
func resolveNamedResource(kind, name string) (string, error) {
	rk, ok := resourceKinds[kind]
	if !ok {
		return "", fmt.Errorf("unknown resource kind: %q", kind)
	}
	kapiHome, err := kapiHomeDir()
	if err != nil {
		return "", err
	}
	return NamedStorePath(filepath.Join(kapiHome, rk.Dir), kind, name, rk.Ext)
}

// NoStoreError is a named store that does not exist. It matches
// fs.ErrNotExist.
type NoStoreError struct {
	Kind, Name, Dir string
}

func (e *NoStoreError) Error() string {
	return fmt.Sprintf("%s %q does not exist under %s, and a recipe never creates one", e.Kind, e.Name, e.Dir)
}

// Is reports fs.ErrNotExist.
func (e *NoStoreError) Is(target error) bool { return target == fs.ErrNotExist }

// CheckStoreName refuses a store name that is not a plain name: empty, "."
// or "..", or holding a path separator or a volume name.
func CheckStoreName(name string) error {
	if name == "" {
		return errors.New("resource name is required")
	}
	if name == "." || strings.ContainsAny(name, "/\\") || !filepath.IsLocal(name) || filepath.Base(name) != name {
		return fmt.Errorf("resource name must be a plain name, with no path separators: %q", name)
	}
	return nil
}

// NamedStorePath returns the file of the store named name in dir, a kind's
// directory under the user config dir, for a name that recipe content gives:
// name must be a plain name (CheckStoreName), and dir/<name><ext> must already
// exist as a regular file that resolves inside dir. The returned path is the
// resolved one, with no link left on it. kind names the store in an error.
func NamedStorePath(dir, kind, name, ext string) (string, error) {
	if err := CheckStoreName(name); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name+ext)
	real, err := rootpath.CheckRegular(dir, p)
	if err == nil {
		return real, nil
	}
	var esc *rootpath.EscapeError
	switch {
	case errors.As(err, &esc):
		return "", fmt.Errorf("%s %q: %w", kind, name, err)
	case errors.Is(err, fs.ErrNotExist):
		return "", &NoStoreError{Kind: kind, Name: name, Dir: dir}
	default:
		return "", fmt.Errorf("%s %q: %w", kind, name, err)
	}
}

// kapiHomeDir returns the KAPI_HOME directory (~/.config/kapi/).
func kapiHomeDir() (string, error) {
	if dir := os.Getenv("KAPI_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	configDir := dataroot.UserConfigDir()
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
