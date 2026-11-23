package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/neokapi/neokapi/bowrain/storage"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/terms"
)

// conceptInfoAt is a concept on the read API with the revision it was read
// at, which an editor sends back as its base revision.
func conceptInfoAt(concept terms.Concept, revision string) ConceptInfoResponse {
	info := editorConceptToInfo(concept)
	info.Revision = revision
	return info
}

// withConceptRevisions fills the revision of each listed concept.
func withConceptRevisions(ctx context.Context, w sqlterms.ConceptWriter, infos []ConceptInfoResponse) error {
	ids := make([]string, len(infos))
	for i, info := range infos {
		ids[i] = info.ID
	}
	revs, err := w.ConceptRevisions(ctx, ids)
	if err != nil {
		return err
	}
	for i := range infos {
		infos[i].Revision = revs[infos[i].ID]
	}
	return nil
}

// refuseConceptWrite answers a concept write the store refused: 409 with the
// concept as it now stands when it moved since the writer read it, 404 when
// it is gone, 503 when it kept changing under an automated rewrite.
func refuseConceptWrite(c *echo.Context, err error) error {
	var stale *sqlterms.StaleConceptError
	switch {
	case errors.As(err, &stale):
		return refuseChanged(c, "concept_changed", "concept", conceptInfoAt(stale.Current, stale.Revision))
	case errors.Is(err, storage.ErrStale):
		return refuseChanged(c, "concept_changed", "concept", nil)
	case errors.Is(err, sqlterms.ErrConceptNotFound):
		return refuseGone(c, "concept")
	case errors.Is(err, sqlterms.ErrKeptChanging):
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "this concept is being changed by several saves at once; try again"})
	}
	return serverErr(c, err)
}
