package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ScopeAction maps to a set of permissions.
type ScopeAction string

const (
	ScopeAll       ScopeAction = "*"
	ScopeRead      ScopeAction = "read"
	ScopeTranslate ScopeAction = "translate"
	// ScopeContribute is the machine's share of the convergence loop: push and
	// pull content, seed memory, run flows, and PROPOSE governed terminology —
	// without the permissions that decide a review (review, manage_voice). A
	// CI token on this scope structurally cannot approve what it proposed.
	ScopeContribute ScopeAction = "contribute"
	ScopeReview     ScopeAction = "review"
	ScopeManage     ScopeAction = "manage"
	ScopeAdmin      ScopeAction = "admin"
)

// ResolvedScope is the result of parsing a single scope string.
type ResolvedScope struct {
	Action      ScopeAction
	Permissions Permission       // bitmask
	Languages   []string         // empty = all
	ProjectID   string           // empty = all projects
	Coordinates CoordinateFilter // empty = the whole context space
}

// ResolvedScopes is the combined result of parsing all scopes on a token.
type ResolvedScopes struct {
	Permissions Permission // union of all scope permissions
	Languages   []string   // intersection of language constraints (empty = all)
	ProjectIDs  []string   // allowed projects (empty = all)
	// Coordinates is the union of the regions the token's scopes name — a token
	// holding two regions holds both. This deliberately differs from Languages,
	// which intersects: a reach is a set of places the holder may act, so
	// intersecting two disjoint regions would leave a token that can act nowhere
	// while plainly having been granted two places.
	Coordinates  CoordinateReach
	IsFullAccess bool // true if "*" scope present
}

// actionPermissions maps a ScopeAction to its Permission bitmask.
func actionPermissions(action ScopeAction) Permission {
	switch action {
	case ScopeRead:
		return PermViewContent
	case ScopeTranslate:
		return PermViewContent | PermTranslate
	case ScopeContribute:
		return PermViewContent | PermTranslate | PermManageFiles |
			PermManageMemory | PermManageTerms | PermRunFlows
	case ScopeReview:
		return PermViewContent | PermTranslate | PermReview
	case ScopeManage:
		return PermAll &^ (PermManageProject | PermManageMembers)
	case ScopeAdmin:
		return PermAll
	case ScopeAll:
		return PermAll
	default:
		return 0
	}
}

// ParseScope parses a single scope string into a ResolvedScope.
//
// Supported formats:
//
//	"*"                                → all permissions
//	"read"                             → PermViewContent
//	"translate"                        → PermViewContent | PermTranslate
//	"translate:fr,de"                  → + language constraint
//	"contribute"                       → + files, memory, terms, flows (no review powers)
//	"review"                           → PermViewContent | PermTranslate | PermReview
//	"manage"                           → all except PermManageProject and PermManageMembers
//	"admin"                            → all permissions
//	"project:proj-123:translate"       → project-scoped
//	"project:proj-123:translate:fr,de" → project-scoped + language constraint
//
// Any of these may carry a region of the context space after an "@":
//
//	"review:de@brand=acme"                          German, acme content only
//	"project:p-7:manage@brand=acme,channel=support" one region, full powers in it
//	"contribute@brand=acme"                         a machine that may only propose into acme
//
// The region goes behind an "@" rather than in a fifth colon-separated segment
// because the grammar is positional: a fifth field would be unreadable at a
// glance and ambiguous against the language list it follows.
//
// The last example is the point of carrying regions on tokens at all. A pipeline
// that pushes one brand's content structurally cannot propose into another —
// the same separation-of-duties argument ScopeContribute already makes about
// approval, extended one axis.
func ParseScope(s string) (ResolvedScope, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return ResolvedScope{}, errors.New("empty scope string")
	}

	// Split the region off before anything positional is parsed, so an "@" can
	// never be mistaken for part of a project ID or a language tag.
	var coords CoordinateFilter
	if base, region, found := strings.Cut(s, "@"); found {
		parsed, err := ParseCoordinateFilter(region)
		if err != nil {
			return ResolvedScope{}, fmt.Errorf("invalid scope %q: %w", s, err)
		}
		if parsed.Unconstrained() {
			return ResolvedScope{}, fmt.Errorf("invalid scope %q: empty coordinate filter after '@'", s)
		}
		coords = parsed
		s = strings.TrimSpace(base)
		if s == "" {
			return ResolvedScope{}, fmt.Errorf("invalid scope %q: coordinate filter without an action", s)
		}
	}

	// Handle wildcard.
	if s == "*" {
		return ResolvedScope{
			Action:      ScopeAll,
			Permissions: actionPermissions(ScopeAll),
			Coordinates: coords,
		}, nil
	}

	var (
		projectID string
		action    ScopeAction
		languages []string
	)

	parts := strings.Split(s, ":")

	if parts[0] == "project" {
		// project:<id>:<action>[:<languages>]
		if len(parts) < 3 || len(parts) > 4 {
			return ResolvedScope{}, fmt.Errorf("invalid project scope %q: expected project:<id>:<action>[:<languages>]", s)
		}
		projectID = parts[1]
		if projectID == "" {
			return ResolvedScope{}, fmt.Errorf("invalid project scope %q: empty project ID", s)
		}
		action = ScopeAction(parts[2])
		if len(parts) == 4 {
			languages = parseLanguages(parts[3])
		}
	} else {
		// <action>[:<languages>]
		if len(parts) > 2 {
			return ResolvedScope{}, fmt.Errorf("invalid scope %q: too many segments", s)
		}
		action = ScopeAction(parts[0])
		if len(parts) == 2 {
			languages = parseLanguages(parts[1])
		}
	}

	perms := actionPermissions(action)
	if perms == 0 {
		return ResolvedScope{}, fmt.Errorf("unknown scope action %q", action)
	}

	return ResolvedScope{
		Action:      action,
		Permissions: perms,
		Languages:   languages,
		ProjectID:   projectID,
		Coordinates: coords,
	}, nil
}

// parseLanguages splits a comma-separated language list and trims whitespace.
func parseLanguages(s string) []string {
	if s == "" {
		return nil
	}
	raw := strings.Split(s, ",")
	langs := make([]string, 0, len(raw))
	for _, l := range raw {
		l = strings.TrimSpace(l)
		if l != "" {
			langs = append(langs, l)
		}
	}
	return langs
}

// ValidateScopes validates a JSON array of scope strings. Returns an error if
// the JSON is malformed or any individual scope string is invalid.
func ValidateScopes(scopesJSON string) error {
	var scopes []string
	if err := json.Unmarshal([]byte(scopesJSON), &scopes); err != nil {
		return fmt.Errorf("invalid scopes JSON: %w", err)
	}
	if len(scopes) == 0 {
		return errors.New("empty scopes array")
	}
	for _, s := range scopes {
		if _, err := ParseScope(s); err != nil {
			return err
		}
	}
	return nil
}

// ParseScopes parses a JSON array of scope strings (from api_tokens.scopes column)
// and returns the combined resolved scopes.
func ParseScopes(scopesJSON string) (*ResolvedScopes, error) {
	var scopes []string
	if err := json.Unmarshal([]byte(scopesJSON), &scopes); err != nil {
		return nil, fmt.Errorf("invalid scopes JSON: %w", err)
	}

	if len(scopes) == 0 {
		return nil, errors.New("empty scopes array")
	}

	result := &ResolvedScopes{}
	projectSet := map[string]struct{}{}
	allLanguages := true // track whether all scopes are unconstrained

	// For language intersection: collect per-scope language sets.
	var langSets []map[string]struct{}

	for _, s := range scopes {
		resolved, err := ParseScope(s)
		if err != nil {
			return nil, fmt.Errorf("parsing scope %q: %w", s, err)
		}

		// Union permissions.
		result.Permissions |= resolved.Permissions

		// "*" is full access only when it is also unbounded in space. "*@brand=acme"
		// grants every permission inside one region, and marking that full access
		// would hand it the whole space: IsFullAccess makes ScopeRestrictionMiddleware
		// skip narrowing altogether, so the region would be dropped rather than applied.
		if resolved.Action == ScopeAll && resolved.Coordinates.Unconstrained() {
			result.IsFullAccess = true
		}

		// Collect project IDs.
		if resolved.ProjectID != "" {
			projectSet[resolved.ProjectID] = struct{}{}
		}

		// Track languages.
		if len(resolved.Languages) > 0 {
			allLanguages = false
			set := make(map[string]struct{}, len(resolved.Languages))
			for _, l := range resolved.Languages {
				set[l] = struct{}{}
			}
			langSets = append(langSets, set)
		}

		// Union regions. Add collapses the reach to unconstrained as soon as one
		// scope names no region, which is the correct reading: a token that may
		// act anywhere is not narrowed by also being told it may act in acme.
		result.Coordinates = result.Coordinates.Add(resolved.Coordinates)
	}

	// Compute language intersection. If any scope has no language constraint,
	// then languages are unrestricted (empty = all).
	if !allLanguages && len(langSets) > 0 {
		intersection := langSets[0]
		for _, set := range langSets[1:] {
			for lang := range intersection {
				if _, ok := set[lang]; !ok {
					delete(intersection, lang)
				}
			}
		}
		for lang := range intersection {
			result.Languages = append(result.Languages, lang)
		}
	}

	// Collect project IDs.
	for id := range projectSet {
		result.ProjectIDs = append(result.ProjectIDs, id)
	}

	// Full access overrides constraints.
	if result.IsFullAccess {
		result.Permissions = PermAll
		result.Languages = nil
		result.ProjectIDs = nil
		result.Coordinates = nil
	}

	return result, nil
}
