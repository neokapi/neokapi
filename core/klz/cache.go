package klz

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/klz/internal/db"
)

// CacheRoot returns the per-user cache directory neokapi uses for
// .klz runtime cache entries. Re-exported from internal/db so the
// kapi cache CLI doesn't have to import the internal package.
func CacheRoot() string { return db.CacheRoot() }

// CacheEntryDir returns the cache directory for a given manifest
// hash, sharded by the first two hex characters.
func CacheEntryDir(manifestHash string) string { return db.EntryDir(manifestHash) }

// WarmCache eagerly builds the runtime cache entry for this archive.
// When the klzcache build tag is set, the cache is populated and
// subsequent queries (BlockByID, SimilarSources, TM) hit it in O(ms);
// when the tag is unset, the stub returns a deferred error.
func (r *Reader) WarmCache(ctx context.Context) error {
	src, err := r.buildSource()
	if err != nil {
		return err
	}
	return db.Build(ctx, r.ManifestHash(), src)
}

// CacheEntries returns the on-disk cache entries. With the klzcache
// tag enabled, this walks the sharded directory tree and reports
// each entry's size and atime/mtime for `kapi cache info`.
func CacheEntries() ([]db.Entry, error) { return db.Entries() }

// CacheGC evicts cache entries to bring the total on-disk size under
// maxBytes, using LRU ordering. maxAge > 0 additionally prunes any
// entry whose mtime is older than the cutoff.
func CacheGC(ctx context.Context, maxBytes int64, maxAge time.Duration) (db.GCReport, error) {
	return db.GC(ctx, maxBytes, maxAge)
}

// openCache returns the cache handle for this archive, opening an
// existing on-disk entry, building one on first query if needed, or
// returning the noop cache on non-tagged builds.
func (r *Reader) openCache(ctx context.Context) (db.Cache, error) {
	cache, err := db.Open(ctx, r.ManifestHash())
	if err != nil {
		return nil, err
	}
	return cache, nil
}

// buildSource materializes the db.Source iterator the cache builder
// consumes. Runs and metadata are serialized once per block.
func (r *Reader) buildSource() (db.Source, error) {
	docs, err := r.Documents()
	if err != nil {
		return nil, err
	}
	src := &readerSource{locale: r.Manifest().Project.SourceLocale}
	for _, f := range docs {
		for _, d := range f.Documents {
			for i := range d.Blocks {
				row, err := blockRowFor(&d, &d.Blocks[i])
				if err != nil {
					return nil, err
				}
				src.blocks = append(src.blocks, row)
				for loc, runs := range d.Blocks[i].Targets {
					tRow, err := targetRowFor(d.Blocks[i].ID, loc, runs)
					if err != nil {
						return nil, err
					}
					src.targets = append(src.targets, tRow)
				}
			}
		}
	}
	// Target overlays stored under targets/<locale>/ land here too.
	for _, p := range r.Manifest().Parts {
		if p.Role != RoleTarget {
			continue
		}
		overlay, err := r.ReadDocument(p.Path)
		if err != nil {
			return nil, err
		}
		for _, od := range overlay.Documents {
			for _, b := range od.Blocks {
				for loc, runs := range b.Targets {
					tRow, err := targetRowFor(b.ID, loc, runs)
					if err != nil {
						return nil, err
					}
					src.targets = append(src.targets, tRow)
				}
			}
		}
	}
	return src, nil
}

func blockRowFor(doc *klf.Document, b *klf.Block) (db.BlockRow, error) {
	sourceJSON, err := json.Marshal(b.Source)
	if err != nil {
		return db.BlockRow{}, err
	}
	optional, required := 0, 0
	for _, p := range b.Placeholders {
		if p.Optional {
			optional++
		} else {
			required++
		}
	}
	return db.BlockRow{
		ID:                   b.ID,
		DocumentPath:         doc.Path,
		Hash:                 b.Hash,
		Type:                 string(b.Type),
		Component:            b.Properties.Component,
		JSXPath:              b.Properties.JSXPath,
		OptionalPlaceholders: optional,
		RequiredPlaceholders: required,
		SourceText:           flattenRuns(b.Source),
		SourceJSON:           string(sourceJSON),
		Context:              b.Properties.JSXPath,
	}, nil
}

func targetRowFor(blockID, locale string, runs []klf.Run) (db.TargetRow, error) {
	j, err := json.Marshal(runs)
	if err != nil {
		return db.TargetRow{}, err
	}
	return db.TargetRow{
		BlockID:    blockID,
		Locale:     locale,
		TargetJSON: string(j),
		Status:     "translated",
		Origin:     "import",
	}, nil
}

// flattenRuns returns a plain text representation of a run sequence
// suitable for FTS5 indexing and LLM prompts. Placeholders are
// substituted with their equiv wrapped in braces; paired codes
// contribute their inner text only.
func flattenRuns(runs []klf.Run) string {
	var b strings.Builder
	walkFlatten(&b, runs)
	return b.String()
}

func walkFlatten(b *strings.Builder, runs []klf.Run) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			b.WriteString(r.Text.Text)
		case r.Ph != nil:
			b.WriteString("{")
			b.WriteString(r.Ph.Equiv)
			b.WriteString("}")
		case r.Sub != nil:
			b.WriteString("[")
			b.WriteString(r.Sub.Equiv)
			b.WriteString("]")
		case r.Plural != nil:
			if form, ok := r.Plural.Forms[klf.PluralOther]; ok {
				walkFlatten(b, form)
			} else {
				for _, form := range r.Plural.Forms {
					walkFlatten(b, form)
					break
				}
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				walkFlatten(b, form)
			} else {
				for _, form := range r.Select.Cases {
					walkFlatten(b, form)
					break
				}
			}
		}
	}
}

// readerSource wires a prepared list of BlockRow/TargetRow into the
// db.Source interface the cache builder consumes.
type readerSource struct {
	locale  string
	blocks  []db.BlockRow
	targets []db.TargetRow
}

func (s *readerSource) Blocks() ([]db.BlockRow, error)   { return s.blocks, nil }
func (s *readerSource) Targets() ([]db.TargetRow, error) { return s.targets, nil }
func (s *readerSource) SourceLocale() string             { return s.locale }

// compile-time check: readerSource satisfies db.Source.
var _ db.Source = (*readerSource)(nil)
