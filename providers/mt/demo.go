package mtprovider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/model"
)

// Demo is the provider id for the deterministic, offline demo MT provider.
const Demo ProviderID = "demo"

// DemoNotice is the one-line, honest disclaimer printed to stderr the first
// time the demo MT provider is exercised in a process. Brand guidelines
// require the output be unmistakably illustrative, not a real engine (#666).
const DemoNotice = "demo mode: illustrative output from a built-in stub — not a real machine-translation engine."

// DemoProvider is a deterministic, offline MTProvider used by the browser
// playground (and anywhere the demo provider is requested) so that MT commands
// produce illustrative output without API keys or network access.
//
// It is NOT a translation engine: known UI words come from a small built-in
// lexicon and everything else gets a visible per-language mark, so output looks
// plausible but is obviously synthetic.
type DemoProvider struct{}

// NewDemoProvider creates a demo MT provider.
func NewDemoProvider() *DemoProvider { return &DemoProvider{} }

func (p *DemoProvider) Name() ProviderID { return Demo }

// Translate produces a deterministic demo translation of the source text.
func (p *DemoProvider) Translate(_ context.Context, req TranslateRequest) (*TranslateResponse, error) {
	noticeOnce()
	return &TranslateResponse{Translation: demoTranslate(req.Source, req.TargetLocale)}, nil
}

func (p *DemoProvider) Close() error { return nil }

// DemoToolConfig holds configuration for the demo MT tool. It carries no
// credentials — the whole point of the demo provider is to run with none.
type DemoToolConfig struct {
	SourceLocale model.LocaleID `schema:"description=Source locale of the content"`
	TargetLocale model.LocaleID `schema:"description=Target locale for processing"`
}

// ToolName returns the tool name this config applies to.
func (c *DemoToolConfig) ToolName() string { return "demo-translate" }

// Reset restores default values.
func (c *DemoToolConfig) Reset() {
	c.SourceLocale = ""
	c.TargetLocale = ""
}

// Validate checks configuration validity.
func (c *DemoToolConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("demo: TargetLocale is required")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Provider registry (mirrors the AI provider registry)
// ---------------------------------------------------------------------------

// ProviderFactory creates an MTProvider from a generic config map. Only the
// demo provider is registered by default; real providers are constructed with
// their typed configs (e.g. NewDeepLProvider) because they require credentials.
type ProviderFactory func() MTProvider

var (
	providerMu sync.RWMutex
	providers  = map[ProviderID]ProviderFactory{}
)

// RegisterProvider registers a credential-free MT provider factory by id.
func RegisterProvider(id ProviderID, factory ProviderFactory) {
	providerMu.Lock()
	defer providerMu.Unlock()
	providers[id] = factory
}

// NewProvider returns a credential-free MT provider by id, or an error if the
// id is not registered. Real providers requiring credentials are not registered
// here and must be constructed directly with their typed configs.
func NewProvider(id ProviderID) (MTProvider, error) {
	providerMu.RLock()
	defer providerMu.RUnlock()
	if f, ok := providers[id]; ok {
		return f(), nil
	}
	return nil, fmt.Errorf("unknown MT provider: %s", id)
}

func init() {
	RegisterProvider(Demo, func() MTProvider { return NewDemoProvider() })
}

// ---------------------------------------------------------------------------
// Deterministic demo translation engine
// ---------------------------------------------------------------------------

// demoLexicon maps lowercase English UI words to per-language demo
// translations. Intentionally small; covers words common in localization
// fixtures so the demo reads plausibly.
var demoLexicon = map[string]map[string]string{
	"fr": {
		"hello": "bonjour", "welcome": "bienvenue", "goodbye": "au revoir",
		"yes": "oui", "no": "non", "save": "enregistrer", "cancel": "annuler",
		"delete": "supprimer", "edit": "modifier", "settings": "paramètres",
		"file": "fichier", "open": "ouvrir", "close": "fermer", "new": "nouveau",
		"search": "rechercher", "help": "aide", "home": "accueil", "back": "retour",
		"next": "suivant", "previous": "précédent", "loading": "chargement",
		"error": "erreur", "warning": "avertissement", "success": "succès",
		"login": "connexion", "logout": "déconnexion", "name": "nom",
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
		"login": "iniciar sesión", "logout": "cerrar sesión", "name": "nombre",
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
		"login": "anmelden", "logout": "abmelden", "name": "name",
		"submit": "absenden", "send": "senden", "the": "der", "and": "und",
		"or": "oder", "user": "benutzer", "account": "konto", "language": "sprache",
		"add": "hinzufügen", "remove": "entfernen", "continue": "fortfahren", "done": "fertig",
	},
}

func demoBaseLang(loc model.LocaleID) string {
	s := strings.ToLower(string(loc))
	for i := 0; i < len(s); i++ {
		if s[i] == '-' || s[i] == '_' {
			return s[:i]
		}
	}
	return s
}

// wordSplit splits text into alternating word / non-word tokens so punctuation,
// whitespace, and HTML tags are preserved exactly.
var wordSplit = regexp.MustCompile(`<[^>]*>|[\p{L}\p{N}]+|[^<\p{L}\p{N}]+`)

var wordRe = regexp.MustCompile(`^[\p{L}\p{N}]+$`)

// demoTranslate deterministically maps source text into a marked demo
// translation for the target locale. The whole string is wrapped so no reader
// could mistake it for a real translation. HTML tags pass through unchanged so
// inline-code blocks round-trip.
func demoTranslate(source string, target model.LocaleID) string {
	lang := demoBaseLang(target)
	lex := demoLexicon[lang]

	var b strings.Builder
	for _, tok := range wordSplit.FindAllString(source, -1) {
		if !wordRe.MatchString(tok) {
			b.WriteString(tok)
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
		return "⟦demo:" + string(target) + "⟧ " + body
	}
	return "⟦" + lang + "⟧ " + body
}

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

func matchCase(src, repl string) string {
	if src == "" || repl == "" {
		return repl
	}
	if src == strings.ToUpper(src) && src != strings.ToLower(src) {
		return strings.ToUpper(repl)
	}
	first := src[:1]
	if first == strings.ToUpper(first) && first != strings.ToLower(first) {
		return strings.ToUpper(repl[:1]) + repl[1:]
	}
	return repl
}

// ---------------------------------------------------------------------------
// One-time honesty notice
// ---------------------------------------------------------------------------

var (
	demoNoticeWriter io.Writer = os.Stderr
	demoNoticeOnce   sync.Once
)

// SetDemoNoticeWriter overrides where the one-time demo notice is written. Pass
// io.Discard to suppress it. Intended for the wasm entrypoint and tests.
func SetDemoNoticeWriter(w io.Writer) {
	if w != nil {
		demoNoticeWriter = w
	}
}

func noticeOnce() {
	demoNoticeOnce.Do(func() {
		_, _ = io.WriteString(demoNoticeWriter, DemoNotice+"\n")
	})
}
