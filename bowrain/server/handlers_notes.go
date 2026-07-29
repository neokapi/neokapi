package server

import (
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
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
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
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
		return serverErr(c, err)
	}

	// Dispatch mention notifications.
	if s.NotificationDispatcher != nil && s.AuthStore != nil {
		usernames := parseMentions(note.Text)
		for _, username := range usernames {
			user, err := s.AuthStore.GetUserByEmail(c.Request().Context(), username)
			if err == nil && user != nil {
				actorID, _ := c.Get("user_id").(string)
				actorName, _ := c.Get("name").(string)
				s.NotificationDispatcher.DispatchMention(
					c.Request().Context(),
					user.ID,
					actorID,
					actorName,
					note.Text,
					pid,
					"",
				)
			}
		}
	}

	return c.JSON(http.StatusCreated, blockNoteToResponse(note))
}

// HandleListBlockNotes returns all notes for a block.
func (s *Server) HandleListBlockNotes(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	bid := c.Param("bid")

	notes, err := s.ContentStore.ListBlockNotes(c.Request().Context(), pid, "main", bid)
	if err != nil {
		return serverErr(c, err)
	}

	result := make([]BlockNoteResponse, len(notes))
	for i, n := range notes {
		result[i] = blockNoteToResponse(n)
	}

	return c.JSON(http.StatusOK, result)
}

// HandleDeleteBlockNote deletes a note by ID.
//
// Reading a note and deleting it are not the same act. This gated on
// PermViewContent alone, which is exactly — and only — what the built-in
// observer role holds: read-only access to project content. So every member of
// a project, including one deliberately given no write access at all, could
// delete anyone's note. Deleting a note is either your own housekeeping or a
// manager's, and it now takes one of those.
func (s *Server) HandleDeleteBlockNote(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := projectParam(c)
	bid := c.Param("bid")
	nid := c.Param("nid")

	// The note has to be read before it can be authorized on — there is no
	// store method that fetches one by id, but the route carries the block, so
	// the block's notes are enough.
	notes, err := s.ContentStore.ListBlockNotes(c.Request().Context(), pid, "main", bid)
	if err != nil {
		return serverErr(c, err)
	}
	idx := slices.IndexFunc(notes, func(n model.BlockNote) bool { return n.ID == nid })
	if idx < 0 {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "note not found"})
	}
	if !s.mayDeleteNote(c, notes[idx]) {
		return deny(c, "only the note's author or a project manager can delete a note")
	}

	if err := s.ContentStore.DeleteBlockNote(c.Request().Context(), pid, "main", nid); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// mayDeleteNote reports whether the caller may delete this note: its author, or
// anyone holding PermManageProject.
//
// Authorship is compared through extractAuthor, the same derivation that wrote
// the value when the note was created — so whoever left a note always matches
// their own. That value is a display name or an email rather than a stable user
// id, which is the weaker half of this check: two people sharing a display name
// inside one project could still delete each other's notes. Closing that means
// giving model.BlockNote an author id, which is a change to stored data and a
// migration; this closes the part that does not need one. An empty author
// matches nobody, so a caller whose token carries neither claim gains nothing.
func (s *Server) mayDeleteNote(c echo.Context, note model.BlockNote) bool {
	if perms, ok := c.Get("project_permissions").(platauth.Permission); ok &&
		perms.Has(platauth.PermManageProject) {
		return true
	}
	author := extractAuthor(c)
	return author != "" && author == note.Author
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

// parseMentions extracts @username mentions from text.
func parseMentions(text string) []string {
	re := regexp.MustCompile(`@(\w+)`)
	matches := re.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var usernames []string
	for _, m := range matches {
		if len(m) > 1 && !seen[m[1]] {
			seen[m[1]] = true
			usernames = append(usernames, m[1])
		}
	}
	return usernames
}

// extractAuthor pulls the user name from the auth context if available.
func extractAuthor(c echo.Context) string {
	if claims, ok := c.Get("user_claims").(map[string]any); ok {
		if name, ok := claims["name"].(string); ok {
			return name
		}
		if email, ok := claims["email"].(string); ok {
			return email
		}
	}
	return ""
}
