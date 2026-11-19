package server

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	coreproject "github.com/neokapi/neokapi/core/project"
)

// The coordinates map: the project's points laid out as products by
// channels, with the blocks, the voice and the coverage per target language
// at each, and what changed since the last confirmation
// (docs/internals/review-by-leverage.md, level 1).
//
// The baseline is shared. A confirmation records the map (points, voices,
// target languages; no counts) on the project, and every reviewer compares
// against the last one, whoever made it. A project confirmed before the map
// was recorded has no baseline: it reads as confirmed with no changes, and
// the next confirmation records one.

// Kinds of change against the confirmed map.
const (
	changePointAdded      = "point_added"
	changePointRemoved    = "point_removed"
	changePointMoved      = "point_moved"
	changeVoiceChanged    = "voice_changed"
	changeSourceOnly      = "source_only_changed"
	changeLanguageAdded   = "language_added"
	changeLanguageRemoved = "language_removed"
)

// coordinatesSnapshot is the map a confirmation records.
type coordinatesSnapshot struct {
	TargetLanguages []string        `json:"target_languages"`
	Points          []snapshotPoint `json:"points"`
}

type snapshotPoint struct {
	Collection  string            `json:"collection"`
	Coordinates map[string]string `json:"coordinates,omitempty"`
	VoiceID     string            `json:"voice_id,omitempty"`
	Voice       string            `json:"voice,omitempty"`
	SourceOnly  bool              `json:"source_only,omitempty"`
}

// planCoordinateChange is one difference between the confirmed map and the
// current one. A point is named by its collection; Coordinates and Voice are
// the current ones, or the last confirmed ones for a removed point.
type planCoordinateChange struct {
	Kind                string            `json:"kind"`
	Collection          string            `json:"collection,omitempty"`
	Coordinates         map[string]string `json:"coordinates,omitempty"`
	PreviousCoordinates map[string]string `json:"previous_coordinates,omitempty"`
	Voice               string            `json:"voice,omitempty"`
	PreviousVoice       string            `json:"previous_voice,omitempty"`
	SourceOnly          bool              `json:"source_only,omitempty"`
	Locale              string            `json:"locale,omitempty"`
}

func encodeCoordinatesSnapshot(c planCoordinatesDTO) (string, error) {
	snap := coordinatesSnapshot{TargetLanguages: c.TargetLanguages, Points: make([]snapshotPoint, 0, len(c.Points))}
	for _, p := range c.Points {
		snap.Points = append(snap.Points, snapshotPoint{
			Collection: p.Collection, Coordinates: p.Coordinates,
			VoiceID: p.VoiceID, Voice: p.Voice, SourceOnly: p.SourceOnly,
		})
	}
	b, err := json.Marshal(snap)
	return string(b), err
}

// coordinatesSnapshotOf reads the project's confirmed map; false when there
// is none or it does not parse.
func coordinatesSnapshotOf(p *platstore.Project) (coordinatesSnapshot, bool) {
	var snap coordinatesSnapshot
	raw := p.Properties[propCoordinatesSnapshot]
	if raw == "" || json.Unmarshal([]byte(raw), &snap) != nil {
		return snap, false
	}
	return snap, true
}

// coordinateChanges lists what differs between the confirmed map and the
// current coordinates: points by collection, then target languages.
func coordinateChanges(snap coordinatesSnapshot, cur planCoordinatesDTO) []planCoordinateChange {
	out := []planCoordinateChange{}
	before := make(map[string]snapshotPoint, len(snap.Points))
	for _, p := range snap.Points {
		before[p.Collection] = p
	}
	seen := map[string]bool{}
	for _, p := range cur.Points {
		seen[p.Collection] = true
		was, ok := before[p.Collection]
		if !ok {
			out = append(out, planCoordinateChange{
				Kind: changePointAdded, Collection: p.Collection,
				Coordinates: p.Coordinates, Voice: p.Voice, SourceOnly: p.SourceOnly,
			})
			continue
		}
		if !maps.Equal(nonEmpty(was.Coordinates), nonEmpty(p.Coordinates)) {
			out = append(out, planCoordinateChange{
				Kind: changePointMoved, Collection: p.Collection,
				Coordinates: p.Coordinates, PreviousCoordinates: was.Coordinates, Voice: p.Voice,
			})
		}
		if voiceDiffers(was, p) {
			out = append(out, planCoordinateChange{
				Kind: changeVoiceChanged, Collection: p.Collection,
				Coordinates: p.Coordinates, Voice: p.Voice, PreviousVoice: was.Voice,
			})
		}
		if was.SourceOnly != p.SourceOnly {
			out = append(out, planCoordinateChange{
				Kind: changeSourceOnly, Collection: p.Collection,
				Coordinates: p.Coordinates, SourceOnly: p.SourceOnly,
			})
		}
	}
	for _, p := range snap.Points {
		if !seen[p.Collection] {
			out = append(out, planCoordinateChange{
				Kind: changePointRemoved, Collection: p.Collection,
				Coordinates: p.Coordinates, Voice: p.Voice, SourceOnly: p.SourceOnly,
			})
		}
	}
	for _, l := range cur.TargetLanguages {
		if !slices.Contains(snap.TargetLanguages, l) {
			out = append(out, planCoordinateChange{Kind: changeLanguageAdded, Locale: l})
		}
	}
	for _, l := range snap.TargetLanguages {
		if !slices.Contains(cur.TargetLanguages, l) {
			out = append(out, planCoordinateChange{Kind: changeLanguageRemoved, Locale: l})
		}
	}
	return out
}

// voiceDiffers compares voices by profile id when both sides name one, and
// by name otherwise, so renaming a profile is not a change of voice.
func voiceDiffers(was snapshotPoint, p planPoint) bool {
	if was.VoiceID != "" && p.VoiceID != "" {
		return was.VoiceID != p.VoiceID
	}
	return was.VoiceID != p.VoiceID || was.Voice != p.Voice
}

func nonEmpty(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

// planCellDTO is one product and channel of the map. Most cells hold one
// collection; a cell holds several when collections share the point or
// differ only on another axis.
type planCellDTO struct {
	Product     string                  `json:"product"`
	Channel     string                  `json:"channel"`
	Collections []planCellCollectionDTO `json:"collections"`
	// Blocks counts the translatable blocks of the cell's collections.
	Blocks int `json:"blocks"`
	// Voices names the voices bound at the cell, each once.
	Voices []string `json:"voices"`
	// SourceOnly is set when every collection of the cell is source only.
	SourceOnly bool `json:"source_only,omitempty"`
	// Coverage is, per target language, the share of the cell's blocks
	// established in it, over the collections that are not source only.
	Coverage map[string]float64 `json:"coverage"`
	// Changes names the kinds of change touching the cell since the last
	// confirmation: point_added, point_moved, voice_changed,
	// source_only_changed.
	Changes []string `json:"changes,omitempty"`
}

// planCellCollectionDTO is one collection of a cell.
type planCellCollectionDTO struct {
	Name        string             `json:"name"`
	Coordinates map[string]string  `json:"coordinates"`
	Voice       string             `json:"voice,omitempty"`
	SourceOnly  bool               `json:"source_only,omitempty"`
	Blocks      int                `json:"blocks"`
	Coverage    map[string]float64 `json:"coverage"`
}

type reviewCoordinatesResponse struct {
	Stream      string             `json:"stream"`
	Coordinates planCoordinatesDTO `json:"coordinates"`
	Cells       []planCellDTO      `json:"cells"`
}

// HandleGetReviewCoordinates answers GET /:ws/:id/review/coordinates
// [?stream=] with the project's coordinates map: the plan's coordinates and
// their changes, and the cells with blocks, voices and coverage.
func (s *Server) HandleGetReviewCoordinates(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "store not configured")
	}
	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || proj == nil {
		return apiErr(c, http.StatusNotFound, errNoProject.Error())
	}
	stream := strings.TrimSpace(c.QueryParam("stream"))
	if stream == "" {
		stream = reviewInboxStream
	}
	out := reviewCoordinatesResponse{Stream: stream}
	if out.Coordinates, err = s.planCoordinates(ctx, proj, stream); err != nil {
		return serverErr(c, err)
	}
	if out.Cells, err = s.coordinateCells(ctx, proj.ID, stream, out.Coordinates); err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// coordinateCells groups the points by product and channel and counts each
// cell's blocks and coverage with the store's grouped counts.
func (s *Server) coordinateCells(ctx context.Context, projectID, stream string, coords planCoordinatesDTO) ([]planCellDTO, error) {
	counts := map[string]platstore.CollectionCoverage{}
	if cs, ok := platstore.CollectionCoverages(s.ContentStore); ok {
		rows, err := cs.CountCollectionCoverage(ctx, projectID, stream, coords.TargetLanguages)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			counts[r.CollectionID] = r
		}
	}
	changed := map[string][]string{}
	for _, ch := range coords.Changes {
		switch ch.Kind {
		case changePointAdded, changePointMoved, changeVoiceChanged, changeSourceOnly:
			changed[ch.Collection] = append(changed[ch.Collection], ch.Kind)
		}
	}

	type acc struct {
		cell        planCellDTO
		established map[string]int
		translated  int
	}
	var order []string
	cells := map[string]*acc{}
	for _, p := range coords.Points {
		product, channel := p.Coordinates[coreproject.ProductAxis], p.Coordinates[coreproject.ChannelAxis]
		key := product + "\x00" + channel
		a := cells[key]
		if a == nil {
			a = &acc{
				cell:        planCellDTO{Product: product, Channel: channel, Voices: []string{}, SourceOnly: true, Coverage: map[string]float64{}},
				established: map[string]int{},
			}
			cells[key] = a
			order = append(order, key)
		}
		n := counts[p.id]
		coll := planCellCollectionDTO{
			Name: p.Collection, Coordinates: p.Coordinates, Voice: p.Voice,
			SourceOnly: p.SourceOnly, Blocks: n.Blocks, Coverage: map[string]float64{},
		}
		a.cell.Blocks += n.Blocks
		if p.Voice != "" && !slices.Contains(a.cell.Voices, p.Voice) {
			a.cell.Voices = append(a.cell.Voices, p.Voice)
		}
		if !p.SourceOnly {
			a.cell.SourceOnly = false
			a.translated += n.Blocks
			for _, l := range coords.TargetLanguages {
				coll.Coverage[l] = share(n.Established[l], n.Blocks)
				a.established[l] += n.Established[l]
			}
		}
		for _, k := range changed[p.Collection] {
			if !slices.Contains(a.cell.Changes, k) {
				a.cell.Changes = append(a.cell.Changes, k)
			}
		}
		a.cell.Collections = append(a.cell.Collections, coll)
	}
	out := make([]planCellDTO, 0, len(order))
	for _, key := range order {
		a := cells[key]
		if !a.cell.SourceOnly {
			for _, l := range coords.TargetLanguages {
				a.cell.Coverage[l] = share(a.established[l], a.translated)
			}
		}
		out = append(out, a.cell)
	}
	slices.SortFunc(out, func(a, b planCellDTO) int {
		if c := strings.Compare(a.Product, b.Product); c != 0 {
			return c
		}
		return strings.Compare(a.Channel, b.Channel)
	})
	return out, nil
}

func share(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total)
}
