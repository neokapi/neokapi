package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The corpus is the experiment. These tests pin the properties that make a
// fixture measure what it claims to measure — a mandate the steered pass never
// receives, or a regex that does not compile, would score models against a
// context they were never shown.

func TestEveryTargetHasACorpus(t *testing.T) {
	for _, target := range Targets() {
		c := CorpusFor(target)
		require.NotEmpty(t, c.Fixtures, "target %s has no fixtures", target)
		require.Positive(t, c.Checks(), "target %s has no checks", target)
		// Every dimension must be represented, or the per-dimension report has
		// holes that read as measurements.
		byDim := map[string]bool{}
		for _, f := range c.Fixtures {
			for _, chk := range f.Checks[target] {
				byDim[chk.Dimension] = true
			}
		}
		for _, d := range []string{DimTerminology, DimVoice, DimInstruction} {
			assert.True(t, byDim[d], "target %s has no %s checks", target, d)
		}
	}
}

func TestDigestIsStableAndPerTarget(t *testing.T) {
	seen := map[string]string{}
	for _, target := range Targets() {
		c := CorpusFor(target)
		assert.Equal(t, c.Digest(), CorpusFor(target).Digest(), "digest must be deterministic")
		for prev, d := range seen {
			assert.NotEqual(t, d, c.Digest(), "targets %s and %s share a digest — they are different experiments", prev, target)
		}
		seen[target] = c.Digest()
	}
}

// TestMandatesReachTheSteeredPass pins the injection contract: every term the
// scorer demands must be in the glossary the steered pass injects (verbatim),
// and every DNT term must ride the glossary as an identity pin — that is how a
// do-not-translate name reaches generation in production.
func TestMandatesReachTheSteeredPass(t *testing.T) {
	for _, target := range Targets() {
		c := CorpusFor(target)
		for _, f := range c.Fixtures {
			for _, chk := range f.Checks[target] {
				if chk.Term != nil {
					got, ok := c.Ctx.Glossary[chk.Term.Source]
					require.True(t, ok, "%s/%s: term %q is scored but not injected", target, f.Key, chk.Term.Source)
					assert.Equal(t, chk.Term.Target, got,
						"%s/%s: the scorer demands %q but the glossary injects %q — the eval would punish obedience",
						target, f.Key, chk.Term.Target, got)
				}
				if chk.DNT != "" {
					assert.Contains(t, c.Ctx.DNT, chk.DNT, "%s/%s: DNT term not in the check list", target, f.Key)
					assert.Equal(t, chk.DNT, c.Ctx.Glossary[chk.DNT],
						"%s/%s: DNT term %q needs an identity glossary pin to reach generation", target, f.Key, chk.DNT)
				}
			}
		}
	}
}

// TestForbiddenTermsCarryReplacements pins the eval's scoring contract: the
// voice fixtures score swap adherence, which needs a declared replacement to
// check against. (The compact guide renders bare bans too, but a ban with no
// replacement has no single correct rendering to assert.)
func TestForbiddenTermsCarryReplacements(t *testing.T) {
	for _, target := range Targets() {
		p := contextFor(target).Profile
		for _, rule := range p.Vocabulary.ForbiddenTerms {
			assert.NotEmpty(t, rule.Replacement, "%s: forbidden term %q has no replacement and is invisible to the model", target, rule.Term)
		}
	}
}

func TestEveryRegexCompiles(t *testing.T) {
	for _, f := range allFixtures() {
		for target, checks := range f.Checks {
			for _, chk := range checks {
				for _, pat := range []string{chk.MustMatch, chk.MustNotMatch} {
					if pat == "" {
						continue
					}
					_, err := regexp.Compile(pat)
					assert.NoError(t, err, "%s/%s: bad pattern %q", target, f.Key, pat)
				}
			}
		}
	}
}

func TestEveryCheckHasExactlyOneBackend(t *testing.T) {
	for _, f := range allFixtures() {
		for target, checks := range f.Checks {
			for i, chk := range checks {
				n := 0
				if chk.Term != nil {
					n++
				}
				if chk.DNT != "" {
					n++
				}
				if chk.VocabClean {
					n++
				}
				if chk.MustMatch != "" || chk.MustNotMatch != "" {
					n++
				}
				assert.Equal(t, 1, n, "%s/%s check %d must declare exactly one scoring backend", target, f.Key, i)
				assert.NotEmpty(t, chk.Dimension, "%s/%s check %d has no dimension", target, f.Key, i)
				assert.NotEmpty(t, chk.Kind, "%s/%s check %d has no kind", target, f.Key, i)
			}
		}
	}
}

// TestConflictsDeclareTheirWinner: an AllowVocab entry is a declared conflict
// winner. It must excuse a term the profile actually forbids (else it excuses
// nothing) and the fixture must carry the conflict checks it adjudicates.
func TestConflictsDeclareTheirWinner(t *testing.T) {
	for _, f := range allFixtures() {
		if len(f.AllowVocab) == 0 {
			continue
		}
		for target, checks := range f.Checks {
			profile := contextFor(target).Profile
			hasVocabCheck := false
			for _, chk := range checks {
				if chk.VocabClean {
					hasVocabCheck = true
				}
			}
			require.True(t, hasVocabCheck, "%s/%s excuses vocabulary but scores none", target, f.Key)
			for _, allow := range f.AllowVocab {
				found := false
				for _, rule := range profile.Vocabulary.ForbiddenTerms {
					if strings.EqualFold(rule.Term, allow) {
						found = true
					}
				}
				assert.True(t, found, "%s/%s: AllowVocab %q is not a forbidden term — it excuses nothing", target, f.Key, allow)
			}
		}
	}
}

// TestHouseStyleNamesEveryMandate: what the judge and the human labeler are
// told must cover every non-identity mandate and every forbidden swap — a
// mandate they don't know about gets judged as unnaturalness, punishing
// obedience. Identity pins ride the product-names line instead.
func TestHouseStyleNamesEveryMandate(t *testing.T) {
	for _, target := range Targets() {
		c := contextFor(target)
		joined := strings.Join(c.HouseStyle(), "\n")
		for k, v := range c.Glossary {
			if k == v {
				assert.NotContains(t, joined, k+" → ", "%s: identity pin %q should not be a mandate line", target, k)
				continue
			}
			assert.Contains(t, joined, k+" → "+v, "%s: mandate %q missing from house style", target, k)
		}
		for _, r := range c.Profile.Vocabulary.ForbiddenTerms {
			assert.Contains(t, joined, r.Term, "%s: forbidden term %q missing from house style", target, r.Term)
		}
		for _, dnt := range c.DNT {
			assert.Contains(t, joined, dnt, "%s: product name %q missing from house style", target, dnt)
		}
	}
}

// TestKeysDoNotLeakExpectations: fixture keys are sent to the model (Context:
// key), so a key naming its trap would hand the bare pass the answer.
func TestKeysDoNotLeakExpectations(t *testing.T) {
	leaky := []string{"formal", "casual", "forbidden", "exclaim", "contraction", "spelling", "trap", "dnt", "glossary", "conflict", "distractor"}
	for _, f := range allFixtures() {
		key := strings.ToLower(f.Key)
		for _, word := range leaky {
			assert.NotContains(t, key, word, "key %q leaks its expectation to the model", f.Key)
		}
	}
}
