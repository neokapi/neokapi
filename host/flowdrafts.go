package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
)

// A convergence pass under a delivery gate writes drafts into the run's own
// tree (host/convergedrafts.go), and delivery puts a locale's drafts where
// the recipe points once the locale clears its gate. Delivery is the write
// that reaches a content file, so it is the one that commits through the file
// home and is recorded: each draft is committed onto its destination under
// the destination's lock, only while the destination still holds what the
// run first read there, and what it changed is recorded as the flow's
// content.edit. A destination a person saved while the run worked is applied
// again through the change service from the run's own operations, guarded by
// the revisions the run first read, as a moved file of an ungated run is.

// draftDeliveries holds, for each destination a run drafts, what its
// delivery needs: the digest and the revisions the run first read there, and
// the follower of the latest pass that drafted it.
type draftDeliveries struct {
	mu     sync.Mutex
	byDest map[string]*draftDelivery
}

// draftDelivery is one destination's delivery.
type draftDelivery struct {
	// before and revisions are the destination as the run first read it.
	before    string
	revisions revisions
	// doc follows the latest pass's draft of the destination.
	doc *flowDoc
}

func newDraftDeliveries() *draftDeliveries {
	return &draftDeliveries{byDest: map[string]*draftDelivery{}}
}

// first notes the destination's digest and revisions the first time a pass
// reads it, and returns what the run first read.
func (d *draftDeliveries) first(dest string, read func() (string, revisions)) *draftDelivery {
	if d == nil {
		return nil
	}
	key := absPath(dest)
	d.mu.Lock()
	defer d.mu.Unlock()
	if dd, ok := d.byDest[key]; ok {
		return dd
	}
	before, revs := read()
	dd := &draftDelivery{before: before, revisions: revs}
	d.byDest[key] = dd
	return dd
}

// drafted notes the follower of the pass that last drafted a destination.
func (d *draftDeliveries) drafted(doc *flowDoc) {
	if d == nil {
		return
	}
	key := absPath(doc.dest)
	d.mu.Lock()
	defer d.mu.Unlock()
	dd, ok := d.byDest[key]
	if !ok {
		dd = &draftDelivery{before: doc.destBefore, revisions: doc.before}
		d.byDest[key] = dd
	}
	dd.doc = doc
}

// take returns and forgets the delivery of a destination.
func (d *draftDeliveries) take(dest string) *draftDelivery {
	if d == nil {
		return nil
	}
	key := absPath(dest)
	d.mu.Lock()
	defer d.mu.Unlock()
	dd := d.byDest[key]
	delete(d.byDest, key)
	return dd
}

func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// noteDraft records that doc's pass drafted its destination.
func (a *App) noteDraft(doc *flowDoc, _ *filehome.Produced) {
	a.convergeDeliveries.drafted(doc)
}

// deliverDrafts commits one locale's drafted files onto the destinations the
// recipe resolved for them and returns those destinations. It is the delivery
// the ship gate governs: it runs for a locale that cleared its gate and for no
// other, so a parked locale's files are genuinely absent rather than present
// and unblessed. It delivers the run's own output rather than re-deriving it,
// because the pass is what produced the bytes; the block store is a second,
// independent record of the same work and is written on top afterwards.
//
// The destinations are named rather than counted because they are also the
// run's answer to "which translations on disk did I write".
func (a *App) deliverDrafts(ctx context.Context, locale model.LocaleID) ([]string, error) {
	if a.convergeDraftDir == "" || a.convergeDraftRoot == "" {
		return nil, nil
	}
	base := filepath.Join(a.convergeDraftDir, string(locale))
	if _, err := os.Stat(base); err != nil {
		return nil, nil
	}
	home := a.flowHome(a.convergeDraftRoot)
	var delivered []string
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(base, path)
		if rerr != nil {
			return rerr
		}
		dest := filepath.Join(a.convergeDraftRoot, rel)
		if derr := a.deliverDraft(ctx, home, path, dest); derr != nil {
			return fmt.Errorf("deliver %s: %w", rel, derr)
		}
		delivered = append(delivered, dest)
		return nil
	})
	return delivered, err
}

// deliverDraft commits the draft at path onto dest and records what it
// changed.
func (a *App) deliverDraft(ctx context.Context, home *filehome.Home, path, dest string) error {
	dd := a.convergeDeliveries.take(dest)
	var doc *flowDoc
	before := ""
	if dd != nil {
		before, doc = dd.before, dd.doc
		if doc != nil {
			doc.before = dd.revisions
		}
	} else {
		var err error
		if before, err = filehome.Digest(dest); err != nil {
			return err
		}
	}
	p, err := home.Produce(ctx, dest, before, func(w io.Writer) error {
		f, oerr := os.Open(path)
		if oerr != nil {
			return oerr
		}
		defer f.Close()
		_, cerr := io.Copy(w, f)
		return cerr
	})
	if err != nil {
		return err
	}
	defer func() { _ = p.Release() }()
	err = p.Commit(ctx)
	if errors.Is(err, filehome.ErrMoved) && doc != nil && doc.track && doc.svc != nil {
		return doc.applyAgain(ctx, err)
	}
	if err != nil {
		return err
	}
	if doc == nil || !doc.track || doc.fc.rec == nil {
		return nil
	}
	return doc.record(ctx, p.Written(), p.Before(), p.After())
}
