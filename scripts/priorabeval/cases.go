package main

import coreprofile "github.com/neokapi/neokapi/core/profile"

// The corpus. Each case is a sentence that was translated once, approved, and
// then edited by an author. The question is whether showing the model what was
// approved before changes what it produces now.
//
// Every case turns on a wording a translator settled and a model cannot infer:
// Norwegian has two ordinary words for a shopping basket, two for signing in,
// and several for a subscription, and picking the wrong one is not an error a
// checker catches. It is simply not the word this product uses.
//
// keep is the approved wording. drift is what a model reaches for instead when
// it has nothing to go on. Both are checked in the output, so consistency is
// measured rather than judged.

type abCase struct {
	// name is what the table calls this case.
	name string
	// priorSource is the English that was approved; source is the English now.
	priorSource string
	source      string
	// priorTarget is the approved Norwegian.
	priorTarget string
	// keep is the wording from priorTarget that should survive the edit, listed
	// in every form Norwegian inflection puts it in. Any one counts:
	// arbeidsområde and arbeidsområdet are the same choice of word, and a check
	// demanding one exact form would score grammar as drift.
	keep []string
	// drift is the wording a model reaches for instead. Its presence is
	// evidence rather than a failure on its own: a translation can avoid both.
	drift []string
	// why says what the choice is, for a reader who does not read Norwegian.
	why string
	// withheld runs the case with no reference at all, because governance moved
	// since the translation was approved. Both prompts are then identical, so a
	// difference in outcome would mean the harness is measuring noise.
	withheld bool
}

var abCases = []abCase{
	{
		name:        "the basket",
		priorSource: "Add to your cart",
		source:      "Add this to your cart",
		priorTarget: "Legg i kurven",
		keep:        []string{"kurven", "kurv"},
		drift:       []string{"handlekurven", "handlekurv"},
		why:         "Norwegian has kurven and handlekurven for a shopping basket. This product says kurven.",
	},
	{
		name:        "signing in",
		priorSource: "Sign in to continue",
		source:      "Sign in to continue to your account",
		priorTarget: "Logg inn for å fortsette",
		keep:        []string{"logg inn"},
		drift:       []string{"logg på"},
		why:         "Logg inn and logg på are both ordinary Norwegian. This product says logg inn.",
	},
	{
		name:        "the subscription",
		priorSource: "Choose a plan",
		source:      "Choose a plan that fits your team",
		priorTarget: "Velg et abonnement",
		keep:        []string{"abonnement", "abonnementet", "abonnementer"},
		drift:       []string{"plan", "planen", "plantype"},
		why:         "A plan can be a plan or an abonnement in Norwegian. This product sells abonnementer.",
	},
	{
		name:        "the workspace",
		priorSource: "Open your workspace settings",
		source:      "Open your workspace settings page",
		priorTarget: "Åpne innstillingene for arbeidsområdet",
		keep:        []string{"arbeidsområde", "arbeidsområdet", "arbeidsområdeinnstillinger"},
		drift:       []string{"arbeidsplass", "arbeidsplassen", "workspace"},
		why:         "Arbeidsområde and arbeidsplass both render workspace. This product says arbeidsområde.",
	},
	{
		name:        "cancelling",
		priorSource: "You can cancel at any time",
		source:      "You can cancel your subscription at any time",
		priorTarget: "Du kan si opp når som helst",
		keep:        []string{"si opp"},
		drift:       []string{"kansellere", "avbryte", "avlyse"},
		why:         "Si opp is what a Norwegian says about ending a subscription; kansellere is a loan word.",
	},
	{
		name:        "the control: governance moved",
		priorSource: "Add to your cart",
		source:      "Add this to your cart",
		priorTarget: "Legg i kurven",
		keep:        []string{"kurven", "kurv"},
		drift:       []string{"handlekurven", "handlekurv"},
		why:         "The same case with the reference withheld, so both prompts are identical. Any difference here is noise, not signal.",
		withheld:    true,
	},
}

// The governance in force, rendered into every prompt exactly as production
// renders it. It deliberately says nothing about the words under test: a term
// rule pinning "cart" would make the reference redundant and the eval circular.
var abVoice = &coreprofile.VoiceProfile{
	ID:   "acme-web",
	Name: "Acme web",
	Tone: coreprofile.ToneProfile{
		Personality: []string{"precise", "calm"},
		Formality:   "neutral",
	},
}

// A terms store of the size a real collection has. Most of these have nothing
// to do with the document under test, which is the point: they are what a
// coordinate accumulates, and what every prompt used to carry.
//
// None of them names a word the A/B turns on. A rule pinning "cart" would make
// the reference redundant and the eval circular — the model would be told the
// answer the prior version exists to supply.
var abTermRules = []coreprofile.TermRule{
	{Term: "log-in", Replacement: "sign in"},
	{Term: "e-mail", Replacement: "email"},
	{Term: "web site", Replacement: "website"},
	{Term: "user name", Replacement: "username"},
	{Term: "back-up", Replacement: "backup"},
	{Term: "click here", Replacement: "select"},
	{Term: "utilise", Replacement: "use"},
	{Term: "commence", Replacement: "start"},
	{Term: "terminate", Replacement: "end"},
	{Term: "endeavour", Replacement: "try"},
	{Term: "purchase", Replacement: "buy"},
	{Term: "obtain", Replacement: "get"},
	{Term: "require", Replacement: "need"},
	{Term: "sufficient", Replacement: "enough"},
	{Term: "additional", Replacement: "more"},
	{Term: "prior to", Replacement: "before"},
	{Term: "in order to", Replacement: "to"},
	{Term: "at this time", Replacement: "now"},
	{Term: "please note", Replacement: "note"},
	{Term: "kindly", Replacement: "please"},
}
