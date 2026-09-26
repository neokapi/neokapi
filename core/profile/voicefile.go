package profile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// A voice file is a voice profile as written, and it may carry word rules
// beside the voice: a starter pack lists its terms under `terms:`. Word rules
// are terms, so the voice profile itself holds none. Reading a file splits
// the two, and importing one moves the word rules into the project's terms.
//
// A file written before word rules moved to terms lists them under
// `vocabulary:` (preferred, forbidden and competitor terms). Reading it
// converts that list into term rules: a forbidden term stays a rule, a
// competitor term becomes a rule marked Competitor, and a preferred term
// becomes a preferred form (or, when it names a replacement, an advisory rule
// preferring the replacement).

// VoiceFile is a voice file read: the voice profile and the word rules the file
// carries beside it.
type VoiceFile struct {
	Profile *VoiceProfile
	// Terms are the file's word rules: its `terms:` list, then what its
	// `vocabulary:` list held, converted.
	Terms []TermRule
	// Converted counts the rules converted from a `vocabulary:` list.
	Converted VocabularyConversion
}

// VocabularyConversion counts the rules a file's `vocabulary:` list held, by
// the list they sat in.
type VocabularyConversion struct {
	Forbidden  int `json:"forbidden,omitempty"`
	Competitor int `json:"competitor,omitempty"`
	Preferred  int `json:"preferred,omitempty"`
}

// Total is the number of rules converted.
func (c VocabularyConversion) Total() int { return c.Forbidden + c.Competitor + c.Preferred }

// String describes the conversion as a reader reads it, such as
// "3 forbidden, 1 competitor and 2 preferred terms".
func (c VocabularyConversion) String() string {
	var parts []string
	for _, p := range []struct {
		n    int
		kind string
	}{{c.Forbidden, "forbidden"}, {c.Competitor, "competitor"}, {c.Preferred, "preferred"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.kind))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	noun := "terms"
	if c.Total() == 1 {
		noun = "term"
	}
	if len(parts) == 1 {
		return parts[0] + " " + noun
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1] + " " + noun
}

// voiceFileDoc is the document a voice file decodes into.
type voiceFileDoc struct {
	VoiceProfile `yaml:",inline"`
	Terms        []TermRule        `yaml:"terms,omitempty"`
	Vocabulary   *vocabularySource `yaml:"vocabulary,omitempty"`
}

// vocabularySource is the `vocabulary:` list a voice file written before word
// rules moved to terms carries.
type vocabularySource struct {
	PreferredTerms  []TermRule `yaml:"preferred_terms,omitempty"`
	ForbiddenTerms  []TermRule `yaml:"forbidden_terms,omitempty"`
	CompetitorTerms []TermRule `yaml:"competitor_terms,omitempty"`
	// Abbreviations are read so a file that lists them still loads, and
	// dropped: an abbreviation map is not a rule any check applies.
	Abbreviations map[string]string `yaml:"abbreviations,omitempty"`
}

// ParseVoiceFile reads a voice file: the voice profile, the word rules the
// file carries, and what it converted from a `vocabulary:` list.
func ParseVoiceFile(data []byte) (VoiceFile, error) {
	var doc voiceFileDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return VoiceFile{}, fmt.Errorf("parse profile: %w", err)
	}
	p := doc.VoiceProfile
	if err := constraintError(&p); err != nil {
		return VoiceFile{}, err
	}
	if err := commentRulesError(&p); err != nil {
		return VoiceFile{}, fmt.Errorf("parse profile: %w", err)
	}
	out := VoiceFile{Profile: &p, Terms: doc.Terms}
	if v := doc.Vocabulary; v != nil {
		converted, conv := convertVocabulary(*v)
		out.Terms = append(out.Terms, converted...)
		out.Converted = conv
	}
	return out, nil
}

// convertVocabulary turns a `vocabulary:` list into term rules.
func convertVocabulary(v vocabularySource) ([]TermRule, VocabularyConversion) {
	var out []TermRule
	var conv VocabularyConversion
	for _, r := range v.ForbiddenTerms {
		if strings.TrimSpace(r.Term) == "" {
			continue
		}
		out = append(out, r)
		conv.Forbidden++
	}
	for _, r := range v.CompetitorTerms {
		if strings.TrimSpace(r.Term) == "" {
			continue
		}
		r.Competitor = true
		out = append(out, r)
		conv.Competitor++
	}
	for _, r := range v.PreferredTerms {
		if rule, ok := preferredRule(r); ok {
			out = append(out, rule)
			conv.Preferred++
		}
	}
	return out, conv
}

// preferredRule converts a preferred term. A preferred term naming a
// replacement asked for the replacement over the term, which is an advisory
// rule: a preferred term was never checked, so it reports rather than fails.
// One naming none is a preferred form, which rejects nothing.
func preferredRule(r TermRule) (TermRule, bool) {
	term := strings.TrimSpace(r.Term)
	replacement := strings.TrimSpace(r.Replacement)
	switch {
	case term == "" && replacement == "":
		return TermRule{}, false
	case replacement != "" && term != "" && !strings.EqualFold(term, replacement):
		r.Advisory = true
		return r, true
	}
	if replacement == "" {
		replacement = term
	}
	return TermRule{
		Replacement:      replacement,
		ReplacementForms: r.Forms,
		Note:             r.Note,
		ConceptID:        r.ConceptID,
	}, true
}

// LoadProfileYAML decodes a voice profile from a voice file. It is the loader
// for standalone, git-shareable voice files and for the embedded starter
// packs, so a voice works with or without a backing store. The word rules the
// file carries travel with the profile (CarriedTerms), named as coming from a
// voice file; a caller that knows the file better names it with Carry.
func LoadProfileYAML(r io.Reader) (*VoiceProfile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	f, err := ParseVoiceFile(data)
	if err != nil {
		return nil, err
	}
	if len(f.Terms) > 0 {
		f.Profile.Carry(CarriedFromVoiceFile, f.Terms)
	}
	return f.Profile, nil
}

// CarriedFromVoiceFile is where the word rules of a profile read from a voice
// file come from, when the caller names nothing more precise.
const CarriedFromVoiceFile = "voice file"

// DecodeProfileStrict decodes a voice file, refusing any key the voice file
// does not define. It returns the profile it decoded as far as it got, and the
// error.
func DecodeProfileStrict(r io.Reader) (*VoiceProfile, error) {
	var doc voiceFileDoc
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	err := dec.Decode(&doc)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	p := &doc.VoiceProfile
	words := doc.Terms
	if doc.Vocabulary != nil {
		converted, _ := convertVocabulary(*doc.Vocabulary)
		words = append(words, converted...)
	}
	if len(words) > 0 {
		p.Carry(CarriedFromVoiceFile, words)
	}
	return p, err
}

// EncodeVoiceFile writes a voice profile and the word rules it carries as a
// voice file, with the rules under `terms:`.
func EncodeVoiceFile(p *VoiceProfile) ([]byte, error) {
	doc := voiceFileDoc{VoiceProfile: *p, Terms: p.CarriedTerms().Rules}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ConvertVocabularyLists turns word lists in the shape a `vocabulary:` list
// held (preferred, forbidden and competitor terms) into term rules, the way
// reading a voice file converts one.
func ConvertVocabularyLists(preferred, forbidden, competitor []TermRule) ([]TermRule, VocabularyConversion) {
	return convertVocabulary(vocabularySource{PreferredTerms: preferred, ForbiddenTerms: forbidden, CompetitorTerms: competitor})
}
