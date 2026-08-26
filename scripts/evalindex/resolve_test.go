package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The two tests that give the registry teeth.
//
// A card is prose until something checks it. The first draft of evals.go named
// four commands that do not exist (`make parity`, `make format-maturity`,
// `make kbf-tests`, `make pseudobench`) and linked a page that was retired two
// releases ago, and every one of them read perfectly plausibly. A reader who
// pasted `make parity` would have learned that the evidence page does not know
// what it is describing, which is worse than publishing nothing.
//
// So Reproduce and Page are resolved against the repository, not trusted.

// Reproduce must take one of these forms, so that resolving it is mechanical.
// The constraint is on the author rather than the reader: a card is free to say
// anything, right up until it has to name a thing that exists.
var reproduceForms = []struct {
	name string
	re   *regexp.Regexp
	// resolve returns the repo-relative path that must exist, or "" when the
	// form verifies itself (a make target, checked against the Makefile).
	resolve func(m []string) string
}{
	{
		name:    "make target",
		re:      regexp.MustCompile(`^make ([a-z0-9][a-z0-9-]*)$`),
		resolve: func(m []string) string { return "" },
	},
	{
		name:    "go run",
		re:      regexp.MustCompile(`^go run (?:\$\(GOTAGS\) )?\./(\S+)`),
		resolve: func(m []string) string { return m[1] },
	},
	{
		name:    "python script",
		re:      regexp.MustCompile(`^python3 (\S+\.py)`),
		resolve: func(m []string) string { return m[1] },
	},
	{
		name:    "skill ritual",
		re:      regexp.MustCompile(`^/([a-z0-9-]+) [a-z0-9-]+$`),
		resolve: func(m []string) string { return filepath.Join(".skills", m[1], "SKILL.md") },
	},
	{
		name:    "open a page that runs it",
		re:      regexp.MustCompile(`^open /(\S+)$`),
		resolve: func(m []string) string { return filepath.Join("web", "src", "pages", m[1], "index.tsx") },
	},
}

// makeTargets reads the Makefile once and returns every target it declares.
func makeTargets(t *testing.T, root string) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, "Makefile"))
	require.NoError(t, err)

	// Target lines only: a name at column zero followed by a colon. Variable
	// assignments (`X := y`) and target-scoped variables (`bench-stress: X := y`)
	// are excluded by requiring the colon to be the first one on the line.
	decl := regexp.MustCompile(`(?m)^([a-zA-Z0-9][a-zA-Z0-9._-]*)\s*:(?:[^=]|$)`)
	out := map[string]bool{}
	for _, m := range decl.FindAllStringSubmatch(string(body), -1) {
		out[m[1]] = true
	}
	return out
}

// TestReproduceCommandExists.
//
// Every command a card offers must be runnable. This is the field the
// evaluation-card literature finds missing most often, and a wrong command is
// worse than an absent one: it looks like a promise until someone tries it.
func TestReproduceCommandExists(t *testing.T) {
	root := repoRoot(t)
	targets := makeTargets(t, root)

	for _, e := range evals {
		if e.Reproduce == "" {
			continue
		}
		t.Run(e.ID, func(t *testing.T) {
			for _, form := range reproduceForms {
				m := form.re.FindStringSubmatch(e.Reproduce)
				if m == nil {
					continue
				}
				if form.name == "make target" {
					assert.True(t, targets[m[1]],
						"card says %q, but the Makefile declares no %q target", e.Reproduce, m[1])
					return
				}
				rel := form.resolve(m)
				_, err := os.Stat(filepath.Join(root, rel))
				assert.NoError(t, err, "card says %q, and %s does not exist", e.Reproduce, rel)
				return
			}
			t.Errorf("reproduce %q matches no known form; add the form to reproduceForms "+
				"or the test cannot check that the command exists", e.Reproduce)
		})
	}
}

// TestPageExists: a card that links a dashboard must link one that is there.
// Docusaurus renders a broken internal link as a build failure in production
// and a soft 404 in development, so a stale link survives local work and breaks
// a deploy.
func TestPageExists(t *testing.T) {
	root := repoRoot(t)

	for _, e := range evals {
		if e.Page == "" {
			continue
		}
		t.Run(e.ID, func(t *testing.T) {
			slug := strings.TrimPrefix(e.Page, "/")
			dir := filepath.Join(root, "web", "src", "pages", slug)
			for _, name := range []string{"index.tsx", "index.md", "index.mdx"} {
				if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
					return
				}
			}
			if _, err := os.Stat(dir + ".tsx"); err == nil {
				return
			}
			t.Errorf("card links %s, and no page renders it", e.Page)
		})
	}
}

// TestAnUnpublishedEvalSaysSo: an eval that runs but shows a reader nothing is
// a real and easily hidden state, and parity is in it today. It is a gap in the
// evidence rather than in the measurement, so absent is the wrong status and the
// card is the only place that can admit it.
func TestAnUnpublishedEvalSaysSo(t *testing.T) {
	for _, e := range evals {
		if e.Status == StatusAbsent || e.Page != "" || e.Data != "" {
			continue
		}
		t.Run(e.ID, func(t *testing.T) {
			assert.NotEqual(t, StatusMeasured, e.Status,
				"nothing is published, so a reader cannot read the numbers as they stand")
			assert.Contains(t, strings.ToLower(e.Misses), "publish",
				"the card shows no data and links no page, so it owes the reader that fact plainly")
		})
	}
}
