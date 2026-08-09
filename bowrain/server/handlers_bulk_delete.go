package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
)

// Bulk deletes for the resource browser's multi-select. Selecting rows and
// deleting them is one request, not one per row.

// maxBulkDeleteIDs bounds a single bulk-delete batch.
const maxBulkDeleteIDs = 500

// BulkDeleteRequest is the body of a bulk-delete call.
type BulkDeleteRequest struct {
	IDs []string `json:"ids"`
}

// BulkDeleteResult is one id's outcome.
type BulkDeleteResult struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
	Error   string `json:"error,omitempty"`
}

// BulkDeleteResponse reports every id's outcome plus the totals. The call
// succeeds as a whole even when individual ids fail — a caller reads Results to
// see which, and the surviving rows stay selected.
type BulkDeleteResponse struct {
	Results []BulkDeleteResult `json:"results"`
	Deleted int                `json:"deleted"`
	Failed  int                `json:"failed"`
}

// bulkDeleteIDs binds and validates the id list, rejecting an empty or
// oversized batch.
func bulkDeleteIDs(c echo.Context) ([]string, error) {
	var req BulkDeleteRequest
	if err := c.Bind(&req); err != nil {
		return nil, c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	if len(req.IDs) == 0 {
		return nil, c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ids is required"})
	}
	if len(req.IDs) > maxBulkDeleteIDs {
		return nil, c.JSON(http.StatusBadRequest, ErrorResponse{Error: "too many ids in one batch"})
	}
	return req.IDs, nil
}

// HandleBulkDeleteMemoryEntries deletes several content-memory entries in one
// call: POST /api/v1/:ws/translation-memory/bulk-delete {"ids": [...]}.
//
// A repeated id is reported once. An id that fails to delete is reported as
// failed and the rest of the batch still goes through — a single missing entry
// must not strand the selection.
func (s *Server) HandleBulkDeleteMemoryEntries(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageMemory); err != nil {
		return err
	}
	if s.wsStores == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ids, errResp := bulkDeleteIDs(c)
	if errResp != nil {
		return errResp
	}

	tm, err := s.wsStores.getMemory(c.Param("ws"))
	if err != nil {
		return serverErr(c, err)
	}

	ctx := c.Request().Context()
	resp := BulkDeleteResponse{Results: make([]BulkDeleteResult, 0, len(ids))}
	seen := make(map[string]struct{}, len(ids))
	for _, entryID := range ids {
		if _, dup := seen[entryID]; dup {
			continue
		}
		seen[entryID] = struct{}{}

		result := BulkDeleteResult{ID: entryID}
		if delErr := tm.Delete(ctx, entryID); delErr != nil {
			result.Error = delErr.Error()
			resp.Failed++
		} else {
			result.Deleted = true
			resp.Deleted++
		}
		resp.Results = append(resp.Results, result)
	}

	return c.JSON(http.StatusOK, resp)
}

// HandleBulkDeleteConcepts answers a bulk concept delete:
// POST /api/v1/:ws/concepts/bulk-delete {"ids": [...]}.
//
// Deleting a concept is a governed transition (AD-021), so the batch is refused
// with the same 409 and change-set hint the per-concept route returns. The
// route exists so a multi-select learns that in one round trip instead of one
// refusal per selected row.
func (s *Server) HandleBulkDeleteConcepts(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageTerms); err != nil {
		return err
	}
	if _, errResp := bulkDeleteIDs(c); errResp != nil {
		return errResp
	}
	return conceptGovernedConflict(c, "deleting concepts")
}
