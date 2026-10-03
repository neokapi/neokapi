package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// The graders here read the output files with parsers of their own, never with
// kapi's readers: a grader that shared the system under test's parser would
// pass whatever that parser gets wrong.

// pairedJSONLeaf is one scalar of a JSON document: its dotted key path and its
// value as JSON.
type pairedJSONLeaf struct {
	Path  string
	Value string
}

// pairedJSONLeaves lists a JSON document's scalars in document order. A
// duplicate key is an error, so a later value cannot hide an earlier one.
func pairedJSONLeaves(body []byte) ([]pairedJSONLeaf, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var leaves []pairedJSONLeaf
	if err := pairedJSONWalk(decoder, "", &leaves); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("unexpected content after the JSON document")
	}
	return leaves, nil
}

func pairedJSONWalk(decoder *json.Decoder, prefix string, leaves *[]pairedJSONLeaf) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("expected an object key")
				}
				if seen[key] {
					return fmt.Errorf("duplicate key %q", pairedJoinKey(prefix, key))
				}
				seen[key] = true
				if err := pairedJSONWalk(decoder, pairedJoinKey(prefix, key), leaves); err != nil {
					return err
				}
			}
		case '[':
			for index := 0; decoder.More(); index++ {
				if err := pairedJSONWalk(decoder, pairedJoinKey(prefix, strconv.Itoa(index)), leaves); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected delimiter %q", value)
		}
		_, err := decoder.Token()
		return err
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		*leaves = append(*leaves, pairedJSONLeaf{Path: prefix, Value: string(encoded)})
		return nil
	}
}

func pairedJoinKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func pairedLeafDifference(want, got []pairedJSONLeaf) string {
	for i := range max(len(want), len(got)) {
		var w, g pairedJSONLeaf
		if i < len(want) {
			w = want[i]
		}
		if i < len(got) {
			g = got[i]
		}
		if w != g {
			return fmt.Sprintf("leaf %d: want %s=%s, got %s=%s", i+1, w.Path, w.Value, g.Path, g.Value)
		}
	}
	return "differs"
}

// pairedMDBlock is one block of a Markdown page: a heading, a paragraph, a
// list item or a fenced code block.
type pairedMDBlock struct {
	Kind  string
	Level int
	Text  string
}

var (
	pairedHeadingLine = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	pairedOrderedItem = regexp.MustCompile(`^\d+[.)]\s+`)
	pairedBulletItem  = regexp.MustCompile(`^[-*+]\s+`)
	pairedLinkAddress = regexp.MustCompile(`\]\(\s*([^)\s]+)(?:\s+"[^"]*")?\s*\)`)
	pairedCodeSpan    = regexp.MustCompile("`([^`]+)`")
	pairedBoldSpan    = regexp.MustCompile(`\*\*[^*]+\*\*|__[^_]+__`)
	pairedLinkText    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	pairedSentenceEnd = regexp.MustCompile(`[.!?:;]+\s*`)
)

// pairedMarkdownBlocks splits a page into blocks. A fence keeps its content
// verbatim; a list item continues over the indented lines below it.
func pairedMarkdownBlocks(text string) []pairedMDBlock {
	var blocks []pairedMDBlock
	var current *pairedMDBlock
	flush := func() {
		if current != nil {
			blocks = append(blocks, *current)
			current = nil
		}
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"):
			flush()
			fence := trimmed[:3]
			var body []string
			for i++; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), fence); i++ {
				body = append(body, lines[i])
			}
			blocks = append(blocks, pairedMDBlock{Kind: "code", Text: strings.Join(body, "\n")})
		case trimmed == "":
			flush()
		case pairedHeadingLine.MatchString(trimmed):
			flush()
			match := pairedHeadingLine.FindStringSubmatch(trimmed)
			blocks = append(blocks, pairedMDBlock{Kind: "heading", Level: len(match[1]), Text: match[2]})
		case pairedOrderedItem.MatchString(trimmed):
			flush()
			current = &pairedMDBlock{Kind: "ordered-item", Text: pairedOrderedItem.ReplaceAllString(trimmed, "")}
		case pairedBulletItem.MatchString(trimmed):
			flush()
			current = &pairedMDBlock{Kind: "bullet-item", Text: pairedBulletItem.ReplaceAllString(trimmed, "")}
		default:
			if current == nil {
				current = &pairedMDBlock{Kind: "paragraph", Text: trimmed}
			} else {
				current.Text += "\n" + trimmed
			}
		}
	}
	flush()
	return blocks
}

// pairedSkeleton is what a translation keeps of a block: its kind and level,
// its link addresses, its inline code and its bold spans, and a code block's
// content.
func pairedSkeleton(block pairedMDBlock) string {
	if block.Kind == "code" {
		return "code|" + block.Text
	}
	var parts []string
	for _, match := range pairedLinkAddress.FindAllStringSubmatch(block.Text, -1) {
		parts = append(parts, "link:"+match[1])
	}
	for _, match := range pairedCodeSpan.FindAllStringSubmatch(block.Text, -1) {
		parts = append(parts, "code:"+match[1])
	}
	return fmt.Sprintf("%s|%d|%s|bold:%d", block.Kind, block.Level, strings.Join(parts, ","),
		len(pairedBoldSpan.FindAllString(block.Text, -1)))
}

func pairedMarkdownSkeletonMatches(source, output string) (bool, string, error) {
	want := pairedMarkdownBlocks(source)
	got := pairedMarkdownBlocks(output)
	if len(want) != len(got) {
		return false, fmt.Sprintf("%d blocks, the source has %d", len(got), len(want)), nil
	}
	for i := range want {
		if w, g := pairedSkeleton(want[i]), pairedSkeleton(got[i]); w != g {
			return false, fmt.Sprintf("block %d: want %q, got %q", i+1, w, g), nil
		}
	}
	return true, "", nil
}

// pairedProse is a block's readable text: link text without its address,
// without inline code and emphasis markers, lower-cased with its whitespace
// collapsed.
func pairedProse(text string) string {
	text = pairedLinkText.ReplaceAllString(text, "$1")
	text = pairedCodeSpan.ReplaceAllString(text, " ")
	text = strings.NewReplacer("**", "", "__", "", "*", "", "_", "").Replace(text)
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

func pairedMarkdownTranslated(source, output string) (bool, string) {
	want := pairedMarkdownBlocks(source)
	got := pairedMarkdownBlocks(output)
	if len(want) != len(got) {
		return false, fmt.Sprintf("%d blocks, the source has %d", len(got), len(want))
	}
	whole := pairedProse(output)
	for i := range want {
		if want[i].Kind == "code" {
			continue
		}
		if pairedProse(want[i].Text) == pairedProse(got[i].Text) {
			return false, fmt.Sprintf("block %d is untranslated: %q", i+1, pairedClip(want[i].Text))
		}
		for _, sentence := range pairedSentenceEnd.Split(pairedProse(want[i].Text), -1) {
			if len(strings.Fields(sentence)) >= 4 && strings.Contains(whole, sentence) {
				return false, fmt.Sprintf("English remains: %q", sentence)
			}
		}
	}
	return true, ""
}

// Function words of the languages the corpus checks. A word both languages use
// in the same spelling ("for", "i") sits in neither list.
var pairedFunctionWords = map[string][]string{
	"nb": {"og", "på", "til", "med", "en", "et", "ei", "du", "deg", "din", "ditt", "dine", "som", "av",
		"er", "har", "kan", "ikke", "eller", "når", "å", "vil", "skal", "må", "fra", "om", "før", "etter",
		"inn", "mer", "den", "det", "de", "dette", "hvis", "også"},
	"en": {"the", "and", "your", "you", "with", "before", "from", "this", "that", "are", "is", "of",
		"to", "on", "an", "a", "if", "more", "after", "can"},
}

// pairedReadsAs reports whether a page's prose reads as language, by the
// distinct function words of that language it uses against those of English.
func pairedReadsAs(text, language string) (bool, string, error) {
	words, ok := pairedFunctionWords[language]
	if !ok || language == "en" {
		return false, "", fmt.Errorf("no function words for %q", language)
	}
	var prose strings.Builder
	for _, block := range pairedMarkdownBlocks(text) {
		if block.Kind != "code" {
			prose.WriteString(pairedProse(block.Text))
			prose.WriteByte(' ')
		}
	}
	tokens := strings.FieldsFunc(prose.String(), func(r rune) bool { return !unicode.IsLetter(r) })
	target, english := map[string]bool{}, map[string]bool{}
	for _, token := range tokens {
		if slices.Contains(words, token) {
			target[token] = true
		}
		if slices.Contains(pairedFunctionWords["en"], token) {
			english[token] = true
		}
	}
	detail := fmt.Sprintf("%d %s function words, %d English", len(target), language, len(english))
	return len(target) >= 5 && len(english) <= 2, detail, nil
}

func pairedNormalized(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// pairedBlockExtends reports whether block index of output keeps the
// original's text, whitespace aside, and adds text that contains every word in
// require and none in forbid.
func pairedBlockExtends(original, output string, index int, require, forbid []string) (bool, string) {
	want := pairedMarkdownBlocks(original)
	got := pairedMarkdownBlocks(output)
	if index >= len(want) || index >= len(got) {
		return false, fmt.Sprintf("no block %d", index+1)
	}
	before := pairedNormalized(want[index].Text)
	after := pairedNormalized(got[index].Text)
	added, ok := strings.CutPrefix(after, before)
	if !ok || strings.TrimSpace(added) == "" {
		return false, fmt.Sprintf("block %d does not extend the original: %q", index+1, pairedClip(after))
	}
	lower := strings.ToLower(added)
	for _, word := range require {
		if !strings.Contains(lower, strings.ToLower(word)) {
			return false, fmt.Sprintf("the added text lacks %q: %q", word, pairedClip(added))
		}
	}
	for _, word := range forbid {
		if pairedContainsWord(added, word) {
			return false, fmt.Sprintf("the added text contains %q: %q", word, pairedClip(added))
		}
	}
	return true, strings.TrimSpace(added)
}

// pairedBlocksUnchanged reports whether every block of output but those in
// except keeps the original's kind and text, whitespace aside.
func pairedBlocksUnchanged(original, output string, except []int) (bool, string) {
	want := pairedMarkdownBlocks(original)
	got := pairedMarkdownBlocks(output)
	if len(want) != len(got) {
		return false, fmt.Sprintf("%d blocks, the original has %d", len(got), len(want))
	}
	for i := range want {
		if slices.Contains(except, i) {
			continue
		}
		if want[i].Kind != got[i].Kind || want[i].Level != got[i].Level ||
			pairedNormalized(want[i].Text) != pairedNormalized(got[i].Text) {
			return false, fmt.Sprintf("block %d changed: %q", i+1, pairedClip(got[i].Text))
		}
	}
	return true, ""
}

// pairedContainsWord reports whether text uses word, or a word that starts with
// it, in any case.
func pairedContainsWord(text, word string) bool {
	return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word)).MatchString(text)
}
