package extractor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// PluginDescriptor is the `kapi-plugin` object kapi expects in a
// package's `package.json` (or a dedicated `kapi-plugin.json`).
//
//	{
//	  "name": "@neokapi/kapi-react",
//	  "kapi-plugin": {
//	    "extensions": [".tsx", ".jsx"],
//	    "extract": {
//	      "exec": ["kapi-react", "extract", "--blocks-stream"],
//	      "stdin": "paths-nul-separated"
//	    }
//	  }
//	}
type PluginDescriptor struct {
	Extensions []string           `json:"extensions"`
	Extract    *ExtractDescriptor `json:"extract,omitempty"`
}

// ExtractDescriptor describes how to invoke the extractor. The
// `stdin` field is documentary — the only supported protocol today
// is NUL-separated paths — but we accept + verify it so a future
// protocol change can live alongside the current one.
type ExtractDescriptor struct {
	Exec  []string `json:"exec"`
	Stdin string   `json:"stdin,omitempty"`
}

// DiscoveredExtractor bundles a plugin descriptor with the package
// directory it was loaded from. The directory is the CWD we'll launch
// the extractor subprocess from so it inherits the project's
// package.json / tsconfig / etc.
type DiscoveredExtractor struct {
	PackageName string
	PackageDir  string
	Descriptor  PluginDescriptor
}

// Discover walks ancestor `node_modules/` directories starting from
// `projectRoot`, mirroring Node's resolution algorithm, and returns
// every package whose package.json carries a `kapi-plugin` field.
//
// Workspace layouts (`node_modules` at a parent directory) are
// handled naturally by the ancestor walk — the same algorithm the
// i18n-manifest resolver uses.
func Discover(projectRoot string) ([]DiscoveredExtractor, error) {
	seen := map[string]bool{}
	var out []DiscoveredExtractor

	dir := projectRoot
	for {
		nm := filepath.Join(dir, "node_modules")
		if info, err := os.Stat(nm); err == nil && info.IsDir() {
			if err := scanNodeModules(nm, seen, &out); err != nil {
				return out, err
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return out, nil
}

// scanNodeModules walks one level of `node_modules`, including
// `@scope/...` subdirectories. For each package it tries to load a
// plugin descriptor; packages without one are silently ignored.
func scanNodeModules(root string, seen map[string]bool, out *[]DiscoveredExtractor) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// @scope/<package> — recurse one level.
		if strings.HasPrefix(name, "@") {
			scopeRoot := filepath.Join(root, name)
			subs, err := os.ReadDir(scopeRoot)
			if err != nil {
				continue
			}
			for _, sub := range subs {
				if !sub.IsDir() {
					continue
				}
				loadOne(scopeRoot, name+"/"+sub.Name(), seen, out)
			}
			continue
		}
		loadOne(root, name, seen, out)
	}
	return nil
}

// loadOne tries to parse <root>/<relPkgName>/package.json (relPkgName
// is `foo` or `@scope/bar`). Extracts the `kapi-plugin` field if
// present. Dedupes by package name — a package found in an inner
// node_modules wins over one hoisted higher.
func loadOne(root, relPkgName string, seen map[string]bool, out *[]DiscoveredExtractor) {
	if seen[relPkgName] {
		return
	}
	pkgDir := filepath.Join(root, relPkgName)
	// For `@scope/<pkg>` the caller passes relPkgName including the
	// scope; we strip the scope prefix before joining.
	if strings.Contains(relPkgName, "/") {
		parts := strings.SplitN(relPkgName, "/", 2)
		pkgDir = filepath.Join(root, parts[1])
	}
	pkgJSON := filepath.Join(pkgDir, "package.json")
	data, err := os.ReadFile(pkgJSON) // #nosec G304 — project-local node_modules
	if err != nil {
		return
	}
	var parsed struct {
		Name       string           `json:"name"`
		KapiPlugin PluginDescriptor `json:"kapi-plugin"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return
	}
	if len(parsed.KapiPlugin.Extensions) == 0 && parsed.KapiPlugin.Extract == nil {
		return
	}
	seen[relPkgName] = true
	*out = append(*out, DiscoveredExtractor{
		PackageName: parsed.Name,
		PackageDir:  pkgDir,
		Descriptor:  parsed.KapiPlugin,
	})
}

// ByExtension returns a map from lowercase extension (`.tsx`) to the
// first discovered extractor that handles it. Later discoveries for
// the same extension are ignored, matching the "inner wins" dedup
// rule. Uses the input slice's order — call sites that want a
// deterministic order should sort before calling.
func ByExtension(discovered []DiscoveredExtractor) map[string]DiscoveredExtractor {
	out := make(map[string]DiscoveredExtractor)
	for _, d := range discovered {
		for _, ext := range d.Descriptor.Extensions {
			e := strings.ToLower(ext)
			if _, exists := out[e]; exists {
				continue
			}
			out[e] = d
		}
	}
	return out
}
