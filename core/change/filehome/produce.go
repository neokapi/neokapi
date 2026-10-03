package filehome

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/atomicfile"
)

// A flow writes a document whole. Its writer renders every block the run
// passed through into the file the run names: the input itself for a run that
// edits its own content, or a target-language file the writer builds from the
// input's skeleton. Produce stages those bytes beside the file they replace,
// and Commit puts them in place under the lock a change set's commit takes
// for the same file, with the same check: the rename happens only while the
// file still holds what it held when the run read it. A flow and a change set
// writing one file therefore take turns, and a file a person saved while the
// run worked is never overwritten.
//
// A moved file is not re-applied here: the home holds the flow's bytes, not
// its operations. Commit reports it with ErrMoved, and a caller that recorded
// the run's operations applies them through the change service, which reads
// the file again under the lock and refuses the operations whose if_match no
// longer holds.

// ErrMoved is what Commit returns, wrapped in a *MovedError, when the file
// changed after the producer read it.
var ErrMoved = errors.New("the file changed after the run read it")

// MovedError says which file moved and from what.
type MovedError struct {
	Path string
	// Before is the digest the producer read; empty when the file did not
	// exist. Now is the digest it holds; empty when it was removed.
	Before, Now string
}

func (e *MovedError) Error() string {
	switch {
	case e.Before == "":
		return e.Path + " was created by another writer while the run worked; nothing was written to it"
	case e.Now == "":
		return e.Path + " was removed while the run worked; nothing was written to it"
	}
	return e.Path + " changed while the run worked; nothing was written to it"
}

// Is reports ErrMoved.
func (e *MovedError) Is(target error) bool { return target == ErrMoved }

// Digest is the digest of the file at path in the form Produce takes as
// before, or "" when there is no file.
func Digest(path string) (string, error) { return hashFile(path) }

// Produced is a document a producer wrote, staged and not yet committed.
type Produced struct {
	h       *Home
	path    string
	before  string
	after   string
	tmp     *atomicfile.Staged
	written bool
	done    bool
}

// Produce stages what write produces as the new content of the file at path.
// before is the file's digest when the producer read it (Digest), "" when
// there was no file then. Nothing a reader sees changes until Commit. The
// staged file sits beside the destination with the destination's mode, and a
// directory the destination needs is created when it commits, never before.
//
// A home whose options set WriteNothing stages nothing: write runs into the
// digest alone, and Commit writes nothing.
func (h *Home) Produce(ctx context.Context, path, before string, write func(io.Writer) error) (*Produced, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p := &Produced{h: h, path: path, before: before}
	sum := sha256.New()
	if h.writeNothing {
		if err := write(sum); err != nil {
			return nil, err
		}
		p.after = digestOf(sum)
		return p, nil
	}
	tmp, err := atomicfile.StageWithParents(path, func(w io.Writer) error {
		return write(io.MultiWriter(w, sum))
	})
	if err != nil {
		return nil, err
	}
	p.tmp = tmp
	p.after = digestOf(sum)
	return p, nil
}

// Path is the file the document is written to.
func (p *Produced) Path() string { return p.path }

// Before is the file's digest when the producer read it; empty when there was
// no file.
func (p *Produced) Before() string { return p.before }

// After is the digest of the bytes the producer wrote.
func (p *Produced) After() string { return p.after }

// Written says Commit put the bytes in place. A commit that found the file
// already holding them writes nothing.
func (p *Produced) Written() bool { return p.written }

// Commit takes the lock of the file, checks that it still holds what the
// producer read, and renames the staged file onto it. A file that already
// holds the produced bytes is left as it is. A file that moved is left as it
// is too, and Commit returns a *MovedError (errors.Is ErrMoved). Commit runs
// once; Release after it is safe.
func (p *Produced) Commit(ctx context.Context) error {
	if p.done {
		return fmt.Errorf("commit %s: the produced document was already committed or released", p.path)
	}
	p.done = true
	if p.tmp == nil {
		// A home that writes nothing.
		return nil
	}
	tmp := p.tmp
	p.tmp = nil
	key, err := p.h.lockPath(p.path)
	if err != nil {
		_ = tmp.Discard()
		return err
	}
	l, err := p.h.openLock(key)
	if err != nil {
		_ = tmp.Discard()
		return err
	}
	defer l.Close()
	if err := l.Lock(ctx); err != nil {
		_ = tmp.Discard()
		return err
	}
	defer l.Unlock()
	now, err := hashFile(p.path)
	if err != nil {
		_ = tmp.Discard()
		return err
	}
	switch {
	case now == p.after:
		// The file holds these bytes already: an identical run committed
		// first, or the run changed nothing.
		return tmp.Discard()
	case now != p.before:
		_ = tmp.Discard()
		return &MovedError{Path: p.path, Before: p.before, Now: now}
	}
	if err := tmp.Commit(); err != nil {
		return err
	}
	p.written = true
	return nil
}

// Release discards a staged document that was not committed. It is safe after
// Commit and more than once.
func (p *Produced) Release() error {
	p.done = true
	if p.tmp == nil {
		return nil
	}
	tmp := p.tmp
	p.tmp = nil
	return tmp.Discard()
}
