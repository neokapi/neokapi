package host

import (
	"context"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/workhome"
)

// A project that declares redaction (defaults.redaction) keeps no withheld
// value in a record, and a write to the workspace home is a record that
// travels to every context backend the project shares its context through.
// So the workspace home redacts what it records of a kept edition as the
// recorder redacts a file edit: the runs before and after each change, and
// the note, with the project's rules, the originals going to the project
// vault under the name of the edition and revision they belong to
// (edit:<doc>#<block>@<edition>:<revision>), and the change set as sent left
// out. Where the workspace home reads an edition back it puts the originals
// back from the vault, so on the machine that withheld them the edition reads
// as it was written; another machine, which has no vault entry for it, reads
// the placeholders.
//
// A policy that detects entities needs the entity annotations of a read,
// which a kept edition does not carry, so under one the workspace home keeps
// no edition: a write to one is refused, and a gated run leaves its parked
// drafts in the producer's cache.

// keptRedaction is a project's redaction policy as the workspace home
// applies it (workhome.Redaction).
type keptRedaction struct {
	policy *recordRedaction

	// The vault as last read: its size and time of change, and what it holds
	// by vault id.
	mu     sync.Mutex
	loaded bool
	size   int64
	mod    time.Time
	values map[string][]redaction.RedactedValue
}

var _ workhome.Redaction = (*keptRedaction)(nil)

// keeps reports why the policy keeps no edition in the workspace home, or
// nil when it redacts what a kept edition records.
func (k *keptRedaction) keeps() error {
	if slices.Contains(k.policy.spec.Detectors, "entities") {
		return &change.Error{Code: change.CodeUnsupported, Capability: "redaction",
			Message: "the project's redaction policy detects entities, which needs a read of the content the workspace home does not keep, so a translation with no file cannot be kept without the values the policy withholds"}
	}
	return nil
}

// Redact redacts, in place, the runs and the note a write to the workspace
// home records, and drops the change set as sent.
func (k *keptRedaction) Redact(ctx context.Context, c *workhome.Commit) error {
	if err := k.keeps(); err != nil {
		return err
	}
	c.Set = nil
	var (
		blocks []*model.Block
		sinks  []*[]model.Run
	)
	add := func(id string, runs *[]model.Run) {
		if len(*runs) == 0 {
			return
		}
		b := &model.Block{ID: id, Translatable: true}
		b.SetSourceRuns(slices.Clone(*runs))
		blocks = append(blocks, b)
		sinks = append(sinks, runs)
	}
	for i := range c.Blocks {
		b := &c.Blocks[i]
		at := keptVaultID(c.Doc, b.Block, c.Edition, "")
		if len(b.BeforeRuns) > 0 {
			b.BeforeRuns = slices.Clone(b.BeforeRuns)
			add(at+b.Before, &b.BeforeRuns)
		}
		if b.Edition != nil {
			ed := *b.Edition
			ed.Runs = slices.Clone(ed.Runs)
			b.Edition = &ed
			add(at+b.After, &b.Edition.Runs)
		}
	}
	var note []model.Run
	if c.Note != "" {
		note = []model.Run{{Text: &model.TextRun{Text: c.Note}}}
		add("note:"+model.ComputeContentHash(c.Note), &note)
	}
	if len(blocks) == 0 {
		return nil
	}
	p := k.policy
	if err := RedactAtIngest(ctx, blocks, p.spec, p.root, p.vault, ""); err != nil {
		return err
	}
	for i, b := range blocks {
		src, _ := b.Edition(model.EditionKey{})
		*sinks[i] = src.Runs
	}
	if note != nil {
		c.Note = model.RenderRunsWithData(note)
	}
	// The vault holds new originals: the next read loads it again, whatever
	// the clock's resolution made of the file's time of change.
	k.mu.Lock()
	k.loaded, k.values = false, nil
	k.mu.Unlock()
	return nil
}

// Restore puts back, from the project vault, the originals Redact withheld
// from the runs a row keeps. Runs the vault holds nothing for come back as
// they are.
func (k *keptRedaction) Restore(_ context.Context, r workhome.Row, runs []model.Run) []model.Run {
	if len(runs) == 0 {
		return runs
	}
	id := keptVaultID(r.Doc, r.Block, r.Edition, r.Rev)
	entries := k.valuesFor(id)
	if len(entries) == 0 {
		return runs
	}
	byToken := make(map[string]string, len(entries))
	for _, e := range entries {
		byToken[e.Token] = e.Original
	}
	get := func(token string) (string, bool) {
		v, ok := byToken[token]
		return v, ok
	}
	restored, n, _ := redaction.RestorePlan(runs, get, entries)
	if n == 0 {
		return runs
	}
	return restored
}

// valuesFor returns what the project vault holds for one vault id. The vault
// is read again whenever it changed since the last read, which another
// process writing a kept edition, or this one, changes it.
func (k *keptRedaction) valuesFor(id string) []redaction.RedactedValue {
	k.mu.Lock()
	defer k.mu.Unlock()
	info, err := os.Stat(k.policy.vault)
	if err != nil {
		return nil
	}
	if !k.loaded || info.Size() != k.size || !info.ModTime().Equal(k.mod) {
		k.loaded, k.size, k.mod = true, info.Size(), info.ModTime()
		k.values = map[string][]redaction.RedactedValue{}
		if v, err := redaction.OpenFileVault(k.policy.vault); err == nil {
			for _, rv := range v.All() {
				k.values[rv.BlockID] = append(k.values[rv.BlockID], rv)
			}
		}
	}
	return k.values[id]
}

// keptVaultID names the run sequence of one edition at one revision in the
// project vault, as the recorder names the runs a record keeps.
func keptVaultID(doc, block, edition, rev string) string {
	return "edit:" + doc + "#" + block + "@" + edition + ":" + rev
}
