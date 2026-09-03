package backend

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/project"
)

// ChannelMapRow is one channel in the map: where content sits, what governs
// there, the collections at it, and how many items sit there.
type ChannelMapRow struct {
	// Ref addresses the channel the way a collection names it: profile/channel.
	Ref         string            `json:"ref"`
	Profile     string            `json:"profile"`
	Channel     string            `json:"channel"`
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// Declared is true when a profile declares the channel, so it can be
	// renamed. A channel that exists only because a collection names it is
	// derived, and read-only.
	Declared    bool     `json:"declared"`
	Voice       string   `json:"voice,omitempty"`
	Collections []string `json:"collections"`
	ItemCount   int      `json:"item_count"`
}

// ChannelMapResult is the channel map for a project.
type ChannelMapResult struct {
	Channels []ChannelMapRow `json:"channels"`
	Notes    []string        `json:"notes,omitempty"`
}

// ChannelMap lists every channel content sits at: the profile-declared channels
// first, then any a collection names without a declaration (derived), each with
// the voice governing there, the collections at it, and how many items sit
// there. Per-channel review coverage is a follow-up.
func (a *App) ChannelMap(tabID string) (*ChannelMapResult, error) {
	points, err := a.ProjectPoints(tabID)
	if err != nil {
		return nil, err
	}
	out := &ChannelMapResult{Channels: []ChannelMapRow{}, Notes: points.Notes}

	// Item counts per collection, from the block store. Absent before the first
	// extraction, in which case every count is zero.
	counts := map[string]int{}
	if status, serr := a.GetProjectStatus(tabID); serr == nil && status != nil {
		for _, c := range status.Collections {
			counts[c.Name] = c.BlockCount
		}
	}

	declared := map[string]bool{}
	for _, p := range points.Points {
		if p.Channel == "" {
			continue // the project's own point is not a channel
		}
		row := ChannelMapRow{
			Ref:         p.Ref,
			Profile:     p.Profile,
			Channel:     p.Channel,
			Coordinates: p.Coordinates,
			Declared:    true,
			Voice:       p.Voice,
			Collections: p.Collections,
		}
		for _, c := range p.Collections {
			row.ItemCount += counts[c]
		}
		out.Channels = append(out.Channels, row)
		declared[p.Ref] = true
	}

	// Derived channels: a collection names a channel no profile declares. A
	// loaded recipe usually has none, since an undeclared channel does not load,
	// so this guards a hand-edited or in-progress recipe.
	op := a.getOpenProject(tabID)
	if op != nil && op.Project != nil {
		byRef := map[string][]string{}
		for _, coll := range op.Project.Collections {
			ref := coll.Channel
			if ref == "" || declared[ref] || !strings.Contains(ref, "/") {
				continue
			}
			byRef[ref] = append(byRef[ref], coll.Name)
		}
		refs := make([]string, 0, len(byRef))
		for ref := range byRef {
			refs = append(refs, ref)
		}
		sort.Strings(refs)
		for _, ref := range refs {
			profile, channel, _ := strings.Cut(ref, "/")
			row := ChannelMapRow{
				Ref:         ref,
				Profile:     profile,
				Channel:     channel,
				Declared:    false,
				Collections: byRef[ref],
			}
			for _, c := range byRef[ref] {
				row.ItemCount += counts[c]
			}
			out.Channels = append(out.Channels, row)
		}
	}
	return out, nil
}

// DeclareChannel adds a channel to a profile and writes the recipe.
func (a *App) DeclareChannel(tabID, profile, channel string) (*project.KapiProject, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	op := a.projects[tabID]
	if op == nil {
		return nil, fmt.Errorf("tab %q not found", tabID)
	}
	if op.Project == nil {
		return nil, errors.New("the tab has no project recipe")
	}
	if err := op.Project.DeclareChannel(profile, channel); err != nil {
		return nil, err
	}
	if op.Path != "" {
		if err := project.Save(op.Path, op.Project); err != nil {
			return nil, err
		}
	}
	return op.Project, nil
}

// RenameChannel renames a profile-declared channel, moves the collections that
// named it, and writes the recipe.
func (a *App) RenameChannel(tabID, profile, oldChannel, newChannel string) (*project.KapiProject, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	op := a.projects[tabID]
	if op == nil {
		return nil, fmt.Errorf("tab %q not found", tabID)
	}
	if op.Project == nil {
		return nil, errors.New("the tab has no project recipe")
	}
	if err := op.Project.RenameChannel(profile, oldChannel, newChannel); err != nil {
		return nil, err
	}
	if op.Path != "" {
		if err := project.Save(op.Path, op.Project); err != nil {
			return nil, err
		}
	}
	return op.Project, nil
}
