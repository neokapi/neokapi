package mdspike

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/clip"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
	"gopkg.in/yaml.v3"
)

// maxExtendsDepth bounds the `extends:` chain, so a cycle reports a diagnosable
// error instead of exhausting the stack.
const maxExtendsDepth = 8

// Options configures a load.
type Options struct {
	// Terms resolves a `vocabulary.from_terms` block. A document that declares
	// one without a store here is an error rather than a silently empty
	// vocabulary: a profile whose bans quietly vanish is worse than one that
	// refuses to load.
	Terms terms.Terminology
	// TermsScope filters terms-store entries by validity. nil applies no
	// validity filtering, matching terms.MatchesScope.
	TermsScope *graph.Scope
}

// LoadFile reads a markdown+frontmatter voice profile from disk, resolving any
// `extends:` chain relative to the file's own directory.
func LoadFile(ctx context.Context, filename string, opts Options) (*profile.VoiceProfile, error) {
	dir, base := filepath.Split(filename)
	if dir == "" {
		dir = "."
	}
	return LoadFS(ctx, os.DirFS(dir), base, opts)
}

// LoadFS reads a markdown+frontmatter voice profile from fsys, resolving any
// `extends:` chain against the same filesystem. It produces the same
// *profile.VoiceProfile type the YAML loader produces, so every consumer —
// RenderVoiceGuide, MatchVocabulary, ValidateProfile — works unchanged.
func LoadFS(ctx context.Context, fsys fs.FS, name string, opts Options) (*profile.VoiceProfile, error) {
	return loadFS(ctx, fsys, name, opts, 0)
}

func loadFS(ctx context.Context, fsys fs.FS, name string, opts Options, depth int) (*profile.VoiceProfile, error) {
	if depth > maxExtendsDepth {
		return nil, fmt.Errorf("%s: extends chain deeper than %d, possibly a cycle", name, maxExtendsDepth)
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	doc, err := parseDocument(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	p, err := doc.toProfile(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	if doc.front.Extends == "" {
		return p, nil
	}
	parentName := path.Join(path.Dir(name), doc.front.Extends)
	parent, err := loadFS(ctx, fsys, parentName, opts, depth+1)
	if err != nil {
		return nil, fmt.Errorf("%s extends %s: %w", name, doc.front.Extends, err)
	}
	return merge(parent, p), nil
}

// frontmatter is the authored, never-generated half of the document. It is
// decoded strictly (unknown fields are errors), which is what keeps the split
// honest: `tone.guidelines` and `locales.<id>.cultural_notes` are absent here
// on purpose, so putting prose in the frontmatter fails to load rather than
// quietly bypassing review of the body.
type frontmatter struct {
	Extends    string                        `yaml:"extends,omitempty"`
	ID         string                        `yaml:"id,omitempty"`
	Name       string                        `yaml:"name,omitempty"`
	Scope      string                        `yaml:"workspace_id,omitempty"`
	MinScore   int                           `yaml:"min_score,omitempty"`
	Tone       toneFront                     `yaml:"tone,omitempty"`
	Style      profile.StyleRules            `yaml:"style,omitempty"`
	Vocabulary vocabularyFront               `yaml:"vocabulary,omitempty"`
	Locales    map[model.LocaleID]localeMeta `yaml:"locales,omitempty"`
}

// toneFront is ToneProfile without Guidelines: the enums are frontmatter, the
// guidance prose is the body's "## Guidelines" section.
type toneFront struct {
	Personality []string `yaml:"personality,omitempty"`
	Formality   string   `yaml:"formality,omitempty"`
	Emotion     string   `yaml:"emotion,omitempty"`
	Humor       string   `yaml:"humor,omitempty"`
}

// vocabularyFront is VocabularyRules plus the terms-store source.
type vocabularyFront struct {
	FromTerms       *TermsSource       `yaml:"from_terms,omitempty"`
	PreferredTerms  []profile.TermRule `yaml:"preferred_terms,omitempty"`
	ForbiddenTerms  []profile.TermRule `yaml:"forbidden_terms,omitempty"`
	CompetitorTerms []profile.TermRule `yaml:"competitor_terms,omitempty"`
	Abbreviations   map[string]string  `yaml:"abbreviations,omitempty"`
}

// TermsSource declares vocabulary rules derived from the terms store rather
// than restated in the profile. Statuses are the terms store's own lifecycle
// vocabulary (model.TermPreferred, model.TermAdmitted, model.TermDeprecated,
// model.TermForbidden), so "which words are out" is decided in one place.
type TermsSource struct {
	// Locale selects the terms whose text the rules are written against.
	// Defaults to "en".
	Locale model.LocaleID `yaml:"locale,omitempty"`
	// Domains restricts sourcing to these concept domains; empty means all.
	Domains []string `yaml:"domains,omitempty"`
	// Prefer maps concept terms with these statuses onto preferred terms.
	Prefer []model.TermStatus `yaml:"prefer,omitempty"`
	// Forbid maps concept terms with these statuses onto forbidden terms, each
	// carrying the concept's preferred term of the same locale as replacement.
	Forbid []model.TermStatus `yaml:"forbid,omitempty"`
	// ForbidSeverity sets the severity on the derived forbidden rules. Empty
	// leaves it unset, which MatchVocabulary reads as major.
	ForbidSeverity string `yaml:"forbid_severity,omitempty"`
	// CompetitorSeverity sets the severity on rules derived from terms flagged
	// CompetitorTerm in the store. Empty leaves it unset (critical).
	CompetitorSeverity string `yaml:"competitor_severity,omitempty"`
}

// document is a parsed markdown+frontmatter profile before terms resolution.
type document struct {
	front       frontmatter
	description string
	guidelines  string
	examples    []profile.VoiceExample
	// culturalNotes holds the "## Locale: <id>" sections' prose.
	culturalNotes map[model.LocaleID]string
}

var (
	fenceRE   = regexp.MustCompile(`^(-{3,}|\.{3,})\s*$`)
	headingRE = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
)

// parseDocument splits the frontmatter from the body and parses both.
func parseDocument(src string) (*document, error) {
	front, body, err := splitFrontmatter(src)
	if err != nil {
		return nil, err
	}
	doc := &document{culturalNotes: map[model.LocaleID]string{}}

	dec := yaml.NewDecoder(strings.NewReader(front))
	dec.KnownFields(true)
	// An empty frontmatter block decodes as io.EOF; a profile that is all body
	// and no rules is legal to parse and will fail profile.ValidateProfile on
	// the missing name, which is where that error belongs.
	if err := dec.Decode(&doc.front); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	if err := doc.parseBody(body); err != nil {
		return nil, err
	}
	return doc, nil
}

// splitFrontmatter separates a leading `---` fenced YAML block from the
// markdown body. A document with no frontmatter is an error: the governing
// rules are the point, and a prose-only file would load as a profile that
// enforces nothing.
func splitFrontmatter(src string) (front, body string, err error) {
	sc := bufio.NewScanner(strings.NewReader(src))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	if !sc.Scan() {
		return "", "", errors.New("empty document")
	}
	if !fenceRE.MatchString(strings.TrimSpace(sc.Text())) {
		return "", "", errors.New("document does not start with a `---` frontmatter fence")
	}
	var f, b strings.Builder
	closed := false
	for sc.Scan() {
		line := sc.Text()
		if !closed && fenceRE.MatchString(strings.TrimSpace(line)) {
			closed = true
			continue
		}
		if closed {
			b.WriteString(line)
			b.WriteString("\n")
		} else {
			f.WriteString(line)
			f.WriteString("\n")
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", fmt.Errorf("scan document: %w", err)
	}
	if !closed {
		return "", "", errors.New("frontmatter fence is never closed")
	}
	return f.String(), b.String(), nil
}

// parseBody walks the markdown body one `##` section at a time. An unrecognized
// section heading is an error rather than ignored prose — silently dropping a
// section is how a markdown format loses the guidance it exists to carry.
func (d *document) parseBody(body string) error {
	heading, buf := "", &strings.Builder{}
	flush := func() error {
		text := buf.String()
		buf.Reset()
		return d.section(heading, text)
	}

	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		m := headingRE.FindStringSubmatch(line)
		if m == nil || len(m[1]) != 2 {
			// H1 is the human title and carries no field; deeper headings are
			// ordinary prose inside whichever section is open.
			if m != nil && len(m[1]) == 1 {
				continue
			}
			buf.WriteString(line)
			buf.WriteString("\n")
			continue
		}
		if err := flush(); err != nil {
			return err
		}
		heading = strings.TrimSpace(m[2])
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("scan body: %w", err)
	}
	return flush()
}

// section routes one parsed `##` section onto its profile field.
func (d *document) section(heading, text string) error {
	switch {
	case heading == "":
		// Everything before the first `##` is the profile description.
		d.description = joinParagraphs(paragraphs(text))
		return nil
	case strings.EqualFold(heading, "guidelines"):
		d.guidelines = joinParagraphs(paragraphs(text))
		return nil
	case hasPrefixFold(heading, "example"):
		ex, err := parseExample(heading, text)
		if err != nil {
			return err
		}
		d.examples = append(d.examples, ex)
		return nil
	case hasPrefixFold(heading, "locale:"):
		id := model.LocaleID(strings.TrimSpace(heading[len("locale:"):]))
		if id == "" {
			return errors.New(`section "## Locale:" names no locale`)
		}
		d.culturalNotes[id] = joinParagraphs(paragraphs(text))
		return nil
	default:
		return fmt.Errorf("unknown section %q (expected Guidelines, Example…, or Locale: <id>)", heading)
	}
}

var labelRE = regexp.MustCompile(`^\*\*(Before|After|Why):\*\*\s*(.*)$`)

// parseExample reads a "## Example: <category>" section. Each of the labelled
// paragraphs (**Before:**, **After:**, **Why:**) maps onto one VoiceExample
// field; before and after are required.
func parseExample(heading, text string) (profile.VoiceExample, error) {
	var ex profile.VoiceExample
	if _, category, ok := strings.Cut(heading, ":"); ok {
		ex.Category = strings.TrimSpace(category)
	}
	for _, para := range paragraphs(text) {
		m := labelRE.FindStringSubmatch(para)
		if m == nil {
			return ex, fmt.Errorf("example %q: paragraph %q is not labelled **Before:**, **After:** or **Why:**", heading, clip.Runes(para, 40))
		}
		switch m[1] {
		case "Before":
			ex.Before = m[2]
		case "After":
			ex.After = m[2]
		case "Why":
			ex.Explanation = m[2]
		}
	}
	if ex.Before == "" || ex.After == "" {
		return ex, fmt.Errorf("example %q: both **Before:** and **After:** are required", heading)
	}
	return ex, nil
}

// toProfile assembles the parsed document into a VoiceProfile, resolving the
// terms-store source when one is declared.
func (d *document) toProfile(ctx context.Context, opts Options) (*profile.VoiceProfile, error) {
	p := &profile.VoiceProfile{
		ID:          d.front.ID,
		Name:        d.front.Name,
		Description: d.description,
		Scope:       d.front.Scope,
		MinScore:    d.front.MinScore,
		Tone: profile.ToneProfile{
			Personality: d.front.Tone.Personality,
			Formality:   d.front.Tone.Formality,
			Emotion:     d.front.Tone.Emotion,
			Humor:       d.front.Tone.Humor,
			Guidelines:  d.guidelines,
		},
		Style:    d.front.Style,
		Examples: d.examples,
		Vocabulary: profile.VocabularyRules{
			PreferredTerms:  d.front.Vocabulary.PreferredTerms,
			ForbiddenTerms:  d.front.Vocabulary.ForbiddenTerms,
			CompetitorTerms: d.front.Vocabulary.CompetitorTerms,
			Abbreviations:   d.front.Vocabulary.Abbreviations,
		},
	}

	if src := d.front.Vocabulary.FromTerms; src != nil {
		if opts.Terms == nil {
			return nil, errors.New("vocabulary.from_terms is declared but no terms store was supplied")
		}
		derived, err := SourceVocabulary(ctx, opts.Terms, *src, opts.TermsScope)
		if err != nil {
			return nil, err
		}
		// Derived rules come first, hand-authored rules after, so a profile can
		// still tighten severity or add a note for a term the store also names:
		// mergeTerms keeps the later rule.
		p.Vocabulary.PreferredTerms = mergeTerms(derived.PreferredTerms, p.Vocabulary.PreferredTerms)
		p.Vocabulary.ForbiddenTerms = mergeTerms(derived.ForbiddenTerms, p.Vocabulary.ForbiddenTerms)
		p.Vocabulary.CompetitorTerms = mergeTerms(derived.CompetitorTerms, p.Vocabulary.CompetitorTerms)
	}

	locales := map[model.LocaleID]profile.LocaleOverride{}
	for id, meta := range d.front.Locales {
		locales[id] = profile.LocaleOverride{
			Formality:           meta.Formality,
			Humor:               meta.Humor,
			PersonPOV:           meta.PersonPOV,
			VocabularyOverrides: meta.VocabularyOverrides,
		}
	}
	for id, notes := range d.culturalNotes {
		lo := locales[id]
		lo.CulturalNotes = notes
		locales[id] = lo
	}
	if len(locales) > 0 {
		p.Locales = locales
	}
	return p, nil
}

// localeMeta is LocaleOverride minus the prose (cultural notes) and minus
// example overrides, which are body sections.
type localeMeta struct {
	Formality           string             `yaml:"formality,omitempty"`
	Humor               string             `yaml:"humor,omitempty"`
	PersonPOV           string             `yaml:"person_pov,omitempty"`
	VocabularyOverrides []profile.TermRule `yaml:"vocabulary_overrides,omitempty"`
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}
