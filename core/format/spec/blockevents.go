package spec

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
)

// blockevents.go implements the neutral block-event dump — the un-blessed
// oracle described in format-spec-cases.md §4. DumpBlockEvents serializes a
// part stream as a deterministic JSON Lines event stream that any
// implementation (the native Go reader, the okapi-bridge daemon, a WASM
// build, a future port) can be compared against. The dump is the canonical
// expected encoding behind the `expected.blocks` view.
//
// Determinism contract (§4.2):
//   - object keys emit in fixed schema order (struct field order);
//   - map-valued fields (properties, targets, plural forms, select cases)
//     are sorted by key (encoding/json sorts string/TextMarshaler map keys);
//   - empty/zero fields are omitted (omitempty);
//   - HTML escaping is disabled, matching model.Run.MarshalJSON and the KLF
//     wire form, so `<b>` stays literal and content hashes stay
//     implementation-independent;
//   - UTF-8, one event per line, LF separators, trailing newline, no
//     trailing whitespace on any line.
//
// Excluded by design (engine internals / tool products, not the reader
// contract): Skeleton, Identity, ContentRef, DisplayHint, Annotations,
// IsReferent.

// eventLine is one JSON Lines record. Exactly one field is non-nil per line;
// the field's key names the part type. Declaration order is irrelevant since
// only one field is ever set, but the per-event structs below fix key order
// within each event object.
type eventLine struct {
	LayerStart *layerEvent    `json:"layer_start,omitempty"`
	LayerEnd   *layerEndEvent `json:"layer_end,omitempty"`
	GroupStart *groupEvent    `json:"group_start,omitempty"`
	GroupEnd   *groupEndEvent `json:"group_end,omitempty"`
	Block      *blockEvent    `json:"block,omitempty"`
	Data       *dataEvent     `json:"data,omitempty"`
	Media      *mediaEvent    `json:"media,omitempty"`
}

type layerEvent struct {
	ID           string            `json:"id"`
	Name         string            `json:"name,omitempty"`
	Format       string            `json:"format,omitempty"`
	Locale       string            `json:"locale,omitempty"`
	MimeType     string            `json:"mime_type,omitempty"`
	Encoding     string            `json:"encoding,omitempty"`
	Multilingual bool              `json:"multilingual,omitempty"`
	Properties   map[string]string `json:"properties,omitempty"`
}

type layerEndEvent struct {
	ID string `json:"id"`
}

type groupEvent struct {
	ID         string            `json:"id"`
	Name       string            `json:"name,omitempty"`
	Type       string            `json:"type,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

type groupEndEvent struct {
	ID string `json:"id"`
}

type blockEvent struct {
	ID                 string               `json:"id"`
	Name               string               `json:"name,omitempty"`
	Type               string               `json:"type,omitempty"`
	Translatable       bool                 `json:"translatable,omitempty"`
	Source             []runDump            `json:"source"`
	Targets            map[string][]runDump `json:"targets,omitempty"`
	Properties         map[string]string    `json:"properties,omitempty"`
	Overlays           []overlayDump        `json:"overlays,omitempty"`
	PreserveWhitespace bool                 `json:"preserve_whitespace,omitempty"`
}

type dataEvent struct {
	ID         string            `json:"id"`
	Name       string            `json:"name,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

type mediaEvent struct {
	ID         string            `json:"id"`
	Filename   string            `json:"filename,omitempty"`
	MimeType   string            `json:"mime_type,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// runDump is the flattened wire form of a model.Run for the event dump. It is
// intentionally NOT model.Run's own discriminated-union JSON shape
// (`{"text":...}` / `{"pcOpen":{...}}`): the dump uses a single `type`
// discriminator key plus flat fields, so the typed-code contract
// (`<b>` → pcOpen with semantic fmt:bold) reads as one object per run.
type runDump struct {
	Type     string               `json:"type"`
	Text     string               `json:"text,omitempty"`
	ID       string               `json:"id,omitempty"`
	Semantic string               `json:"semantic,omitempty"`
	SubType  string               `json:"subtype,omitempty"`
	Data     string               `json:"data,omitempty"`
	Equiv    string               `json:"equiv,omitempty"`
	Ref      string               `json:"ref,omitempty"`
	Pivot    string               `json:"pivot,omitempty"`
	Forms    map[string][]runDump `json:"forms,omitempty"`
	Cases    map[string][]runDump `json:"cases,omitempty"`
}

type overlayDump struct {
	Type    string     `json:"type"`
	Layer   string     `json:"layer,omitempty"`
	Variant string     `json:"variant,omitempty"`
	Spans   []spanDump `json:"spans,omitempty"`
}

type spanDump struct {
	ID    string            `json:"id,omitempty"`
	Range [4]int            `json:"range"`
	Props map[string]string `json:"props,omitempty"`
}

// DumpBlockEvents serializes a part stream as the canonical block-event dump:
// JSON Lines, one event per part, UTF-8, LF-separated, with a trailing
// newline. Dumping the same parts twice is byte-identical. nil parts are
// skipped. See the package-level contract above for the determinism rules.
func DumpBlockEvents(parts []*model.Part) ([]byte, error) {
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	for _, p := range parts {
		if p == nil {
			continue
		}
		line, ok := eventFor(p)
		if !ok {
			// PartRawDocument / PartCustom / PartUnknown carry no reader
			// contract; skip them rather than inventing an encoding.
			continue
		}
		if err := enc.Encode(line); err != nil {
			return nil, fmt.Errorf("spec: dump block events: %w", err)
		}
	}
	return out.Bytes(), nil
}

// eventFor maps one Part to its event line. The bool reports whether the
// part type has a dump encoding (false for raw/custom/unknown).
func eventFor(p *model.Part) (eventLine, bool) {
	switch p.Type {
	case model.PartLayerStart:
		l, _ := p.Resource.(*model.Layer)
		if l == nil {
			return eventLine{}, false
		}
		return eventLine{LayerStart: &layerEvent{
			ID:           l.ID,
			Name:         l.Name,
			Format:       l.Format,
			Locale:       string(l.Locale),
			MimeType:     l.MimeType,
			Encoding:     l.Encoding,
			Multilingual: l.IsMultilingual,
			Properties:   nonEmptyStrMap(l.Properties),
		}}, true
	case model.PartLayerEnd:
		l, _ := p.Resource.(*model.Layer)
		if l == nil {
			return eventLine{}, false
		}
		return eventLine{LayerEnd: &layerEndEvent{ID: l.ID}}, true
	case model.PartGroupStart:
		g, _ := p.Resource.(*model.GroupStart)
		if g == nil {
			return eventLine{}, false
		}
		return eventLine{GroupStart: &groupEvent{
			ID:         g.ID,
			Name:       g.Name,
			Type:       g.Type,
			Properties: nonEmptyStrMap(g.Properties),
		}}, true
	case model.PartGroupEnd:
		g, _ := p.Resource.(*model.GroupEnd)
		if g == nil {
			return eventLine{}, false
		}
		return eventLine{GroupEnd: &groupEndEvent{ID: g.ID}}, true
	case model.PartBlock:
		b, _ := p.Resource.(*model.Block)
		if b == nil {
			return eventLine{}, false
		}
		return eventLine{Block: blockEventFor(b)}, true
	case model.PartData:
		d, _ := p.Resource.(*model.Data)
		if d == nil {
			return eventLine{}, false
		}
		return eventLine{Data: &dataEvent{
			ID:         d.ID,
			Name:       d.Name,
			Properties: nonEmptyStrMap(d.Properties),
		}}, true
	case model.PartMedia:
		m, _ := p.Resource.(*model.Media)
		if m == nil {
			return eventLine{}, false
		}
		return eventLine{Media: &mediaEvent{
			ID:         m.ID,
			Filename:   m.Filename,
			MimeType:   m.MimeType,
			Properties: nonEmptyStrMap(m.Properties),
		}}, true
	}
	return eventLine{}, false
}

func blockEventFor(b *model.Block) *blockEvent {
	ev := &blockEvent{
		ID:                 b.ID,
		Name:               b.Name,
		Type:               b.Type,
		Translatable:       b.Translatable,
		Source:             dumpRuns(b.Source),
		Properties:         nonEmptyStrMap(b.Properties),
		Overlays:           dumpOverlays(b.Overlays),
		PreserveWhitespace: b.PreserveWhitespace,
	}
	if ev.Source == nil {
		// `source` is a required key even for an empty source so the shape
		// stays stable; emit an empty array rather than omitting it.
		ev.Source = []runDump{}
	}
	if len(b.Targets) > 0 {
		ev.Targets = make(map[string][]runDump, len(b.Targets))
		for key, tgt := range b.Targets {
			if tgt == nil {
				continue
			}
			text, _ := key.MarshalText()
			ev.Targets[string(text)] = dumpRuns(tgt.Runs)
		}
	}
	return ev
}

func dumpRuns(runs []model.Run) []runDump {
	if len(runs) == 0 {
		return nil
	}
	out := make([]runDump, 0, len(runs))
	for _, r := range runs {
		out = append(out, dumpRun(r))
	}
	return out
}

func dumpRun(r model.Run) runDump {
	switch r.Kind() {
	case model.RunKindText:
		return runDump{Type: string(model.RunKindText), Text: r.Text.Text}
	case model.RunKindPh:
		return runDump{
			Type:     string(model.RunKindPh),
			ID:       r.Ph.ID,
			Semantic: r.Ph.Type,
			SubType:  r.Ph.SubType,
			Data:     r.Ph.Data,
			Equiv:    r.Ph.Equiv,
		}
	case model.RunKindPcOpen:
		return runDump{
			Type:     string(model.RunKindPcOpen),
			ID:       r.PcOpen.ID,
			Semantic: r.PcOpen.Type,
			SubType:  r.PcOpen.SubType,
			Data:     r.PcOpen.Data,
			Equiv:    r.PcOpen.Equiv,
		}
	case model.RunKindPcClose:
		return runDump{
			Type:     string(model.RunKindPcClose),
			ID:       r.PcClose.ID,
			Semantic: r.PcClose.Type,
			SubType:  r.PcClose.SubType,
			Data:     r.PcClose.Data,
			Equiv:    r.PcClose.Equiv,
		}
	case model.RunKindSub:
		return runDump{
			Type: string(model.RunKindSub),
			ID:   r.Sub.ID,
			Ref:  r.Sub.Ref,
		}
	case model.RunKindPlural:
		forms := make(map[string][]runDump, len(r.Plural.Forms))
		for form, branch := range r.Plural.Forms {
			forms[string(form)] = dumpRuns(branch)
		}
		return runDump{Type: string(model.RunKindPlural), Pivot: r.Plural.Pivot, Forms: forms}
	case model.RunKindSelect:
		cases := make(map[string][]runDump, len(r.Select.Cases))
		for c, branch := range r.Select.Cases {
			cases[c] = dumpRuns(branch)
		}
		return runDump{Type: string(model.RunKindSelect), Pivot: r.Select.Pivot, Cases: cases}
	}
	// A zero run is invalid; surface it visibly rather than panicking.
	return runDump{Type: "invalid"}
}

func dumpOverlays(overlays []model.Overlay) []overlayDump {
	if len(overlays) == 0 {
		return nil
	}
	out := make([]overlayDump, 0, len(overlays))
	for _, o := range overlays {
		od := overlayDump{Type: string(o.Type), Layer: o.Layer}
		if o.Variant != nil {
			text, _ := o.Variant.MarshalText()
			od.Variant = string(text)
		}
		for _, s := range o.Spans {
			od.Spans = append(od.Spans, spanDump{
				ID:    s.ID,
				Range: [4]int{s.Range.StartRun, s.Range.StartOffset, s.Range.EndRun, s.Range.EndOffset},
				Props: nonEmptyStrMap(s.Props),
			})
		}
		out = append(out, od)
	}
	return out
}

// nonEmptyStrMap returns m when it has entries, else nil so omitempty drops
// the key. encoding/json sorts the keys on marshal.
func nonEmptyStrMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	return m
}
