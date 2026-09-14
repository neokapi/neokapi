package comments

import (
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/comment"
)

// LexicalComments returns the comment spans a language's lexical scan reads
// from src, with no grammar.
func LexicalComments(language string, src []byte) ([][2]int, error) {
	for _, l := range languages {
		if l.Name == language && l.syntax.lexical != nil {
			return l.syntax.lexical(src)
		}
	}
	return nil, fmt.Errorf("no lexical scan for %q", language)
}

// DirectiveForms names the directive forms a language's syntax tests, in order.
func DirectiveForms(language string) []string {
	var names []string
	for _, l := range languages {
		if l.Name == language {
			for _, f := range l.syntax.directives {
				names = append(names, f.name)
			}
		}
	}
	return names
}

// LocateWithout is Locate with the named directive forms removed from the
// language's syntax. With no names given it removes every form.
func LocateWithout(language, name string, src []byte, forms ...string) (*comment.File, error) {
	for _, l := range languages {
		if l.Name != language {
			continue
		}
		syn := *l.syntax
		syn.directives = slices.DeleteFunc(slices.Clone(syn.directives), func(f directiveForm) bool {
			return len(forms) == 0 || slices.Contains(forms, f.name)
		})
		broken := *l
		broken.syntax = &syn
		saved := languages
		languages = []*Language{&broken}
		defer func() { languages = saved }()
		return Locate(language, name, src)
	}
	return Locate(language, name, src)
}
