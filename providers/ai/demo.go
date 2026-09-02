package aiprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/neokapi/neokapi/core/icu"
	"github.com/neokapi/neokapi/core/model"
)

// DemoModelName is the value surfaced as the model in every demo response. It
// is intentionally explicit so that --json metadata never implies a real model.
const DemoModelName = "demo-stub"

// DemoNotice is the one-line, honest disclaimer printed to stderr the first
// time a demo provider is exercised in a process. The wording is deliberately
// unambiguous: the output is illustrative, deterministic, and not the product
// of a language model. Brand guidelines require this honesty (see #666).
const DemoNotice = "demo mode: illustrative output from a built-in stub — not a real language model."

// DemoProvider is a deterministic, offline LLMProvider used by the browser
// playground (and anywhere `--provider demo` is requested) so that AI commands
// produce illustrative output without API keys or network access.
//
// It is NOT a translation engine. It applies a small built-in lexicon for
// common UI words and a deterministic, visibly-marked transform for everything
// else, so the result is plausible-looking but obviously a stub. Quality-style
// schemas (checks, voice profile) deliberately return empty/neutral results rather
// than inventing findings.
type DemoProvider struct {
	config Config
}

// NewDemoProvider creates a demo LLM provider. The supplied config is accepted
// for interface symmetry; only its (optional) Model is used, and it never
// reaches a network.
func NewDemoProvider(cfg Config) *DemoProvider {
	return &DemoProvider{config: cfg}
}

// Compile-time check that DemoProvider implements StreamingLLMProvider.
var _ StreamingLLMProvider = (*DemoProvider)(nil)

func (p *DemoProvider) Name() ProviderID { return Demo }

// InputModalities — the keyless demo provider is text-only.
func (p *DemoProvider) InputModalities() []Modality { return nil }

func (p *DemoProvider) modelName() string {
	if p.config.Model != "" {
		return p.config.Model
	}
	return DemoModelName
}

// Translate produces a deterministic demo translation of the source text.
//
// It goes through StandardTranslate like every other provider, rather than
// short-circuiting on req.Source: that renders the real prompt and routes it
// through Chat, so `--provider demo --explain` previews exactly the prompt a
// paid provider would receive — with no API key, no network and no spend. The
// reported confidence stays 0: a stub has no real confidence.
func (p *DemoProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	noticeOnce()
	return StandardTranslate(ctx, p.Name(), p.Chat, req, 0)
}

// Chat returns a deterministic demo reply. When the message looks like a
// translation prompt (identified by its prompt.Meta, not by its wording) the
// user turn is translated; otherwise a short, clearly-labeled stub reply is
// returned.
func (p *DemoProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	noticeOnce()
	last := lastUserMessage(messages)
	content := demoChatReply(ctx, last)
	return &ChatResponse{
		Content: content,
		Model:   p.modelName(),
		Usage:   demoUsage(last, content),
	}, nil
}

// ChatStructured returns JSON conforming to the requested schema. The batch
// translation schema (emitted by translate) is honoured by parsing the
// numbered prompt and translating each segment. The voice inference
// schema (emitted by voice-infer) is honoured by deterministic surface
// heuristics over the corpus embedded in the prompt — a generative onboarding
// draft, clearly marked as illustrative. All other schemas get a neutral,
// schema-valid response (empty arrays / zero values) so that the `qa` and
// voice-check tools run without fabricating findings.
func (p *DemoProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	noticeOnce()
	userTurn := lastUserMessage(messages)

	var content string
	switch schema.Name {
	case "batch_translations":
		content = demoBatchTranslations(userTurn, demoTargetLocale(ctx))
	case "brand_voice_inference":
		content = demoVoiceInference(userTurn)
	default:
		content = neutralSchemaJSON(schema)
	}

	return &ChatResponse{
		Content: content,
		Model:   p.modelName(),
		Usage:   demoUsage(userTurn, content),
	}, nil
}

// demoTargetLocale reads the target locale from the prompt metadata the caller
// attached to ctx. The demo provider used to regex "to <locale>" out of the
// prompt text, which coupled it to the prompt's wording and broke silently
// whenever the prompt changed. It reads structured intent instead.
func demoTargetLocale(ctx context.Context) model.LocaleID {
	if m, ok := prompt.MetaFrom(ctx); ok {
		return model.LocaleID(m.Param("target_locale"))
	}
	return ""
}

// ChatStream implements StreamingLLMProvider by emitting the deterministic Chat
// result as a single content event followed by a done event.
func (p *DemoProvider) ChatStream(ctx context.Context, messages []Message, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	resp, err := p.Chat(ctx, messages)
	if err != nil {
		return nil, err
	}
	if onEvent != nil {
		onEvent(ChatStreamEvent{Type: StreamEventContent, Content: resp.Content})
		onEvent(ChatStreamEvent{Type: StreamEventDone, Usage: resp.Usage, Model: resp.Model})
	}
	return resp, nil
}

// ChatStructuredStream implements StreamingLLMProvider by delegating to
// ChatStructured and emitting a single content + done event.
func (p *DemoProvider) ChatStructuredStream(ctx context.Context, messages []Message, schema JSONSchema, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	resp, err := p.ChatStructured(ctx, messages, schema)
	if err != nil {
		return nil, err
	}
	if onEvent != nil {
		onEvent(ChatStreamEvent{Type: StreamEventContent, Content: resp.Content})
		onEvent(ChatStreamEvent{Type: StreamEventDone, Usage: resp.Usage, Model: resp.Model})
	}
	return resp, nil
}

func (p *DemoProvider) Close() error { return nil }

// ---------------------------------------------------------------------------
// Deterministic demo translation engine
// ---------------------------------------------------------------------------

// demoLexicon maps lowercase English UI words to per-language demo
// translations. The vocabulary is intentionally small and covers words that
// appear in typical UI fixtures (buttons, menus, status). Entries
// are real words so the output reads plausibly, but the marked transform on
// everything else keeps the result unmistakably a stub.
var demoLexicon = map[string]map[string]string{
	"fr": {
		"hello": "bonjour", "welcome": "bienvenue", "goodbye": "au revoir",
		"yes": "oui", "no": "non", "save": "enregistrer", "cancel": "annuler",
		"delete": "supprimer", "edit": "modifier", "settings": "paramètres",
		"file": "fichier", "open": "ouvrir", "close": "fermer", "new": "nouveau",
		"search": "rechercher", "help": "aide", "home": "accueil", "back": "retour",
		"next": "suivant", "previous": "précédent", "loading": "chargement",
		"error": "erreur", "warning": "avertissement", "success": "succès",
		"login": "connexion", "logout": "déconnexion", "username": "nom d'utilisateur",
		"password": "mot de passe", "email": "courriel", "name": "nom",
		"submit": "soumettre", "send": "envoyer", "the": "le", "and": "et",
		"or": "ou", "user": "utilisateur", "account": "compte", "language": "langue",
		"add": "ajouter", "remove": "retirer", "continue": "continuer", "done": "terminé",
	},
	"es": {
		"hello": "hola", "welcome": "bienvenido", "goodbye": "adiós",
		"yes": "sí", "no": "no", "save": "guardar", "cancel": "cancelar",
		"delete": "eliminar", "edit": "editar", "settings": "configuración",
		"file": "archivo", "open": "abrir", "close": "cerrar", "new": "nuevo",
		"search": "buscar", "help": "ayuda", "home": "inicio", "back": "atrás",
		"next": "siguiente", "previous": "anterior", "loading": "cargando",
		"error": "error", "warning": "advertencia", "success": "éxito",
		"login": "iniciar sesión", "logout": "cerrar sesión", "username": "usuario",
		"password": "contraseña", "email": "correo", "name": "nombre",
		"submit": "enviar", "send": "enviar", "the": "el", "and": "y",
		"or": "o", "user": "usuario", "account": "cuenta", "language": "idioma",
		"add": "añadir", "remove": "quitar", "continue": "continuar", "done": "hecho",
	},
	"de": {
		"hello": "hallo", "welcome": "willkommen", "goodbye": "auf wiedersehen",
		"yes": "ja", "no": "nein", "save": "speichern", "cancel": "abbrechen",
		"delete": "löschen", "edit": "bearbeiten", "settings": "einstellungen",
		"file": "datei", "open": "öffnen", "close": "schließen", "new": "neu",
		"search": "suchen", "help": "hilfe", "home": "startseite", "back": "zurück",
		"next": "weiter", "previous": "zurück", "loading": "laden",
		"error": "fehler", "warning": "warnung", "success": "erfolg",
		"login": "anmelden", "logout": "abmelden", "username": "benutzername",
		"password": "passwort", "email": "e-mail", "name": "name",
		"submit": "absenden", "send": "senden", "the": "der", "and": "und",
		"or": "oder", "user": "benutzer", "account": "konto", "language": "sprache",
		"add": "hinzufügen", "remove": "entfernen", "continue": "fortfahren", "done": "fertig",
	},
}

// demoBaseLang returns the lowercase base language subtag (e.g. "fr-FR" → "fr").
func demoBaseLang(loc model.LocaleID) string {
	s := strings.ToLower(string(loc))
	for i := range len(s) {
		if s[i] == '-' || s[i] == '_' {
			return s[:i]
		}
	}
	return s
}

// wordSplit splits the text between brace placeholders into tokens: whole tags
// (<...>), whole printf conversions (%s), the double-bracket `[[…]]` sentinels
// the AI translate tool masks do-not-translate spans with, runs of
// letters/digits (words), and everything else (punctuation/whitespace). Only
// words are transformed; every other token is emitted verbatim, so inline
// markup survives intact.
//
// Placeholders must be matched as single tokens, not left to the word class:
// `%d` would otherwise split into `%` and the word `d`, and the demo would
// cheerfully accent the inside of a placeholder — producing output that fails
// kapi's own placeholder check. The same holds for the sentinels: accenting the
// inside of one would break the tool's verbatim restore, so the demo must pass
// them through like any real model does. Alternation is leftmost-first, so the
// tag, conversion and sentinel forms are tried before the word and catch-all
// classes; the trailing single-character alternative guarantees every rune is
// emitted, since `{`, `%`, `[`, and `]` are excluded from the catch-all class
// (a greedy catch-all would otherwise swallow the `[[` opener from the
// preceding whitespace and split the sentinel) and would otherwise be dropped
// when they appear outside a placeholder.
//
// Braces are splitTokens' business, not this expression's: a brace placeholder
// can contain brace placeholders, which no RE2 pattern can express.
var wordSplit = regexp.MustCompile(`<[^>]*>|\[\[[^\[\]]*\]\]|%[a-zA-Z]|[\p{L}\p{N}]+|[^<{}%\[\]\p{L}\p{N}]+|[^\p{L}\p{N}]`)

// splitTokens splits source into the tokens demoTranslate walks: every balanced
// brace group is one opaque token, and the text around those groups is split by
// wordSplit.
//
// A brace group is opaque whatever it holds. `{0}` and `{{name}}` are
// interpolation the host program fills in, and `{count, plural, one {# berth}
// other {# berths}}` is an ICU picker whose argument name, keyword and category
// keywords are program syntax — marking a word inside either produces a string
// the program cannot format. What a real model is handed for these spans is a
// mask; what the demo does with them is nothing at all.
func splitTokens(source string) []string {
	var out []string
	last := 0
	for _, sp := range icu.Spans(source) {
		out = append(out, wordSplit.FindAllString(source[last:sp.Start], -1)...)
		out = append(out, source[sp.Start:sp.End])
		last = sp.End
	}
	return append(out, wordSplit.FindAllString(source[last:], -1)...)
}

// isWord reports whether tok is a run of letters/digits (vs punctuation/space).
var wordRe = regexp.MustCompile(`^[\p{L}\p{N}]+$`)

// demoTranslate deterministically maps source text into a marked demo
// translation for the target locale. Known words use the lexicon; unknown
// words get a visible per-language accent marker so the output is plausible
// yet obviously synthetic. The whole string is wrapped so no reader could
// mistake it for a real translation.
func demoTranslate(source string, target model.LocaleID) string {
	lang := demoBaseLang(target)
	lex := demoLexicon[lang]

	var b strings.Builder
	for _, tok := range splitTokens(source) {
		if !wordRe.MatchString(tok) {
			b.WriteString(tok) // preserve punctuation / whitespace / markup verbatim
			continue
		}
		if lex != nil {
			if t, ok := lex[strings.ToLower(tok)]; ok {
				b.WriteString(matchCase(tok, t))
				continue
			}
		}
		b.WriteString(markWord(tok, lang))
	}

	body := b.String()
	if lang == "" {
		// Unknown target language: still produce a clearly-labeled stub.
		return "⟦demo:" + string(target) + "⟧ " + body
	}
	return "⟦" + lang + "⟧ " + body
}

// markWord applies a small, deterministic per-language suffix to an unknown
// word so the output looks language-flavoured while remaining obviously a
// stub. The mark is reversible-looking but purely cosmetic.
func markWord(word, lang string) string {
	switch lang {
	case "fr":
		return word + "é"
	case "es":
		return word + "o"
	case "de":
		return word + "en"
	default:
		return word + "~"
	}
}

// matchCase copies the capitalization pattern of src onto repl (handles the
// common Title and UPPER cases; otherwise returns repl unchanged).
func matchCase(src, repl string) string {
	if src == "" || repl == "" {
		return repl
	}
	if src == strings.ToUpper(src) && src != strings.ToLower(src) {
		return strings.ToUpper(repl)
	}
	// Title case: first rune upper, rest not all-upper.
	first := src[:1]
	if first == strings.ToUpper(first) && first != strings.ToLower(first) {
		return strings.ToUpper(repl[:1]) + repl[1:]
	}
	return repl
}

// demoBatchTranslations decodes the batch prompt's JSON payload, translates each
// segment, and returns JSON matching the batch_translations schema — echoing the
// id it was given, because that is the contract the real providers are held to
// and the demo must be held to the same one.
//
// This used to regex "[N] text" lines out of the prompt. Parsing the payload
// means the demo breaks loudly if the payload shape changes, rather than quietly
// returning nothing.
func demoBatchTranslations(userTurn string, target model.LocaleID) string {
	var payload struct {
		Segments []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"segments"`
	}
	if err := json.Unmarshal([]byte(userTurn), &payload); err != nil {
		return `{"translations":[]}`
	}

	type entry struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	var out struct {
		Translations []entry `json:"translations"`
	}
	for _, seg := range payload.Segments {
		out.Translations = append(out.Translations, entry{
			ID:   seg.ID,
			Text: demoTranslate(strings.TrimSpace(seg.Text), target),
		})
	}

	b, err := json.Marshal(out)
	if err != nil {
		return `{"translations":[]}`
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// Deterministic demo voice inference
// ---------------------------------------------------------------------------

// demoContractionRe matches common English contractions ("we're", "don't").
var demoContractionRe = regexp.MustCompile(`(?i)\b[a-z]+'(?:t|s|re|ve|ll|d|m)\b`)

// demoPassiveRe approximates passive-voice constructions ("was processed").
var demoPassiveRe = regexp.MustCompile(`(?i)\b(?:was|were|is|are|been|being|be)\s+\w+ed\b`)

// demoCapTermRe matches capitalized terms of three or more characters —
// product-name candidates for the preferred-terms heuristic.
var demoCapTermRe = regexp.MustCompile(`\b[A-Z][A-Za-z0-9]{2,}\b`)

// demoSentenceSplitRe splits a corpus into sentences on terminal punctuation.
var demoSentenceSplitRe = regexp.MustCompile(`[.!?]+`)

// demoWeRe / demoYouRe count first-person-plural and second-person pronouns
// for the point-of-view heuristic.
var (
	demoWeRe  = regexp.MustCompile(`(?i)\b(?:we|our|us)\b`)
	demoYouRe = regexp.MustCompile(`(?i)\b(?:you|your)\b`)
)

// demoCapStopwords excludes ordinary sentence-initial title-case words from
// the capitalized-term heuristic.
var demoCapStopwords = map[string]bool{
	"The": true, "This": true, "That": true, "These": true, "Those": true,
	"And": true, "But": true, "You": true, "Your": true, "Our": true,
	"With": true, "For": true, "When": true, "What": true, "How": true,
	"Not": true, "All": true, "Are": true, "Was": true, "Will": true,
	"Can": true, "It's": true, "Its": true, "They": true, "There": true,
	"Here": true, "Then": true, "Now": true, "Get": true, "Try": true,
}

// demoVoiceInference derives an illustrative draft voice profile
// from the corpus embedded in an inference prompt (the text after the
// "Corpus:" delimiter line that voice-infer emits), using deterministic
// surface heuristics: contraction, exclamation, pronoun, passive-voice, and
// sentence-length counts, plus recurring capitalized terms. Like the demo
// translator it is honest about being a stub — the guidelines and every
// evidence note carry the ⟦demo⟧ marker. Returns JSON matching the
// brand_voice_inference schema.
func demoVoiceInference(userTurn string) string {
	corpus := userTurn
	if idx := strings.LastIndex(userTurn, prompt.CorpusDelimiter); idx >= 0 {
		corpus = userTurn[idx+len(prompt.CorpusDelimiter):]
	}
	corpus = strings.TrimSpace(corpus)

	words := strings.Fields(corpus)
	var sentences []string
	for _, s := range demoSentenceSplitRe.Split(corpus, -1) {
		if s = strings.TrimSpace(s); s != "" {
			sentences = append(sentences, s)
		}
	}
	sentenceCount := max(len(sentences), 1)
	avgWords := float64(len(words)) / float64(sentenceCount)

	contractions := len(demoContractionRe.FindAllString(corpus, -1))
	exclamations := strings.Count(corpus, "!")
	passives := len(demoPassiveRe.FindAllString(corpus, -1))
	weCount := len(demoWeRe.FindAllString(corpus, -1))
	youCount := len(demoYouRe.FindAllString(corpus, -1))

	// Tone.
	var personality []string
	if contractions > 0 || exclamations > 0 {
		personality = append(personality, "friendly")
	}
	if avgWords <= 12 {
		personality = append(personality, "direct")
	}
	if weCount > 0 {
		personality = append(personality, "collaborative")
	}
	if len(personality) == 0 {
		personality = []string{"measured"}
	}
	formality := "neutral"
	if contractions > 0 || exclamations > 0 {
		formality = "casual"
	}
	emotion := "neutral"
	if exclamations > 0 {
		emotion = "warm"
	}

	// Style.
	sentenceLength := "varied"
	switch {
	case avgWords < 12:
		sentenceLength = "short"
	case avgWords < 20:
		sentenceLength = "medium"
	}
	personPOV := "third"
	switch {
	case weCount > 0 && weCount >= youCount:
		personPOV = "first_plural"
	case youCount > 0:
		personPOV = "second"
	}
	contractionUse := "never"
	switch {
	case contractions >= sentenceCount:
		contractionUse = "always"
	case contractions > 0:
		contractionUse = "sometimes"
	}
	activeVoice := passives == 0 || passives*4 < sentenceCount

	// Vocabulary: recurring capitalized terms, most frequent first (ties
	// alphabetical) for a deterministic order.
	capCounts := map[string]int{}
	for _, m := range demoCapTermRe.FindAllString(corpus, -1) {
		if !demoCapStopwords[m] {
			capCounts[m]++
		}
	}
	var capTerms []string
	for term, n := range capCounts {
		if n >= 2 {
			capTerms = append(capTerms, term)
		}
	}
	sort.Slice(capTerms, func(i, j int) bool {
		if capCounts[capTerms[i]] != capCounts[capTerms[j]] {
			return capCounts[capTerms[i]] > capCounts[capTerms[j]]
		}
		return capTerms[i] < capTerms[j]
	})
	if len(capTerms) > 5 {
		capTerms = capTerms[:5]
	}
	preferred := make([]map[string]any, 0, len(capTerms))
	for _, term := range capTerms {
		preferred = append(preferred, map[string]any{
			"term":        term,
			"replacement": "",
			"note":        fmt.Sprintf("⟦demo⟧ appears %d times in the corpus", capCounts[term]),
		})
	}

	// Examples: contrast the first corpus sentence (on-voice) with a hedged,
	// off-voice rewrite of itself.
	examples := make([]map[string]any, 0, 1)
	if len(sentences) > 0 {
		first := sentences[0]
		lowered := first
		if r := []rune(first); len(r) > 0 {
			lowered = strings.ToLower(string(r[0])) + string(r[1:])
		}
		examples = append(examples, map[string]any{
			"before":      "It should be noted that " + lowered + ".",
			"after":       first + ".",
			"explanation": "⟦demo⟧ the corpus favors direct sentences over hedged openers",
		})
	}

	// Confidence grows with corpus size, clamped to a modest ceiling: a stub
	// never reports high certainty.
	conf := 0.2 + float64(len(words))/400
	if conf > 0.9 {
		conf = 0.9
	}
	evidence := []map[string]any{
		{"field": "tone", "confidence": conf, "source": fmt.Sprintf("⟦demo⟧ %d exclamation marks and %d contractions across %d sentences", exclamations, contractions, sentenceCount)},
		{"field": "style", "confidence": conf, "source": fmt.Sprintf("⟦demo⟧ average sentence length %.1f words; pronoun counts we=%d you=%d; %d passive constructions", avgWords, weCount, youCount, passives)},
		{"field": "vocabulary", "confidence": conf, "source": fmt.Sprintf("⟦demo⟧ %d recurring capitalized terms", len(preferred))},
		{"field": "examples", "confidence": conf, "source": "⟦demo⟧ representative sentence drawn from the corpus"},
	}

	result := map[string]any{
		"tone": map[string]any{
			"personality": personality,
			"formality":   formality,
			"emotion":     emotion,
			"humor":       "none",
			"guidelines":  fmt.Sprintf("⟦demo⟧ illustrative draft inferred from %d words across %d sentences", len(words), sentenceCount),
		},
		"style": map[string]any{
			"active_voice":        activeVoice,
			"sentence_length":     sentenceLength,
			"person_pov":          personPOV,
			"contractions":        contractionUse,
			"prohibited_patterns": []any{},
		},
		"vocabulary": map[string]any{
			"preferred":  preferred,
			"forbidden":  []any{},
			"competitor": []any{},
		},
		"examples": examples,
		"evidence": evidence,
	}
	b, err := json.Marshal(result)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// demoChatReply translates the trailing text of a translation-style prompt, or
// returns a short labeled stub for anything else.
func demoChatReply(ctx context.Context, userTurn string) string {
	// A translate prompt puts the content to translate — and nothing else — in
	// the user turn, and identifies itself via prompt.Meta. Anything else gets a
	// labeled stub.
	if m, ok := prompt.MetaFrom(ctx); ok && m.ID == prompt.IDTranslateSingle {
		return demoTranslate(strings.TrimSpace(userTurn), demoTargetLocale(ctx))
	}
	return "⟦demo⟧ illustrative stub response (no real language model)"
}

// neutralSchemaJSON builds a minimal schema-valid JSON document: arrays become
// empty arrays, strings empty, numbers zero, booleans false, objects recursed.
// Used for check / voice style schemas where inventing findings would be
// dishonest.
func neutralSchemaJSON(s JSONSchema) string {
	v := neutralValue(s.Schema)
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func neutralValue(node map[string]any) any {
	if node == nil {
		return map[string]any{}
	}
	switch node["type"] {
	case "array":
		return []any{}
	case "string":
		return ""
	case "integer", "number":
		return 0
	case "boolean":
		return false
	case "object":
		result := map[string]any{}
		if props, ok := node["properties"].(map[string]any); ok {
			for key, prop := range props {
				if pm, ok := prop.(map[string]any); ok {
					result[key] = neutralValue(pm)
				} else {
					result[key] = nil
				}
			}
		}
		return result
	default:
		return map[string]any{}
	}
}

// lastUserMessage returns the content of the last user message, or the last
// message of any role if no user message is present.
func lastUserMessage(messages []Message) string {
	last := ""
	for _, m := range messages {
		if m.Role == "user" {
			last = m.Text()
		}
	}
	if last == "" && len(messages) > 0 {
		last = messages[len(messages)-1].Text()
	}
	return last
}

// demoUsage returns a deterministic, clearly-synthetic token count derived from
// input/output length. It is illustrative only; real usage is never reported by
// a stub.
func demoUsage(in, out string) TokenUsage {
	return TokenUsage{
		InputTokens:  len(strings.Fields(in)),
		OutputTokens: len(strings.Fields(out)),
	}
}

// demoNoticeWriter is where the one-time demo notice is printed. It defaults to
// os.Stderr; the wasm wiring (and tests) may swap it. Access is unsynchronised
// and expected to be set once at startup before any provider runs.
var demoNoticeWriter io.Writer = os.Stderr

// noticeOnce prints DemoNotice to demoNoticeWriter the first time it is called.
var noticeOnce = sync.OnceFunc(func() {
	_, _ = io.WriteString(demoNoticeWriter, DemoNotice+"\n")
})

// SetDemoNoticeWriter overrides where the one-time demo notice is written. Pass
// io.Discard to suppress it. Intended for the wasm entrypoint and tests.
func SetDemoNoticeWriter(w io.Writer) {
	if w != nil {
		demoNoticeWriter = w
	}
}
