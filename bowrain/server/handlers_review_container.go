package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// The container of a block: what a reader meets the block inside. A string
// from a React component is read with the component's other strings, in JSX
// order; a paragraph of a document with the section it sits in, from its
// heading to the next; anything else with the blocks around it. Each unit
// carries its role (heading, paragraph, button, list item), so a client can
// lay the container out as it reads, and the revisions a decision binds to,
// so a reviewer can decide any waiting unit in place.

const (
	// containerReach is how far either side of the block the container looks
	// for its edges.
	containerReach = 80
	// containerWindow is how many blocks either side a container with no
	// edges (no component, no headings) shows.
	containerWindow = 12
)

// containerUnit is one block of a container.
type containerUnit struct {
	BlockID  string `json:"block_id"`
	ItemName string `json:"item_name"`
	Key      string `json:"key"`
	// Role is heading, paragraph, list_item, button, label, link, caption,
	// cell, code or text.
	Role string `json:"role"`
	// Level is a heading's level, 1 to 6.
	Level      int         `json:"level,omitempty"`
	SourceRuns []model.Run `json:"source_runs"`
	TargetRuns []model.Run `json:"target_runs"`
	// TargetStatus is the translation's status; empty when there is none.
	TargetStatus string `json:"target_status,omitempty"`
	// Waiting is a translation waiting for review: present and not approved.
	Waiting        bool   `json:"waiting"`
	SourceRevision string `json:"source_revision"`
	TargetRevision string `json:"target_revision"`
}

// reviewContainerResponse is the body of GET .../container.
type reviewContainerResponse struct {
	// Kind is component, section or window.
	Kind string `json:"kind"`
	// Title names the container: the component, the section's heading, or
	// the file.
	Title    string          `json:"title"`
	ItemName string          `json:"item_name"`
	Locale   string          `json:"locale"`
	Focus    string          `json:"focus"`
	Units    []containerUnit `json:"units"`
	// More reports blocks of the container past what the response holds.
	MoreBefore bool `json:"more_before"`
	MoreAfter  bool `json:"more_after"`
}

// HandleGetReviewContainer answers GET /:ws/:id/blocks/:ref/:bid/container
// ?target_locale=nb with the block's container.
func (s *Server) HandleGetReviewContainer(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	loc := strings.TrimSpace(c.QueryParam("target_locale"))
	if loc == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "target_locale is required"})
	}
	ctx := c.Request().Context()
	pid := projectParam(c)
	stream := streamParam(c)
	sb, err := s.ContentStore.GetBlock(ctx, pid, stream, c.Param("bid"))
	if err != nil || sb == nil || sb.Block == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found"})
	}
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	var before, after []*venue.StoredBlock
	if sb.ItemName != "" {
		translatable := true
		read := func(q store.BlockQuery) []*venue.StoredBlock {
			q.ProjectID, q.Stream, q.ItemName = pid, stream, sb.ItemName
			q.Translatable, q.Limit = &translatable, containerReach
			blocks, err := s.ContentStore.GetBlocks(ctx, q)
			if err != nil {
				return nil
			}
			return blocks
		}
		// Before arrives nearest last, After nearest first: document order.
		before = read(store.BlockQuery{DocumentBefore: sb.Block.ID})
		after = read(store.BlockQuery{DocumentAfter: sb.Block.ID})
	}

	out := containerOf(sb, before, after)
	out.Locale = loc
	targetLoc := model.LocaleID(loc)
	for _, b := range out.blocks {
		out.Units = append(out.Units, containerUnitOf(b, targetLoc, proj.DefaultSourceLanguage))
	}
	if out.Units == nil {
		out.Units = []containerUnit{}
	}
	return c.JSON(http.StatusOK, out.reviewContainerResponse)
}

// container is a response with the blocks it was built from.
type container struct {
	reviewContainerResponse
	blocks []*venue.StoredBlock
}

// containerOf picks the block's container from the blocks before and after
// it, both in document order.
func containerOf(sb *venue.StoredBlock, before, after []*venue.StoredBlock) container {
	out := container{reviewContainerResponse: reviewContainerResponse{
		ItemName: sb.ItemName,
		Focus:    sb.Block.ID,
	}}

	// A string from a component: the component's other strings.
	if comp := sb.Block.Properties["component"]; comp != "" {
		same := func(b *venue.StoredBlock) bool {
			return b != nil && b.Block != nil && b.Block.Properties["component"] == comp
		}
		i := len(before)
		for i > 0 && same(before[i-1]) {
			i--
		}
		j := 0
		for j < len(after) && same(after[j]) {
			j++
		}
		out.Kind, out.Title = "component", comp
		out.blocks = append(append(append([]*venue.StoredBlock{}, before[i:]...), sb), after[:j]...)
		out.MoreBefore = i == 0 && len(before) == containerReach
		out.MoreAfter = j == len(after) && len(after) == containerReach
		return out
	}

	// A block of a document with headings: its section, from the heading at
	// or above it to the next heading of the same or a higher level.
	headIdx, headLevel := -1, 0
	if r, lvl := blockRole(sb.Block); r == "heading" {
		headIdx, headLevel = len(before), lvl
	} else {
		for k := len(before) - 1; k >= 0; k-- {
			if r, lvl := blockRole(before[k].Block); r == "heading" {
				headIdx, headLevel = k, lvl
				break
			}
		}
	}
	if headIdx >= 0 {
		head, start := sb, []*venue.StoredBlock(nil)
		if headIdx < len(before) {
			head, start = before[headIdx], before[headIdx:]
		}
		end := len(after)
		for k, b := range after {
			if r, lvl := blockRole(b.Block); r == "heading" && lvl <= headLevel {
				end = k
				break
			}
		}
		out.Kind = "section"
		out.Title = model.RunsDisplayText(head.Block.SourceRuns())
		out.blocks = append(append(append([]*venue.StoredBlock{}, start...), sb), after[:end]...)
		out.MoreAfter = end == len(after) && len(after) == containerReach
		return out
	}

	// Otherwise the blocks around it.
	i := max(0, len(before)-containerWindow)
	j := min(len(after), containerWindow)
	out.Kind = "window"
	out.Title = sb.ItemName
	out.blocks = append(append(append([]*venue.StoredBlock{}, before[i:]...), sb), after[:j]...)
	out.MoreBefore = i > 0 || len(before) == containerReach
	out.MoreAfter = j < len(after)
	return out
}

func containerUnitOf(sb *venue.StoredBlock, loc model.LocaleID, sourceLang model.LocaleID) containerUnit {
	b := sb.Block
	role, level := blockRole(b)
	target, has := b.TargetEdition(loc)
	u := containerUnit{
		BlockID:        b.ID,
		ItemName:       sb.ItemName,
		Key:            b.Name,
		Role:           role,
		Level:          level,
		SourceRuns:     nonNilRuns(b.SourceRuns()),
		TargetRuns:     []model.Run{},
		SourceRevision: check.SourceRevision(b, sourceLang),
		TargetRevision: store.TargetRevision(sb, loc),
	}
	if has && len(target.Runs) > 0 {
		u.TargetRuns = target.Runs
		u.TargetStatus = string(target.Status)
		u.Waiting = target.Status != model.Status(model.TargetStatusEstablished)
	}
	return u
}

// blockRole reads what a block is in its document from what the reader
// recorded: the structure annotation, the block's type (a markdown heading
// is "heading" with a "level"), or a React string's element and JSX path.
func blockRole(b *model.Block) (string, int) {
	if b == nil {
		return "text", 0
	}
	if st, ok := b.Structure(); ok && st != nil {
		switch st.Role {
		case model.RoleHeading, model.RoleTitle:
			return "heading", clampLevel(st.Level)
		case model.RoleListItem:
			return "list_item", 0
		case model.RoleCaption:
			return "caption", 0
		case model.RoleTableCell, model.RoleTableHeader:
			return "cell", 0
		case model.RoleCode:
			return "code", 0
		case model.RoleParagraph:
			return "paragraph", 0
		}
	}
	if role, level := roleOfName(strings.ToLower(b.Type)); role != "" {
		if role == "heading" && level == 0 {
			level = clampLevel(atoi(b.Properties["level"]))
		}
		return role, level
	}
	// A React string: the element it renders in, then the last JSX path step.
	for _, name := range []string{b.Properties["jsxPath"], b.Properties["element"]} {
		if i := strings.LastIndexAny(name, "/>. "); i >= 0 {
			name = name[i+1:]
		}
		if role, level := roleOfName(strings.ToLower(name)); role != "" {
			return role, level
		}
	}
	if el := strings.ToLower(b.Properties["element"]); el != "" {
		switch {
		case strings.Contains(el, "button"):
			return "button", 0
		case strings.Contains(el, "title"), strings.Contains(el, "heading"):
			return "heading", 2
		case strings.Contains(el, "label"):
			return "label", 0
		case strings.Contains(el, "link"):
			return "link", 0
		}
	}
	return "paragraph", 0
}

// roleOfName maps a block type or element name to a role.
func roleOfName(name string) (string, int) {
	switch name {
	case "heading", "title":
		return "heading", 0
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "heading", int(name[1] - '0')
	case "p", "paragraph":
		return "paragraph", 0
	case "li", "list-item", "listitem":
		return "list_item", 0
	case "button":
		return "button", 0
	case "label", "legend":
		return "label", 0
	case "a", "link":
		return "link", 0
	case "caption", "figcaption":
		return "caption", 0
	case "td", "th", "cell", "table-cell":
		return "cell", 0
	case "code", "code-block", "pre":
		return "code", 0
	case "admonition-title", "docusaurus-admonition-title":
		return "heading", 5
	}
	return "", 0
}

func clampLevel(n int) int {
	return min(max(n, 1), 6)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
