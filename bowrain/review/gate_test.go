package review_test

import (
	"context"
	"errors"
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/review"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gate is one answer asked from three places: the review endpoint, the bulk
// routes, and the sync worker. These are its rules on their own, without a
// store or a request in the way.

type policy struct {
	mode platauth.SoDMode
	err  error
}

func (p policy) GetSoDMode(context.Context, string) (platauth.SoDMode, error) {
	return p.mode, p.err
}

type authors struct {
	by  map[platstore.TargetRef]string
	err error
}

func (a authors) LastTargetAuthors(_ context.Context, _, _ string, _, _ []string) (map[platstore.TargetRef]string, error) {
	return a.by, a.err
}

func permits(allowed ...string) func(string) bool {
	set := map[string]bool{}
	for _, l := range allowed {
		set[l] = true
	}
	return func(locale string) bool { return set[locale] }
}

func TestGateAllow(t *testing.T) {
	const actor = "u-1"
	wrote := authors{by: map[platstore.TargetRef]string{{BlockID: "b1", Locale: "fr"}: actor}}

	tests := []struct {
		name    string
		cfg     review.Config
		block   string
		locale  string
		wantErr string
	}{
		{
			name:   "a reviewer of the language may decide",
			cfg:    review.Config{Actor: actor, Permits: permits("fr")},
			block:  "b1",
			locale: "fr",
		},
		{
			name:    "another language is another permission",
			cfg:     review.Config{Actor: actor, Permits: permits("fr")},
			block:   "b1",
			locale:  "de",
			wantErr: venue.RefusedNoReviewPermission,
		},
		{
			name:    "a gate with no way to ask refuses",
			cfg:     review.Config{Actor: actor},
			block:   "b1",
			locale:  "fr",
			wantErr: venue.RefusedNoReviewPermission,
		},
		{
			name: "the policy refuses the decider their own writing",
			cfg: review.Config{
				Actor: actor, Permits: permits("fr"),
				Policy: policy{mode: platauth.SoDBlock}, Authors: wrote,
				BlockIDs: []string{"b1"}, Locales: []string{"fr"},
			},
			block:   "b1",
			locale:  "fr",
			wantErr: venue.RefusedSeparationOfDuties,
		},
		{
			name: "somebody else's writing is not a conflict",
			cfg: review.Config{
				Actor: "u-2", Permits: permits("fr"),
				Policy: policy{mode: platauth.SoDBlock}, Authors: wrote,
				BlockIDs: []string{"b1"}, Locales: []string{"fr"},
			},
			block:  "b1",
			locale: "fr",
		},
		{
			name: "a target the store attributes to nobody stays approvable",
			cfg: review.Config{
				Actor: actor, Permits: permits("fr"),
				Policy: policy{mode: platauth.SoDBlock}, Authors: authors{},
				BlockIDs: []string{"b1"}, Locales: []string{"fr"},
			},
			block:  "b1",
			locale: "fr",
		},
		{
			name: "a warning policy records the conflict and allows it",
			cfg: review.Config{
				Actor: actor, Permits: permits("fr"),
				Policy: policy{mode: platauth.SoDWarn}, Authors: wrote,
				BlockIDs: []string{"b1"}, Locales: []string{"fr"},
			},
			block:  "b1",
			locale: "fr",
		},
		{
			name: "an unreadable policy is not a reason to refuse a review",
			cfg: review.Config{
				Actor: actor, Permits: permits("fr"),
				Policy: policy{err: errors.New("unreachable")}, Authors: wrote,
				BlockIDs: []string{"b1"}, Locales: []string{"fr"},
			},
			block:  "b1",
			locale: "fr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := review.Open(t.Context(), tt.cfg)
			require.NoError(t, err)
			err = g.Allow(tt.block, tt.locale)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			refusal, ok := errors.AsType[review.Refusal](err)
			require.True(t, ok, "a refusal is typed so a caller can tell it from a failure to ask: %v", err)
			assert.Equal(t, tt.wantErr, refusal.Reason)
			assert.Equal(t, tt.locale, refusal.Locale)
		})
	}
}

// An authorship read that failed refuses the whole pass. Discarding the error
// would disable the four-eyes check rather than tighten it.
func TestGateOpenRefusesAnUnreadableAuthorship(t *testing.T) {
	_, err := review.Open(t.Context(), review.Config{
		Actor: "u-1", Permits: permits("fr"),
		Policy:   policy{mode: platauth.SoDBlock},
		Authors:  authors{err: errors.New("table unreachable")},
		BlockIDs: []string{"b1"}, Locales: []string{"fr"},
	})
	require.Error(t, err)
}

// The violation count is what a bulk pass files one record with, rather than
// one per block on a bus that drops what it cannot keep up with.
func TestGateCountsViolationsQuietly(t *testing.T) {
	filed := 0
	g, err := review.Open(t.Context(), review.Config{
		Actor: "u-1", Permits: permits("fr"),
		Policy: policy{mode: platauth.SoDWarn},
		Authors: authors{by: map[platstore.TargetRef]string{
			{BlockID: "b1", Locale: "fr"}: "u-1",
			{BlockID: "b2", Locale: "fr"}: "u-1",
		}},
		BlockIDs: []string{"b1", "b2"}, Locales: []string{"fr"},
		Record: func(string, platauth.SoDMode, int) { filed++ },
	})
	require.NoError(t, err)
	g.Quiet()
	require.NoError(t, g.Allow("b1", "fr"))
	require.NoError(t, g.Allow("b2", "fr"))
	assert.Equal(t, 2, g.Violations())
	assert.Zero(t, filed, "a quiet gate counts what it holds back from the bus")
	assert.Equal(t, platauth.SoDWarn, g.Mode())
}

// A permission that could not be resolved is not a permission that was denied.
func TestLanguagePermitsKeepsTheFailure(t *testing.T) {
	p := review.NewLanguagePermits(t.Context(), failingAuthority{}, review.Query{UserID: "u-1"})
	assert.False(t, p.Allows("fr"))
	require.Error(t, p.Err())
}

type failingAuthority struct{}

func (failingAuthority) GetSoDMode(context.Context, string) (platauth.SoDMode, error) {
	return platauth.SoDOff, nil
}

func (failingAuthority) AllowsLanguage(context.Context, review.Query) (bool, error) {
	return false, errors.New("auth store unreachable")
}
