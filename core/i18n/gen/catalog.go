package gen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/formats/mo"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// CatalogCollections names the recipe collections whose committed translated
// JSON compiles into the gettext MO catalogs the binary embeds. Everything
// else about them — source document, reader configuration, target pattern —
// is read from the recipe, so the compiler and the pull that writes those
// targets cannot disagree about what a catalog entry is.
var CatalogCollections = []string{"neokapi-engine", "neokapi-cli"}

// langMark stands in for {lang} while the compiler is still looking for which
// locales have a committed catalog. A NUL byte cannot occur in a path, so
// splitting the expanded target on it recovers the pattern's two fixed halves.
const langMark = "\x00lang\x00"

// CatalogSummary reports one compiled MO catalog.
type CatalogSummary struct {
	// Collection is the recipe collection the catalog belongs to.
	Collection string
	// Locale is the target language the catalog carries.
	Locale model.LocaleID
	// Path is the written MO file, relative to the repo root.
	Path string
	// Translated counts the entries the MO carries.
	Translated int
	// Pending counts source strings the locale has no translation for. It is
	// reported, never enforced: target-language drift is the ordinary state of
	// a repository whose source moves, not a build failure.
	Pending int
}

// CompileCatalogs compiles the committed translated JSON catalogs named by
// CatalogCollections into the sibling MO files that core/i18n and host/i18n
// embed. root is the repository root — the directory holding kapi.yaml.
//
// Pairing is by block name: the JSON reader, configured exactly as the recipe
// configures it, derives a key path for every translatable string, and that
// key path is both the runtime lookup scope and the MO msgctxt. A source
// string the translated catalog does not carry — or carries unchanged — is
// left out of the MO, so the runtime falls back to the English source.
//
// Progress is written to log. The only errors returned are structural: a
// missing recipe, a collection the recipe does not declare, an unreadable
// source document.
func CompileCatalogs(root string, log io.Writer) ([]CatalogSummary, error) {
	return CompileCollections(root, CatalogCollections, log)
}

// CompileCollections is CompileCatalogs over an explicit set of collection
// names.
func CompileCollections(root string, collections []string, log io.Writer) ([]CatalogSummary, error) {
	recipePath := filepath.Join(root, "kapi.yaml")
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load recipe %s: %w", recipePath, err)
	}

	var summaries []CatalogSummary
	for _, name := range collections {
		coll := findCollection(proj, name)
		if coll == nil {
			return nil, fmt.Errorf("recipe %s declares no collection %q", recipePath, name)
		}
		s, err := compileCollection(root, proj, coll, log)
		if err != nil {
			return nil, fmt.Errorf("collection %s: %w", name, err)
		}
		summaries = append(summaries, s...)
	}
	for _, s := range summaries {
		fmt.Fprintf(log, "i18n-catalogs: %s — %d translated, %d awaiting translation\n",
			s.Path, s.Translated, s.Pending)
	}
	return summaries, nil
}

func findCollection(proj *project.KapiProject, name string) *project.Collection {
	for i := range proj.Collections {
		if proj.Collections[i].Name == name {
			return &proj.Collections[i]
		}
	}
	return nil
}

// catalog accumulates the entries destined for one MO file. Several source
// documents may feed one catalog when the collection's target names no
// per-file token.
type catalog struct {
	locale  model.LocaleID
	blocks  []*model.Block
	pending int
}

func compileCollection(
	root string,
	proj *project.KapiProject,
	coll *project.Collection,
	log io.Writer,
) ([]CatalogSummary, error) {
	catalogs := map[string]*catalog{}
	var order []string

	for _, item := range coll.EffectiveItems() {
		cfgValues, err := readerConfig(proj, &item)
		if err != nil {
			return nil, err
		}
		sources, err := expandSources(root, item.Path)
		if err != nil {
			return nil, err
		}
		declared := item.ResolvedTargetLanguages(coll, proj.Defaults)

		for _, src := range sources {
			entries, err := readEntries(filepath.Join(root, filepath.FromSlash(src)), cfgValues)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", src, err)
			}
			// ResolveTargetPath names a file to write, so it returns an
			// OS-native path. Everything below treats the pattern as slash
			// form — it is split, rejoined with a locale, logged, and reported
			// as a repo-relative path, and each read converts back with
			// FromSlash — so the one native form entering is normalized here.
			pattern := filepath.ToSlash(
				project.ResolveTargetPath(item.Path, item.Base, item.Target, src, langMark))
			prefix, suffix, ok := strings.Cut(pattern, langMark)
			if !ok {
				return nil, fmt.Errorf("target %q names no {lang}: one catalog per locale needs it", item.Target)
			}
			locales, err := discoverLocales(root, prefix, suffix)
			if err != nil {
				return nil, err
			}
			for _, locale := range declared {
				if !slices.Contains(locales, locale) {
					fmt.Fprintf(log, "i18n-catalogs: %s%s%s is not committed yet — %s falls back to source\n",
						prefix, locale, suffix, locale)
				}
			}
			for _, locale := range locales {
				jsonPath := prefix + string(locale) + suffix
				translated, err := readEntries(filepath.Join(root, filepath.FromSlash(jsonPath)), cfgValues)
				if err != nil {
					return nil, fmt.Errorf("read %s: %w", jsonPath, err)
				}
				byName := make(map[string]string, len(translated))
				for _, e := range translated {
					if _, seen := byName[e.name]; !seen {
						byName[e.name] = e.text
					}
				}

				moPath := strings.TrimSuffix(jsonPath, filepath.Ext(jsonPath)) + ".mo"
				cat := catalogs[moPath]
				if cat == nil {
					cat = &catalog{locale: locale}
					catalogs[moPath] = cat
					order = append(order, moPath)
				}
				for _, e := range entries {
					tgt := byName[e.name]
					if tgt == "" || tgt == e.text {
						cat.pending++
						continue
					}
					b := model.NewBlock(e.name, e.text)
					b.Name = e.name
					b.SetTargetText(locale, tgt)
					cat.blocks = append(cat.blocks, b)
				}
			}
		}
	}

	summaries := make([]CatalogSummary, 0, len(order))
	for _, moPath := range order {
		cat := catalogs[moPath]
		data, err := encodeMO(cat)
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", moPath, err)
		}
		if err := writeIfChanged(filepath.Join(root, filepath.FromSlash(moPath)), data); err != nil {
			return nil, err
		}
		summaries = append(summaries, CatalogSummary{
			Collection: coll.Name,
			Locale:     cat.locale,
			Path:       moPath,
			Translated: len(cat.blocks),
			Pending:    cat.pending,
		})
	}
	return summaries, nil
}

// readerConfig returns the JSON reader values for an item: the project's
// per-format defaults with the item's own format config layered on top, which
// is the same precedence every other run of this content sees.
func readerConfig(proj *project.KapiProject, item *project.ContentItem) (map[string]any, error) {
	if item.Format == nil || item.Format.Name != "json" {
		return nil, fmt.Errorf("path %s: an embedded catalog is compiled from a json source", item.Path)
	}
	if item.Format.Preset != "" {
		return nil, fmt.Errorf("path %s: format preset %q is not resolved by the catalog compiler",
			item.Path, item.Format.Preset)
	}
	values := map[string]any{}
	if d, ok := proj.Defaults.Formats["json"]; ok {
		maps.Copy(values, d.Config)
	}
	maps.Copy(values, item.Format.Config)
	return values, nil
}

// expandSources resolves an item path to the documents it names, in a stable
// order. A path with no glob metacharacter must exist.
func expandSources(root, itemPath string) ([]string, error) {
	full := filepath.Join(root, filepath.FromSlash(itemPath))
	if !strings.ContainsAny(itemPath, "*?[") {
		if _, err := os.Stat(full); err != nil {
			return nil, fmt.Errorf("source %s: %w", itemPath, err)
		}
		return []string{itemPath}, nil
	}
	matches, err := filepath.Glob(full)
	if err != nil {
		return nil, fmt.Errorf("source %s: %w", itemPath, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("source %s matches no file", itemPath)
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		rel, err := filepath.Rel(root, m)
		if err != nil {
			return nil, err
		}
		out = append(out, filepath.ToSlash(rel))
	}
	slices.Sort(out)
	return out, nil
}

// discoverLocales lists the locales that have a committed catalog, by matching
// the target pattern's two fixed halves against the directory.
// The halves and the match are compared in one normalization — the platform's
// own. filepath.Rel returns a native path, so trimming a slash-spelled half off
// it silently matches nothing on a platform whose separator is not "/", which
// reads as a locale having no committed catalog rather than as a path bug.
// Either spelling may arrive here: a target pattern comes back OS-native from
// project.ResolveTargetPath, and slash form is what the compiler passes around.
func discoverLocales(root, prefix, suffix string) ([]model.LocaleID, error) {
	head := filepath.FromSlash(prefix)
	tail := filepath.FromSlash(suffix)
	matches, err := filepath.Glob(filepath.Join(root, head+"*"+tail))
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", prefix+"*"+suffix, err)
	}
	locales := make([]model.LocaleID, 0, len(matches))
	for _, m := range matches {
		rel, err := filepath.Rel(root, m)
		if err != nil {
			return nil, err
		}
		tag := strings.TrimSuffix(strings.TrimPrefix(rel, head), tail)
		if tag == "" || strings.ContainsAny(tag, `/\`) {
			continue
		}
		// The filename was written in the project's declared style, so the tag
		// read back out of it arrives in that style. Canonicalizing here is what
		// lets a posix project's nb_NO catalog answer to the nb-NO its recipe
		// declares; the caller compares these against the declared list.
		locales = append(locales, project.LocaleFromPath(tag))
	}
	slices.Sort(locales)
	return locales, nil
}

// entry is one translatable string in a catalog document: the key path the
// JSON reader derives, and the text at it.
type entry struct {
	name string
	text string
}

// readEntries reads a JSON catalog document with the recipe's reader
// configuration and returns its translatable strings in document order.
func readEntries(path string, cfgValues map[string]any) ([]entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	reader := jsonfmt.NewReader()
	cfg, ok := reader.Config().(*jsonfmt.Config)
	if !ok {
		return nil, errors.New("json reader did not carry a json config")
	}
	if err := cfg.ApplyMap(cfgValues); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	ctx := context.Background()
	doc := &model.RawDocument{
		URI:          "file://" + filepath.ToSlash(path),
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       f,
	}
	if err := reader.Open(ctx, doc); err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	var entries []entry
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			return nil, res.Error
		}
		if res.Part == nil || res.Part.Type != model.PartBlock {
			continue
		}
		block, ok := res.Part.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}
		text := block.SourceText()
		if block.Name == "" || text == "" {
			continue
		}
		entries = append(entries, entry{name: block.Name, text: text})
	}
	return entries, nil
}

// encodeMO runs the MO writer over the catalog's blocks. The writer sorts its
// entries by contextualized msgid and writes no timestamp, so the same inputs
// always produce the same bytes.
func encodeMO(cat *catalog) ([]byte, error) {
	writer := mo.NewWriter()
	var buf bytes.Buffer
	if err := writer.SetOutputWriter(&buf); err != nil {
		return nil, err
	}
	writer.SetLocale(cat.locale)

	parts := make(chan *model.Part, 64)
	go func() {
		defer close(parts)
		for _, b := range cat.blocks {
			parts <- &model.Part{Type: model.PartBlock, Resource: b}
		}
	}()
	if err := writer.Write(context.Background(), parts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeIfChanged leaves an up-to-date file's mtime alone, so a rebuild that
// changes nothing does not invalidate everything downstream of it.
func writeIfChanged(path string, data []byte) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
