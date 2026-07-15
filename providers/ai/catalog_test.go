package aiprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The catalog is only a single source of truth if the things that used to be the
// source — the provider defaults and the limits table — actually agree with it.
// These tests are the fold's guardrail: change a default in a provider file without
// adding it to the catalog, and one of them fails rather than the two drifting
// apart in silence.

func TestEveryProviderDefaultIsCatalogued(t *testing.T) {
	t.Parallel()

	for _, info := range Providers() {
		if info.DefaultModel == "" || info.Local {
			continue // no default to check, or a local model with no pinned ceilings
		}
		if info.Name == ClaudeCode {
			// claude-code's default is a bare alias (sonnet) resolved against the
			// signed-in account; it goes through the alias path, not prefix match.
			_, ok := ModelByAlias(info.DefaultModel)
			require.Truef(t, ok, "claude-code default %q has no catalog entry", info.DefaultModel)
			continue
		}

		m, ok := ModelForID(info.DefaultModel)
		require.Truef(t, ok, "provider %s default %q has no catalog entry", info.Name, info.DefaultModel)
		assert.Containsf(t, m.DefaultFor, info.Name,
			"catalog entry %q is provider %s's default but does not list it in default_for", m.ID, info.Name)
	}
}

func TestDefaultForOnlyNamesRealProviders(t *testing.T) {
	t.Parallel()

	for _, m := range Models() {
		for _, p := range m.DefaultFor {
			_, ok := ProviderInfoFor(p)
			assert.Truef(t, ok, "catalog entry %q claims to be default_for unknown provider %q", m.ID, p)
		}
	}
}

// An active model a provider defaults to cannot be "not recommended" — demoting
// your own current default to the Advanced shelf is incoherent. (A superseded
// default, like Azure's gpt-4o, is a separate already-flagged smell and is exempt:
// it is legacy by status, not by this axis.)
func TestActiveDefaultsAreRecommended(t *testing.T) {
	t.Parallel()

	for _, m := range Models() {
		if len(m.DefaultFor) > 0 && m.Status == StatusActive {
			assert.Truef(t, m.IsRecommended(),
				"%q is an active default for %v but is marked not recommended", m.ID, m.DefaultFor)
		}
	}
}

func TestLimitsResolveThroughTheCatalog(t *testing.T) {
	t.Parallel()

	// The exact ceilings the limits table used to hold, now asserted against the
	// catalog so the fold is provably behaviour-preserving.
	cases := []struct {
		model      string
		maxOut     int
		contextWin int
	}{
		{"claude-opus-4-8", 128_000, 1_000_000},
		{"claude-sonnet-5-20260115", 128_000, 1_000_000}, // dated id → family
		{"claude-haiku-4-5", 64_000, 200_000},
		{"claude-sonnet-4", 64_000, 200_000},
		{"gpt-5.6-sol", 128_000, 1_050_000}, // the real default id, via prefix
		{"gpt-4o", 16_384, 128_000},
		{"gemini-3.5-flash", 65_536, 1_048_576},
		{"gemini-3.1-pro-preview", 64_000, 1_048_576}, // longest-prefix beats "gemini-3"
	}
	for _, c := range cases {
		lim, ok := LimitsForModel(c.model)
		require.Truef(t, ok, "no limits for %q", c.model)
		assert.Equalf(t, c.maxOut, lim.MaxOutputTokens, "max output for %q", c.model)
		assert.Equalf(t, c.contextWin, lim.ContextWindow, "context window for %q", c.model)
	}

	// A model the catalog has never heard of falls back to unknown, not a guess.
	_, ok := LimitsForModel("some-model-from-the-future")
	assert.False(t, ok)
}

func TestCatalogIsInternallyConsistent(t *testing.T) {
	t.Parallel()

	ids := map[string]bool{}
	for _, m := range Models() {
		assert.NotEmptyf(t, m.ID, "every model needs an id")
		assert.NotEmptyf(t, m.Label, "%q needs a label", m.ID)
		assert.Falsef(t, ids[m.ID], "duplicate catalog id %q", m.ID)
		ids[m.ID] = true

		_, ok := ProviderInfoFor(m.Provider)
		assert.Truef(t, ok, "%q names unknown provider %q", m.ID, m.Provider)

		switch m.Status {
		case StatusActive, StatusSuperseded:
		default:
			t.Errorf("%q has unknown status %q", m.ID, m.Status)
		}
	}

	// A superseded model must point at a successor, and that successor must exist —
	// a dangling superseded_by is a broken promise on the dashboard.
	for _, m := range Models() {
		if m.Status == StatusSuperseded {
			assert.NotEmptyf(t, m.SupersededBy, "%q is superseded but names no successor", m.ID)
			if m.SupersededBy != "" {
				assert.Truef(t, ids[m.SupersededBy], "%q is superseded_by %q, which is not in the catalog", m.ID, m.SupersededBy)
			}
		}
	}
}
