package kbf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// A block carries its content as peer editions, each under the text form of
// its edition key (model.EditionKey.MarshalText): "fr", "fr;tone=formal",
// "en;channel=short". The edition the block was read in, its source, is filed
// under the zero key, whose text form is empty, in the language the file's
// ProjectInfo.SourceLocale names. A translation a reader filed under no
// language has no key of its own, because the empty key names the source, so a
// block carries it apart from Editions, as Unlabelled.
//
// Schema 1 carried the same content as `source` runs beside a `targets` map of
// runs keyed by locale and a `targetOrigins` map. A block in that shape decodes
// into the editions it describes (UnmarshalJSON), so a reader of either schema
// sees one shape.

// SourceEdition is the key the edition a block was read in is filed under in
// Block.Editions: the zero edition key, whose text form is empty.
const SourceEdition = ""

// Edition is one edition of a block: its runs, where it stands on its status
// ladder, how it was produced, and the edition it was made from.
type Edition struct {
	// Runs is the edition's content.
	Runs []Run `json:"runs"`
	// Status is the edition's lifecycle state: written or established for a
	// source, draft, translated or established for a translation. Omitted
	// while none is recorded.
	Status string `json:"status,omitempty"`
	// Origin records how the content was produced and under what context.
	// Omitted while it records nothing.
	Origin Origin `json:"origin,omitzero"`
	// Score is the producer's quality score for a derived edition. Omitted
	// while zero. The source carries none: a reader of a bundle leaves a score
	// on the source behind, and a writer writes none there.
	Score float64 `json:"score,omitempty"`
	// Derived names the edition this one was made from and that edition's
	// revision when it was made. Omitted for an authored edition.
	Derived *Derivation `json:"derived,omitempty"`
}

// Derivation records where a derived edition came from: the key text of the
// edition it was made from (empty for the source) and that edition's revision
// (model.EditionRevision) at the time.
type Derivation struct {
	From string `json:"from"`
	Rev  string `json:"rev"`
}

// MarshalJSON writes the edition with its runs as a JSON array even when there
// are none, so every reader finds a run sequence under `runs`.
func (e Edition) MarshalJSON() ([]byte, error) {
	type plain Edition
	p := plain(e)
	if p.Runs == nil {
		p.Runs = []Run{}
	}
	return marshalUnescaped(p)
}

// SourceEditions returns an editions map holding runs as the source: the
// editions of a block a reader extracted and nothing has translated.
func SourceEditions(runs []Run) map[string]Edition {
	return map[string]Edition{SourceEdition: {Runs: runs}}
}

// SourceRuns returns the runs of the edition the block was read in.
func (b *Block) SourceRuns() []Run { return b.Editions[SourceEdition].Runs }

// SetSourceRuns replaces the runs of the edition the block was read in and
// keeps what else is recorded about it.
func (b *Block) SetSourceRuns(runs []Run) {
	e := b.Editions[SourceEdition]
	e.Runs = runs
	b.SetEdition(SourceEdition, e)
}

// Edition returns the edition filed under key and whether the block holds it.
func (b *Block) Edition(key string) (Edition, bool) {
	e, ok := b.Editions[key]
	return e, ok
}

// SetEdition files e under key, replacing any edition held there.
func (b *Block) SetEdition(key string, e Edition) {
	if b.Editions == nil {
		b.Editions = make(map[string]Edition)
	}
	b.Editions[key] = e
}

// TargetKeys returns the key of every edition but the source, in byte order.
func (b *Block) TargetKeys() []string {
	keys := make([]string, 0, len(b.Editions))
	for k := range b.Editions {
		if k != SourceEdition {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// MarshalJSON writes the block with its source edition present, empty if the
// block holds none, and its placeholders as a JSON array even when there are
// none: a reader finds the edition the block was read in and the placeholder
// list on every block.
func (b Block) MarshalJSON() ([]byte, error) {
	type plain Block
	p := plain(b)
	if _, ok := p.Editions[SourceEdition]; !ok {
		editions := make(map[string]Edition, len(p.Editions)+1)
		maps.Copy(editions, p.Editions)
		editions[SourceEdition] = Edition{}
		p.Editions = editions
	}
	if p.Placeholders == nil {
		p.Placeholders = []Placeholder{}
	}
	return marshalUnescaped(p)
}

// UnmarshalJSON reads a block in either schema. A block in the schema 1 shape,
// `source` runs beside `targets` and `targetOrigins` maps keyed by locale,
// becomes the editions it describes: the source runs under SourceEdition, each
// target under its locale with the origin recorded for it, and a target under
// the empty locale as the unlabelled edition. An origin recorded for a locale
// with no target describes nothing and is not kept. A block that carries both
// shapes is refused, because neither can be read without losing the other.
func (b *Block) UnmarshalJSON(data []byte) error {
	type plain Block
	var raw struct {
		plain
		Source        json.RawMessage   `json:"source"`
		Targets       map[string][]Run  `json:"targets"`
		TargetOrigins map[string]Origin `json:"targetOrigins"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*b = Block(raw.plain)
	if raw.Source == nil && raw.Targets == nil && raw.TargetOrigins == nil {
		return nil
	}
	if b.Editions != nil || b.Unlabelled != nil {
		return fmt.Errorf("kbf: block %q carries both editions and the schema 1 source and targets", b.ID)
	}
	var source []Run
	if raw.Source != nil {
		if err := json.Unmarshal(raw.Source, &source); err != nil {
			return err
		}
	}
	b.Editions = SourceEditions(source)
	for locale, runs := range raw.Targets {
		e := Edition{Runs: runs, Origin: raw.TargetOrigins[locale]}
		if locale == "" {
			b.Unlabelled = &e
			continue
		}
		b.Editions[locale] = e
	}
	return nil
}

// marshalUnescaped encodes v as compact JSON without HTML escaping, the form
// Marshal writes, so a marshaler on a type inside a bundle never brings back
// the escaping the bundle's own encoder turns off.
func marshalUnescaped(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// ───────── the content model ─────────

// KeyText returns the text form key is filed under in Block.Editions.
func KeyText(key model.EditionKey) string {
	text, _ := key.MarshalText()
	return string(text)
}

// ReadKey returns the edition key text names, canonical: the language
// normalized the way the model files it, with its tone and its channel. The
// empty text is the zero key, the source.
//
// A key this build cannot hold is refused rather than read as a shorter one: a
// key that names no language, a dimension other than tone and channel, a
// dimension named twice, or a dimension with no value. Read leniently,
// `nb;audience=kids` would be `nb` and replace the plain Norwegian edition, and
// `;x=y` would be the source. The language is read as leniently as the model
// reads it, so a pseudo-locale such as `qps` is a key like any other.
func ReadKey(text string) (model.EditionKey, error) {
	if text == SourceEdition {
		return model.EditionKey{}, nil
	}
	parts := strings.Split(text, ";")
	if strings.TrimSpace(parts[0]) == "" {
		return model.EditionKey{}, fmt.Errorf("kbf: edition key %q names no language", text)
	}
	key := model.EditionKey{Locale: model.NormalizeLocale(model.LocaleID(parts[0]))}
	for _, p := range parts[1:] {
		name, val, ok := strings.Cut(p, "=")
		if !ok || val == "" {
			return model.EditionKey{}, fmt.Errorf("kbf: edition key %q: %q is not name=value", text, p)
		}
		switch name {
		case "tone":
			if key.Tone != "" {
				return model.EditionKey{}, fmt.Errorf("kbf: edition key %q names its tone twice", text)
			}
			key.Tone = val
		case "channel":
			if key.Channel != "" {
				return model.EditionKey{}, fmt.Errorf("kbf: edition key %q names its channel twice", text)
			}
			key.Channel = val
		default:
			return model.EditionKey{}, fmt.Errorf("kbf: edition key %q has a dimension this build does not read: %q", text, name)
		}
	}
	return key, nil
}

// FromModelEdition returns e in the bundle's shape, with a run sequence of its
// own. Its derivation names the edition it was made from by that edition's key
// text; EditionsOf, which knows the block, names a derivation from the source
// SourceEdition.
func FromModelEdition(e model.Edition) Edition {
	out := Edition{
		Runs:   slices.Clone(e.Runs),
		Status: string(e.Status),
		Origin: e.Origin,
		Score:  e.Score,
	}
	if e.Derived != nil {
		out.Derived = &Derivation{From: KeyText(e.Derived.From), Rev: e.Derived.Rev}
	}
	return out
}

// fromModelEdition is FromModelEdition for an edition of mb, with a derivation
// from the edition mb was read in naming it SourceEdition. The model knows the
// source by its language when the block has one, and in a bundle that also
// holds a same-language edition that language names the other edition.
func fromModelEdition(mb *model.Block, e model.Edition) Edition {
	out := FromModelEdition(e)
	if e.Derived != nil && mb.IsSourceEdition(e.Derived.From) {
		out.Derived.From = SourceEdition
	}
	return out
}

// ModelEdition returns e as the content model holds it, with a run sequence of
// its own. A derivation from SourceEdition names the zero key, which reaches
// the edition a block was read in on every block. A derivation from a key
// ReadKey refuses names an edition the model cannot hold, so the edition comes
// back with no basis recorded.
func (e Edition) ModelEdition() model.Edition {
	out := model.Edition{
		Runs:   slices.Clone(e.Runs),
		Status: model.Status(e.Status),
		Origin: e.Origin,
		Score:  e.Score,
	}
	if e.Derived != nil {
		if from, err := ReadKey(e.Derived.From); err == nil {
			out.Derived = &model.Derivation{From: from, Rev: e.Derived.Rev}
		}
	}
	return out
}

// EditionsOf returns every edition mb holds in the bundle's shape: the edition
// mb was read in under SourceEdition, every other edition under the text of its
// canonical key, and a translation filed under no language as the unlabelled
// edition. A derivation from the edition mb was read in names it
// SourceEdition. An edition other than the source that holds no runs is left
// out, whatever status it records: no edition and an empty one say the same
// thing to every reader of a bundle. The source carries no score, because the
// model keeps a score for derived editions only.
func EditionsOf(mb *model.Block) (editions map[string]Edition, unlabelled *Edition) {
	editions = make(map[string]Edition)
	first := true
	for key, e := range mb.EachEdition {
		if first {
			first = false
			editions[SourceEdition] = fromModelEdition(mb, e)
			continue
		}
		if len(e.Runs) == 0 {
			continue
		}
		editions[KeyText(key)] = fromModelEdition(mb, e)
	}
	if e, ok := mb.TargetEdition(""); ok && len(e.Runs) > 0 {
		u := fromModelEdition(mb, e)
		unlabelled = &u
	}
	return editions, unlabelled
}

// UnreadEditions returns the editions b holds under a key ReadKey refuses, by
// key text, or nil when it holds none. FileEditions leaves them off the model;
// a reader that writes the bundle back carries them through verbatim.
func (b *Block) UnreadEditions() map[string]Edition {
	var out map[string]Edition
	for text, e := range b.Editions {
		if _, err := ReadKey(text); err != nil {
			if out == nil {
				out = make(map[string]Edition)
			}
			out[text] = e
		}
	}
	return out
}

// FileEditions files the editions b holds on mb, a block whose source already
// holds b's source runs: the source's status, origin and derivation on the
// edition mb was read in, then every other edition under its canonical key,
// each marked native because the bundle holds it, and the unlabelled edition
// as mb's translation filed under no language. Editions are filed in the order
// of their key text, so two keys that name one edition (`nb_NO` and `nb-NO`)
// settle the same way on every read.
//
// An edition under a key ReadKey refuses is left off mb: the model has no key
// for it, and the key its text would shorten to belongs to another edition.
// FileEditions files every other edition and returns an error naming each one
// it left (UnreadEditions returns them).
func (b *Block) FileEditions(mb *model.Block) error {
	if src, ok := b.Editions[SourceEdition]; ok {
		if src.Derived != nil {
			mb.SetEdition(model.EditionKey{}, src.ModelEdition())
		} else {
			if src.Status != "" {
				mb.SetEditionStatus(model.EditionKey{}, model.Status(src.Status))
			}
			if src.Origin != (Origin{}) {
				o := src.Origin
				mb.SetSourceOrigin(&o)
			}
		}
	}
	var unread []error
	for _, text := range b.TargetKeys() {
		key, err := ReadKey(text)
		if err != nil {
			unread = append(unread, fmt.Errorf("block %q: %w", b.ID, err))
			continue
		}
		mb.SetTargetEdition(key, b.Editions[text].ModelEdition())
		mb.MarkNative(key)
	}
	if b.Unlabelled != nil {
		mb.SetTargetEdition(model.EditionKey{}, b.Unlabelled.ModelEdition())
	}
	return errors.Join(unread...)
}
