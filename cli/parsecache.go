package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// cachedPart is one Part with its concrete Resource hoisted into a typed field —
// Part.Resource is an interface, so it is serialized as a tagged union keyed by
// the PartType. Exactly one payload field is set per part. It is the per-part
// record the streaming document cache (docCache) appends to its log.
type cachedPart struct {
	Type   model.PartType    `json:"t"`
	Block  *model.Block      `json:"b,omitempty"`
	Layer  *model.Layer      `json:"l,omitempty"`
	Data   *model.Data       `json:"d,omitempty"`
	Media  *model.Media      `json:"m,omitempty"`
	GStart *model.GroupStart `json:"gs,omitempty"`
	GEnd   *model.GroupEnd   `json:"ge,omitempty"`
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
			cp.Block = r
		case *model.Layer:
			cp.Layer = r
		case *model.Data:
			cp.Data = r
		case *model.Media:
			cp.Media = r
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
			res = cp.Block
		case model.PartLayerStart, model.PartLayerEnd:
			res = cp.Layer
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
	c, err := openDocCache(layout.CacheDir())
	if err != nil {
		return func() {}
	}
	a.docCache = c
	return func() {
		c.close()
		a.docCache = nil
	}
}

// runnerPartCache returns the runner document-cache seam and its config-key
// fingerprint for the current project, or (nil, "") when not in a project or the
// cache isn't open. The fingerprint folds in the source locale, any preset/format
// config the caller merged, and the project recipe hash — every non-byte input
// that shapes the parse — so a recipe or config change re-parses.
func (a *App) runnerPartCache(root string, mergedConfig map[string]any) (flow.PartCache, string) {
	if a.projectContext == nil || a.docCache == nil {
		return nil, ""
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00", a.SourceLang)
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
