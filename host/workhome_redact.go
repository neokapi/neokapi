package host

import (
	"context"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/workhome"
)

// A project that declares redaction keeps no withheld value in a record, and
// a write to the workspace home is a record that travels to every context
// backend the project shares its context through. So the workspace home
// redacts what it records of a kept edition as the recorder redacts a file
// edit: the runs before and after each change, and the note, under the policy
// that governs the document's file (RedactionFor). The originals go to the
// project vault under the name of the edition and revision they belong to
// (edit:<doc>#<block>@<edition>:<revision>), and the change set as sent is
// left out. Where the workspace home reads an edition back it puts the originals
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

// errKeepsNoEdition is the refusal of a policy that detects entities.
var errKeepsNoEdition = &change.Error{Code: change.CodeUnsupported, Capability: "redaction",
	Message: "the project's redaction policy detects entities, which needs a read of the content the workspace home does not keep, so a translation with no file cannot be kept without the values the policy withholds"}

// keeps reports why the project's policies keep no edition in the workspace
// home, or nil when they redact what a kept edition records. It asks of every
// policy the project declares, for a caller that writes many documents.
func (k *keptRedaction) keeps() error {
	if detectsEntities(k.policy.proj.DeclaredRedactions()) {
		return errKeepsNoEdition
	}
	return nil
}

// Redact redacts, in place, the runs and the note a write to the workspace
// home records, under the policy that governs the document's file, and drops
// the change set as sent.
func (k *keptRedaction) Redact(ctx context.Context, c *workhome.Commit) error {
	path := c.Path
	if reconcile.IsDocumentKey(path) {
		path = ""
	}
	policies, keepsRuns := k.policy.policiesFor(ctx, c.Doc, path)
	switch {
	case detectsEntities(policies):
		return errKeepsNoEdition
	case !keepsRuns:
		return &change.Error{Code: change.CodeUnsupported, Capability: "redaction",
			Message: "the project declares more than one redaction policy and kapi cannot name the file this translation belongs to, so it cannot be kept without the values one of them withholds"}
	}
	c.Set = nil
	if len(policies) == 0 {
		return nil
	}
	var seqs redactRuns
	for i := range c.Blocks {
		b := &c.Blocks[i]
		at := keptVaultID(c.Doc, b.Block, c.Edition, "")
		if len(b.BeforeRuns) > 0 {
			b.BeforeRuns = slices.Clone(b.BeforeRuns)
			seqs.add(at+b.Before, &b.BeforeRuns)
		}
		if b.Edition != nil {
			ed := *b.Edition
			ed.Runs = slices.Clone(ed.Runs)
			b.Edition = &ed
			seqs.add(at+b.After, &b.Edition.Runs)
		}
	}
	var note []model.Run
	if c.Note != "" {
		note = []model.Run{{Text: &model.TextRun{Text: c.Note}}}
		seqs.add("note:"+model.ComputeContentHash(c.Note), &note)
	}
	p := policies[0]
	groups := map[string]*redactRuns{p.At: {spec: p.Spec, ids: seqs.ids, sinks: seqs.sinks}}
	if err := k.policy.redactGroups(ctx, groups); err != nil {
		return err
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
// they are. A vault it cannot read fails the read, as it refuses a write: an
// edition read with its placeholders where the originals are on this machine
// would pass for the edition as written.
func (k *keptRedaction) Restore(_ context.Context, r workhome.Row, runs []model.Run) ([]model.Run, error) {
	if len(runs) == 0 {
		return runs, nil
	}
	id := keptVaultID(r.Doc, r.Block, r.Edition, r.Rev)
	entries, err := k.valuesFor(id)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return runs, nil
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
		return runs, nil
	}
	return restored, nil
}

// valuesFor returns what the project vault holds for one vault id. The vault
// is read again whenever it changed since the last read, which another
// process writing a kept edition, or this one, changes it. A missing vault
// holds nothing. A vault that cannot be read is an error, with the refusal of
// one an earlier kapi wrote naming what to run (runForProjectVault), and the
// next call reads it again.
func (k *keptRedaction) valuesFor(id string) ([]redaction.RedactedValue, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	info, err := os.Stat(k.policy.vault)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !k.loaded || info.Size() != k.size || !info.ModTime().Equal(k.mod) {
		v, err := redaction.OpenFileVault(k.policy.vault)
		if err != nil {
			k.loaded, k.values = false, nil
			return nil, redaction.WithRun(err, runForProjectVault)
		}
		k.loaded, k.size, k.mod = true, info.Size(), info.ModTime()
		k.values = map[string][]redaction.RedactedValue{}
		// A kept edition's originals sit in no document: its vault id names
		// it (keptVaultID). An entry a push recorded for a file names the file,
		// and a vault id never answers for it.
		for _, rv := range v.All() {
			if rv.Doc != "" {
				continue
			}
			k.values[rv.BlockID] = append(k.values[rv.BlockID], rv)
		}
	}
	return k.values[id], nil
}

// keptVaultID names the run sequence of one edition at one revision in the
// project vault, as the recorder names the runs a record keeps.
func keptVaultID(doc, block, edition, rev string) string {
	return "edit:" + doc + "#" + block + "@" + edition + ":" + rev
}
