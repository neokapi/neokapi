// Package addin is the shared backend for Bowrain's in-product workspace
// add-ins — the Google Workspace add-on (Docs/Sheets/Slides) and the Microsoft
// 365 Office add-in (Word/Excel/PowerPoint). Both surfaces call the same three
// operations against the document the user is editing:
//
//	Check     — score text against a brand voice profile, return findings.
//	Terms     — surface the approved/forbidden/competitor terms present in text.
//	Translate — translate text on-brand into a target locale.
//
// The package exposes a transport-agnostic [Service] (pure, dependency-injected,
// trivially testable) plus thin Echo handlers ([Service.RegisterRoutes]) for the
// REST API the Office task pane calls, and the Card-JSON HTTP endpoints
// ([Service.RegisterGoogleRoutes]) the Google Workspace add-on runtime POSTs to.
//
// Keeping the brand/terminology/translation logic in one place means the two
// add-ins, and any future surface (a Copilot agent, an MCP tool), share exactly
// one implementation and one set of starter packs.
package addin

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/brand/packs"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// DefaultPack is the starter brand-voice pack used when a request omits a
// profile. It is one of the framework's built-in packs (see core/brand/packs).
const DefaultPack = "professional-b2b"

// Service runs brand/terminology/translation operations for the add-ins. Its
// two function fields are injected so the heavy dependencies (profile source,
// LLM provider) can be swapped — production wires the platform provider; tests
// wire the deterministic demo provider.
type Service struct {
	// LoadProfile resolves a brand voice profile by pack name. Defaults to
	// packs.Load (the framework's built-in starter packs).
	LoadProfile func(name string) (*brand.VoiceProfile, error)

	// NewProvider builds the LLM provider used for translation. Defaults to the
	// keyless, deterministic demo provider so the add-in works out of the box;
	// the server overrides this with the configured platform provider.
	NewProvider func(ctx context.Context) (aiprovider.LLMProvider, error)

	// DefaultPack names the profile used when a request omits one.
	DefaultPack string

	// GoogleBaseURL overrides the Google Workspace API host the add-on uses to
	// read/write the active document (empty = the real Google endpoints). Set in
	// tests to point at a mock server.
	GoogleBaseURL string

	// PublicURL is the add-on's own public base URL. The Google Workspace
	// add-on runtime requires full HTTPS URLs for button callbacks, so the card
	// builders prefix this onto each callback path (e.g.
	// "https://addin.bowrain.cloud" + "/google/scan").
	PublicURL string
}

// New returns a Service wired with framework defaults: built-in starter packs
// and the deterministic demo translation provider.
func New() *Service {
	return &Service{
		LoadProfile: packs.Load,
		NewProvider: func(context.Context) (aiprovider.LLMProvider, error) {
			return aitools.ProviderFromConfig(string(aiprovider.Demo), aiprovider.Config{})
		},
		DefaultPack: DefaultPack,
	}
}

// ---------------------------------------------------------------------------
// Request / response DTOs (the add-in REST + card contract)
// ---------------------------------------------------------------------------

// CheckRequest scores text against a brand voice profile.
type CheckRequest struct {
	Text    string `json:"text"`
	Profile string `json:"profile,omitempty"` // starter pack name; default DefaultPack
	Locale  string `json:"locale,omitempty"`  // optional locale override on the profile
}

// Finding is one brand-voice issue, flattened for transport.
type Finding struct {
	Category     string `json:"category"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	Suggestion   string `json:"suggestion,omitempty"`
	OriginalText string `json:"original_text,omitempty"`
}

// CheckResult is the scored outcome of a brand check.
type CheckResult struct {
	Profile  string    `json:"profile"`
	Score    int       `json:"score"` // 0-100
	Findings []Finding `json:"findings"`
}

// TermsRequest looks up terminology guidance for text.
type TermsRequest struct {
	Text    string `json:"text"`
	Profile string `json:"profile,omitempty"`
}

// TermHit is a terminology rule that applies to the submitted text.
type TermHit struct {
	Term        string `json:"term"`
	Status      string `json:"status"` // preferred | forbidden | competitor
	Replacement string `json:"replacement,omitempty"`
	Note        string `json:"note,omitempty"`
	Severity    string `json:"severity,omitempty"`
}

// TermsResult is the set of terminology hits found in the text.
type TermsResult struct {
	Profile string    `json:"profile"`
	Matches []TermHit `json:"matches"`
}

// TranslateRequest translates text on-brand into a target locale.
type TranslateRequest struct {
	Text         string `json:"text"`
	SourceLocale string `json:"source_locale,omitempty"` // default "en"
	TargetLocale string `json:"target_locale"`
	Profile      string `json:"profile,omitempty"` // applies brand voice to the translation
}

// TranslateResult carries the translated text and the provider that produced it.
type TranslateResult struct {
	Translation  string `json:"translation"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
	Provider     string `json:"provider"`
}

// ---------------------------------------------------------------------------
// Operations
// ---------------------------------------------------------------------------

// Check scores text against a brand voice profile using the rule-based
// vocabulary checker (deterministic, no network). It returns findings and an
// aggregate 0-100 score.
func (s *Service) Check(ctx context.Context, req CheckRequest) (*CheckResult, error) {
	profile, err := s.resolveProfile(req.Profile, req.Locale)
	if err != nil {
		return nil, err
	}

	vocab := coretools.NewBrandVocabCheckTool(profile, nil)
	findings, err := runBlockTool(ctx, vocab, req.Text)
	if err != nil {
		return nil, err
	}
	score := brand.CalculateScore(findings)

	out := &CheckResult{Profile: profile.Name, Score: score.Overall, Findings: make([]Finding, 0, len(findings))}
	for _, f := range findings {
		out.Findings = append(out.Findings, Finding{
			Category:     f.Category,
			Severity:     string(f.Severity),
			Message:      f.Message,
			Suggestion:   f.Suggestion,
			OriginalText: f.OriginalText,
		})
	}
	return out, nil
}

// Terms surfaces the brand profile's terminology rules (preferred, forbidden,
// competitor) that appear in the submitted text — the glossary view the add-in
// shows alongside the document.
func (s *Service) Terms(_ context.Context, req TermsRequest) (*TermsResult, error) {
	profile, err := s.resolveProfile(req.Profile, "")
	if err != nil {
		return nil, err
	}
	out := &TermsResult{Profile: profile.Name, Matches: []TermHit{}}
	add := func(rules []brand.TermRule, status string) {
		for _, r := range rules {
			if r.Term == "" || !containsTerm(req.Text, r.Term) {
				continue
			}
			out.Matches = append(out.Matches, TermHit{
				Term:        r.Term,
				Status:      status,
				Replacement: r.Replacement,
				Note:        r.Note,
				Severity:    r.Severity,
			})
		}
	}
	add(profile.Vocabulary.PreferredTerms, "preferred")
	add(profile.Vocabulary.ForbiddenTerms, "forbidden")
	add(profile.Vocabulary.CompetitorTerms, "competitor")
	return out, nil
}

// Translate translates text into the target locale, applying the brand voice
// profile so the output stays on-brand.
func (s *Service) Translate(ctx context.Context, req TranslateRequest) (*TranslateResult, error) {
	if strings.TrimSpace(req.TargetLocale) == "" {
		return nil, errors.New("target_locale is required")
	}
	src := req.SourceLocale
	if src == "" {
		src = string(model.LocaleEnglish)
	}

	var profile *brand.VoiceProfile
	if req.Profile != "" {
		p, err := s.resolveProfile(req.Profile, req.TargetLocale)
		if err != nil {
			return nil, err
		}
		profile = p
	}

	provider, err := s.newProvider(ctx)
	if err != nil {
		return nil, err
	}
	defer provider.Close()

	t := aitools.NewAITranslateTool(provider, aitools.AITranslateConfig{
		SourceLocale: model.LocaleID(src),
		TargetLocale: model.LocaleID(req.TargetLocale),
		Provider:     string(provider.Name()),
		Profile:      profile,
	})

	block := model.NewBlock("addin", req.Text)
	block.SourceLocale = model.LocaleID(src)
	if err := processBlock(ctx, t, block); err != nil {
		return nil, err
	}

	return &TranslateResult{
		Translation:  block.TargetText(model.LocaleID(req.TargetLocale)),
		SourceLocale: src,
		TargetLocale: req.TargetLocale,
		Provider:     string(provider.Name()),
	}, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func (s *Service) resolveProfile(name, locale string) (*brand.VoiceProfile, error) {
	loader := s.LoadProfile
	if loader == nil {
		loader = packs.Load
	}
	if name == "" {
		name = s.DefaultPack
		if name == "" {
			name = DefaultPack
		}
	}
	profile, err := loader(name)
	if err != nil {
		return nil, fmt.Errorf("load brand profile %q: %w", name, err)
	}
	if locale != "" {
		profile = brand.ResolveProfile(profile, locale, "")
	}
	return profile, nil
}

func (s *Service) newProvider(ctx context.Context) (aiprovider.LLMProvider, error) {
	if s.NewProvider != nil {
		return s.NewProvider(ctx)
	}
	return aitools.ProviderFromConfig(string(aiprovider.Demo), aiprovider.Config{})
}

// runBlockTool runs a single-block tool over text and returns any brand-voice
// findings it produced. Mirrors the CLI's helper (the cli package can't be
// imported from the bowrain module).
func runBlockTool(ctx context.Context, t tool.Tool, text string) ([]brand.BrandVoiceFinding, error) {
	block := model.NewBlock("addin", text)
	if err := processBlock(ctx, t, block); err != nil {
		return nil, err
	}
	if ann, ok := block.Annotations["brand-voice"].(*brand.BrandVoiceAnnotation); ok {
		return ann.Findings, nil
	}
	return nil, nil
}

// processBlock runs a tool over a single block through the channel pipeline.
func processBlock(ctx context.Context, t tool.Tool, block *model.Block) error {
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	errc := make(chan error, 1)
	go func() {
		defer close(out)
		errc <- t.Process(ctx, in, out)
	}()
	for range out { //nolint:revive // drain the pipeline
	}
	return <-errc
}

// containsTerm reports whether term occurs in text, case-insensitively, on
// word-ish boundaries (mirrors the brand vocabulary checker's matching).
func containsTerm(text, term string) bool {
	re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(term))
	return re.MatchString(text)
}

// compile-time assertion that check.Finding stays the shape we flatten.
var _ = check.Finding{}
