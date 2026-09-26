package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	jsonformat "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory/kmb"
)

// Unit is one reviewed source and target pair the evaluation checks.
type Unit struct {
	Corpus     string
	ID         string
	Target     model.LocaleID
	Source     []model.Run
	TargetRuns []model.Run
}

// loadCorpus reads the three corpora the research pilot measured: the dogfood
// project's Norwegian content memory (a frozen copy under testdata/dogfood,
// taken when the project's context moved to refs/kapi/context), the compass sample's catalogs in
// Norwegian, German and Dutch, and the tidewatch sample's Norwegian content
// memory. root is the repository root.
func loadCorpus(root string) ([]Unit, error) {
	var units []Unit
	dogfood, err := loadMemoryUnits("dogfood", filepath.Join(root, "scripts", "termeval", "testdata", "dogfood", "memory", "*-nb.memory.json"), "en", "nb")
	if err != nil {
		return nil, err
	}
	units = append(units, dogfood...)

	locales := filepath.Join(root, "samples", "compass", "site", "locales")
	for _, lang := range []model.LocaleID{"nb", "de", "nl"} {
		compass, err := loadCatalogUnits("compass", filepath.Join(locales, "en-GB.json"), filepath.Join(locales, string(lang)+".json"), lang)
		if err != nil {
			return nil, err
		}
		units = append(units, compass...)
	}

	tidewatch, err := loadMemoryUnits("tidewatch", filepath.Join(root, "samples", "tidewatch-docs", "context", "memory", "tidewatch-nb.memory.json"), "en-GB", "nb")
	if err != nil {
		return nil, err
	}
	return append(units, tidewatch...), nil
}

// loadMemoryUnits reads every entry of the content-memory bundles matching
// pattern that carries content in both src and tgt.
func loadMemoryUnits(corpus, pattern string, src, tgt model.LocaleID) ([]Unit, error) {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("%s: no memory bundle matches %s", corpus, pattern)
	}
	sort.Strings(paths)

	var units []Unit
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		file, err := kmb.Decode(f)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, e := range file.Entries {
			source, target := e.Variants[string(src)], e.Variants[string(tgt)]
			if !model.RunsHaveContent(source) || !model.RunsHaveContent(target) {
				continue
			}
			units = append(units, Unit{Corpus: corpus, ID: e.ID, Target: tgt, Source: source, TargetRuns: target})
		}
	}
	return units, nil
}

// loadCatalogUnits pairs a source catalog with a target catalog by key path,
// reading both through the JSON format reader so each string carries the runs
// a check sees.
func loadCatalogUnits(corpus, srcPath, tgtPath string, tgt model.LocaleID) ([]Unit, error) {
	source, err := readCatalog(srcPath)
	if err != nil {
		return nil, err
	}
	target, err := readCatalog(tgtPath)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(source))
	for k := range source {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var units []Unit
	for _, k := range keys {
		t, ok := target[k]
		if !ok || !model.RunsHaveContent(source[k]) || !model.RunsHaveContent(t) {
			continue
		}
		units = append(units, Unit{Corpus: corpus, ID: k, Target: tgt, Source: source[k], TargetRuns: t})
	}
	return units, nil
}

// readCatalog returns the translatable strings of a JSON catalog, keyed by the
// dotted key path the reader names each block with.
func readCatalog(path string) (map[string][]model.Run, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	r := jsonformat.NewReader()
	if err := r.Open(ctx, &model.RawDocument{URI: path, FormatID: "json", Reader: f}); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = r.Close() }()

	out := map[string][]model.Run{}
	var readErr error
	for res := range r.Read(ctx) {
		if res.Error != nil && readErr == nil {
			readErr = fmt.Errorf("read %s: %w", path, res.Error)
		}
		if res.Part == nil || res.Part.Type != model.PartBlock {
			continue
		}
		if b, ok := res.Part.Resource.(*model.Block); ok && b.Translatable {
			out[b.Name] = b.Source
		}
	}
	return out, readErr
}
