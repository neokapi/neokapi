package server

import (
	"net/http"
	"time"

	"github.com/gokapi/gokapi/core/id"
	"github.com/gokapi/gokapi/core/model"
	"github.com/labstack/echo/v4"
)

// BlockNoteResponse is the API response for a block note.
type BlockNoteResponse struct {
	ID        string `json:"id"`
	BlockID   string `json:"blockId"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
}

// HandleAddBlockNote creates a new note on a block.
func (s *Server) HandleAddBlockNote(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := c.Param("pid")
	bid := c.Param("bid")

	var req struct {
		Text string `json:"text"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Text == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "text is required"})
	}

	note := model.BlockNote{
		ID:        id.New(),
		BlockID:   bid,
		Author:    extractAuthor(c),
		Text:      req.Text,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.ContentStore.AddBlockNote(c.Request().Context(), pid, "main", bid, note); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusCreated, blockNoteToResponse(note))
}

// HandleListBlockNotes returns all notes for a block.
func (s *Server) HandleListBlockNotes(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := c.Param("pid")
	bid := c.Param("bid")

	notes, err := s.ContentStore.ListBlockNotes(c.Request().Context(), pid, "main", bid)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	result := make([]BlockNoteResponse, len(notes))
	for i, n := range notes {
		result[i] = blockNoteToResponse(n)
	}

	return c.JSON(http.StatusOK, result)
}

// HandleDeleteBlockNote deletes a note by ID.
func (s *Server) HandleDeleteBlockNote(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := c.Param("pid")
	nid := c.Param("nid")

	if err := s.ContentStore.DeleteBlockNote(c.Request().Context(), pid, "main", nid); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

func blockNoteToResponse(n model.BlockNote) BlockNoteResponse {
	return BlockNoteResponse{
		ID:        n.ID,
		BlockID:   n.BlockID,
		Author:    n.Author,
		Text:      n.Text,
		CreatedAt: n.CreatedAt.Format(time.RFC3339),
	}
}

// extractAuthor pulls the user name from the auth context if available.
func extractAuthor(c echo.Context) string {
	if claims, ok := c.Get("user_claims").(map[string]interface{}); ok {
		if name, ok := claims["name"].(string); ok {
			return name
		}
		if email, ok := claims["email"].(string); ok {
			return email
		}
	}
	return ""
}
