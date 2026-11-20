package server

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/host/reviewdeck"
)

// A deck summary is what the review plan and the review inbox read from a
// project's language decks: each language's counts by group, the rules
// behind its desk group, and its pages. Counting it reads every translation
// waiting for review, so the server keeps each summary while the project's
// deck revision (platstore.ReviewDeckStore.ReviewDeckRevision) stands.
//
// The revision lives in the database and moves when a transaction commits a
// change the deck is counted from, so every server instance reads the same
// one. A summary is never served past deckSummaryMaxAge, because facts also
// go stale with time.

// deckSummaryMaxAge bounds how long a summary is served while the revision
// stands.
const deckSummaryMaxAge = time.Minute

// deckSummaryCacheSize is the number of summaries past which storing one
// drops those past deckSummaryMaxAge.
const deckSummaryCacheSize = 256

// deckSummary is the language decks of one project, stream and set of
// locales, keyed by locale. Callers share it and only read it.
type deckSummary struct {
	counts map[string]platstore.ReviewDeckCounts
	rules  map[string][]platstore.RuleTally
	pages  map[string][]platstore.ReviewDeckPage
}

// deckSummaryCache keeps deck summaries by key, each with the revision it was
// counted at. The zero value is ready to use.
type deckSummaryCache struct {
	mu      sync.Mutex
	entries map[string]deckSummaryEntry
	flight  singleflight.Group
}

type deckSummaryEntry struct {
	revision int64
	at       time.Time
	summary  deckSummary
}

// get returns the summary stored under key at revision, if it is young
// enough.
func (c *deckSummaryCache) get(key string, revision int64, now time.Time) (deckSummary, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || e.revision != revision || now.Sub(e.at) >= deckSummaryMaxAge {
		return deckSummary{}, false
	}
	return e.summary, true
}

// put stores a summary counted at revision.
func (c *deckSummaryCache) put(key string, revision int64, at time.Time, s deckSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]deckSummaryEntry{}
	}
	if len(c.entries) >= deckSummaryCacheSize {
		for k, e := range c.entries {
			if at.Sub(e.at) >= deckSummaryMaxAge {
				delete(c.entries, k)
			}
		}
	}
	c.entries[key] = deckSummaryEntry{revision: revision, at: at, summary: s}
}

// deckSummaryOf returns the language decks of a project for locales, from
// the cache while the project's deck revision stands. q names the project,
// the stream, the policy and the collection scope; its Locale is unused.
//
// The revision is read before the decks are counted, so a change that
// commits while they are counted moves the revision past the one stored,
// and the next request counts again.
func (s *Server) deckSummaryOf(ctx context.Context, decks platstore.ReviewDeckStore, q platstore.ReviewDeckQuery, locales []string) (deckSummary, error) {
	revision, err := decks.ReviewDeckRevision(ctx, q.ProjectID)
	if err != nil {
		return deckSummary{}, err
	}
	key := deckSummaryKey(q, locales)
	if sum, ok := s.deckSummaries.get(key, revision, time.Now()); ok {
		return sum, nil
	}
	// Requests that arrive together for the same decks share one count.
	v, err, _ := s.deckSummaries.flight.Do(fmt.Sprintf("%s@%d", key, revision), func() (any, error) {
		at := time.Now()
		sum, err := countDeckSummary(context.WithoutCancel(ctx), decks, q, locales)
		if err != nil {
			return deckSummary{}, err
		}
		s.deckSummaries.put(key, revision, at, sum)
		return sum, nil
	})
	if err != nil {
		return deckSummary{}, err
	}
	sum, _ := v.(deckSummary)
	return sum, nil
}

// countDeckSummary counts the language decks: each read is one query over
// every language, and the three run at once.
func countDeckSummary(ctx context.Context, decks platstore.ReviewDeckStore, q platstore.ReviewDeckQuery, locales []string) (deckSummary, error) {
	var sum deckSummary
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		sum.counts, err = decks.CountReviewDeckByLocale(gctx, q, locales)
		return err
	})
	g.Go(func() (err error) {
		// A language with nothing in the desk group counts no rule.
		sum.rules, err = decks.CountReviewDeckRulesByLocale(gctx, q, locales, deckRank(reviewdeck.DeckDesk), planRuleLimit)
		return err
	})
	g.Go(func() (err error) {
		sum.pages, err = decks.ListReviewDeckPagesByLocale(gctx, q, locales, reviewPagesCountLimit+1)
		return err
	})
	return sum, g.Wait()
}

// deckSummaryKey names what a summary counts: the project, the stream, the
// policy, the collection scope and the locales.
func deckSummaryKey(q platstore.ReviewDeckQuery, locales []string) string {
	sorted := slices.Sorted(slices.Values(locales))
	return strings.Join([]string{
		q.ProjectID, q.Stream, q.Policy,
		strings.Join(q.ExcludeCollections, ","),
		strings.Join(q.HighCollections, ","),
		strings.Join(q.LowCollections, ","),
		strings.Join(sorted, ","),
	}, "|")
}
