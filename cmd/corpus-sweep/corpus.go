package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// corpusFile is one resolved, on-disk file to sweep.
type corpusFile struct {
	AbsPath string // absolute path on disk
	RelPath string // repo-root-relative path (stable across machines)
	Tier    string // "A" | "B" | "C"
}

// manifest mirrors the committed core/formats/<id>/corpus.yaml shape
// (docs/internals/format-maturity.md §2.5). Only the fields the sweep needs are
// decoded; the manifest is never mutated by the harness.
type manifest struct {
	Format  string `yaml:"format"`
	Entries []struct {
		Path string `yaml:"path"`
		Tier string `yaml:"tier"`
	} `yaml:"entries"`
}

// findRepoRoot walks up from cwd to the directory containing go.work (the
// workspace root that all root-relative manifest paths resolve against),
// mirroring core/format/spec.findRepoRoot.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repo root (go.work) walking up from %s", dir)
		}
		dir = parent
	}
}

// readManifest parses a corpus.yaml at the given path. A missing manifest is
// not an error — it returns an empty entry list so a format with no manifest
// simply contributes no files.
func readManifest(path string) (manifest, error) {
	var m manifest
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return m, fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

// enumerate returns the files to sweep for one format plus whether the Tier B
// set was empty. Tier B files (origin url/archive-member/bug, fetched into
// corpus/<version>/<id>/) are preferred; when none are present on disk — the
// state until `make fetch-corpus` has a release — it falls back to the Tier A
// committed testdata as a smoke corpus and reports tierBEmpty=true so the
// caller can say so honestly. Tier C entries are generator/seed specs, not
// files to execute, and are skipped.
func enumerate(repoRoot, formatID string) (files []corpusFile, tierBEmpty bool, err error) {
	manifestPath := filepath.Join(repoRoot, "core", "formats", formatID, "corpus.yaml")
	m, err := readManifest(manifestPath)
	if err != nil {
		return nil, true, err
	}
	var tierB, tierA []corpusFile
	for _, e := range m.Entries {
		abs := filepath.Join(repoRoot, filepath.FromSlash(e.Path))
		switch e.Tier {
		case "B":
			if fileExists(abs) {
				tierB = append(tierB, corpusFile{AbsPath: abs, RelPath: e.Path, Tier: "B"})
			}
		case "A":
			if fileExists(abs) {
				tierA = append(tierA, corpusFile{AbsPath: abs, RelPath: e.Path, Tier: "A"})
			}
		}
	}
	if len(tierB) > 0 {
		sortFiles(tierB)
		return tierB, false, nil
	}
	sortFiles(tierA)
	return tierA, true, nil
}

func sortFiles(fs []corpusFile) {
	sort.Slice(fs, func(i, j int) bool { return fs[i].RelPath < fs[j].RelPath })
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// corpusTagPrefix is the release-tag prefix of the Tier B corpus store
// (release format-corpus-vN), mirroring core/format/spec.
const corpusTagPrefix = "format-corpus-v"

// findCorpusRoot returns the lexically-latest format-corpus-v* dir under
// <repoRoot>/corpus, or an error when none is staged (the state until
// `make fetch-corpus` has a release). Informational only — Tier B files
// resolve from their root-relative manifest paths, not from this root.
func findCorpusRoot(repoRoot string) (string, error) {
	base := filepath.Join(repoRoot, "corpus")
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", err
	}
	var latest string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), corpusTagPrefix) && e.Name() > latest {
			latest = e.Name()
		}
	}
	if latest == "" {
		return "", fmt.Errorf("no %s* dir under %s", corpusTagPrefix, base)
	}
	return filepath.Join(base, latest), nil
}

// manifestFormats returns every format id with a corpus.yaml under
// core/formats, sorted. Used when the driver is asked to sweep "all".
func manifestFormats(repoRoot string) ([]string, error) {
	base := filepath.Join(repoRoot, "core", "formats")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", base, err)
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if fileExists(filepath.Join(base, e.Name(), "corpus.yaml")) {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}
