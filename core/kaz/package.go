package kaz

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/gokapi/gokapi/core/model"
	"gopkg.in/yaml.v3"
)

// PackOptions configures how a .kaz package is built.
type PackOptions struct {
	Name          string
	SourceLocale  string
	TargetLocales []string
	Items         []PackItem
	TMData        []byte // JSON-encoded TM entries (optional, stored in tm/entries.json)
	TermsData     []byte // JSON-encoded term concepts (optional, stored in terms/concepts.json)
}

// PackItem represents a single item to include in a .kaz package.
type PackItem struct {
	Name        string        // Item name (e.g., "page.html")
	Type        string        // "file", "data", etc.
	Format      string        // Format identifier (e.g., "html")
	SourceBytes []byte        // Original source bytes (optional)
	Parts       []*model.Part // Parsed Part stream
}

// Package is the in-memory representation of a .kaz package.
type Package struct {
	Manifest  Manifest
	Items     map[string][]byte      // source items (may be empty)
	Blocks    map[string]*BlockIndex // block indices per item
	Previews  map[string]string      // preview HTML per item
	TMData    []byte                 // JSON-encoded TM entries (from tm/entries.json)
	TermsData []byte                 // JSON-encoded term concepts (from terms/concepts.json)
}

// Pack writes a .kaz package to the writer.
func Pack(w io.Writer, opts PackOptions) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	formatSet := make(map[string]bool)
	var items []ItemManifest

	for _, item := range opts.Items {
		formatSet[item.Format] = true

		// Build block index
		blockIndex := BuildBlockIndex(item.Parts, opts.SourceLocale, item.Format, item.Name)

		// Write block index
		bw, err := zw.Create("blocks/" + item.Name + ".json")
		if err != nil {
			return fmt.Errorf("create block index %q: %w", item.Name, err)
		}
		if err := WriteBlockIndex(bw, blockIndex); err != nil {
			return fmt.Errorf("write block index %q: %w", item.Name, err)
		}

		// Build and write preview
		preview := BuildPreview(item.Parts, item.SourceBytes, item.Format, model.LocaleID(opts.SourceLocale))
		pw, err := zw.Create("preview/" + item.Name + ".html")
		if err != nil {
			return fmt.Errorf("create preview %q: %w", item.Name, err)
		}
		if _, err := io.WriteString(pw, preview); err != nil {
			return fmt.Errorf("write preview %q: %w", item.Name, err)
		}

		// Write source item if available
		if len(item.SourceBytes) > 0 {
			sw, err := zw.Create("items/" + item.Name)
			if err != nil {
				return fmt.Errorf("create item %q: %w", item.Name, err)
			}
			if _, err := sw.Write(item.SourceBytes); err != nil {
				return fmt.Errorf("write item %q: %w", item.Name, err)
			}
		}

		// Compute stats
		blockCount, wordCount := countBlockStats(item.Parts)
		itemType := item.Type
		if itemType == "" {
			itemType = "file"
		}

		items = append(items, ItemManifest{
			Path:       item.Name,
			Format:     item.Format,
			Type:       itemType,
			Size:       int64(len(item.SourceBytes)),
			BlockCount: blockCount,
			WordCount:  wordCount,
		})
	}

	var fmts []string
	for f := range formatSet {
		fmts = append(fmts, f)
	}

	manifest := Manifest{
		Name:            opts.Name,
		Version:         "1.0",
		GokapiVersion:   "0.1.0",
		SourceLocale:    opts.SourceLocale,
		TargetLocales:   opts.TargetLocales,
		CreatedAt:       now,
		ModifiedAt:      now,
		FormatsRequired: fmts,
		PluginsRequired: []string{},
		Items:           items,
	}

	manifestData, err := yaml.Marshal(&manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	mw, err := zw.Create("manifest.yaml")
	if err != nil {
		return fmt.Errorf("create manifest: %w", err)
	}
	if _, err := mw.Write(manifestData); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	// Write TM data if present
	if len(opts.TMData) > 0 {
		tw, err := zw.Create("tm/entries.json")
		if err != nil {
			return fmt.Errorf("create tm/entries.json: %w", err)
		}
		if _, err := tw.Write(opts.TMData); err != nil {
			return fmt.Errorf("write tm/entries.json: %w", err)
		}
	} else {
		if _, err := zw.Create("tm/"); err != nil {
			return err
		}
	}

	// Write terms data if present
	if len(opts.TermsData) > 0 {
		tw, err := zw.Create("terms/concepts.json")
		if err != nil {
			return fmt.Errorf("create terms/concepts.json: %w", err)
		}
		if _, err := tw.Write(opts.TermsData); err != nil {
			return fmt.Errorf("write terms/concepts.json: %w", err)
		}
	}

	// Create empty dirs for structure
	if _, err := zw.Create("assets/"); err != nil {
		return err
	}

	return nil
}

// Unpack reads a .kaz package from a reader.
func Unpack(r io.ReaderAt, size int64) (*Package, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	pkg := &Package{
		Items:    make(map[string][]byte),
		Blocks:   make(map[string]*BlockIndex),
		Previews: make(map[string]string),
	}

	var manifestData []byte
	fileContents := make(map[string][]byte)

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open entry %q: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read entry %q: %w", f.Name, err)
		}
		fileContents[f.Name] = data

		switch {
		case f.Name == "manifest.yaml":
			manifestData = data
		case strings.HasPrefix(f.Name, "items/") && !strings.HasSuffix(f.Name, "/"):
			name := strings.TrimPrefix(f.Name, "items/")
			pkg.Items[name] = data
		case strings.HasPrefix(f.Name, "blocks/") && strings.HasSuffix(f.Name, ".json"):
			name := strings.TrimPrefix(f.Name, "blocks/")
			name = strings.TrimSuffix(name, ".json")
			bi, err := ReadBlockIndex(bytes.NewReader(data))
			if err != nil {
				return nil, fmt.Errorf("parse block index %q: %w", f.Name, err)
			}
			pkg.Blocks[name] = bi
		case strings.HasPrefix(f.Name, "preview/") && strings.HasSuffix(f.Name, ".html"):
			name := strings.TrimPrefix(f.Name, "preview/")
			name = strings.TrimSuffix(name, ".html")
			pkg.Previews[name] = string(data)
		case f.Name == "tm/entries.json":
			pkg.TMData = data
		case f.Name == "terms/concepts.json":
			pkg.TermsData = data
		}
	}

	if manifestData == nil {
		return nil, fmt.Errorf("manifest.yaml not found in package")
	}

	if err := yaml.Unmarshal(manifestData, &pkg.Manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	return pkg, nil
}

// UnpackFromBytes is a convenience wrapper around Unpack that accepts a byte slice.
func UnpackFromBytes(data []byte) (*Package, error) {
	return Unpack(bytes.NewReader(data), int64(len(data)))
}

// countBlockStats counts blocks and words in a Part stream.
func countBlockStats(parts []*model.Part) (blockCount, wordCount int) {
	for _, pt := range parts {
		if pt.Type == model.PartBlock {
			block, ok := pt.Resource.(*model.Block)
			if ok {
				blockCount++
				wordCount += countWords(block.SourceText())
			}
		}
	}
	return
}

// countWords counts words in text by splitting on whitespace.
func countWords(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count
}
