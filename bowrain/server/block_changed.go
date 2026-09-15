package server

import (
	"errors"
	"net/http"
	"slices"

	"github.com/labstack/echo/v4"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// blockChangedError is a single block write refused because the block changed
// after the writer read it. It carries the block as it stands.
type blockChangedError struct {
	current *venue.StoredBlock
}

func (e *blockChangedError) Error() string { return platstore.ErrBlockChanged.Error() }

func (e *blockChangedError) Unwrap() error { return platstore.ErrBlockChanged }

// checkBaseRevision refuses a write when the writer named the target revision
// it read and the block no longer has it.
func checkBaseRevision(current *venue.StoredBlock, locale model.LocaleID, base string) error {
	if base == "" || platstore.TargetRevision(current, locale) == base {
		return nil
	}
	return &blockChangedError{current: current}
}

// errNothingToWrite ends an UpdateBlock whose change leaves the block as it
// was, so nothing is stored or logged.
var errNothingToWrite = errors.New("nothing to write")

// nothingToWrite reports whether err is errNothingToWrite.
func nothingToWrite(err error) bool { return errors.Is(err, errNothingToWrite) }

// asBlockChanged reports whether err refused a write for a changed block.
func asBlockChanged(err error) (*blockChangedError, bool) {
	var changed *blockChangedError
	ok := errors.As(err, &changed)
	return changed, ok
}

// BlockChangedResponse answers a single block write made against an older read
// of the block: why it was refused, and the block as it now stands, so the
// writer decides again on the current wording.
type BlockChangedResponse struct {
	Code    string            `json:"code"`
	Error   string            `json:"error"`
	Current BlockInfoResponse `json:"current"`
}

// answerBlockChanged writes the 409 for a refused write, carrying the block as
// the editor's blocks route reads it.
func (s *Server) answerBlockChanged(c echo.Context, projectID string, changed *blockChangedError, locale string) error {
	locales, _ := s.projectTargetLocales(c.Request().Context(), projectID)
	if locale != "" && !slices.Contains(locales, locale) {
		locales = append(locales, locale)
	}
	return c.JSON(http.StatusConflict, BlockChangedResponse{
		Code:    "block_changed",
		Error:   "this block changed after it was read; the current block is attached so the change can be decided on its wording",
		Current: storedBlockToInfoResponse(changed.current, locales),
	})
}
