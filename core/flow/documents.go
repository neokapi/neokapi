package flow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
)

// A file-writing run commits each document through the file home, the place a
// change set commits too (core/change/filehome). The runner digests the
// destination before it reads anything, streams the writer's output into a
// file staged beside the destination, and hands that staged document to its
// commit, which renames it into place under the destination's advisory lock
// only while the destination still holds what it held when the run began. A
// flow and a change set writing one file take turns, and a file a person
// saved while the run worked keeps the person's bytes: the run reports the
// document as moved (filehome.ErrMoved) and writes nothing to it.
//
// A caller that keeps a record of what a run changed, or prints it as a change
// set, follows each document through FileRunnerConfig.Documents: it sees every
// block as it leaves the reader and as it reaches the writer, and it commits
// the staged document itself.

// Document names one document a run writes.
type Document struct {
	// Flow is the name the run was started with.
	Flow string
	// InputPath is the file the run reads; OutputPath the file it writes,
	// which is InputPath for a run that edits its own content.
	InputPath  string
	OutputPath string
	// TargetLocale is the language the run writes, empty for a run over the
	// source alone.
	TargetLocale model.LocaleID
	// Format is the format the run reads the input with.
	Format string
}

// InPlace reports whether the run writes the file it reads.
func (d Document) InPlace() bool {
	if d.InputPath == d.OutputPath {
		return true
	}
	a, aerr := filepath.Abs(d.InputPath)
	b, berr := filepath.Abs(d.OutputPath)
	return aerr == nil && berr == nil && a == b
}

// Documents follows the documents a run writes.
type Documents interface {
	// Open is called once per document, before the run reads it. A nil run
	// commits the document as the default does.
	Open(ctx context.Context, d Document) (DocumentRun, error)
}

// DocumentRun follows one document through a run. Enter and Leave are called
// from different goroutines, for different blocks at once.
type DocumentRun interface {
	// Enter sees a block as it leaves the reader, before the first tool.
	Enter(b *model.Block)
	// Leave sees a block as the writer receives it, after the last tool.
	Leave(b *model.Block)
	// Commit is called once the writer has produced the document. It puts
	// the produced document in place (p.Commit) or leaves it, and returns
	// what the run reports for the document.
	Commit(ctx context.Context, p *filehome.Produced) error
	// Abort ends a document the run did not produce.
	Abort()
}

// home is the file home the run commits through: the configured one, else a
// home over the default lock directory.
func (r *FileRunner) home() *filehome.Home {
	if r.cfg.Home != nil {
		return r.cfg.Home
	}
	return defaultHome()
}

// defaultHome is the home a runner with none configured commits through.
var defaultHome = sync.OnceValue(func() *filehome.Home { return filehome.New(nil, filehome.Options{}) })

// openDocument digests the destination and opens the caller's follower for
// one document. It is called before the run reads the input, so the digest is
// of the file as the run found it.
func (r *FileRunner) openDocument(ctx context.Context, flowName, inputPath, outputPath, targetLang, formatName string) (*writtenDocument, error) {
	// A blocked destination is refused here, before anything is read, with
	// the error that names what stands in the way.
	if err := CheckOutputPath(outputPath); err != nil {
		return nil, err
	}
	before, err := filehome.Digest(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(outputPath), err)
	}
	w := &writtenDocument{before: before}
	if r.cfg.Documents != nil {
		run, oerr := r.cfg.Documents.Open(ctx, Document{
			Flow: flowName, InputPath: inputPath, OutputPath: outputPath,
			TargetLocale: model.LocaleID(targetLang), Format: formatName,
		})
		if oerr != nil {
			return nil, oerr
		}
		w.run = run
	}
	return w, nil
}

// writtenDocument is one document on its way to the file home: the digest of
// the destination the run began from, and the caller's follower.
type writtenDocument struct {
	before string
	run    DocumentRun
	done   bool
}

func (w *writtenDocument) enter(b *model.Block) {
	if w != nil && w.run != nil {
		w.run.Enter(b)
	}
}

func (w *writtenDocument) leave(b *model.Block) {
	if w != nil && w.run != nil {
		w.run.Leave(b)
	}
}

// following reports whether a caller follows the blocks.
func (w *writtenDocument) following() bool { return w != nil && w.run != nil }

// commit hands the produced document to its commit.
func (w *writtenDocument) commit(ctx context.Context, p *filehome.Produced) error {
	w.done = true
	if w.run != nil {
		return w.run.Commit(ctx, p)
	}
	return p.Commit(ctx)
}

// abort ends a document that was not produced. It is safe after commit.
func (w *writtenDocument) abort() {
	if w == nil || w.done {
		return
	}
	w.done = true
	if w.run != nil {
		w.run.Abort()
	}
}

// IsMoved reports whether a run's error says a destination changed while the
// run worked, so nothing was written to it.
func IsMoved(err error) bool { return errors.Is(err, filehome.ErrMoved) }
