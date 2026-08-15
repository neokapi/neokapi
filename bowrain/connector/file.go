package connector

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	venueconn "github.com/neokapi/neokapi/core/venue/connector"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// FileConnector reads and writes localization content from the filesystem
// using the format registry for format detection and parsing.
type FileConnector struct {
	id             string
	name           string
	basePath       string
	formatRegistry *registry.FormatRegistry
	config         map[string]string
}

// NewFileConnector creates a new FileConnector.
func NewFileConnector(formatReg *registry.FormatRegistry, config map[string]string) (*FileConnector, error) {
	basePath := config["path"]
	if basePath == "" {
		basePath = "."
	}
	id := config["id"]
	if id == "" {
		id = "file-" + filepath.Base(basePath)
	}
	name := config["name"]
	if name == "" {
		name = "File: " + basePath
	}

	return &FileConnector{
		id:             id,
		name:           name,
		basePath:       basePath,
		formatRegistry: formatReg,
		config:         config,
	}, nil
}

func (c *FileConnector) ID() string                   { return c.id }
func (c *FileConnector) Name() string                 { return c.name }
func (c *FileConnector) Category() venueconn.Category { return venueconn.CategoryFile }

// resolve joins a connector-relative path onto the connector root and returns
// the result only if it stays inside that root.
//
// The paths that reach here are content, not configuration: an item's Name and
// Path are whatever the ingest side recorded, and on the delivery side they
// decide which file gets written. A name carrying ".." would otherwise resolve
// to a real location outside the checkout the connector was pointed at, and
// MkdirAll would create the directories to reach it. Containment is checked
// after the join, so the answer accounts for what Join actually produced —
// the same idiom serveSPAFile uses.
func (c *FileConnector) resolve(rel string) (string, error) {
	if rel == "" {
		return "", errors.New("empty connector-relative path")
	}
	// Refused rather than reinterpreted. filepath.Join would absorb the leading
	// separator and quietly turn "/etc/passwd" into "<root>/etc/passwd", which
	// is contained but is not what the caller asked for; a connector-relative
	// path is never absolute, so this is a malformed input, not a location.
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") || filepath.VolumeName(rel) != "" {
		return "", fmt.Errorf("path %q is not connector-relative", rel)
	}

	base, err := filepath.Abs(c.basePath)
	if err != nil {
		return "", fmt.Errorf("resolve connector root: %w", err)
	}
	p := filepath.Join(base, filepath.Clean(rel))
	if p != base && !strings.HasPrefix(p, base+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q resolves outside the connector root", rel)
	}
	return p, nil
}

func (c *FileConnector) Configure(config map[string]string) error {
	maps.Copy(c.config, config)
	if p, ok := config["path"]; ok {
		c.basePath = p
	}
	return nil
}

func (c *FileConnector) Close() error { return nil }

// Fetch reads files from the filesystem, parses them with format readers,
// and returns ContentItems containing the extracted blocks.
func (c *FileConnector) Fetch(ctx context.Context, opts platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	paths := opts.Paths
	if len(paths) == 0 {
		// Discover all files if no specific paths given.
		items, err := c.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			paths = append(paths, item.Path)
		}
	}

	var result []*platconn.ContentItem
	for _, p := range paths {
		abs, err := c.resolve(p)
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", p, err)
		}
		item, err := c.fetchFile(ctx, abs)
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", p, err)
		}
		if item != nil {
			result = append(result, item)
		}
	}
	return result, nil
}

func (c *FileConnector) fetchFile(ctx context.Context, path string) (*platconn.ContentItem, error) {
	ext := filepath.Ext(path)
	detector := c.formatRegistry.Detector()
	formatName, err := detector.DetectByExtension(ext)
	if err != nil {
		return nil, fmt.Errorf("detect format for %s: %w", path, err)
	}

	reader, err := c.formatRegistry.NewReader(registry.FormatID(formatName))
	if err != nil {
		return nil, fmt.Errorf("create reader for %s: %w", formatName, err)
	}
	defer reader.Close()

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	doc := &model.RawDocument{
		URI:      path,
		FormatID: formatName,
		Reader:   f,
	}
	if err := reader.Open(ctx, doc); err != nil {
		f.Close()
		return nil, fmt.Errorf("open document %s: %w", path, err)
	}

	var blocks []*model.Block
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return nil, fmt.Errorf("read %s: %w", path, pr.Error)
		}
		if pr.Part != nil && pr.Part.Type == model.PartBlock {
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		}
	}

	relPath, _ := filepath.Rel(c.basePath, path)
	info, _ := os.Stat(path)
	var lastChanged time.Time
	if info != nil {
		lastChanged = info.ModTime()
	}

	// No absolute path in the metadata: a content item travels out through the
	// connector-content API, which is gated by workspace membership alone, and
	// the host's directory layout is not part of what a workspace member asked
	// for. The connector-relative path is what a caller can act on anyway.
	return &platconn.ContentItem{
		ID:          relPath,
		Name:        filepath.Base(path),
		Path:        relPath,
		Format:      formatName,
		Blocks:      blocks,
		LastChanged: lastChanged,
	}, nil
}

// Publish writes translated content items back to the filesystem.
func (c *FileConnector) Publish(ctx context.Context, items []*platconn.ContentItem, opts platconn.PublishOptions) error {
	for _, item := range items {
		if err := c.publishFile(ctx, item, opts); err != nil {
			return fmt.Errorf("publish %s: %w", item.Path, err)
		}
	}
	return nil
}

func (c *FileConnector) publishFile(ctx context.Context, item *platconn.ContentItem, opts platconn.PublishOptions) error {
	// item.Path is content the connector is delivering, not a location the
	// connector chose, so it decides the write target only within the root.
	path, err := c.resolve(item.Path)
	if err != nil {
		return err
	}

	writer, err := c.formatRegistry.NewWriter(registry.FormatID(item.Format))
	if err != nil {
		return fmt.Errorf("create writer for %s: %w", item.Format, err)
	}
	defer writer.Close()

	// Delivering to a new locale directory (values-fr/, locales/fr/, …) must not
	// fail because the directory does not exist yet — the local merge path
	// (host/merge.go) creates it too.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output dir for %s: %w", path, err)
	}

	if err := writer.SetOutput(path); err != nil {
		return fmt.Errorf("set output %s: %w", path, err)
	}

	// Faithful re-parse delivery. A skeleton-dependent format (Android
	// strings.xml, HTML, XML, …) reconstructs its non-translatable frame —
	// comments, attribute order, non-translatable entries, element ordering — only
	// from a skeleton captured off the SOURCE document; reconstructing from stored
	// blocks alone loses that frame. When the untranslated source file is
	// co-located on disk — the forge/git delivery runs inside the tracked-branch
	// checkout, and the plain file connector shares one root — re-read it to
	// capture that skeleton and splice the reviewed targets back into it, exactly
	// as the local `kapi merge` roundtrip does (host/merge.go
	// writeMergedSourceWithSkeleton). Bowrain's store stays format-agnostic and
	// skeleton-free; faithfulness is reconstructed at the edge from the source
	// that is already on disk. Pure-structure formats (json/yaml/arb/po/…) whose
	// block set fully determines the file produce byte-identical output either way
	// (verified), so this is transparent for them.
	if consumer, ok := writer.(format.SkeletonStoreConsumer); ok && item.Locale != "" {
		if blocks, store, ok := c.reparseSourceSkeleton(ctx, item); ok {
			defer store.Close()
			writer.SetLocale(item.Locale)
			consumer.SetSkeletonStore(store)
			return writeBlockParts(ctx, writer, blocks)
		}
	}

	// Default: reconstruct from the delivered blocks (target already promoted to
	// the source position by the caller). Used for pure-structure formats and
	// whenever no co-located source is available.
	return writeBlockParts(ctx, writer, item.Blocks)
}

// writeBlockParts feeds a block slice through a writer's Part channel.
func writeBlockParts(ctx context.Context, writer format.DataFormatWriter, blocks []*model.Block) error {
	ch := make(chan *model.Part, len(blocks))
	for _, b := range blocks {
		ch <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(ch)
	return writer.Write(ctx, ch)
}

// reparseSourceSkeleton re-reads the delivery item's co-located source document
// and returns the source blocks — with the item's reviewed targets spliced onto
// them by block id — plus the source-captured skeleton store, ready to hand to a
// skeleton-consuming writer. Every source block is returned (not just the
// reviewed subset), so an untranslated entry delivers its original source text
// rather than an empty element, matching the local merge.
//
// It returns ok=false (caller falls back to the from-blocks path) when there is
// no co-located source, the format's reader emits no skeleton, or the source
// cannot be read — so a missing source never fails a delivery, it just degrades
// to the previous behaviour.
func (c *FileConnector) reparseSourceSkeleton(ctx context.Context, item *platconn.ContentItem) ([]*model.Block, *format.SkeletonStore, bool) {
	sourcePath := c.resolveSourcePath(item)
	if sourcePath == "" {
		return nil, nil, false
	}

	reader, err := c.formatRegistry.NewReader(registry.FormatID(item.Format))
	if err != nil {
		return nil, nil, false
	}
	defer reader.Close()
	emitter, ok := reader.(format.SkeletonStoreEmitter)
	if !ok {
		return nil, nil, false
	}

	f, err := os.Open(sourcePath)
	if err != nil {
		return nil, nil, false
	}
	defer f.Close()

	store := format.NewMemorySkeletonStore()
	emitter.SetSkeletonStore(store)

	doc := &model.RawDocument{
		URI:          sourcePath,
		FormatID:     item.Format,
		Reader:       f,
		TargetLocale: item.Locale,
	}
	if err := reader.Open(ctx, doc); err != nil {
		return nil, nil, false
	}
	var sourceBlocks []*model.Block
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return nil, nil, false
		}
		if pr.Part != nil && pr.Part.Type == model.PartBlock {
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				sourceBlocks = append(sourceBlocks, b)
			}
		}
	}
	if err := store.Flush(); err != nil {
		return nil, nil, false
	}
	// A reader that registered as an emitter but wrote no skeleton (a stub, or a
	// document with no reconstructable frame) offers nothing over the from-blocks
	// path — fall back rather than emit an empty document.
	if store.EntriesWritten() == 0 {
		return nil, nil, false
	}

	// Splice the reviewed targets onto the re-read source blocks by block id. The
	// delivered blocks carry the source-reader id (restored by the delivery
	// materializer), which matches the ids the freshly-parsed source assigns and
	// the skeleton refs, so the writer resolves each ref to the reviewed target.
	reviewed := make(map[string][]model.Run, len(item.Blocks))
	for _, rb := range item.Blocks {
		if runs := deliveredRuns(rb, item.Locale); len(runs) > 0 {
			reviewed[rb.ID] = runs
		}
	}
	matched := 0
	for _, sb := range sourceBlocks {
		if runs, ok := reviewed[sb.ID]; ok {
			sb.SetTargetRuns(item.Locale, runs)
			matched++
		}
	}
	// If there are reviewed targets but none bound to the re-parsed source (the
	// source-reader ids are absent or have drifted), fall back to the from-blocks
	// path: a faithful frame full of untranslated source text is a worse delivery
	// than the translated-but-lossy output. A genuinely untranslated locale
	// (reviewed empty) still delivers the faithful frame with source text — correct.
	if len(reviewed) > 0 && matched == 0 {
		store.Close()
		return nil, nil, false
	}
	return sourceBlocks, store, true
}

// resolveSourcePath locates the item's untranslated source document under the
// connector root. The server-side delivery materializer stamps an explicit
// source_path (the source-relative path the target path was derived from); the
// store item name is that same relative path, so it is the fallback. The
// delivery target path (item.Path) is never treated as the source.
//
// Both candidates are connector-relative and are resolved as such. An absolute
// source_path used to be honoured outright, which let item metadata name any
// readable file on the host and have its content parsed into the delivery; the
// materializer has no reason to emit one, and the connector has no business
// reading outside the root it was pointed at.
func (c *FileConnector) resolveSourcePath(item *platconn.ContentItem) string {
	candidate := item.Metadata["source_path"]
	if candidate == "" {
		candidate = item.Name
	}
	if candidate == "" {
		return ""
	}
	abs, err := c.resolve(candidate)
	if err != nil || !isRegularFile(abs) {
		return ""
	}
	return abs
}

func isRegularFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// deliveredRuns returns the reviewed target runs a delivery block carries for the
// locale: the committed target when present, else the block's source runs (the
// delivery materializer promotes the reviewed target into the source position,
// so a promoted block's source already holds the reviewed text).
func deliveredRuns(b *model.Block, locale model.LocaleID) []model.Run {
	if t := b.Target(locale); t != nil && len(t.Runs) > 0 {
		return t.Runs
	}
	return b.SourceRuns()
}

// List scans the base directory for files whose extensions match known formats.
func (c *FileConnector) List(ctx context.Context) ([]*platconn.ContentItem, error) {
	detector := c.formatRegistry.Detector()
	var items []*platconn.ContentItem

	err := filepath.Walk(c.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		ext := filepath.Ext(path)
		formatName, detectErr := detector.DetectByExtension(ext)
		if detectErr != nil {
			return nil // Skip unrecognized files.
		}
		relPath, _ := filepath.Rel(c.basePath, path)
		items = append(items, &platconn.ContentItem{
			ID:          relPath,
			Name:        info.Name(),
			Path:        relPath,
			Format:      formatName,
			LastChanged: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", c.basePath, err)
	}
	return items, nil
}

// Status returns the current sync status.
func (c *FileConnector) Status(ctx context.Context) (*venueconn.SyncStatus, error) {
	items, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	return &venueconn.SyncStatus{
		ConnectorID: c.id,
		LastSync:    time.Now(),
		ItemCount:   len(items),
	}, nil
}
