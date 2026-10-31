package redaction

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/model"
)

// A redact step withholds the sensitive spans of a block, but a request to a
// provider carries more than the block: the term rules, the voice profile,
// the content memory's prior version, the neighbouring blocks, the
// instruction. An [Egress] redacts every string of one request under the
// policy that redacted the block, so a value the policy withholds leaves the
// machine as a token wherever the request carries it.
//
// The redact step leaves its [Policy] on each block it reads, in the block's
// [SecretAnnotation]. A tool reads a block through a view whose context
// carries an Egress over that annotation ([SecretAnnotation.BindContext]),
// and a tool that sends several blocks in one request binds them together
// ([BindBlocks]). The provider layer reads the Egress from the call's context
// and redacts the request before it is sent.

// Policy is what redaction applies to a string: the detector that finds the
// spans, the placeholder options that render a token, and the locale
// detection reads the string in.
type Policy struct {
	Detector Detector
	Options  RedactOptions
	Locale   model.LocaleID
}

// Egress redacts the strings of one provider request. A value a block of the
// request already withholds is drawn as that block's token. A value met only
// in the rest of the request gets the next token in the block's sequence and
// is kept in the block's entry, so a token the model copies into its answer
// is restored with the block.
type Egress struct {
	policy  *Policy
	entries []*SecretAnnotation
	// spreads keeps a value one entry holds in every other entry, for a
	// request whose answers go back to each of its blocks (BindBlocks).
	spreads bool
}

// egressMu serializes the writes an Egress makes to the entries it holds. A
// block is read by one tool at a time, so it guards against two requests
// over one block, which no tool makes today.
var egressMu sync.Mutex

// NewEgress returns an Egress that redacts under policy and draws its tokens
// from entries, the vault entries of the blocks a request carries. With no
// entry it keeps its tokens in an entry of its own, so one value is still one
// token across the request; nothing restores them.
func NewEgress(policy *Policy, entries ...*SecretAnnotation) *Egress {
	if policy == nil {
		return nil
	}
	held := slices.DeleteFunc(slices.Clone(entries), func(a *SecretAnnotation) bool { return a == nil })
	if len(held) == 0 {
		held = []*SecretAnnotation{{}}
	}
	return &Egress{policy: policy, entries: held}
}

type egressKey struct{}

// WithEgress returns ctx carrying e. A nil e clears the Egress ctx carries,
// which is how a provider wrapper marks a request it has redacted.
func WithEgress(ctx context.Context, e *Egress) context.Context {
	return context.WithValue(ctx, egressKey{}, e)
}

// EgressFrom returns the Egress ctx carries, or nil.
func EgressFrom(ctx context.Context) *Egress {
	if ctx == nil {
		return nil
	}
	e, _ := ctx.Value(egressKey{}).(*Egress)
	return e
}

// BindContext returns ctx carrying an Egress over this entry, when a redact
// step left its policy on it, and ctx as it is otherwise. A tool's view of
// the block calls it, so every provider call the tool makes with the view's
// context is redacted under the policy that redacted the block.
func (a *SecretAnnotation) BindContext(ctx context.Context) context.Context {
	if a == nil || a.Policy == nil {
		return ctx
	}
	return WithEgress(ctx, NewEgress(a.Policy, a))
}

// BindBlocks returns ctx carrying an Egress over the entries of blocks, for a
// tool that sends several blocks in one request and writes each answer back
// to its block. A value one block withholds is kept in the others too, so it
// restores in whichever block's answer the model copies it into. The blocks
// a redact step read carry its policy; ctx is returned as it is when none
// does.
func BindBlocks(ctx context.Context, blocks ...*model.Block) context.Context {
	entries := make([]*SecretAnnotation, 0, len(blocks))
	for _, b := range blocks {
		entries = append(entries, EntryOf(b))
	}
	e := bindEntries(entries)
	if e == nil {
		return ctx
	}
	e.spreads = true
	return WithEgress(ctx, e)
}

// BindCorpus returns ctx carrying an Egress over entries, for a tool that
// sends the text of many blocks as one corpus and writes no answer back to
// them. ctx is returned as it is when no entry carries a policy.
func BindCorpus(ctx context.Context, entries ...*SecretAnnotation) context.Context {
	if e := bindEntries(entries); e != nil {
		return WithEgress(ctx, e)
	}
	return ctx
}

// EntryOf returns the redaction entry a redact step left on b, or nil.
func EntryOf(b *model.Block) *SecretAnnotation {
	if b == nil {
		return nil
	}
	a, ok := model.AnnoAs[*SecretAnnotation](b, SecretAnnotationKey)
	if !ok || a == nil || a.Policy == nil {
		return nil
	}
	return a
}

// bindEntries returns an Egress over the entries that carry a policy, under
// the first one's policy, or nil when none does.
func bindEntries(entries []*SecretAnnotation) *Egress {
	var (
		policy *Policy
		held   []*SecretAnnotation
	)
	for _, a := range entries {
		if a == nil || a.Policy == nil {
			continue
		}
		if policy == nil {
			policy = a.Policy
		}
		held = append(held, a)
	}
	if policy == nil {
		return nil
	}
	return NewEgress(policy, held...)
}

// Redact returns texts, the strings of one request, with every span the
// policy detects, and every value the request's blocks withhold, replaced by
// a token. The second reaches what the detector cannot: a value an entity
// step found in the block. A value the blocks withhold is drawn as the token
// the request already shows for it: its placeholder tag
// (model.PhPlaceholderTag) when texts carry the block's runs as tags, its
// visible token (Disp) otherwise. A span inside a token the request already
// shows is left alone.
func (e *Egress) Redact(ctx context.Context, texts []string) ([]string, error) {
	if e == nil {
		return texts, nil
	}
	egressMu.Lock()
	withheld := e.withheld()
	egressMu.Unlock()
	found := make([][]Match, len(texts))
	hit := false
	for i, s := range texts {
		if s == "" {
			continue
		}
		var ms []Match
		if e.policy.Detector != nil {
			detected, err := e.policy.Detector.Detect(ctx, s, e.policy.Locale)
			if err != nil {
				return nil, err
			}
			ms = detected
		}
		ms = append(ms, literalMatches(s, withheld)...)
		found[i] = NormalizeMatches(ms)
		hit = hit || len(found[i]) > 0
	}
	if !hit {
		return texts, nil
	}
	shown := strings.Join(texts, "\x00")

	egressMu.Lock()
	defer egressMu.Unlock()
	out := make([]string, len(texts))
	for i, s := range texts {
		out[i] = e.replace(s, found[i], shown)
	}
	return out, nil
}

// withheld returns the values the entries hold, longest original first.
func (e *Egress) withheld() []RedactedValue {
	var out []RedactedValue
	for _, a := range e.entries {
		for _, v := range a.Values {
			if strings.TrimSpace(v.Original) != "" {
				out = append(out, v)
			}
		}
	}
	slices.SortFunc(out, func(x, y RedactedValue) int {
		if c := cmp.Compare(len(y.Original), len(x.Original)); c != 0 {
			return c
		}
		return cmp.Compare(x.Original, y.Original)
	})
	return out
}

// literalMatches returns every occurrence of a withheld original in s.
func literalMatches(s string, withheld []RedactedValue) []Match {
	var out []Match
	for _, v := range withheld {
		for from := 0; ; {
			i := strings.Index(s[from:], v.Original)
			if i < 0 {
				break
			}
			start := from + i
			out = append(out, Match{Start: start, End: start + len(v.Original), Category: v.Category, Original: v.Original})
			from = start + len(v.Original)
		}
	}
	return out
}

// replace rewrites s with each match drawn as its token.
func (e *Egress) replace(s string, matches []Match, shown string) string {
	if len(matches) == 0 {
		return s
	}
	shownTokens := e.tokenSpans(s)
	var b strings.Builder
	at := 0
	for _, m := range matches {
		if m.Start < at || m.End > len(s) || overlaps(shownTokens, m) {
			continue
		}
		b.WriteString(s[at:m.Start])
		b.WriteString(e.tokenFor(m, shown))
		at = m.End
	}
	b.WriteString(s[at:])
	return b.String()
}

// tokenSpans returns the byte spans of s that draw a token the entries hold,
// as its visible token or its placeholder tag.
func (e *Egress) tokenSpans(s string) [][2]int {
	var spans [][2]int
	for _, a := range e.entries {
		for _, v := range a.Values {
			for _, tok := range []string{v.Disp, model.PhPlaceholderTag(v.Token)} {
				if tok == "" {
					continue
				}
				for from := 0; ; {
					i := strings.Index(s[from:], tok)
					if i < 0 {
						break
					}
					spans = append(spans, [2]int{from + i, from + i + len(tok)})
					from += i + len(tok)
				}
			}
		}
	}
	return spans
}

func overlaps(spans [][2]int, m Match) bool {
	for _, sp := range spans {
		if m.Start < sp[1] && sp[0] < m.End {
			return true
		}
	}
	return false
}

// tokenFor returns the token the request draws m's value as, adding the
// value to the entries when none holds it.
func (e *Egress) tokenFor(m Match, shown string) string {
	if v, ok := e.held(m.Original); ok {
		// A placeholder tag names a token in one block only, so it stands for
		// the value when the request carries one block.
		if tag := model.PhPlaceholderTag(v.Token); len(e.entries) == 1 && strings.Contains(shown, tag) {
			return tag
		}
		e.spread(v)
		return v.Disp
	}
	return e.add(m).Disp
}

// spread keeps v in every entry of the request that lacks it, under the next
// token in that entry's sequence, so its visible token restores in whichever
// block's answer the model copies it into. An entry that shows another value
// as the same visible token keeps its own.
func (e *Egress) spread(v RedactedValue) {
	if !e.spreads || len(e.entries) < 2 {
		return
	}
	for _, a := range e.entries {
		if slices.ContainsFunc(slices.Collect(maps.Values(a.Values)), func(h RedactedValue) bool { return h.Disp == v.Disp }) {
			continue
		}
		kept := v
		kept.Token = e.nextToken(a)
		if a.Values == nil {
			a.Values = map[string]RedactedValue{}
		}
		a.Values[kept.Token] = kept
	}
}

// nextToken returns the token after the highest the given entries hold, or
// every entry of the request when none is given.
func (e *Egress) nextToken(only ...*SecretAnnotation) string {
	prefix := e.policy.Options.tokenPrefix()
	scan := e.entries
	if len(only) > 0 {
		scan = only
	}
	n := 0
	for _, a := range scan {
		for tok := range a.Values {
			if rest, ok := strings.CutPrefix(tok, prefix); ok {
				if k, err := strconv.Atoi(rest); err == nil && k > n {
					n = k
				}
			}
		}
	}
	return prefix + strconv.Itoa(n+1)
}

// held returns the value an entry holds for original: the first entry's
// first, a source value before a translation's, the lowest token first.
func (e *Egress) held(original string) (RedactedValue, bool) {
	for _, a := range e.entries {
		var cands []RedactedValue
		for _, v := range a.Values {
			if v.Original == original && v.Disp != "" {
				cands = append(cands, v)
			}
		}
		if len(cands) == 0 {
			continue
		}
		slices.SortFunc(cands, func(x, y RedactedValue) int {
			if c := cmp.Compare(rank(x, e.policy.Locale), rank(y, e.policy.Locale)); c != 0 {
				return c
			}
			if c := cmp.Compare(len(x.Token), len(y.Token)); c != 0 {
				return c
			}
			return cmp.Compare(x.Token, y.Token)
		})
		return cands[0], true
	}
	return RedactedValue{}, false
}

func rank(v RedactedValue, source model.LocaleID) int {
	if v.Locale.IsEmpty() || v.Locale == source {
		return 0
	}
	return 1
}

// add keeps a value the request's blocks do not withhold under the next token
// in their sequence and a visible token none of them uses for another value:
// in every entry of a request whose answers go back to each block, and in
// the first entry otherwise.
func (e *Egress) add(m Match) RedactedValue {
	token := e.nextToken()
	n, _ := strconv.Atoi(strings.TrimPrefix(token, e.policy.Options.tokenPrefix()))
	v := RedactedValue{
		Token:    token,
		Category: m.Category,
		Disp:     e.freeDisp(m.Category, n),
		Original: m.Original,
		Locale:   e.policy.Locale,
	}
	keep := e.entries
	if !e.spreads {
		keep = e.entries[:1]
	}
	for _, a := range keep {
		if a.Values == nil {
			a.Values = map[string]RedactedValue{}
		}
		a.Values[v.Token] = v
	}
	return v
}

// freeDisp renders the visible token for the nth value of category, adding
// a number when another value already shows as it.
func (e *Egress) freeDisp(category string, n int) string {
	tmpl := e.policy.Options.placeholder()
	base := renderPlaceholder(tmpl, category, n)
	if !e.dispTaken(base) {
		return base
	}
	for k := 1; ; k++ {
		if d := base + "#" + strconv.Itoa(k); !e.dispTaken(d) {
			return d
		}
	}
}

func (e *Egress) dispTaken(disp string) bool {
	for _, a := range e.entries {
		for _, v := range a.Values {
			if v.Disp == disp {
				return true
			}
		}
	}
	return false
}
