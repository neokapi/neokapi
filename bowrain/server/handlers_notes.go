package server

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/core/model"
)

// BlockNoteResponse is the API response for a block note.
//
// A note is an annotation of type note on the block's own edition: a change set
// adds one with annotate ({"type": "note", "value": {"text": …}}) and removes
// one with unannotate by its id, through the stream's changes route. The
// server stamps who wrote it and when as it lands.
type BlockNoteResponse struct {
	ID        string `json:"id"`
	BlockID   string `json:"blockId"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
}

// HandleListBlockNotes returns a block's notes, oldest first.
//
// GET /:ws/:id/blocks/:ref/:bid/notes
func (s *Server) HandleListBlockNotes(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	sb, err := s.ContentStore.GetBlock(c.Request().Context(), projectParam(c), streamParam(c), c.Param("bid"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, blockNotes(sb.Block))
}

// blockNotes reads the note annotations on b's own edition, oldest first.
func blockNotes(b *model.Block) []BlockNoteResponse {
	out := []BlockNoteResponse{}
	if b == nil {
		return out
	}
	o := b.OverlayOf(model.OverlayType(noteAnnotation))
	if o == nil {
		return out
	}
	for _, span := range o.Spans {
		out = append(out, BlockNoteResponse{
			ID:        span.ID,
			BlockID:   b.ID,
			Author:    span.Props[notePropAuthor],
			Text:      noteText(span.Value),
			CreatedAt: span.Props[notePropCreated],
		})
	}
	slices.SortStableFunc(out, func(a, b BlockNoteResponse) int { return strings.Compare(a.CreatedAt, b.CreatedAt) })
	return out
}

// noteText is the text a note annotation carries.
func noteText(v model.Payload) string {
	switch n := v.(type) {
	case nil:
		return ""
	case *model.NoteAnnotation:
		return n.Text
	case *model.Notes:
		texts := make([]string, 0, len(n.Items))
		for _, item := range n.Items {
			if item != nil && item.Text != "" {
				texts = append(texts, item.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	var body struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &body) == nil && body.Text != "" {
		return body.Text
	}
	if ra, ok := v.(*model.RawAnnotation); ok {
		if json.Unmarshal(ra.Body, &body) == nil {
			return body.Text
		}
	}
	return ""
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
