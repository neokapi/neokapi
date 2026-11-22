package store

import (
	"context"
	"encoding/json"
)

// FollowedLanguagesStore holds the target languages each person follows in a
// project: the ones their review inbox lists. The list only filters the
// project's shared queues. Everyone who follows a language shares its one
// queue, and nothing is copied per person.
type FollowedLanguagesStore interface {
	// FollowedLanguages returns the languages a person follows in a project.
	// saved is false when the person has saved no list, which means every
	// language they may review.
	FollowedLanguages(ctx context.Context, userID, projectID string) (locales []string, saved bool, err error)
	// PutFollowedLanguages saves a person's list for a project, replacing the
	// one they held. An empty list is a saved choice to follow none.
	PutFollowedLanguages(ctx context.Context, userID, projectID string, locales []string) error
	// ListFollowedLanguages returns every list a person has saved, by
	// project id.
	ListFollowedLanguages(ctx context.Context, userID string) (map[string][]string, error)
}

// FollowedLanguages finds the followed-languages store behind a content
// store, looking through any decorator that names what it wraps.
func FollowedLanguages(cs ContentStore) (FollowedLanguagesStore, bool) {
	for cs != nil {
		if s, ok := cs.(FollowedLanguagesStore); ok {
			return s, true
		}
		inner, ok := cs.(interface{ Unwrap() ContentStore })
		if !ok {
			return nil, false
		}
		cs = inner.Unwrap()
	}
	return nil, false
}

// EncodeLocales writes a locale list as a JSON array; nil is an empty one.
func EncodeLocales(locales []string) string {
	if locales == nil {
		locales = []string{}
	}
	raw, _ := json.Marshal(locales)
	return string(raw)
}

// DecodeLocales reads a JSON array of locales; anything else reads as none.
func DecodeLocales(raw []byte) []string {
	out := []string{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	if out == nil {
		out = []string{}
	}
	return out
}
