package aiprovider

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

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
// schemas (QA, brand voice) deliberately return empty/neutral results rather
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

func (p *DemoProvider) modelName() string {
	if p.config.Model != "" {
		return p.config.Model
	}
	return DemoModelName
}

// Translate produces a deterministic demo translation of the source text.
func (p *DemoProvider) Translate(_ context.Context, req TranslateRequest) (*TranslateResponse, error) {
	noticeOnce()
	out := demoTranslate(req.Source, req.TargetLocale)
	return &TranslateResponse{
		Translation: out,
		Confidence:  0, // honest: a stub has no real confidence
		Model:       p.modelName(),
		Usage:       demoUsage(req.Source, out),
	}, nil
}

// Chat returns a deterministic demo reply. When the message looks like a
// translation instruction (the shape ai-translate emits for single blocks and
// inline-code blocks) the trailing text is translated; otherwise a short,
// clearly-labeled stub reply is returned.
func (p *DemoProvider) Chat(_ context.Context, messages []Message) (*ChatResponse, error) {
	noticeOnce()
	last := lastUserMessage(messages)
	content := demoChatReply(last)
	return &ChatResponse{
		Content: content,
		Model:   p.modelName(),
		Usage:   demoUsage(last, content),
	}, nil
}

// ChatStructured returns JSON conforming to the requested schema. The batch
// translation schema (emitted by ai-translate) is honoured by parsing the
// numbered prompt and translating each segment. All other schemas get a
// neutral, schema-valid response (empty arrays / zero values) so that QA and
// brand-voice tools run without fabricating findings.
func (p *DemoProvider) ChatStructured(_ context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	noticeOnce()
	prompt := lastUserMessage(messages)

	var content string
	switch schema.Name {
	case "batch_translations":
		content = demoBatchTranslations(prompt)
	default:
		content = neutralSchemaJSON(schema)
	}

	return &ChatResponse{
		Content: content,
		Model:   p.modelName(),
		Usage:   demoUsage(prompt, content),
	}, nil
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
// appear in typical localization fixtures (buttons, menus, status). Entries
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

// wordSplit splits text into alternating tokens: whole tags (<...>), runs of
// letters/digits (words), and everything else (punctuation/whitespace). Tags
// and non-word tokens are emitted verbatim so inline markup, placeholders, and
// structure survive the demo transform intact.
var wordSplit = regexp.MustCompile(`<[^>]*>|[\p{L}\p{N}]+|[^<\p{L}\p{N}]+`)

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
	for _, tok := range wordSplit.FindAllString(source, -1) {
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

// segmentRe matches the numbered segments ai-translate emits in its batch
// prompt: lines of the form "[N] text".
var segmentRe = regexp.MustCompile(`(?m)^\[(\d+)\]\s?(.*)$`)

// demoBatchTranslations parses the numbered batch-translation prompt, finds the
// target locale, translates each segment, and returns JSON matching the
// batch_translations schema.
func demoBatchTranslations(prompt string) string {
	target := targetFromPrompt(prompt)

	type entry struct {
		Index int    `json:"index"`
		Text  string `json:"text"`
	}
	var out struct {
		Translations []entry `json:"translations"`
	}

	for _, m := range segmentRe.FindAllStringSubmatch(prompt, -1) {
		idx, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		out.Translations = append(out.Translations, entry{
			Index: idx,
			Text:  demoTranslate(strings.TrimSpace(m[2]), target),
		})
	}

	b, err := json.Marshal(out)
	if err != nil {
		return `{"translations":[]}`
	}
	return string(b)
}

// targetRe extracts the "to <locale>" target hint from the prompts that
// ai-translate constructs (e.g. "Translate ... from en to fr-FR.").
var targetRe = regexp.MustCompile(`(?i)\bto\s+([A-Za-z]{2,3}(?:[-_][A-Za-z0-9]+)?)`)

// targetFromPrompt best-effort extracts the target locale from a translation
// prompt. Falls back to empty (which still yields a labeled stub).
func targetFromPrompt(prompt string) model.LocaleID {
	if m := targetRe.FindStringSubmatch(prompt); m != nil {
		return model.LocaleID(m[1])
	}
	return ""
}

// demoChatReply translates the trailing text of a translation-style prompt, or
// returns a short labeled stub for anything else.
func demoChatReply(prompt string) string {
	target := targetFromPrompt(prompt)
	// ai-translate's single-block prompt ends with "Text: <source>"; the
	// inline-codes prompt puts the source on the final non-empty line.
	if idx := strings.LastIndex(prompt, "Text: "); idx >= 0 {
		return demoTranslate(strings.TrimSpace(prompt[idx+len("Text: "):]), target)
	}
	lines := strings.Split(strings.TrimRight(prompt, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return demoTranslate(s, target)
		}
	}
	return "⟦demo⟧ illustrative stub response (no real language model)"
}

// neutralSchemaJSON builds a minimal schema-valid JSON document: arrays become
// empty arrays, strings empty, numbers zero, booleans false, objects recursed.
// Used for QA / brand-voice style schemas where inventing findings would be
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
			last = m.Content
		}
	}
	if last == "" && len(messages) > 0 {
		last = messages[len(messages)-1].Content
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
