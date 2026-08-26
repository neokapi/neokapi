package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/leverage"
)

// The cases the chain half of the report walks. Each is a real corpus, a real
// gate decision and a real pair of prompts — the point being that the reader
// can see the gate refuse, not read a sentence claiming it would.
var chainCases = []struct {
	name string
	// approvedUnder is the governance in force when the prior answer was
	// approved; inForce is the governance in force now. Equal means the rules
	// have not moved.
	approvedUnder, inForce voiceAndTerms
	priorSource            string
	priorTarget            string
	currentSource          string
	// atAnotherPoint approves the prior answer somewhere else, so the report can
	// show custody refusing as well as governance refusing.
	atAnotherPoint bool
}{
	{
		name:          "the rules have not moved",
		approvedUnder: houseStyle,
		inForce:       houseStyle,
		priorSource:   "Get started",
		priorTarget:   "Kom i gang",
		currentSource: "Get started today",
	},
	{
		name:          "the terms have moved since it was approved",
		approvedUnder: houseStyle,
		inForce:       houseStyleWithNewTerm,
		priorSource:   "Add to your cart",
		priorTarget:   "Legg i kurven",
		currentSource: "Add this to your cart",
	},
	{
		name:          "the voice has moved since it was approved",
		approvedUnder: houseStyle,
		inForce:       plainerVoice,
		priorSource:   "Commence your onboarding",
		priorTarget:   "Start onboardingen din",
		currentSource: "Start your onboarding",
	},
	{
		name:          "only the profile version was bumped",
		approvedUnder: houseStyle,
		inForce:       versionBumpOnly,
		priorSource:   "Get started",
		priorTarget:   "Kom i gang",
		currentSource: "Get started today",
	},
	{
		name:           "the answer was approved somewhere else",
		approvedUnder:  houseStyle,
		inForce:        houseStyle,
		priorSource:    "Get started",
		priorTarget:    "Sett i gang",
		currentSource:  "Get started today",
		atAnotherPoint: true,
	},
}

// voiceAndTerms is one governing context: what a producer would have been given.
type voiceAndTerms struct {
	profile *coreprofile.VoiceProfile
	rules   []coreprofile.TermRule
}

// fingerprint is the hash the staleness gate compares. It is computed the same
// way every producer computes it, so the report cannot drift from production by
// having its own idea of what governance is.
func (v voiceAndTerms) fingerprint() string {
	_, _, fp := coreprofile.GovernanceContext(v.profile, v.rules)
	return fp
}

var (
	houseStyle = voiceAndTerms{
		profile: &coreprofile.VoiceProfile{
			ID: "acme-support", Name: "Acme support", Version: 3,
			Tone: coreprofile.ToneProfile{Personality: []string{"precise"}, Formality: "formal"},
		},
		rules: []coreprofile.TermRule{{Term: "basket", Replacement: "cart"}},
	}
	// One term added, about words this block does not contain. It invalidates
	// anyway, because the fingerprint covers every rule at the coordinate rather
	// than the ones that bite this block — the over-invalidation is deliberate
	// for a staleness gate, which would rather re-check than miss.
	houseStyleWithNewTerm = voiceAndTerms{
		profile: houseStyle.profile,
		rules: []coreprofile.TermRule{
			{Term: "basket", Replacement: "cart"},
			{Term: "sign-in", Replacement: "log in"},
		},
	}
	// A real change in what the model is told.
	plainerVoice = voiceAndTerms{
		profile: &coreprofile.VoiceProfile{
			ID: "acme-support", Name: "Acme support", Version: 4,
			Tone: coreprofile.ToneProfile{Personality: []string{"direct"}, Formality: "plain"},
		},
		rules: houseStyle.rules,
	}
	// The same guidance, a higher version number. The fingerprint hashes what
	// actually reached the producer, so this does NOT invalidate — which is the
	// right answer: nothing the model was told has changed.
	versionBumpOnly = voiceAndTerms{
		profile: &coreprofile.VoiceProfile{
			ID: "acme-support", Name: "Acme support", Version: 9,
			Tone: houseStyle.profile.Tone,
		},
		rules: houseStyle.rules,
	}
)

const (
	reportPoint      = "acme\x1fsupport\x1facme-help"
	reportOtherPoint = "other\x1femail\x1fother-mail"
	reportUnit       = "onboarding.cta"
)

// buildChainsAndPrompts runs every case through the real corpus, the real gate
// and the real prompt builder.
func buildChainsAndPrompts(ctx context.Context) ([]Chain, []PromptPair, error) {
	approvedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	chains := make([]Chain, 0, len(chainCases))
	prompts := make([]PromptPair, 0, len(chainCases))

	for _, tc := range chainCases {
		approvedFP := tc.approvedUnder.fingerprint()
		inForceFP := tc.inForce.fingerprint()

		point := reportPoint
		if tc.atAnotherPoint {
			point = reportOtherPoint
		}

		tm := memory.NewInMemoryStore()
		entry := memory.Entry{
			ID:          "prior",
			Unit:        reportUnit,
			Point:       point,
			HintSrcLang: "en",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: tc.priorSource}}},
				"nb": {{Text: &model.TextRun{Text: tc.priorTarget}}},
			},
			Origins:   []memory.Origin{{Source: "record", AddedAt: approvedAt, ContextFingerprint: approvedFP}},
			CreatedAt: approvedAt,
			UpdatedAt: approvedAt,
		}
		if err := tm.Add(ctx, entry); err != nil {
			return nil, nil, fmt.Errorf("seed corpus for %q: %w", tc.name, err)
		}

		// The gate, asked exactly as a producer asks it.
		priorSrc, priorTgt, offered := leverage.PriorVersionFor(
			ctx, tm, reportUnit, reportPoint, "en", "nb", inForceFP)

		version := Version{
			Source:      tc.priorSource,
			Target:      tc.priorTarget,
			Fingerprint: shortFP(approvedFP),
			ApprovedAt:  approvedAt.Format(time.RFC3339),
			Governed:    approvedFP == inForceFP && !tc.atAnotherPoint,
		}
		chain := Chain{
			Case:        tc.name,
			Unit:        reportUnit,
			Point:       readablePoint(reportPoint),
			InForce:     shortFP(inForceFP),
			CurrentText: tc.currentSource,
			Versions:    []Version{version},
			Diff:        tools.DiffEdit(tc.priorSource, tc.currentSource),
		}
		if offered {
			chain.Offered = &version
		} else {
			chain.Withheld = withheldReason(tc.atAnotherPoint, approvedFP, inForceFP)
		}
		chains = append(chains, chain)

		pair, err := buildPromptPair(tc.name, tc.currentSource, priorSrc, priorTgt, offered, chain.Withheld, tc.inForce)
		if err != nil {
			return nil, nil, err
		}
		prompts = append(prompts, pair)
	}

	return chains, prompts, nil
}

// withheldReason says why in the terms a reader needs, distinguishing the two
// refusals — governance moved, and the answer belongs to somewhere else.
func withheldReason(elsewhere bool, approved, inForce string) string {
	if elsewhere {
		return "approved at another point, so it is not this block's history here"
	}
	if approved != inForce {
		return "approved under governance that has since moved"
	}
	return "no prior answer"
}

// buildPromptPair renders the same block twice through the real prompt builder.
func buildPromptPair(name, source, priorSrc, priorTgt string, offered bool, withheld string, gov voiceAndTerms) (PromptPair, error) {
	t := prompt.Translate{
		SourceLocale:   "en",
		TargetLocale:   "nb",
		VoiceGuide:     coreprofile.RenderVoiceGuideCompact(gov.profile),
		PreferredTerms: coreprofile.TermRuleMap(gov.rules),
	}

	without := prompt.Context{Key: reportUnit}
	with := prompt.Context{Key: reportUnit}
	if offered {
		with.Prior = &prompt.PriorVersion{Source: priorSrc, Target: priorTgt}
	}

	return PromptPair{
		Case:     name,
		Source:   source,
		Without:  systemSections(t.SingleWithContext(source, false, without)),
		With:     systemSections(t.SingleWithContext(source, false, with)),
		Digests:  DigestPair{Without: shortFP(without.Digest()), With: shortFP(with.Digest())},
		Withheld: withheld,
	}, nil
}

// systemSections lifts the system turn's sections, which is what the model is
// told about the block as opposed to the block itself.
func systemSections(turns []prompt.Turn) []PromptSection {
	var out []PromptSection
	for _, turn := range turns {
		if turn.Role != prompt.RoleSystem {
			continue
		}
		for _, s := range turn.Sections {
			if s.Text == "" {
				continue
			}
			out = append(out, PromptSection{
				Kind:    string(s.Kind),
				Origin:  s.Origin,
				Heading: s.Heading,
				Text:    s.Text,
			})
		}
	}
	return out
}

// shortFP trims a fingerprint to something a reader can compare at a glance.
// The dashboard is asking "are these two the same", never "what is this hash".
func shortFP(fp string) string {
	if len(fp) <= 12 {
		return fp
	}
	return fp[:12]
}

// readablePoint renders a point's unit-separated rungs for display.
func readablePoint(p string) string {
	rungs := make([]string, 0, memory.PointRungs)
	for i := range memory.PointRungs {
		if r := memory.PointRung(p, i); r != "" {
			rungs = append(rungs, r)
		}
	}
	if len(rungs) == 0 {
		return "the default point"
	}
	return joinRungs(rungs)
}

func joinRungs(rungs []string) string {
	return strings.Join(rungs, " / ")
}
