package sync

import (
	"context"
	"fmt"
	"strconv"

	"github.com/neokapi/neokapi/core/venue"
)

// The server's half of the content-model epoch — see core/venue.ContentModelEpoch
// for what the epoch is and when it moves.
//
// The stream records the highest generation its content has received. A push
// from a lower one is refused at init, before anything is uploaded, because
// applying it would flatten structure a richer kapi wrote and the loss is not
// recoverable from the server side. Recorded on commit, not on init: a push
// that negotiated and then failed must not close a stream to producers whose
// content it never delivered.

// ContentModelConflict is what a push from an older model generation is told.
// It names both generations, because "upgrade kapi" without a version is advice
// nobody can act on.
type ContentModelConflict struct {
	Stated   int
	Recorded int
}

func (e *ContentModelConflict) Error() string {
	return fmt.Sprintf(
		"this stream holds content from a newer kapi (content model %d); this one writes %d. "+
			"Upgrade kapi, or push --force to overwrite it with what this version can read",
		e.Recorded, e.Stated)
}

// CheckContentModelEpoch refuses a push that would downgrade the stream's
// content model.
//
// A producer that states nothing reads as generation 0 — it predates the
// mechanism, so it cannot be assumed to write what the stream already holds.
func (d *DiffEngine) CheckContentModelEpoch(
	ctx context.Context,
	projectID, stream string,
	stated int,
) error {
	recorded, err := d.recordedEpoch(ctx, projectID, stream)
	if err != nil {
		return err
	}
	if stated < recorded {
		return &ContentModelConflict{Stated: stated, Recorded: recorded}
	}
	return nil
}

// RecordContentModelEpoch raises the stream's recorded generation to the one a
// committed push carried. It never lowers it: a forced downgrade writes older
// content, and the stream still holds everything the richer producer wrote for
// items that push did not touch.
func (d *DiffEngine) RecordContentModelEpoch(
	ctx context.Context,
	projectID, stream string,
	stated int,
) error {
	if stated <= 0 {
		return nil
	}
	s, err := d.contentStore.GetStream(ctx, projectID, stream)
	if err != nil || s == nil {
		// A stream the store does not hold is not worth failing a committed
		// push over: the next push records it.
		return nil //nolint:nilerr // see comment
	}
	if current := epochOf(s.Properties); stated <= current {
		return nil
	}
	if s.Properties == nil {
		s.Properties = map[string]string{}
	}
	s.Properties[venue.ContentModelEpochProperty] = strconv.Itoa(stated)
	if err := d.contentStore.UpdateStream(ctx, s); err != nil {
		return fmt.Errorf("record content model epoch: %w", err)
	}
	return nil
}

func (d *DiffEngine) recordedEpoch(ctx context.Context, projectID, stream string) (int, error) {
	s, err := d.contentStore.GetStream(ctx, projectID, stream)
	if err != nil || s == nil {
		// A stream with no row holds no content either, so there is nothing to
		// downgrade.
		return 0, nil //nolint:nilerr // see comment
	}
	return epochOf(s.Properties), nil
}

func epochOf(props map[string]string) int {
	n, err := strconv.Atoi(props[venue.ContentModelEpochProperty])
	if err != nil {
		return 0
	}
	return n
}
