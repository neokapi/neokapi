package project

import (
	"errors"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// A recipe that fails to parse is reported with the parser's line and what it
// expected there. The parser, and the decoders a recipe field defines, can
// quote the content they could not read, and a recipe can be a file outside
// the project that a person's -p names. So a parse error quotes no more than
// one line of content: each message is cut at its first line break and capped
// in length (RecipeYAMLError), and a decoder that quotes a value quotes its
// first line (snippet).

// maxParseMessage caps one message of a parse error, in bytes.
const maxParseMessage = 240

// maxSnippet caps a value a recipe error quotes, in bytes.
const maxSnippet = 40

// parseError is a recipe parse error whose message quotes at most one line of
// the content per problem. Unwrap returns what the parser returned.
type parseError struct {
	msg string
	err error
}

func (e *parseError) Error() string { return e.msg }

func (e *parseError) Unwrap() error { return e.err }

// RecipeYAMLError returns the error a recipe's YAML decode returned with each
// of its messages cut to one line of at most maxParseMessage bytes. A
// *yaml.TypeError lists one message per problem, and each is cut on its own.
// A nil err stays nil.
func RecipeYAMLError(err error) error {
	if err == nil {
		return nil
	}
	if te, ok := errors.AsType[*yaml.TypeError](err); ok {
		lines := make([]string, len(te.Errors))
		for i, m := range te.Errors {
			lines[i] = firstLine(m, maxParseMessage, " ...")
		}
		return &parseError{msg: "yaml: unmarshal errors:\n  " + strings.Join(lines, "\n  "), err: err}
	}
	return &parseError{msg: firstLine(err.Error(), maxParseMessage, " ..."), err: err}
}

// snippet returns the first line of a value a recipe holds, capped at
// maxSnippet bytes, for an error that quotes it.
func snippet(v string) string {
	return firstLine(v, maxSnippet, "...")
}

// firstLine returns the first line of s, capped at limit bytes on a character
// boundary, with mark appended when anything was cut.
func firstLine(s string, limit int, mark string) string {
	cut := false
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s, cut = s[:i], true
	}
	if len(s) > limit {
		end := limit
		for end > 0 && !utf8.RuneStart(s[end]) {
			end--
		}
		s, cut = s[:end], true
	}
	if cut {
		s += mark
	}
	return s
}
