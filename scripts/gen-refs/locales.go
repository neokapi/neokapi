package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Locale variants of the reference dataset.
//
// The English dataset is generated from the registries. A locale's variant is
// the same dataset with every string a reference page prints replaced by its
// translation from the catalogs the dogfood loop maintains:
// core/i18n/catalogs/<locale>.json for tools, formats and models, and
// host/i18n/catalogs/<locale>.json for commands. Both mirror the shape of the
// document they translate, so a string is found by walking the same keys. The
// authored dossiers reach a locale the same way the English ones reach the
// dataset: the loop writes each dossier's translation to
// nativedocs/<locale>/<kind>s/<id>.yaml and it is overlaid onto the variant.
//
// A string the catalog lacks stays English and is counted, never failed. A
// target locale that has not caught up with the source is pending work, and a
// variant that keeps English there is what lets the docs site build in every
// locale while the loop catches up.
//
// The variants are written under <out>/<locale>/ with the same file names, so a
// site building one locale swaps the directory and nothing else.

// localeFiles are the datasets that carry prose a reference page prints.
var localeFiles = []string{"tools.json", "formats.json", "commands.json", "models.json"}

// englishDatasets is the committed English dataset, read back from the output
// directory: the variants derive from what is committed, so a run that only
// refreshes the locales (the compile stage of the multilingual pipeline) and a
// full regeneration produce the same bytes from the same tree.
type englishDatasets struct {
	tools    Dataset
	formats  Dataset
	commands CommandDataset
	models   ModelDataset
}

func readEnglishDatasets(outDir string) (*englishDatasets, error) {
	var e englishDatasets
	if err := readJSON(filepath.Join(outDir, "tools.json"), &e.tools); err != nil {
		return nil, err
	}
	if err := readJSON(filepath.Join(outDir, "formats.json"), &e.formats); err != nil {
		return nil, err
	}
	if err := readJSON(filepath.Join(outDir, "commands.json"), &e.commands); err != nil {
		return nil, err
	}
	if err := readJSON(filepath.Join(outDir, "models.json"), &e.models); err != nil {
		return nil, err
	}
	return &e, nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// localeCatalogs holds one locale's translated catalogs, parsed generically.
// Either may be nil when that catalog is not committed for the locale.
type localeCatalogs struct {
	core map[string]any
	cli  map[string]any
}

// discoverLocales lists every locale that has a catalog in any of dirs: the
// basename of each <locale>.json. A directory that does not exist contributes
// nothing.
func discoverLocales(dirs ...string) ([]string, error) {
	seen := map[string]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".json") {
				continue
			}
			seen[strings.TrimSuffix(name, ".json")] = true
		}
	}
	out := make([]string, 0, len(seen))
	for l := range seen {
		out = append(out, l)
	}
	sort.Strings(out)
	return out, nil
}

// parseLocales splits a comma- or space-separated locale list, dropping empty
// items and duplicates. An empty result means "every locale with a catalog".
func parseLocales(spec string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ' ' }) {
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// loadLocaleCatalogs reads a locale's catalogs. A locale with neither catalog
// is an error: there is nothing to derive a variant from.
func loadLocaleCatalogs(coreDir, cliDir, locale string) (*localeCatalogs, error) {
	core, err := readCatalog(filepath.Join(coreDir, locale+".json"))
	if err != nil {
		return nil, err
	}
	cli, err := readCatalog(filepath.Join(cliDir, locale+".json"))
	if err != nil {
		return nil, err
	}
	if core == nil && cli == nil {
		return nil, fmt.Errorf("locale %q has no catalog under %s or %s", locale, coreDir, cliDir)
	}
	return &localeCatalogs{core: core, cli: cli}, nil
}

func readCatalog(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cat map[string]any
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return cat, nil
}

// lookup walks a parsed catalog by key path and returns the string at the leaf.
func lookup(cat map[string]any, path ...string) (string, bool) {
	var cur any = cat
	for _, key := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		cur, ok = m[key]
		if !ok {
			return "", false
		}
	}
	s, ok := cur.(string)
	return s, ok && s != ""
}

// localeCoverage tallies, per locale, the strings a variant translated and the
// ones that kept their English because the catalog had nothing for them.
type localeCoverage struct {
	translated int
	// missing is every string the catalog had nothing for, by key path, so a
	// dossier that later supplies it can take it off the list.
	missing map[string]bool
	// dossiers is how many translated sidecars were overlaid.
	dossiers int
}

func (c *localeCoverage) fallback() int { return len(c.missing) }

// take returns the translation for an English string when the catalog has one,
// and the English string otherwise. An empty English string is nothing to
// translate and is not counted.
func (c *localeCoverage) take(english string, cat map[string]any, path ...string) string {
	if english == "" {
		return english
	}
	if s, ok := lookup(cat, path...); ok {
		c.translated++
		return s
	}
	if c.missing == nil {
		c.missing = map[string]bool{}
	}
	c.missing[strings.Join(path, ".")] = true
	return english
}

// supplied records that a string the catalog lacked came from elsewhere.
func (c *localeCoverage) supplied(path ...string) {
	delete(c.missing, strings.Join(path, "."))
}

// localizeEntries translates the display name and description of every
// built-in entry. Plugin and bridge entries have no catalog and stay as they
// are, uncounted.
func localizeEntries(ds Dataset, cat map[string]any, domain, locale string, cov *localeCoverage) (Dataset, error) {
	out := ds
	out.Entries = make([]Entry, len(ds.Entries))
	for i, e := range ds.Entries {
		if e.Source == SourceBuiltIn {
			e.DisplayName = cov.take(e.DisplayName, cat, domain, e.ID, "displayName")
			e.Description = cov.take(e.Description, cat, domain, e.ID, "description")
			schema, err := localizeSchema(e.Schema, e.Kind, e.ID, cat, locale, cov)
			if err != nil {
				return Dataset{}, err
			}
			e.Schema = schema
		}
		out.Entries[i] = e
	}
	return out, nil
}

// localizeCommands translates each command's short and long help. The CLI
// catalog keys a command by its path under cli.commands.kapi, the same way
// host/i18n/commands.json records it.
func localizeCommands(ds CommandDataset, cat map[string]any, cov *localeCoverage) CommandDataset {
	out := ds
	out.Commands = make([]CommandEntry, len(ds.Commands))
	for i, c := range ds.Commands {
		base := append([]string{"cli", "commands", "kapi"}, c.Path...)
		c.Short = cov.take(c.Short, cat, append(append([]string{}, base...), "short")...)
		c.Long = cov.take(c.Long, cat, append(append([]string{}, base...), "long")...)
		out.Commands[i] = c
	}
	return out
}

// localizeModels translates each model's note. Ids and labels are names.
func localizeModels(ds ModelDataset, cat map[string]any, cov *localeCoverage) ModelDataset {
	out := ds
	out.Models = make([]ModelEntry, len(ds.Models))
	for i, m := range ds.Models {
		m.Note = cov.take(m.Note, cat, "models", m.ID, "note")
		out.Models[i] = m
	}
	return out
}

// localeVariant is one locale's four datasets and what they cover.
type localeVariant struct {
	tools    Dataset
	formats  Dataset
	commands CommandDataset
	models   ModelDataset
	coverage localeCoverage
}

func localizeAll(english *englishDatasets, cats *localeCatalogs, nativeDocs, locale string) (*localeVariant, error) {
	v := &localeVariant{}
	var err error
	if v.tools, err = localizeEntries(english.tools, cats.core, "tools", locale, &v.coverage); err != nil {
		return nil, err
	}
	if v.formats, err = localizeEntries(english.formats, cats.core, "formats", locale, &v.coverage); err != nil {
		return nil, err
	}
	v.commands = localizeCommands(english.commands, cats.cli, &v.coverage)
	v.models = localizeModels(english.models, cats.core, &v.coverage)
	if nativeDocs != "" {
		dir := filepath.Join(nativeDocs, locale)
		if err := overlayLocaleDocs(dir, KindTool, v.tools.Entries, &v.coverage); err != nil {
			return nil, err
		}
		if err := overlayLocaleDocs(dir, KindFormat, v.formats.Entries, &v.coverage); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// overlayLocaleDocs applies the translated dossiers under dir/<kind>s/ to a
// variant's entries, the way the English dossiers are applied to the English
// dataset. A dossier whose id names no entry is the translation of a tool that
// has since been renamed: target drift, reported and skipped, never a failure.
// A file that is not YAML is a defect in what was written and does fail.
func overlayLocaleDocs(dir, kind string, entries []Entry, cov *localeCoverage) error {
	files, err := filepath.Glob(filepath.Join(dir, kind+"s", "*.yaml"))
	if err != nil {
		return err
	}
	byID := make(map[string]int, len(entries))
	for i, e := range entries {
		byID[e.ID] = i
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		var ndf nativeDocFile
		if err := yaml.Unmarshal(data, &ndf); err != nil {
			return fmt.Errorf("parse %s: %w", f, err)
		}
		id := trimSuffix(filepath.Base(f), ".yaml")
		i, ok := byID[id]
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: %s documents no %s in the dataset; skipped\n", f, kind)
			continue
		}
		applyNativeDoc(&entries[i], &ndf)
		// A dossier's name and description outrank the catalog's, as they do
		// in English, so a string the catalog lacked is not missing after all.
		if ndf.DisplayName != "" {
			cov.supplied(kind+"s", id, "displayName")
		}
		if ndf.Description != "" {
			cov.supplied(kind+"s", id, "description")
		}
		cov.dossiers++
	}
	return nil
}

func (v *localeVariant) files() map[string]any {
	return map[string]any{
		"tools.json":    v.tools,
		"formats.json":  v.formats,
		"commands.json": v.commands,
		"models.json":   v.models,
	}
}

// writeLocaleVariants derives <outDir>/<locale>/ for each locale from the
// committed English datasets under outDir, the locale's catalogs and its
// translated dossiers under <nativeDocs>/<locale>/. An empty locale list means
// every locale with a catalog, and then a variant directory for a locale that
// no longer has one is removed.
func writeLocaleVariants(outDir, coreDir, cliDir, nativeDocs string, locales []string) error {
	discover := len(locales) == 0
	if discover {
		var err error
		if locales, err = discoverLocales(coreDir, cliDir); err != nil {
			return err
		}
	}
	english, err := readEnglishDatasets(outDir)
	if err != nil {
		return err
	}
	for _, locale := range locales {
		cats, err := loadLocaleCatalogs(coreDir, cliDir, locale)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: reference locale %s skipped: %v\n", locale, err)
			continue
		}
		v, err := localizeAll(english, cats, nativeDocs, locale)
		if err != nil {
			return err
		}
		dir := filepath.Join(outDir, locale)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for _, name := range localeFiles {
			if err := writeJSON(filepath.Join(dir, name), v.files()[name]); err != nil {
				return err
			}
		}
		reportCoverage(locale, v.coverage)
	}
	if discover {
		if err := pruneLocaleDirs(outDir, locales); err != nil {
			return err
		}
	}
	return nil
}

// reportCoverage says what a locale's variant carries. A locale whose catalog
// has not caught up is reported as a warning and never as a failure.
func reportCoverage(locale string, cov localeCoverage) {
	if cov.fallback() == 0 {
		fmt.Printf("reference locale %s: %d strings translated, %d translated dossiers\n", locale, cov.translated, cov.dossiers)
		return
	}
	keys := make([]string, 0, len(cov.missing))
	for k := range cov.missing {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(os.Stderr, "warning: reference locale %s: %d strings translated, %d fall back to English (pending in the catalog), %d translated dossiers\n  %s\n",
		locale, cov.translated, cov.fallback(), cov.dossiers, strings.Join(keys, " "))
}

// pruneLocaleDirs removes a variant directory whose locale has no catalog any
// more. Only directories are candidates: the English dataset is files.
func pruneLocaleDirs(outDir string, keep []string) error {
	keepSet := map[string]bool{}
	for _, l := range keep {
		keepSet[l] = true
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || keepSet[e.Name()] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(outDir, e.Name())); err != nil {
			return err
		}
		fmt.Printf("removed %s/: no catalog for that locale\n", filepath.Join(outDir, e.Name()))
	}
	return nil
}

// localeVariantDrift regenerates every locale's variant from the committed
// English datasets and catalogs and reports each committed file that differs.
// The generation timestamp is ignored, as it is for the English dataset.
func localeVariantDrift(outDir, coreDir, cliDir, nativeDocs string) []string {
	locales, err := discoverLocales(coreDir, cliDir)
	if err != nil {
		return []string{fmt.Sprintf("cannot list locale catalogs: %v", err)}
	}
	english, err := readEnglishDatasets(outDir)
	if err != nil {
		return []string{fmt.Sprintf("cannot read the committed English dataset: %v", err)}
	}
	var problems []string
	for _, locale := range locales {
		cats, err := loadLocaleCatalogs(coreDir, cliDir, locale)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", locale, err))
			continue
		}
		v, err := localizeAll(english, cats, nativeDocs, locale)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", locale, err))
			continue
		}
		want := v.files()
		for _, name := range localeFiles {
			path := filepath.Join(outDir, locale, name)
			var got any
			switch name {
			case "commands.json":
				got = &CommandDataset{}
			case "models.json":
				got = &ModelDataset{}
			default:
				got = &Dataset{}
			}
			if err := readJSON(path, got); err != nil {
				problems = append(problems, fmt.Sprintf("%s/%s: %v", locale, name, err))
				continue
			}
			if !jsonEqual(clearGeneratedAt(want[name]), clearGeneratedAt(got)) {
				problems = append(problems, fmt.Sprintf("%s/%s is stale against the catalog", locale, name))
			}
		}
	}
	return problems
}

// clearGeneratedAt returns a copy of a dataset with its timestamp blanked, so
// two generations of the same content compare equal. Accepts values and
// pointers, since the committed side is decoded into a pointer.
func clearGeneratedAt(v any) any {
	switch d := v.(type) {
	case Dataset:
		d.GeneratedAt = ""
		return d
	case *Dataset:
		c := *d
		c.GeneratedAt = ""
		return c
	case CommandDataset:
		d.GeneratedAt = ""
		return d
	case *CommandDataset:
		c := *d
		c.GeneratedAt = ""
		return c
	case ModelDataset:
		d.GeneratedAt = ""
		return d
	case *ModelDataset:
		c := *d
		c.GeneratedAt = ""
		return c
	}
	return v
}
