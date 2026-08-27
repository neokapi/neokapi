package main

// The corpus is synthesized, and every surface that reports a number from it
// says so.
//
// Real content was the first choice and it does not work here. Two of the three
// measurements need ground truth that no repository carries: a voice profile a
// person wrote from a known corpus, to compare an inferred draft against, and
// prose whose every voice violation has been marked. Labelling real material to
// that standard is the eval, not preparation for it.
//
// What synthesis costs is external validity: the corpus was written by the same
// kind of model the checks are run against, so a violation planted here is one
// the model already thinks of as a violation. That makes these numbers a
// measure of internal consistency — whether the checks find what the profile
// says they should — and not a measure of how the checks behave on prose
// written by someone who has never read the profile. The dashboard says this
// on the card rather than in a footnote.

// Doc is one document in the corpus.
type Doc struct {
	Name string `json:"name"`
	Body string `json:"body"`
	// Plants are the violations written into this document on purpose. A clean
	// document has none, and those are what make precision measurable: a check
	// that flags everything scores perfect recall.
	Plants []Plant `json:"plants,omitempty"`
	// Kind is on-profile or off-profile. voice-infer reads only the on-profile
	// half, because inferring a profile from prose that violates it measures
	// nothing.
	Kind string `json:"kind"`
}

// Plant is one violation and what makes it one.
type Plant struct {
	// Text is the exact substring in the body. A finding matches this plant
	// when it reports the same text in the same document, compared without
	// case, which is how the check reports it back.
	Text string `json:"text"`
	// Rule is the profile field the plant violates, so a check that finds the
	// right word for the wrong reason can be told apart from one that does not.
	Rule string `json:"rule"`
	// Mechanism is how the profile expresses the rule, and it is the axis the
	// whole measurement turns on. A voice profile has three kinds of rule and
	// they are enforced by different things:
	//
	//   term      vocabulary.{forbidden,preferred,competitor}_terms — string
	//             matching, deterministic, offline.
	//   pattern   style.{prohibited,required}_patterns — regex, deterministic,
	//             offline.
	//   declared  style.active_voice, style.person_pov, tone.formality and the
	//             rest of the enum fields. Nothing offline evaluates these.
	//             They reach `kapi voice guide` and the LLM check, and the
	//             deterministic scorer does not read them at all.
	//
	// That last row is why the corpus separates them. A profile that says
	// `active_voice: true` scores dense passive prose at 100/100 offline, and
	// a reader who does not know which mechanism a rule uses cannot tell a
	// clean document from an unchecked one.
	Mechanism string `json:"mechanism"`
}

// The three rule mechanisms a voice profile can express a constraint through.
const (
	mechTerm     = "term"
	mechPattern  = "pattern"
	mechDeclared = "declared"
)

const (
	onProfile  = "on-profile"
	offProfile = "off-profile"
)

// referenceProfileID is the id `kapi voice import` derives from the profile's
// name, which is how the store-resolving tools address it.
const referenceProfileID = "harbourlight"

// referenceProfile is ground truth for voice-infer-quality: the profile a
// person would write from the on-profile half of the corpus, because the
// on-profile half was written from it.
const referenceProfile = `name: Harbourlight
description: Plain, direct voice for a port logistics tool
tone:
    personality:
        - plain
        - direct
        - calm
    formality: neutral
    emotion: neutral
    humor: none
    guidelines: Address the reader as you. State what to do next. No superlatives, no hedging.
style:
    active_voice: true
    sentence_length: short
    person_pov: second
    contractions: sometimes
    prohibited_patterns:
        - regex: '\b(?:is|are|was|were|be|been|being|may be|has been|have been)\s+\w+(?:ed|en)\b'
          description: passive construction
          severity: major
        - regex: '(?i)\bit should be noted that\b'
          description: hedged filler
          severity: minor
vocabulary:
    forbidden_terms:
        - term: utilize
          severity: major
        - term: leverage
          severity: major
        - term: best-in-class
          severity: major
        - term: seamless
          severity: major
        - term: revolutionary
          severity: major
        - term: game-changing
          severity: major
`

// contrastProfile is the control for authoring-effect.
//
// Steering an assistant with any writing guidance improves any reasonable
// score, so "the guided text scores better" is not evidence that the guide
// works. This profile wants the opposite of the reference on every axis it
// can: third person, passive, long sentences, formal, and it forbids the plain
// words the reference prefers. A guide that steers toward Harbourlight
// specifically moves the reference score and leaves this one alone.
const contrastProfile = `name: Meridian Compliance
description: Formal, impersonal register for regulatory filings
tone:
    personality:
        - formal
        - measured
    formality: formal
    emotion: neutral
    humor: none
    guidelines: Maintain an impersonal register. Refer to the operator in the third person.
style:
    active_voice: false
    sentence_length: varied
    person_pov: third
    contractions: never
vocabulary:
    forbidden_terms:
        - term: you
          severity: major
        - term: we
          severity: major
        - term: let's
          severity: major
`

// corpus is the whole synthesized set.
//
// Six on-profile documents and six off-profile ones. The split matters more
// than the size: precision needs documents where the right answer is silence,
// and a corpus of nothing but violations cannot measure a check that reports
// them everywhere.
var corpus = []Doc{
	// ---- On-profile: the voice the reference profile describes ---------------
	{
		Name: "berth-plan.md", Kind: onProfile,
		Body: `# Plan a berth

You book a berth before the vessel arrives. Open the berth plan and pick a
slot. The plan shows which quays are free for the window you need.

If two vessels want the same slot, the plan flags the clash. Move one of them
or shorten the stay. You can save a draft and come back to it.

Confirm the booking when the times are settled. Harbourlight tells the terminal
and the agent.
`,
	},
	{
		Name: "manifest.md", Kind: onProfile,
		Body: `# Read a manifest

Every call has one manifest. It lists what is on board, who owns it, and where
it goes next.

Open the manifest from the call view. Filter by consignee to find one shipment,
or by hazard class to find what needs special handling.

A manifest changes while the vessel is at sea. Harbourlight keeps each version,
so you can see what the crew declared and when.
`,
	},
	{
		Name: "arrivals.md", Kind: onProfile,
		Body: `# Track arrivals

The arrivals board shows every vessel due in the next seven days. It updates
when the vessel reports a new position.

Sort by estimated arrival to see what lands first. Pin a vessel to keep it at
the top while you work.

If an estimate moves by more than two hours, Harbourlight marks the row and
tells anyone who pinned it.
`,
	},
	{
		Name: "roles.md", Kind: onProfile,
		Body: `# Set up roles

Give each person the access their job needs and no more.

A planner books berths and edits plans. A clerk reads manifests and files
customs paperwork. A supervisor does both and can undo either.

Change a role from the team page. The change takes effect on the next sign-in.
`,
	},
	{
		Name: "customs.md", Kind: onProfile,
		Body: `# File with customs

File before the vessel berths. Late filings hold the cargo on the quay.

Harbourlight builds the filing from the manifest. Check the consignee details
and the hazard codes, then submit.

Customs answers within the hour. If the filing is rejected, the reason appears
on the call view and the clerk who filed it is told.
`,
	},
	{
		Name: "alerts.md", Kind: onProfile,
		Body: `# Choose your alerts

You decide what Harbourlight tells you about.

Turn on arrival changes to hear when an estimate moves. Turn on clash alerts to
hear when two vessels want one berth. Turn on customs alerts to hear when a
filing is answered.

Alerts go to email by default. Add a phone number if you want them by text.
`,
	},

	// ---- Off-profile: the same subject, written against the profile ----------
	{
		Name: "overview-marketing.md", Kind: offProfile,
		Body: `# Harbourlight Overview

Harbourlight is a revolutionary port logistics platform that allows operators to
utilize a single pane of glass across every terminal touchpoint. Organizations
can leverage our best-in-class scheduling engine to deliver seamless berth
allocation at scale.
`,
		Plants: []Plant{
			{Text: "revolutionary", Rule: "vocabulary.forbidden_terms", Mechanism: mechTerm},
			{Text: "utilize", Rule: "vocabulary.forbidden_terms", Mechanism: mechTerm},
			{Text: "leverage", Rule: "vocabulary.forbidden_terms", Mechanism: mechTerm},
			{Text: "best-in-class", Rule: "vocabulary.forbidden_terms", Mechanism: mechTerm},
			{Text: "seamless", Rule: "vocabulary.forbidden_terms", Mechanism: mechTerm},
		},
	},
	{
		Name: "release-notes.md", Kind: offProfile,
		Body: `# Release Notes

This release introduces game-changing improvements to the manifest viewer,
allowing users to utilize filters that were previously unavailable.

Performance has been improved and the experience is now seamless.
`,
		Plants: []Plant{
			{Text: "game-changing", Rule: "vocabulary.forbidden_terms", Mechanism: mechTerm},
			{Text: "utilize", Rule: "vocabulary.forbidden_terms", Mechanism: mechTerm},
			{Text: "seamless", Rule: "vocabulary.forbidden_terms", Mechanism: mechTerm},
		},
	},
	{
		Name: "onboarding-email.md", Kind: offProfile,
		Body: `# Welcome

We are delighted to welcome you to Harbourlight, the best-in-class solution
trusted by leading operators worldwide.

Our team will reach out shortly to help you leverage the full breadth of the
platform's revolutionary capabilities.
`,
		Plants: []Plant{
			{Text: "best-in-class", Rule: "vocabulary.forbidden_terms", Mechanism: mechTerm},
			{Text: "leverage", Rule: "vocabulary.forbidden_terms", Mechanism: mechTerm},
			{Text: "revolutionary", Rule: "vocabulary.forbidden_terms", Mechanism: mechTerm},
		},
	},
	{
		Name: "help-passive.md", Kind: offProfile,
		Body: `# Berth Allocation

A berth may be allocated by a planner once the vessel's estimated arrival has
been confirmed by the agent. It should be noted that allocations which have
been submitted cannot subsequently be amended by users other than those to whom
supervisory permissions have been granted, and any such amendment will be
recorded in the audit trail that is maintained by the platform for the purpose
of subsequent review.
`,
		Plants: []Plant{
			{Text: "may be allocated", Rule: "style.prohibited_patterns/passive", Mechanism: mechPattern},
			{Text: "has been confirmed", Rule: "style.prohibited_patterns/passive", Mechanism: mechPattern},
			{Text: "It should be noted that", Rule: "style.prohibited_patterns/hedge", Mechanism: mechPattern},
		},
	},
	{
		Name: "faq-thirdperson.md", Kind: offProfile,
		Body: `# Frequently Asked Questions

How does the operator amend a filing?

The operator navigates to the call view, where the operator selects the filing
in question. The operator then submits an amendment, which the system routes to
customs on the operator's behalf.
`,
		Plants: []Plant{
			{Text: "The operator", Rule: "style.person_pov", Mechanism: mechDeclared},
		},
	},
	{
		Name: "status-page.md", Kind: offProfile,
		Body: `# Status

Harbourlight utilizes a globally distributed infrastructure to deliver a
seamless experience.

Should degradation occur, remediation is initiated automatically and affected
parties are notified in due course.
`,
		Plants: []Plant{
			{Text: "utilizes", Rule: "vocabulary.forbidden_terms", Mechanism: mechTerm},
			{Text: "seamless", Rule: "vocabulary.forbidden_terms", Mechanism: mechTerm},
			{Text: "is initiated", Rule: "style.prohibited_patterns/passive", Mechanism: mechPattern},
		},
	},
}

// docsOfKind returns one half of the corpus.
func docsOfKind(kind string) []Doc {
	var out []Doc
	for _, d := range corpus {
		if d.Kind == kind {
			out = append(out, d)
		}
	}
	return out
}

// plantsByMechanism counts what the corpus plants for each of the three ways a
// profile can express a rule, so a report can say which mechanisms it has
// evidence about and how much.
func plantsByMechanism() map[string]int {
	out := map[string]int{}
	for _, d := range corpus {
		for _, p := range d.Plants {
			out[p.Mechanism]++
		}
	}
	return out
}
