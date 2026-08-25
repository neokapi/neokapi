package host

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/version"
)

// cachedPart is one Part with its concrete Resource hoisted into a typed field —
// Part.Resource is an interface, so it is serialized as a tagged union keyed by
// the PartType. Exactly one payload field is set per part. It is the per-part
// record the streaming document cache (docCache) appends to its log.
//
// Blocks and Layers are wrapped (cachedBlock/cachedLayer) because their
// Annotations map holds Payload interface values: raw json.Unmarshal cannot
// rehydrate an interface, so a reader that annotates at parse time (e.g. the
// YAML reader's comment notes) would fail on replay. The wrappers shadow the
// Annotations field with a typed envelope that carries each payload's
// registered type name (map keys are usually — but not always — the type
// name, so the name travels explicitly).
type cachedPart struct {
	Type   model.PartType    `json:"t"`
	Block  *cachedBlock      `json:"b,omitempty"`
	Layer  *cachedLayer      `json:"l,omitempty"`
	Data   *model.Data       `json:"d,omitempty"`
	Media  *model.Media      `json:"m,omitempty"`
	GStart *model.GroupStart `json:"gs,omitempty"`
	GEnd   *model.GroupEnd   `json:"ge,omitempty"`
}

// cachedAnno is the serialized form of one annotation payload: the payload
// registry type name plus the payload's own JSON.
type cachedAnno struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// cachedBlock shadows Block.Annotations with the typed envelope (the embedded
// field is suppressed by the outer one under encoding/json's depth rule).
type cachedBlock struct {
	*model.Block
	Annotations map[string]cachedAnno `json:"Annotations,omitempty"`
}

// cachedLayer is the Layer counterpart of cachedBlock.
type cachedLayer struct {
	*model.Layer
	Annotations map[string]cachedAnno `json:"Annotations,omitempty"`
}

// toCachedAnnotations envelopes a Payload map for serialization. Payloads that
// fail to marshal are dropped (the cache is a regenerable parse accelerator,
// never a source of truth).
func toCachedAnnotations(m map[string]model.Payload) map[string]cachedAnno {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]cachedAnno, len(m))
	for k, p := range m {
		if p == nil {
			continue
		}
		data, err := json.Marshal(p)
		if err != nil {
			continue
		}
		out[k] = cachedAnno{Type: p.TypeName(), Data: data}
	}
	return out
}

// fromCachedAnnotations rehydrates the envelope through the payload registry.
// Entries with an unregistered type name are dropped rather than failing the
// replay — same posture as toCachedAnnotations.
func fromCachedAnnotations(cm map[string]cachedAnno) map[string]model.Payload {
	if len(cm) == 0 {
		return nil
	}
	out := make(map[string]model.Payload, len(cm))
	for k, ca := range cm {
		p, ok := model.NewPayload(ca.Type)
		if !ok {
			continue
		}
		if len(ca.Data) > 0 {
			if err := json.Unmarshal(ca.Data, p); err != nil {
				continue
			}
		}
		out[k] = p
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cacheableMedia returns the Media as the cache must record it. A deferred
// slice — a package reader's zip-backed media — is materialized into a copy,
// because the accessor is bound to the document being parsed and would be dead
// by the time the record is replayed. The copy keeps the live part deferred, so
// only the caching run pays for the bytes.
func cacheableMedia(m *model.Media) *model.Media {
	if m == nil || m.Open == nil || len(m.Data) > 0 {
		return m
	}
	rec := *m
	rec.Open = nil
	if data, err := m.Bytes(); err == nil {
		rec.Data = data
	}
	return &rec
}

func toCachedParts(parts []*model.Part) []cachedPart {
	out := make([]cachedPart, 0, len(parts))
	for _, p := range parts {
		if p == nil {
			continue
		}
		cp := cachedPart{Type: p.Type}
		switch r := p.Resource.(type) {
		case *model.Block:
			cp.Block = &cachedBlock{Block: r, Annotations: toCachedAnnotations(r.AnnoMap())}
		case *model.Layer:
			cp.Layer = &cachedLayer{Layer: r, Annotations: toCachedAnnotations(r.Annotations)}
		case *model.Data:
			cp.Data = r
		case *model.Media:
			cp.Media = cacheableMedia(r)
		case *model.GroupStart:
			cp.GStart = r
		case *model.GroupEnd:
			cp.GEnd = r
		default:
			// A Part type the cache doesn't model (RawDocument/Custom) — skip it.
			// Readers never emit these into the processing stream, so a parsed
			// document never relies on round-tripping them.
			continue
		}
		out = append(out, cp)
	}
	return out
}

func fromCachedParts(cps []cachedPart) []*model.Part {
	out := make([]*model.Part, 0, len(cps))
	for i := range cps {
		cp := cps[i]
		var res model.Resource
		switch cp.Type {
		case model.PartBlock:
			if cp.Block != nil && cp.Block.Block != nil {
				// Through the setter, so an annotation the Block holds in a
				// field rather than the map lands where it belongs.
				cp.Block.Block.Annotations = nil
				for k, v := range fromCachedAnnotations(cp.Block.Annotations) {
					cp.Block.Block.SetAnno(k, v)
				}
				res = cp.Block.Block
			}
		case model.PartLayerStart, model.PartLayerEnd:
			if cp.Layer != nil && cp.Layer.Layer != nil {
				cp.Layer.Layer.Annotations = fromCachedAnnotations(cp.Layer.Annotations)
				res = cp.Layer.Layer
			}
		case model.PartData:
			res = cp.Data
		case model.PartMedia:
			res = cp.Media
		case model.PartGroupStart:
			res = cp.GStart
		case model.PartGroupEnd:
			res = cp.GEnd
		default:
			continue
		}
		if res == nil {
			continue
		}
		out = append(out, &model.Part{Type: cp.Type, Resource: res})
	}
	return out
}

// withParseCache opens the project's streaming document cache for the duration of
// fn, so the read paths (readBlocks) and the flow runner serve unchanged files
// from it. A failure to open is non-fatal — fn still runs, just without caching.
// No-op when root is empty (ad-hoc, no project).
func (a *App) withParseCache(root string, fn func() error) error {
	closeCache := a.openParseCacheDefer(root)
	defer closeCache()
	return fn()
}

// openParseCacheDefer opens the project's document cache and returns a closer to
// defer — the non-closure form of withParseCache, for call sites (the flow
// runner) whose body isn't naturally a single fn. Returns a no-op closer when
// there's no project layout or the cache can't open (parse directly), or when a
// cache is already open. The returned closer is always safe to call.
func (a *App) openParseCacheDefer(root string) func() {
	if root == "" || a.docCache != nil {
		return func() {}
	}
	layout, err := project.ResolveLayout(root)
	if err != nil {
		return func() {}
	}
	c, err := OpenDocCache(layout.CacheDir())
	if err != nil {
		return func() {}
	}
	a.docCache = c
	return func() {
		c.Close()
		a.docCache = nil
	}
}

// runnerPartCache returns the runner document-cache seam and its config-key
// fingerprint for the current project, or (nil, "") when not in a project or the
// cache isn't open. The fingerprint folds in the source locale, any preset/format
// config the caller merged, and the project recipe hash — every non-byte input
// that shapes the parse — so a recipe or config change re-parses.
func (a *App) runnerPartCache(root string, mergedConfig map[string]any) (flow.PartCache, string) {
	if a.ProjectContext == nil || a.docCache == nil {
		return nil, ""
	}
	h := sha256.New()
	// The build stamp, first: a cached parse is only replayable by a reader
	// that would produce the same parse. Nothing else in this key moves when a
	// reader's behaviour does, so without it an upgraded kapi replays the old
	// binary's classification — the same content, silently no longer offered
	// to the translator, with nothing failing to say so. A table whose cells
	// became translatable kept coming back untranslated exactly this way.
	fmt.Fprintf(h, "%s\x00%s\x00", version.Version, version.Commit)
	fmt.Fprintf(h, "%s\x00", a.SourceLocale())
	if len(mergedConfig) > 0 {
		if b, err := json.Marshal(mergedConfig); err == nil {
			h.Write(b)
		}
	}
	if layout, err := project.ResolveLayout(root); err == nil {
		if rb, rerr := os.ReadFile(layout.RecipePath); rerr == nil {
			h.Write(rb)
		}
	}
	return a.docCache, hex.EncodeToString(h.Sum(nil))[:16]
}
