package host

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/place"
	"github.com/neokapi/neokapi/core/project"
)

// contextPlacesLimit is how many matches a query lists.
const contextPlacesLimit = 25

// ContextPlacesRequest names what `kapi context places` looks up in a
// project's gazetteer: the built-in places plus the ones its recipe declares.
type ContextPlacesRequest struct {
	// Project is the recipe path; empty reads the built-in places alone.
	Project string
	// Query matches places by id, then by name (place.Gazetteer.Search).
	Query string
	// Within lists the places directly below this one, or, with a query, keeps
	// the matches that sit within it.
	Within string
}

// ContextPlaces is the answer: matching places, each with its path from the
// world down.
type ContextPlaces struct {
	Query  string         `json:"query,omitempty"`
	Within string         `json:"within,omitempty"`
	Places []ContextPlace `json:"places"`
	// Total is how many places matched, which is more than Places holds when
	// a query matched past the listing's limit.
	Total int `json:"total"`
	// Declared reports that the listing is the recipe's declared places,
	// which is what a request with neither a query nor Within lists.
	Declared bool `json:"declared,omitempty"`
}

// ContextPlace is one place in the answer.
type ContextPlace struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Path     []string `json:"path"`
	Within   []string `json:"within,omitempty"`
	Of       []string `json:"of,omitempty"`
	Refs     []string `json:"refs,omitempty"`
	Declared bool     `json:"declared,omitempty"`
}

// ContextPlaces looks places up in the project's gazetteer: with a query, the
// places whose id or name matches; with Within, the places directly below
// that one; with neither, the places the recipe declares.
func (a *App) ContextPlaces(_ context.Context, req ContextPlacesRequest) (ContextPlaces, error) {
	g := place.Builtin()
	if req.Project != "" {
		proj, err := project.LoadWithOptions(req.Project, project.LoadOptions{SkipRequiresCheck: true})
		if err != nil {
			return ContextPlaces{}, err
		}
		g = proj.Gazetteer()
	}
	within := strings.TrimSpace(req.Within)
	if within != "" && !g.Has(within) {
		return ContextPlaces{}, fmt.Errorf("--within: %q is not a place. %s", within, place.Hint(g, within))
	}
	out := ContextPlaces{Query: strings.TrimSpace(req.Query), Within: within, Places: []ContextPlace{}}
	var found []place.Place
	switch {
	case out.Query != "":
		for _, p := range g.Search(out.Query, 0) {
			if within == "" || (p.ID != within && g.Contains(within, p.ID)) {
				found = append(found, p)
			}
		}
	case within != "":
		for _, id := range g.Children(within) {
			p, _ := g.Place(id)
			found = append(found, p)
		}
	default:
		found = g.Declared()
		out.Declared = true
	}
	out.Total = len(found)
	if out.Query != "" && len(found) > contextPlacesLimit {
		found = found[:contextPlacesLimit]
	}
	for _, p := range found {
		out.Places = append(out.Places, ContextPlace{
			ID: p.ID, Name: p.Name, Type: string(p.Type), Path: g.Path(p.ID),
			Within: p.Within, Of: p.Of, Refs: p.Refs, Declared: p.Declared,
		})
	}
	return out, nil
}

// FormatText prints one place per line: id, name, type and path.
func (m ContextPlaces) FormatText(w io.Writer) error {
	var b strings.Builder
	switch {
	case len(m.Places) == 0 && m.Declared:
		b.WriteString("This project declares no places. Look one up with `kapi context places <query>`.\n")
	case len(m.Places) == 0 && m.Query != "":
		fmt.Fprintf(&b, "No place matches %q.\n", m.Query)
	case len(m.Places) == 0:
		fmt.Fprintf(&b, "No place sits directly within %s.\n", m.Within)
	}
	idWidth, nameWidth, typeWidth := 0, 0, 0
	for _, p := range m.Places {
		idWidth, nameWidth, typeWidth = max(idWidth, len(p.ID)), max(nameWidth, len([]rune(p.Name))), max(typeWidth, len(p.Type))
	}
	for _, p := range m.Places {
		name := p.Name + strings.Repeat(" ", nameWidth-len([]rune(p.Name)))
		fmt.Fprintf(&b, "%-*s  %s  %-*s  %s\n", idWidth, p.ID, name, typeWidth, p.Type, strings.Join(p.Path, " › "))
	}
	if m.Total > len(m.Places) {
		fmt.Fprintf(&b, "%d of %d matches; narrow the query or add --within.\n", len(m.Places), m.Total)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
