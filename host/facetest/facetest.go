// Package facetest carries the conformance fixture and the answer shapes the
// kapi faces are held to, bound to the host layer's answer types.
//
// kapi answers the same questions at three faces: a CLI verb, an MCP tool or
// resource, and the desktop's backend method. They read one graph through one
// host layer, and the promise is that a person at a terminal, an agent over
// MCP, and someone in the app cannot learn three different models of a project.
// Drift between the faces was the root of four of the five known loop defects,
// so parity is a test rather than a convention.
//
// The faces live in three Go modules, and the desktop's module pulls in Wails,
// so no single test binary can call all three. What travels between them
// instead is the fixture and the record in the host-free package fixture: one
// fixture written from one description, and one set of answers embedded as
// JSON. Each face's own suite builds the fixture, asks its own entry point,
// projects the reply into the shapes below, and compares against the same
// embedded answers. Agreeing with the record is transitively agreeing with each
// other. The platform's suite reads the same fixture and record through that
// package, so the record holds the server's graph to the local faces' numbers
// too.
package facetest

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/facetest/fixture"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/require"
)

// The fixture and the record, as the host-free package defines them.
type (
	Project      = fixture.Project
	Answers      = fixture.Answers
	PointFacts   = fixture.PointFacts
	VoiceFacts   = fixture.VoiceFacts
	TermFacts    = fixture.TermFacts
	ContextFacts = fixture.ContextFacts
	SearchFacts  = fixture.SearchFacts
	LocaleFacts  = fixture.LocaleFacts
	StatusFacts  = fixture.StatusFacts
	FindingFacts = fixture.FindingFacts
	CheckFacts   = fixture.CheckFacts
)

// PosixSourceLocale and PosixTargetLocale are what every face must answer about
// the WritePosix project, whatever it was asked.
const (
	PosixSourceLocale = fixture.PosixSourceLocale
	PosixTargetLocale = fixture.PosixTargetLocale
)

// Write builds the fixture, extracts its content into the project store and
// materializes the context graph over it, the way one `kapi up` leaves a
// project, so the usage count every face reports is a number the record can
// hold them to rather than a zero they all agree on.
//
// The extraction runs through the one path `kapi up` and the desktop's
// Re-extract share (host.App.ExtractToProjectStore), so the graph the faces
// count uses from is the graph a run would have written.
func Write(t *testing.T) Project {
	t.Helper()
	p := fixture.Write(t)
	extract(t, p)
	return p
}

// WritePosix builds the POSIX-locale project; see fixture.WritePosix.
func WritePosix(t *testing.T) Project { return fixture.WritePosix(t) }

// Concepts is the fixture's vocabulary; see fixture.Concepts.
func Concepts() []terms.Concept { return fixture.Concepts() }

func extract(t *testing.T, p Project) {
	t.Helper()
	a := &host.App{}
	a.InitRegistries()
	defer a.Shutdown()

	proj, err := project.Load(p.Recipe)
	require.NoError(t, err)
	pctx := project.NewProjectContext(proj, p.Recipe)
	resolved, err := pctx.ResolveContent(a.FormatReg)
	require.NoError(t, err)
	stats, err := a.ExtractToProjectStore(t.Context(), a.FormatReg, p.Root, pctx, resolved)
	require.NoError(t, err)
	require.Empty(t, stats.Warnings, "the graph is rebuilt with the blocks")
	require.Positive(t, stats.Blocks, "the fixture has content to count uses in")
}

// ContextFactsFrom projects the host's by-location answer.
//
// The CLI verb, the `context://` resource and the desktop's ContextAt all
// return this type unchanged, so for this question the projection is the whole
// comparison and a face that reshaped the answer would fail here.
func ContextFactsFrom(a *host.ContextAnswer) ContextFacts {
	f := ContextFacts{
		Point: PointFacts{
			Path:       filepath.ToSlash(a.Point.Path),
			Profile:    a.Point.Profile,
			Channel:    a.Point.Channel,
			Collection: a.Point.Collection,
			Default:    a.Point.Default,
		},
		TermsTotal: a.TermsTotal,
		Terms:      termFacts(a.Terms),
	}
	if a.Voice != nil {
		f.Voice = VoiceFacts{Name: a.Voice.Name, Field: a.Voice.Field}
	}
	return f
}

// SearchFactsFrom projects the host's by-content answer.
func SearchFactsFrom(r *host.ContextSearchResult) SearchFacts {
	f := SearchFacts{
		Query: r.Query,
		Scope: string(r.Scope),
		Terms: termFacts(r.Terms),
	}
	for _, p := range r.Precedent {
		f.Precedent = append(f.Precedent, p.Text)
	}
	sort.Strings(f.Precedent)
	return f
}

// CheckFactsFrom projects a check report; see fixture.CheckFactsFrom.
func CheckFactsFrom(r check.Report) CheckFacts { return fixture.CheckFactsFrom(r) }

// SortFindings orders findings; see fixture.SortFindings.
func SortFindings(f []FindingFacts) { fixture.SortFindings(f) }

// SortLocales orders locales by name; see fixture.SortLocales.
func SortLocales(l []LocaleFacts) { fixture.SortLocales(l) }

func termFacts(in []host.ContextTermHit) []TermFacts {
	var out []TermFacts
	for _, h := range in {
		out = append(out, TermFacts{
			ConceptID:   h.ConceptID,
			Term:        h.Term,
			Status:      h.Status,
			Discouraged: h.Discouraged,
			Replacement: h.Replacement,
			Uses:        h.Uses,
			UseBlocks:   h.UseBlocks,
		})
	}
	fixture.SortTerms(out)
	return out
}

// Golden is the recorded answer set; see fixture.Golden.
func Golden(t *testing.T) Answers { return fixture.Golden(t) }

// GoldenPath is where the recorded answers live, relative to this package's
// directory, for the suite that rewrites them under -update.
func GoldenPath() string { return filepath.Join("fixture", fixture.GoldenPath()) }

// Marshal renders an answer set the way the record stores it.
func Marshal(t *testing.T, a Answers) []byte { return fixture.Marshal(t, a) }
