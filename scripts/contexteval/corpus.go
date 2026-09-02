package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// The corpus is engineered so that a *naive* translation violates the injected
// context — otherwise a pass is a freebie and the fixture measures nothing.
// Each fixture exists to tempt a specific violation:
//
//   - a ruled term whose natural translation differs from the mandated one
//     ("dashboard" → the anglicism, where the rules mandate "Leitstand");
//   - a product name that reads like a common noun ("Compass"), which a model
//     will translate unless told not to;
//   - casual English that tempts an informal register where the voice profile is
//     formal, and contractions where the brand forbids them;
//   - rule-shaped instructions the source actively violates ("no exclamation
//     marks" on a source that ends in one).
//
// It also carries distractors — a lowercase "compass" that is a real compass
// and must be translated, an idiomatic "wide berth" that must not receive the
// technical term — and declared-winner conflicts, where a rule pins a
// compound that contains a forbidden vocabulary word. The fixture states what
// is expected to win; the scorer enforces exactly that.
//
// The brand, the products and the guide are synthetic ("Northsea", a fictional
// maritime-operations vendor) and engineered for trap density. They must not
// resemble any real customer's brand guide.
//
// Fixture keys are sent to the model (Context: key, the production default), so
// they must be realistic i18n keys and must never leak the expectation — a key
// like "formal_greeting" would hand the bare run the answer.

// Dimensions attribute every expectation to the piece of context that carries
// it, so the dashboard can say which part of the payload earns its tokens.
const (
	// DimTerminology is carried by the term rules (term mandates and the identity
	// pins that keep do-not-translate names verbatim).
	DimTerminology = "terminology"
	// DimVoice is carried by the voice profile, injected in production
	// via profile.RenderVoiceGuideCompact: tone (personality, formality, humor,
	// guidelines), style (contractions, sentence length, prohibited patterns),
	// and forbidden-term rules.
	DimVoice = "voice"
	// DimInstruction is carried by the free-form instruction string.
	DimInstruction = "instruction"
)

// Check is one deterministic expectation on a fixture's translation. Exactly
// one scoring backend applies, chosen by which fields are set:
//
//   - Term: the real term-check tool (mandated target term must appear);
//   - DNT: the real dnt-check tool (term must survive verbatim);
//   - VocabClean: the real voice-vocab-check tool over the translation
//     (no unexcused forbidden/competitor hits);
//   - MustMatch / MustNotMatch: the real pattern-check tool (either or both).
type Check struct {
	Dimension string
	// Kind labels the trap for the per-kind breakdown: term, term-distractor,
	// term-conflict, dnt, dnt-distractor, vocab, vocab-conflict, formality,
	// contractions, exclamation, digits, verbatim, spelling.
	Kind string

	Term         *coreprofile.TermRule
	DNT          string
	VocabClean   bool
	MustMatch    string
	MustNotMatch string
}

// Fixture is one corpus entry: a source string plus, per target locale, the
// observable properties its translation must (or must not) have. A fixture
// with no checks for the swept target is not part of that target's corpus.
type Fixture struct {
	Key    string
	Source string
	Checks map[string][]Check
	// AllowVocab names profile vocabulary terms excused for this fixture — the
	// declared winner of an engineered conflict (e.g. a rule pins
	// "Nutzer-ID" while the profile forbids "Nutzer"; the pin wins).
	AllowVocab []string
	// Note says what the trap is and what the naive translation gets wrong, so
	// a failure can be inspected rather than trusted.
	Note string
}

// Context is the payload under test for one target locale — injected into the
// translate prompt exactly the way production injects it (term rules,
// profile.VoiceProfile rendered compact, instruction section) and withheld
// entirely on the bare pass.
type Context struct {
	// TermRules are the rules the prompt's terminology section renders, in the
	// shape production passes them (host.ResolveTermRules). Do-not-translate
	// names ride here as identity pins — Replacement equal to Term — which is
	// how a terms store concept whose preferred target term equals the source
	// term reaches generation.
	TermRules []coreprofile.TermRule
	// DNT lists the names that must survive verbatim — what dnt-check enforces.
	DNT []string
	// Profile is the synthetic voice profile. What
	// profile.RenderVoiceGuideCompact renders reaches the model — every populated
	// tone/style constraint plus forbidden-term rules — so every voice fixture
	// tests exactly that surface.
	Profile *coreprofile.VoiceProfile
	// Instruction is the free-form steering string.
	Instruction string
}

// termRules builds the injected rules from a term → mandated-rendering map.
// The corpus authors its mandates as a map because that reads better than a
// column of two-field literals; the rules come out ordered by term, so one
// corpus yields one prompt and one digest.
func termRules(m map[string]string) []coreprofile.TermRule {
	out := make([]coreprofile.TermRule, 0, len(m))
	for _, term := range slices.Sorted(maps.Keys(m)) {
		out = append(out, coreprofile.TermRule{Term: term, Replacement: m[term]})
	}
	return out
}

// Mandate returns the rendering the context mandates for a source term.
func (c Context) Mandate(term string) (string, bool) {
	for _, r := range c.TermRules {
		if r.Term == term {
			return r.Replacement, true
		}
	}
	return "", false
}

// HouseStyle renders the target's mandated vocabulary as short lines for
// anyone judging subjective quality — the LLM judge and the human labeler
// alike. A mandate is deliberately non-naive (that is what makes it
// measurable), so a judge who does not know it will penalize obedience as
// unnaturalness — "alarmmelding" reads odd to a Norwegian who expects
// "varsling", but it is the commanded rendering, and terminology is owned by
// the deterministic checks, never by the rubric. Identity pins are folded into
// the product-names line.
func (c Context) HouseStyle() []string {
	var out []string
	for _, r := range c.TermRules {
		if r.Replacement == r.Term {
			continue
		}
		out = append(out, fmt.Sprintf("%s → %s", r.Term, r.Replacement))
	}
	if c.Profile != nil {
		for _, r := range c.Profile.Vocabulary.ForbiddenTerms {
			out = append(out, fmt.Sprintf("avoid %q (use %q)", r.Term, r.Replacement))
		}
	}
	if len(c.DNT) > 0 {
		out = append(out, "product names kept verbatim: "+strings.Join(c.DNT, ", "))
	}
	return out
}

// TestCorpus is the swept unit: the fixtures that carry expectations for one
// target locale, plus the context injected on the steered pass.
type TestCorpus struct {
	Target   string
	Fixtures []Fixture
	Ctx      Context
}

// CorpusFor assembles the corpus for one target locale. Targets without
// authored expectations yield an empty corpus, which the sweep rejects.
func CorpusFor(target string) TestCorpus {
	var fixtures []Fixture
	for _, f := range allFixtures() {
		if len(f.Checks[target]) > 0 {
			fixtures = append(fixtures, f)
		}
	}
	return TestCorpus{
		Target:   target,
		Fixtures: fixtures,
		Ctx:      contextFor(target),
	}
}

// Targets lists the locales the corpus carries expectations for. de and fr are
// machine-scored only; en-GB and nb are also the human-auditable pair — the
// founder reads English and Norwegian, so judge-validation ground truth comes
// from those two.
func Targets() []string { return []string{"de", "fr", "en-GB", "nb"} }

// Words counts source words across the corpus — the cost denominator, same
// unit as batcheval (content budgets are denominated in words, not tokens).
func (c TestCorpus) Words() int {
	n := 0
	for _, f := range c.Fixtures {
		n += len(strings.Fields(f.Source))
	}
	return n
}

// Checks counts the scoreable expectations in the corpus for its target.
func (c TestCorpus) Checks() int {
	n := 0
	for _, f := range c.Fixtures {
		n += len(f.Checks[c.Target])
	}
	return n
}

// Digest identifies the exact experiment: the fixtures, their expectations,
// and the context payload the steered pass injects. Change any of them — a
// reworded source, a new term mandate, a different voice guide — and the
// digest moves, so runs measured on different experiments are never plotted
// as one trend.
func (c TestCorpus) Digest() string {
	h := sha256.New()
	fmt.Fprintf(h, "target:%s\x00", c.Target)
	for _, f := range c.Fixtures {
		fmt.Fprintf(h, "%s\x00%s\x00", f.Key, f.Source)
		for _, chk := range f.Checks[c.Target] {
			fmt.Fprintf(h, "%s|%s", chk.Dimension, chk.Kind)
			if chk.Term != nil {
				fmt.Fprintf(h, "|term:%s→%s", chk.Term.Term, chk.Term.Replacement)
			}
			if chk.DNT != "" {
				fmt.Fprintf(h, "|dnt:%s", chk.DNT)
			}
			if chk.VocabClean {
				fmt.Fprintf(h, "|vocab")
			}
			if chk.MustMatch != "" {
				fmt.Fprintf(h, "|match:%s", chk.MustMatch)
			}
			if chk.MustNotMatch != "" {
				fmt.Fprintf(h, "|not:%s", chk.MustNotMatch)
			}
			fmt.Fprint(h, "\x00")
		}
		fmt.Fprintf(h, "allow:%s\x00", strings.Join(f.AllowVocab, ","))
	}
	for _, r := range c.Ctx.TermRules {
		fmt.Fprintf(h, "g:%s→%s\x00", r.Term, r.Replacement)
	}
	fmt.Fprintf(h, "dnt:%s\x00", strings.Join(c.Ctx.DNT, ","))
	// The rendered guide, not the profile struct: what the model sees is what
	// the experiment is.
	fmt.Fprintf(h, "voice:%s\x00", coreprofile.RenderVoiceGuideCompact(c.Ctx.Profile))
	fmt.Fprintf(h, "instruction:%s\x00", c.Ctx.Instruction)
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func (c TestCorpus) Describe() string {
	byDim := map[string]int{}
	for _, f := range c.Fixtures {
		for _, chk := range f.Checks[c.Target] {
			byDim[chk.Dimension]++
		}
	}
	var parts []string
	for _, d := range []string{DimTerminology, DimVoice, DimInstruction} {
		if n := byDim[d]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, d))
		}
	}
	return fmt.Sprintf("%d fixtures, %d checks (%s)", len(c.Fixtures), c.Checks(), strings.Join(parts, ", "))
}

// Blocks renders the corpus as translatable blocks. The key rides in Name so
// the production `Context: key` policy has something to send.
func (c TestCorpus) Blocks() []*model.Block {
	out := make([]*model.Block, len(c.Fixtures))
	for i, f := range c.Fixtures {
		b := model.NewBlock(fmt.Sprintf("tu%d", i+1), f.Source)
		b.Name = f.Key
		b.Translatable = true
		out[i] = b
	}
	return out
}

// contractionRe tempts on straight and curly apostrophes, and is restricted to
// unambiguous contractions (n't, pronoun+'re/'ll/'ve/'d, it's/there's, I'm) so a
// legitimate possessive ("the vessel's position") never scores as a violation.
const contractionRe = `(?i)(\b\w+n['’]t\b|\b(it|we|you|they|there|that|who)['’](s|re|ll|ve|d)\b|\bi['’]m\b)`

// Informal-address patterns. Go's \b is ASCII-only, which makes it a word
// boundary next to any accented letter — `\btes\b` matches the tail of "êtes",
// `\bte\b` the middle of "boîte" — so a perfectly formal French sentence would
// score as informal. The guards are explicit non-letter classes instead
// (Unicode-aware), and case-insensitive because informal German capitalizes
// "Du" in letters.
const (
	informalDE = `(?i)(^|[^\p{L}])(du|dich|dir|dein\p{L}*|euch|euer\p{L}*|eure\p{L}*)($|[^\p{L}])`
	informalFR = `(?i)(^|[^\p{L}])(tu|te|toi|ton|ta|tes)($|[^\p{L}])`
)

// contextFor builds the payload for one target. The term rules and the profile
// vocabulary are target-language artefacts (a German mandate is useless in
// French), so each target gets its own; tone, style and the instruction are
// shared, the way one brand config covers many locales in production.
func contextFor(target string) Context {
	instruction := "Never use exclamation marks. Write numbers as digits, never as words. " +
		"Keep any text wrapped in backticks exactly as it appears in the source."

	profile := &coreprofile.VoiceProfile{
		ID:          "northsea-voice",
		Name:        "Northsea",
		Description: "Synthetic voice profile for the context eval, engineered for trap density, resembling no real customer.",
		Tone: coreprofile.ToneProfile{
			Personality: []string{"precise", "calm"},
			Formality:   "formal",
			Emotion:     "neutral",
			Humor:       "none",
		},
		Style: coreprofile.StyleRules{
			ActiveVoice:  true,
			Contractions: "never",
			PersonPOV:    "second",
		},
	}

	ctx := Context{Profile: profile, Instruction: instruction}
	switch target {
	case "de":
		ctx.TermRules = termRules(map[string]string{
			"dashboard": "Leitstand",
			"alert":     "Warnmeldung",
			"sync":      "Datenabgleich",
			"report":    "Report", // the reverse trap: the brand mandates the anglicism, naive German says "Bericht"
			"berth":     "Anlegeplatz",
			"vessel":    "Wasserfahrzeug",
			"user ID":   "Nutzer-ID", // declared conflict: contains the forbidden "Nutzer"; the pin wins
			"Tidewatch": "Tidewatch",
			"Compass":   "Compass",
			"tidectl":   "tidectl",
		})
		// Every forbidden term carries a replacement, deliberately: the eval
		// scores swap adherence ("was the mandated replacement used"), which
		// needs a declared replacement to check against. (The compact guide now
		// renders bare bans too; scoring them would need a different check.)
		profile.Vocabulary.ForbiddenTerms = []coreprofile.TermRule{
			{Term: "einfach", Replacement: "direkt", Note: "filler minimizer"},
			{Term: "Nutzer", Replacement: "Benutzer"},
			{Term: "App", Replacement: "Anwendung"},
		}
	case "fr":
		ctx.TermRules = termRules(map[string]string{
			"dashboard": "poste de pilotage",
			"alert":     "avis de vigilance",
			"sync":      "rapprochement des données",
			"report":    "compte rendu",
			// Apostrophe-free by design: term-check scores by exact substring,
			// and a mandate like "poste d'amarrage" would false-fail whenever a
			// model emits the typographic apostrophe (d’amarrage). That
			// normalization gap belongs to term-check, not to this corpus.
			"berth":     "appontement",
			"vessel":    "bâtiment",
			"workboat":  "bateau de service", // declared conflict: contains the forbidden "bateau"; the pin wins
			"Tidewatch": "Tidewatch",
			"Compass":   "Compass",
			"tidectl":   "tidectl",
		})
		profile.Vocabulary.ForbiddenTerms = []coreprofile.TermRule{
			{Term: "simplement", Replacement: "directement", Note: "filler minimizer"},
			{Term: "bateau", Replacement: "navire"},
		}
	case "en-GB":
		ctx.TermRules = termRules(map[string]string{
			"sign in":   "log on",
			"settings":  "preferences",
			"Tidewatch": "Tidewatch",
			"Compass":   "Compass",
			"tidectl":   "tidectl",
		})
		profile.Vocabulary.ForbiddenTerms = []coreprofile.TermRule{
			{Term: "leverage", Replacement: "use"},
			{Term: "seamless", Replacement: "unified"},
		}
		ctx.Instruction = instruction + " Use British English spelling."
	case "nb":
		// Norwegian mandates are chosen inflection-safe: term-check matches
		// substrings, and Norwegian suffixes both plurals and definiteness onto
		// the stem ("varsel" → "varsler" loses the mandate; "alarmmelding" →
		// "alarmmeldinger"/"alarmmeldingen" keeps it). There are no formality
		// checks for nb — du-form IS the correct register in modern Bokmål, so
		// a De-form mandate would punish natural output.
		ctx.TermRules = termRules(map[string]string{
			"dashboard": "styringspanel",    // naive: dashbord / instrumentpanel
			"alert":     "alarmmelding",     // naive: varsel
			"sync":      "samstilling",      // naive: synkronisering
			"report":    "driftsrapport",    // naive: rapport
			"berth":     "fortøyningsplass", // naive: kaiplass
			"vessel":    "farkost",          // naive: fartøy
			"Tidewatch": "Tidewatch",
			"Compass":   "Compass",
			"tidectl":   "tidectl",
		})
		profile.Vocabulary.ForbiddenTerms = []coreprofile.TermRule{
			{Term: "båt", Replacement: "farkost", Note: "casual register"},
			{Term: "sømløs", Replacement: "helhetlig"},
		}
	}
	ctx.DNT = []string{"Tidewatch", "Compass", "tidectl"}
	return ctx
}

// allFixtures returns the authored corpus. Order is stable — it feeds the
// digest.
func allFixtures() []Fixture {
	return []Fixture{
		// ---- Terminology: mandates whose naive rendering differs ----
		{
			Key:    "nav.overview.title",
			Source: "Open the dashboard to review today's traffic.",
			Note:   "naive: the anglicism (de) / tableau de bord (fr); the rules mandate the house term",
			Checks: map[string][]Check{
				"de": {{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "dashboard", Replacement: "Leitstand"}}},
				"fr": {{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "dashboard", Replacement: "poste de pilotage"}}},
				"nb": {{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "dashboard", Replacement: "styringspanel"}}},
			},
		},
		{
			Key:    "alerts.badge.tooltip",
			Source: "Two new alerts need your attention.",
			Note:   "term trap (alert) plus the digits instruction on a spelled-out number",
			Checks: map[string][]Check{
				"de": {
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "alert", Replacement: "Warnmeldung"}},
					{Dimension: DimInstruction, Kind: "digits", MustMatch: `\b2\b`, MustNotMatch: `(?i)\bzwei\b`},
				},
				"fr": {
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "alert", Replacement: "avis de vigilance"}},
					{Dimension: DimInstruction, Kind: "digits", MustMatch: `\b2\b`, MustNotMatch: `(?i)\bdeux\b`},
				},
				"en-GB": {
					{Dimension: DimInstruction, Kind: "digits", MustMatch: `\b2\b`, MustNotMatch: `(?i)\btwo\b`},
				},
				"nb": {
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "alert", Replacement: "alarmmelding"}},
					{Dimension: DimInstruction, Kind: "digits", MustMatch: `\b2\b`, MustNotMatch: `(?i)(^|[^\p{L}])to($|[^\p{L}])`},
				},
			},
		},
		{
			Key:    "sync.status.failed",
			Source: "The last sync failed. Check your connection and try again.",
			Note:   "naive: Synchronisierung / synchronisation",
			Checks: map[string][]Check{
				"de": {{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "sync", Replacement: "Datenabgleich"}}},
				"fr": {{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "sync", Replacement: "rapprochement des données"}}},
				"nb": {{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "sync", Replacement: "samstilling"}}},
			},
		},
		{
			Key:    "reports.menu.label",
			Source: "Generate a report for the selected period.",
			Note:   "reverse trap in German: the brand mandates the anglicism 'Report', naive is 'Bericht'",
			Checks: map[string][]Check{
				"de": {{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "report", Replacement: "Report"}}},
				"fr": {{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "report", Replacement: "compte rendu"}}},
				"nb": {{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "report", Replacement: "driftsrapport"}}},
			},
		},
		{
			Key:    "berths.assign.action",
			Source: "Assign each vessel a berth before it arrives.",
			Note:   "two mandates in one string; naive de: Liegeplatz/Schiff, fr: poste à quai/navire",
			Checks: map[string][]Check{
				"de": {
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "berth", Replacement: "Anlegeplatz"}},
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "vessel", Replacement: "Wasserfahrzeug"}},
				},
				"fr": {
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "berth", Replacement: "appontement"}},
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "vessel", Replacement: "bâtiment"}},
				},
				"nb": {
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "berth", Replacement: "fortøyningsplass"}},
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "vessel", Replacement: "farkost"}},
				},
			},
		},
		{
			Key:    "settings.menu.label",
			Source: "Open your settings to change how alerts appear.",
			Note:   "en-GB mandate: settings → preferences; models keep 'settings'",
			Checks: map[string][]Check{
				"en-GB": {{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "settings", Replacement: "preferences"}}},
			},
		},
		{
			Key:    "account.signin.button",
			Source: "Sign in to view your fleet.",
			Note:   "en-GB mandate: sign in → log on",
			Checks: map[string][]Check{
				"en-GB": {{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "sign in", Replacement: "log on"}}},
			},
		},
		{
			Key:    "safety.notice.shallow",
			Source: "Give the shallow bank a wide berth.",
			Note:   "distractor: idiomatic 'berth', where forcing the technical mandate is wrong",
			Checks: map[string][]Check{
				"de": {{Dimension: DimTerminology, Kind: "term-distractor", MustNotMatch: `(?i)anlegeplatz`}},
				"fr": {{Dimension: DimTerminology, Kind: "term-distractor", MustNotMatch: `(?i)appontement`}},
				"nb": {{Dimension: DimTerminology, Kind: "term-distractor", MustNotMatch: `(?i)fortøyningsplass`}},
			},
		},
		{
			Key:    "account.link.label",
			Source: "Enter your user ID to link the mobile app.",
			Note:   "declared conflict: a rule pins 'Nutzer-ID' while the profile forbids 'Nutzer', and the pin wins",
			Checks: map[string][]Check{
				"de": {
					{Dimension: DimTerminology, Kind: "term-conflict", Term: &coreprofile.TermRule{Term: "user ID", Replacement: "Nutzer-ID"}},
					{Dimension: DimVoice, Kind: "vocab-conflict", VocabClean: true},
				},
			},
			AllowVocab: []string{"Nutzer"},
		},
		{
			Key: "workboats.map.legend",
			// Singular by design: a plural source ("Workboats") would earn the
			// obedient rendering "bateaux de service", which the substring-based
			// term-check cannot credit against the singular mandate — the eval
			// would punish obedience. Inflection-aware term matching is a
			// term-check concern, not this corpus's.
			Source: "Each workboat appears in amber on the map.",
			Note:   "declared conflict: a rule pins 'bateau de service' while the profile forbids 'bateau', and the pin wins",
			Checks: map[string][]Check{
				"fr": {
					{Dimension: DimTerminology, Kind: "term-conflict", Term: &coreprofile.TermRule{Term: "workboat", Replacement: "bateau de service"}},
					{Dimension: DimVoice, Kind: "vocab-conflict", VocabClean: true},
				},
			},
			AllowVocab: []string{"bateau"},
		},

		// ---- Terminology: do-not-translate names that read like common nouns ----
		{
			Key:    "compass.overview.tagline",
			Source: "Compass shows every vessel in a single view.",
			Note:   "product name that is a common noun; naive: Kompass / boussole",
			Checks: map[string][]Check{
				"de": {
					{Dimension: DimTerminology, Kind: "dnt", DNT: "Compass"},
					// (?i)kompass, not \bKompass\b: the translated product could
					// land in a compound ("Schiffskompass"), and "Compass" itself
					// never matches (C vs K).
					{Dimension: DimTerminology, Kind: "dnt", MustNotMatch: `(?i)kompass`},
				},
				"fr": {
					{Dimension: DimTerminology, Kind: "dnt", DNT: "Compass"},
					{Dimension: DimTerminology, Kind: "dnt", MustNotMatch: `(?i)\b(boussole|compas)\b`},
				},
				"nb": {
					{Dimension: DimTerminology, Kind: "dnt", DNT: "Compass"},
					// "kompass" never matches the preserved "Compass" (k vs C).
					{Dimension: DimTerminology, Kind: "dnt", MustNotMatch: `(?i)kompass`},
				},
			},
		},
		{
			Key:    "tidewatch.intro.body",
			Source: "Tidewatch alerts you before conditions change.",
			Note:   "coined product name; a model may 'localize' it (Gezeitenwacht) without the pin",
			Checks: map[string][]Check{
				"de": {{Dimension: DimTerminology, Kind: "dnt", DNT: "Tidewatch"}},
				"fr": {{Dimension: DimTerminology, Kind: "dnt", DNT: "Tidewatch"}},
				"nb": {{Dimension: DimTerminology, Kind: "dnt", DNT: "Tidewatch"}},
			},
		},
		{
			Key:    "nav.instrument.calibrate",
			Source: "Calibrate the ship's compass before departure.",
			Note:   "distractor: a real compass, lowercase, which must be translated rather than protected",
			Checks: map[string][]Check{
				// (?i)kompass so the idiomatic compound ("Schiffskompass") passes;
				// requiring \bKompass would fail the more natural translation.
				"de": {{Dimension: DimTerminology, Kind: "dnt-distractor", MustMatch: `(?i)kompass`}},
				"fr": {{Dimension: DimTerminology, Kind: "dnt-distractor", MustMatch: `(?i)(boussole|compas)`}},
				"nb": {{Dimension: DimTerminology, Kind: "dnt-distractor", MustMatch: `(?i)kompass`}},
			},
		},
		{
			Key:    "cli.sync.hint",
			Source: "Run `tidectl sync` to fetch the latest data.",
			Note:   "backtick verbatim rule plus a lowercase code identifier under DNT",
			Checks: map[string][]Check{
				"de": {
					{Dimension: DimTerminology, Kind: "dnt", DNT: "tidectl"},
					{Dimension: DimInstruction, Kind: "verbatim", MustMatch: "`tidectl sync`"},
				},
				"fr": {
					{Dimension: DimTerminology, Kind: "dnt", DNT: "tidectl"},
					{Dimension: DimInstruction, Kind: "verbatim", MustMatch: "`tidectl sync`"},
				},
				"en-GB": {
					{Dimension: DimInstruction, Kind: "verbatim", MustMatch: "`tidectl sync`"},
				},
				"nb": {
					{Dimension: DimTerminology, Kind: "dnt", DNT: "tidectl"},
					{Dimension: DimInstruction, Kind: "verbatim", MustMatch: "`tidectl sync`"},
				},
			},
		},
		{
			Key:    "cli.report.hint",
			Source: "Run `tidectl report --daily` before you export.",
			Note:   "verbatim span containing a ruled word ('report'), and the backtick rule wins inside the span",
			Checks: map[string][]Check{
				"de": {
					{Dimension: DimTerminology, Kind: "dnt", DNT: "tidectl"},
					{Dimension: DimInstruction, Kind: "verbatim", MustMatch: "`tidectl report --daily`"},
				},
				"fr": {
					{Dimension: DimTerminology, Kind: "dnt", DNT: "tidectl"},
					{Dimension: DimInstruction, Kind: "verbatim", MustMatch: "`tidectl report --daily`"},
				},
				"en-GB": {
					{Dimension: DimInstruction, Kind: "verbatim", MustMatch: "`tidectl report --daily`"},
				},
				"nb": {
					{Dimension: DimTerminology, Kind: "dnt", DNT: "tidectl"},
					{Dimension: DimInstruction, Kind: "verbatim", MustMatch: "`tidectl report --daily`"},
				},
			},
		},

		// ---- Voice: the slice the compact guide actually injects ----
		{
			Key:    "onboarding.done.note",
			Source: "Hey, you're all set. Grab your files whenever you want.",
			Note:   "casual source tempts du/tu and contractions; the profile is formal, contractions never",
			Checks: map[string][]Check{
				"de":    {{Dimension: DimVoice, Kind: "formality", MustNotMatch: informalDE}},
				"fr":    {{Dimension: DimVoice, Kind: "formality", MustNotMatch: informalFR}},
				"en-GB": {{Dimension: DimVoice, Kind: "contractions", MustNotMatch: contractionRe}},
			},
		},
		{
			Key:    "files.share.hint",
			Source: "Share your files with your crew, and they'll see updates as you make them.",
			Checks: map[string][]Check{
				"de":    {{Dimension: DimVoice, Kind: "formality", MustNotMatch: informalDE}},
				"fr":    {{Dimension: DimVoice, Kind: "formality", MustNotMatch: informalFR}},
				"en-GB": {{Dimension: DimVoice, Kind: "contractions", MustNotMatch: contractionRe}},
			},
		},
		{
			Key:    "status.update.progress",
			Source: "We're updating your workspace. It'll only take a moment. Don't close this window.",
			Note:   "three contractions; the en-GB pass must expand them all",
			Checks: map[string][]Check{
				"en-GB": {{Dimension: DimVoice, Kind: "contractions", MustNotMatch: contractionRe}},
			},
		},
		{
			Key:    "apps.mobile.promo",
			Source: "Download the app to get alerts on your phone.",
			Note:   "'App' is the natural German rendering; the profile forbids it in favour of 'Anwendung'",
			Checks: map[string][]Check{
				"de": {{Dimension: DimVoice, Kind: "vocab", VocabClean: true}},
			},
		},
		{
			Key:    "map.marker.add",
			Source: "Just click the map to add your first marker!",
			Note:   "tempts the filler ('einfach'/'simplement') and keeps the exclamation mark",
			Checks: map[string][]Check{
				"de": {
					{Dimension: DimVoice, Kind: "vocab", VocabClean: true},
					{Dimension: DimInstruction, Kind: "exclamation", MustNotMatch: `!`},
				},
				"fr": {
					{Dimension: DimVoice, Kind: "vocab", VocabClean: true},
					{Dimension: DimInstruction, Kind: "exclamation", MustNotMatch: `!`},
				},
				"en-GB": {
					{Dimension: DimInstruction, Kind: "exclamation", MustNotMatch: `!`},
				},
				"nb": {
					{Dimension: DimInstruction, Kind: "exclamation", MustNotMatch: `!`},
				},
			},
		},
		{
			Key:    "users.alerts.perms",
			Source: "Every user can adjust their alert thresholds.",
			Note:   "'user' tempts the forbidden 'Nutzer'; the swap mandates 'Benutzer'",
			Checks: map[string][]Check{
				"de": {{Dimension: DimVoice, Kind: "vocab", VocabClean: true}},
			},
		},
		{
			Key:    "harbor.traffic.title",
			Source: "Track every boat in the harbor.",
			Note:   "fr/nb: the naive word (bateau/båt) is forbidden; en-GB: harbor → harbour",
			Checks: map[string][]Check{
				"fr": {{Dimension: DimVoice, Kind: "vocab", VocabClean: true}},
				"en-GB": {
					{Dimension: DimInstruction, Kind: "spelling", MustMatch: `\bharbour\b`, MustNotMatch: `\bharbor\b`},
				},
				"nb": {{Dimension: DimVoice, Kind: "vocab", VocabClean: true}},
			},
		},
		{
			Key:    "marketing.api.blurb",
			Source: "Leverage our API for a seamless view of your fleet.",
			Note:   "forbidden marketing vocabulary with mandated swaps: en-GB leverage/seamless, nb 'sømløs'",
			Checks: map[string][]Check{
				"en-GB": {{Dimension: DimVoice, Kind: "vocab", VocabClean: true}},
				"nb":    {{Dimension: DimVoice, Kind: "vocab", VocabClean: true}},
			},
		},

		// ---- Instruction: rule-shaped steering the source actively violates ----
		{
			Key:    "welcome.aboard.title",
			Source: "Welcome aboard! Your account is ready to go!",
			Note:   "two exclamation marks; a bare pass keeps them",
			Checks: map[string][]Check{
				"de":    {{Dimension: DimInstruction, Kind: "exclamation", MustNotMatch: `!`}},
				"fr":    {{Dimension: DimInstruction, Kind: "exclamation", MustNotMatch: `!`}},
				"en-GB": {{Dimension: DimInstruction, Kind: "exclamation", MustNotMatch: `!`}},
				"nb":    {{Dimension: DimInstruction, Kind: "exclamation", MustNotMatch: `!`}},
			},
		},
		{
			Key:    "alerts.count.new",
			Source: "You have three new alerts in your inbox.",
			Note:   "spelled-out number; the instruction mandates digits",
			Checks: map[string][]Check{
				"de":    {{Dimension: DimInstruction, Kind: "digits", MustMatch: `\b3\b`, MustNotMatch: `(?i)\bdrei\b`}},
				"fr":    {{Dimension: DimInstruction, Kind: "digits", MustMatch: `\b3\b`, MustNotMatch: `(?i)\btrois\b`}},
				"en-GB": {{Dimension: DimInstruction, Kind: "digits", MustMatch: `\b3\b`, MustNotMatch: `(?i)\bthree\b`}},
				"nb":    {{Dimension: DimInstruction, Kind: "digits", MustMatch: `\b3\b`, MustNotMatch: `(?i)(^|[^\p{L}])tre($|[^\p{L}])`}},
			},
		},
		{
			Key:    "fleet.category.color",
			Source: "Choose a color for each vessel category.",
			Note:   "en-GB spelling: colour. Both passes know the target is en-GB; the lift isolates what the explicit instruction adds",
			Checks: map[string][]Check{
				"en-GB": {{Dimension: DimInstruction, Kind: "spelling", MustMatch: `\bcolour\b`, MustNotMatch: `\bcolor\b`}},
			},
		},
		{
			Key:    "fleet.groups.hint",
			Source: "Organize your fleet into groups to center the map on what matters.",
			Checks: map[string][]Check{
				"en-GB": {
					{Dimension: DimInstruction, Kind: "spelling", MustMatch: `(?i)\borganis`, MustNotMatch: `(?i)\borganiz`},
					{Dimension: DimInstruction, Kind: "spelling", MustMatch: `\bcentre\b`, MustNotMatch: `\bcenter\b`},
				},
			},
		},

		// ---- Prose: several context types must hold at once ----
		{
			Key: "docs.getting_started.body",
			Source: "Getting started takes about five minutes. Sign in, open the dashboard, and Compass " +
				"will chart every vessel it can see. If anything looks off, you can reach us at any " +
				"time, and don't wait for the next tide!",
			Note: "prose where terminology, voice and instruction apply simultaneously",
			Checks: map[string][]Check{
				"de": {
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "dashboard", Replacement: "Leitstand"}},
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "vessel", Replacement: "Wasserfahrzeug"}},
					{Dimension: DimTerminology, Kind: "dnt", DNT: "Compass"},
					{Dimension: DimVoice, Kind: "formality", MustNotMatch: informalDE},
					{Dimension: DimInstruction, Kind: "digits", MustMatch: `\b5\b`, MustNotMatch: `(?i)\bfünf\b`},
					{Dimension: DimInstruction, Kind: "exclamation", MustNotMatch: `!`},
				},
				"fr": {
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "dashboard", Replacement: "poste de pilotage"}},
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "vessel", Replacement: "bâtiment"}},
					{Dimension: DimTerminology, Kind: "dnt", DNT: "Compass"},
					{Dimension: DimVoice, Kind: "formality", MustNotMatch: informalFR},
					{Dimension: DimInstruction, Kind: "digits", MustMatch: `\b5\b`, MustNotMatch: `(?i)\bcinq\b`},
					{Dimension: DimInstruction, Kind: "exclamation", MustNotMatch: `!`},
				},
				"en-GB": {
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "sign in", Replacement: "log on"}},
					{Dimension: DimVoice, Kind: "contractions", MustNotMatch: contractionRe},
					{Dimension: DimInstruction, Kind: "digits", MustMatch: `\b5\b`, MustNotMatch: `(?i)\bfive\b`},
					{Dimension: DimInstruction, Kind: "exclamation", MustNotMatch: `!`},
				},
				"nb": {
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "dashboard", Replacement: "styringspanel"}},
					{Dimension: DimTerminology, Kind: "term", Term: &coreprofile.TermRule{Term: "vessel", Replacement: "farkost"}},
					{Dimension: DimTerminology, Kind: "dnt", DNT: "Compass"},
					{Dimension: DimInstruction, Kind: "digits", MustMatch: `\b5\b`, MustNotMatch: `(?i)(^|[^\p{L}])fem($|[^\p{L}])`},
					{Dimension: DimInstruction, Kind: "exclamation", MustNotMatch: `!`},
				},
			},
		},
	}
}
