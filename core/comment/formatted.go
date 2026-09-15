package comment

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrFormatterNotRun is wrapped by a Formatter that did not compare a file:
// none is installed for it, or the one installed leaves a file at its path as
// it is. A rewrite held to such a formatter did not run, and nothing is
// written.
var ErrFormatterNotRun = errors.New("the formatter did not run")

// CompareFormatted reports the comments in f, located in src, that formatted,
// the formatter's output for src, writes differently.
//
// p locates src and formatted as they are, with no directives declared, and
// their comments pair by position. A comment of that reading disagrees when a
// line of it differs in its text, or in its indentation when it starts its
// line, and each comment of f inside it disagrees with it. Line endings are not
// compared. Output holding a different number of comments cannot be paired, and
// is an error wrapping ErrUnlocated.
func CompareFormatted(p Provider, name string, src, formatted []byte, f *File) ([]Disagreement, error) {
	if bytes.Equal(src, formatted) || len(f.Comments) == 0 {
		return nil, nil
	}
	was, err := p.Locate(name, src)
	if err != nil {
		return nil, fmt.Errorf("locate %s: %w", name, err)
	}
	now, err := p.Locate(name, formatted)
	if err != nil {
		return nil, fmt.Errorf("locate the formatter's output for %s: %w", name, err)
	}
	if len(was.Comments) != len(now.Comments) {
		return nil, unlocated("%s: the formatter's output holds %d comments, and the file %d", name, len(now.Comments), len(was.Comments))
	}
	var out []Disagreement
	for i, c := range was.Comments {
		n := now.Comments[i]
		if slices.Equal(formattedLines(src, c), formattedLines(formatted, n)) {
			continue
		}
		for j, x := range f.Comments {
			if x.Start >= c.Start && x.End <= c.End {
				out = append(out, Disagreement{Comment: j, Formatted: string(formatted[n.Start:n.End])})
			}
		}
	}
	return out, nil
}

// formattedLines is a comment's lines as a formatter comparison reads them:
// each line of its span without its line ending, the first led by its
// indentation when the comment starts its line.
func formattedLines(src []byte, c Comment) []string {
	lineStart := bytes.LastIndexByte(src[:c.Start], '\n') + 1
	lead := string(src[lineStart:c.Start])
	if strings.TrimLeft(lead, " \t") != "" {
		lead = ""
	}
	lines := fileLines(src[c.Start:c.End])
	lines[0] = lead + lines[0]
	return lines
}

// formatterFailed is what Contain returns when f could not compare or format a
// file: the error itself when the formatter did not run, and a refusal
// otherwise.
func formatterFailed(f Formatter, err error, format string, args ...any) error {
	if errors.Is(err, ErrFormatterNotRun) {
		return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
	}
	return refuse(RefusedFormatter, "%s: %v", fmt.Sprintf(format, args...), err)
}
