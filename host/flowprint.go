package host

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sync"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// --print-ops: a flow run that prints the change set it would apply.
//
// The run reads, runs its tools and renders every document as it would, into
// a home that writes nothing (filehome.Options.WriteNothing), and records
// nothing. What it changed in each document becomes operations: the
// difference between each block as the reader gave it and as the writer
// received it (change.Diff), each guarded by the revision the change service
// read before the run, and a derived edition's set_content carrying the
// revision of the source it was made from as its basis. The operations of
// every document make one change set, which kapi apply applies to the same
// bytes the run would have written.

// WithPrintedOps runs run as the command asks: with --print-ops, every
// document the run's flows write is held back, nothing is recorded, the run's
// own report is silenced, and the change set the run would apply is printed
// to the command's output; without it, run runs as it is.
func (a *App) WithPrintedOps(cmd Command, run func() error) error {
	if print, _ := cmd.Flags().GetBool(printOpsFlag); !print {
		return run()
	}
	ops := newPrintedOps()
	quiet := a.Quiet
	a.printOps, a.Quiet = ops, true
	defer func() { a.printOps, a.Quiet = nil, quiet }()
	if err := run(); err != nil {
		return err
	}
	for _, line := range ops.leftOut() {
		fmt.Fprintln(cmd.ErrOrStderr(), "note: "+line)
	}
	return ops.write(cmd.OutOrStdout())
}

// PrintOpsFlag names the flag every flow porcelain takes, for a command that
// registers it itself.
const PrintOpsFlag = printOpsFlag

// PrintOpsUsage is the flag's help text.
const PrintOpsUsage = printOpsUsage

// printOpsFlag is the flag every flow porcelain takes.
const printOpsFlag = "print-ops"

// printOpsUsage describes --print-ops.
const printOpsUsage = "write nothing and print the change set the run would apply, for kapi apply"

// printedOps collects the operations of every document a run would change.
type printedOps struct {
	mu   sync.Mutex
	docs map[string][]change.Op
	// left are the files the run would write that no change set addresses,
	// with why.
	left []string
}

// leaveOut notes a file the run would write where the change service does not
// keep edition k of the document ref: an output path the run was given rather
// than the one the recipe names. A change set cannot write it there, so its
// operations are not printed.
func (p *printedOps) leaveOut(path, ref string, k model.EditionKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.left = append(p.left, fmt.Sprintf("%s: not where kapi apply writes edition %s of %s; its operations are not printed",
		path, editionText(k), ref))
}

// leftOut lists the files the change set leaves out, sorted.
func (p *printedOps) leftOut() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := slices.Clone(p.left)
	slices.Sort(out)
	return out
}

func newPrintedOps() *printedOps { return &printedOps{docs: map[string][]change.Op{}} }

// add records the operations one document would take. A document written
// twice (a convergence pass per language) gathers both.
func (p *printedOps) add(ref string, ops []change.Op) {
	if len(ops) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.docs[ref] = append(p.docs[ref], ops...)
}

// set is the change set: the documents in reference order, each document's
// operations in the order the run wrote them.
func (p *printedOps) set() change.Set {
	p.mu.Lock()
	defer p.mu.Unlock()
	refs := make([]string, 0, len(p.docs))
	for ref := range p.docs {
		refs = append(refs, ref)
	}
	slices.Sort(refs)
	out := change.Set{Schema: change.SchemaID, Ops: []change.Op{}}
	for _, ref := range refs {
		out.Ops = append(out.Ops, p.docs[ref]...)
	}
	return out
}

// write prints the change set as indented JSON.
func (p *printedOps) write(w io.Writer) error {
	data, err := json.Marshal(p.set())
	if err != nil {
		return fmt.Errorf("print the change set: %w", err)
	}
	var out bytes.Buffer
	if err := json.Indent(&out, data, "", "  "); err != nil {
		return fmt.Errorf("print the change set: %w", err)
	}
	out.WriteByte('\n')
	_, err = w.Write(out.Bytes())
	return err
}

// printOps are the operations that turn the document as the reader gave it
// into the document the run would write.
func (doc *flowDoc) printOps() []change.Op {
	doc.mu.Lock()
	defer doc.mu.Unlock()
	var ops []change.Op
	for _, key := range doc.order {
		lb := doc.left[key]
		if lb == nil || lb.block == nil {
			continue
		}
		before, ok := doc.entered[lb.block.ID]
		if !ok {
			// A block a tool added has no operation that writes it here.
			continue
		}
		for _, op := range change.Diff(before, lb.block) {
			k := op.At.Edition.Canonical()
			if !doc.tracks(lb.block, k) {
				continue
			}
			text := editionText(k)
			was, read := doc.before[key].revision(text)
			if read {
				// The revision the service reads, which is what kapi apply
				// checks the operation against.
				if was == doc.flowRevision(doc.before[key], lb.block, k) {
					continue
				}
				op.IfMatch = was
			}
			if _, isSet := op.Body.(*change.SetContent); isSet && !k.IsZero() && !lb.block.IsSourceEdition(k) {
				op.Basis = doc.basis(key, lb)
			}
			op.At.Doc = doc.ref
			op.At.Block = key
			ops = append(ops, op)
		}
	}
	return ops
}

// tracks reports whether the run's record covers edition k of b.
func (doc *flowDoc) tracks(b *model.Block, k model.EditionKey) bool {
	for _, t := range doc.tracked(b) {
		if t.Canonical() == k.Canonical() {
			return true
		}
	}
	return false
}
