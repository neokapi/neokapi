package sievepen_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// TestSQLiteTM_ConcurrentLookup exercises concurrent Lookup, LookupText, and
// Count calls from 16 goroutines to verify the SQLite TM is safe under
// parallel read access. Run with -race to catch data races.
func TestSQLiteTM_ConcurrentLookup(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { tm.Close() })

	// Seed with a variety of entries so lookups can find real matches.
	entries := []sievepen.TMEntry{
		{
			ID: "e1",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: "Hello world"}}},
				"fr": {{Text: &model.TextRun{Text: "Bonjour le monde"}}},
				"de": {{Text: &model.TextRun{Text: "Hallo Welt"}}},
			},
			HintSrcLang: "en",
		},
		{
			ID: "e2",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: "Save changes"}}},
				"fr": {{Text: &model.TextRun{Text: "Enregistrer les modifications"}}},
				"de": {{Text: &model.TextRun{Text: "Änderungen speichern"}}},
			},
			HintSrcLang: "en",
		},
		{
			ID: "e3",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: "Cancel"}}},
				"fr": {{Text: &model.TextRun{Text: "Annuler"}}},
				"de": {{Text: &model.TextRun{Text: "Abbrechen"}}},
			},
			HintSrcLang: "en",
		},
		{
			ID: "e4",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: "Delete item"}}},
				"fr": {{Text: &model.TextRun{Text: "Supprimer l'élément"}}},
			},
			HintSrcLang: "en",
		},
		{
			ID: "e5",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: "Open file"}}},
				"fr": {{Text: &model.TextRun{Text: "Ouvrir le fichier"}}},
			},
			HintSrcLang: "en",
		},
	}

	for _, e := range entries {
		require.NoError(t, tm.Add(e))
	}
	require.Equal(t, len(entries), tm.Count(), "seed count")

	const goroutines = 16
	opts := sievepen.DefaultLookupOptions()
	opts.MinScore = 0.5

	// Probe blocks used for Lookup.
	probeBlocks := []*model.Block{
		model.NewBlock("p1", "Hello world"),
		model.NewBlock("p2", "Save changes"),
		model.NewBlock("p3", "Cancel"),
	}
	probeTexts := []string{"Hello", "Save", "Cancel", "Delete", "Open"}

	var totalLookups, totalTextLookups, totalCounts atomic.Int64

	g := new(errgroup.Group)
	for i := range goroutines {
		workerID := i
		g.Go(func() error {
			for round := range 5 {
				// Rotate probes round-robin across workers.
				probe := probeBlocks[(workerID+round)%len(probeBlocks)]
				matches, err := tm.Lookup(probe, "en", "fr", opts)
				if err != nil {
					return fmt.Errorf("worker %d Lookup: %w", workerID, err)
				}
				// For an exact-match probe the result must be non-empty.
				if probe.SourceText() == "Hello world" && len(matches) == 0 {
					return fmt.Errorf("worker %d: Lookup('Hello world') returned no matches", workerID)
				}
				totalLookups.Add(int64(len(matches)))

				// LookupText on a rotating probe.
				text := probeTexts[(workerID+round)%len(probeTexts)]
				textMatches, err := tm.LookupText(text, "en", "fr", opts)
				if err != nil {
					return fmt.Errorf("worker %d LookupText(%q): %w", workerID, text, err)
				}
				totalTextLookups.Add(int64(len(textMatches)))

				// Count must always return the seeded value.
				count := tm.Count()
				if count != len(entries) {
					return fmt.Errorf("worker %d Count = %d, want %d", workerID, count, len(entries))
				}
				totalCounts.Add(int64(count))
			}
			return nil
		})
	}

	require.NoError(t, g.Wait(), "concurrent TM operations must not return errors")

	// Sanity: we ran many lookups total.
	assert.Greater(t, totalCounts.Load(), int64(0), "Count was never called")
	// At least the Lookup probes for exact "Hello world" must have found matches.
	assert.Greater(t, totalLookups.Load(), int64(0), "Lookup returned no results across all goroutines")
}

// TestSQLiteTM_ConcurrentFacetStats exercises concurrent FacetStatsFiltered
// calls alongside reads.
func TestSQLiteTM_ConcurrentFacetStats(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { tm.Close() })

	for i := range 10 {
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: fmt.Sprintf("entry-%d", i),
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: fmt.Sprintf("Source text %d", i)}}},
				"fr": {{Text: &model.TextRun{Text: fmt.Sprintf("Texte source %d", i)}}},
			},
			HintSrcLang: "en",
		}))
	}

	const goroutines = 16
	g := new(errgroup.Group)
	for i := range goroutines {
		workerID := i
		g.Go(func() error {
			for range 3 {
				stats := tm.FacetStatsFiltered("", "en", "fr", sievepen.SearchFilter{})
				if len(stats.Locales) == 0 {
					return fmt.Errorf("worker %d: FacetStatsFiltered returned no locale facets", workerID)
				}
			}
			return nil
		})
	}

	require.NoError(t, g.Wait(), "concurrent FacetStats must not error")
}
