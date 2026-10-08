package project

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/neokapi/neokapi/core/ignore"
)

// MatchGlob reports whether path matches the doublestar glob pattern (same
// semantics as ExpandGlob: `**` recursive, `{a,b}` alternation). Both should be
// slash-separated, relative to the same root.
func MatchGlob(pattern, path string) bool {
	ok, err := doublestar.Match(pattern, path)
	return err == nil && ok
}

// ExpandGlob returns relative paths under root that match the given glob
// pattern. Supports `**` for recursive directory matching and `{a,b}`
// alternation. Any matches matching one of the exclude patterns are filtered
// out.
//
// `**` does not follow a symbolic link into another directory, as git ls-files
// does, so the linked packages a package manager keeps under node_modules stay
// out of a match and a link loop is read once. A link that matches the pattern
// itself is returned, and a linked directory the pattern names before its first
// wildcard is read through.
func ExpandGlob(root, pattern string, excludes ...string) ([]string, error) {
	res, err := ExpandGlobs(root, []string{pattern}, GlobOptions{Excludes: excludes})
	if err != nil {
		return nil, err
	}
	return res[0], nil
}

// GlobOptions are the rules ExpandGlobs applies while it walks.
type GlobOptions struct {
	// Excludes are doublestar patterns. A path one of them matches is never
	// returned, and a directory that an exclude ending in `/**` covers is not
	// read at all.
	Excludes []string
	// Ignore, when set, applies the project's ignore rules: an ignored file is
	// never returned, and an ignored directory is neither returned nor read, so
	// nothing beneath it matches (gitignore semantics).
	Ignore *ignore.Matcher
	// FilesOnly leaves directories out of the results. A symbolic link is
	// returned when it names a file; one that cannot be followed (a dangling
	// link, a target that cannot be read) is handed to OnUnreadable instead.
	FilesOnly bool
	// OnUnreadable, when set with FilesOnly, receives each match whose type
	// cannot be read, with the error that says why.
	OnUnreadable func(rel string, err error)
	// OnDir, when set, is called with the running count of directories read.
	OnDir func(dirs int)
}

// ExpandGlobs resolves several patterns against one walk of root and returns
// the matches of each, in the order of patterns. Each list holds the
// slash-separated relative paths in walk order (lexical within a directory),
// with the same semantics as ExpandGlob, including the symbolic-link rules.
//
// The walk reads a directory only when some pattern can still match beneath
// it, and never descends into an excluded or ignored directory, so a recipe
// that names a few folders reads those folders rather than the whole tree, and
// a large dependency directory under an exclude costs one directory entry.
// A brace set is resolved in the same walk rather than once per alternative.
//
// A malformed pattern fails the whole call with doublestar.ErrBadPattern
// (which is path.ErrBadPattern). Filesystem errors on individual entries are
// skipped, as doublestar.Glob skips them.
func ExpandGlobs(root string, patterns []string, opts GlobOptions) ([][]string, error) {
	w := &globWalker{root: root, opts: opts, results: make([][]string, len(patterns))}
	for i, p := range patterns {
		if !doublestar.ValidatePattern(p) {
			return nil, doublestar.ErrBadPattern
		}
		for _, alt := range expandBraces(p) {
			w.addAlt(i, alt)
		}
	}
	for _, exc := range opts.Excludes {
		if prefix, ok := strings.CutSuffix(exc, "/**"); ok && prefix != "" {
			w.pruneExcl = append(w.pruneExcl, prefix)
		}
	}
	w.seen = make([]uint32, w.nstates)
	w.owned = make([]uint32, len(patterns))

	var start []globState
	w.stamp++
	for ai := range w.alts {
		start = w.addState(start, ai, 0)
	}
	// A pattern of `**` alone matches the root itself, as doublestar.Glob
	// reports it.
	if !opts.FilesOnly && !w.excluded(".") {
		for _, st := range start {
			if a := &w.alts[st.alt]; st.pos == len(a.segs) && !slices.Contains(w.results[a.owner], ".") {
				w.results[a.owner] = append(w.results[a.owner], ".")
			}
		}
	}
	if len(start) > 0 {
		w.walk("", root, start)
	}
	return w.results, nil
}

// errLinkedDir marks a symbolic link to a directory that the walk does not
// read through.
var errLinkedDir = errors.New("linked directory")

// globSeg is one slash-separated segment of a brace-free pattern.
type globSeg struct {
	pattern  string
	literal  string // the unescaped name when the segment has no wildcard
	isLit    bool
	starStar bool
}

// globAlt is one brace-free alternative of a pattern.
type globAlt struct {
	owner int // index of the pattern it came from
	segs  []globSeg
	// lit is the number of leading literal segments: the part of the pattern
	// before its first wildcard, through which a symbolic link is followed.
	lit  int
	base int // offset of this alternative's states in the stamp table
}

// globState is a position in an alternative: segs[pos] is the next segment to
// match.
type globState struct{ alt, pos int }

type globWalker struct {
	root      string
	opts      GlobOptions
	alts      []globAlt
	nstates   int
	pruneExcl []string
	results   [][]string

	seen  []uint32 // per state: the stamp of the last set it was added to
	owned []uint32 // per pattern: the stamp of the last entry it matched
	stamp uint32
	dirs  int
}

func (w *globWalker) addAlt(owner int, pattern string) {
	parts := strings.Split(pattern, "/")
	segs := make([]globSeg, 0, len(parts))
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			// doublestar.Glob returns nothing for a rooted pattern or one that
			// steps through `.`/`..`; an alternative like that matches nothing.
			return
		}
		s := globSeg{pattern: p, starStar: p == "**"}
		if !s.starStar && !hasMeta(p) {
			s.isLit = true
			s.literal = unescape(p)
		}
		segs = append(segs, s)
	}
	lit := 0
	for lit < len(segs) && segs[lit].isLit {
		lit++
	}
	w.alts = append(w.alts, globAlt{owner: owner, segs: segs, lit: lit, base: w.nstates})
	w.nstates += len(segs) + 1
}

// addState adds (alt, pos) and, through any `**` at pos (which may match no
// segment at all), the positions after it.
func (w *globWalker) addState(set []globState, alt, pos int) []globState {
	a := &w.alts[alt]
	for {
		id := a.base + pos
		if w.seen[id] == w.stamp {
			return set
		}
		w.seen[id] = w.stamp
		set = append(set, globState{alt, pos})
		if pos < len(a.segs) && a.segs[pos].starStar {
			pos++
			continue
		}
		return set
	}
}

// advance returns the states reached by consuming the entry name from states.
func (w *globWalker) advance(states []globState, name string) []globState {
	w.stamp++
	var next []globState
	for _, st := range states {
		a := &w.alts[st.alt]
		if st.pos >= len(a.segs) {
			continue
		}
		seg := &a.segs[st.pos]
		switch {
		case seg.starStar:
			// `**` consumes this segment and may consume more.
			next = w.addState(next, st.alt, st.pos)
		case seg.isLit:
			if seg.literal == name {
				next = w.addState(next, st.alt, st.pos+1)
			}
		default:
			if ok, _ := doublestar.Match(seg.pattern, name); ok {
				next = w.addState(next, st.alt, st.pos+1)
			}
		}
	}
	return next
}

func (w *globWalker) walk(rel, abs string, states []globState) {
	entries, err := os.ReadDir(abs)
	if err != nil {
		return
	}
	w.dirs++
	if w.opts.OnDir != nil {
		w.opts.OnDir(w.dirs)
	}
	for _, e := range entries {
		name := e.Name()
		next := w.advance(states, name)
		if len(next) == 0 {
			continue
		}
		childRel := name
		if rel != "" {
			childRel = rel + "/" + name
		}
		childAbs := abs + string(os.PathSeparator) + name

		isDir := e.IsDir()
		isLink := e.Type()&os.ModeSymlink != 0
		var linkErr error
		if isLink && (w.opts.FilesOnly || w.followsLink(states, name)) {
			info, serr := os.Stat(childAbs)
			switch {
			case serr != nil:
				linkErr = serr
			case info.IsDir() && w.followsLink(states, name):
				isDir = true
			case info.IsDir():
				// A linked directory past the first wildcard is not read
				// through, and it is not a file either.
				linkErr = errLinkedDir
			}
		}

		if w.opts.Ignore != nil && w.opts.Ignore.Match(childRel, isDir) {
			continue
		}
		if isDir && w.prunedByExclude(childRel) {
			continue
		}

		// emit is decided once per entry: whether a completed match is kept.
		emit := !w.opts.FilesOnly || (!isDir && linkErr == nil)
		checked, unreadable := false, false
		descend := false
		w.stamp++
		for _, st := range next {
			a := &w.alts[st.alt]
			if st.pos < len(a.segs) {
				descend = true
				continue
			}
			if w.owned[a.owner] == w.stamp {
				continue
			}
			w.owned[a.owner] = w.stamp
			if !checked {
				checked = true
				if w.excluded(childRel) {
					emit = false
				} else if w.opts.FilesOnly && linkErr != nil && !errors.Is(linkErr, errLinkedDir) {
					unreadable = true
				}
			}
			if emit {
				w.results[a.owner] = append(w.results[a.owner], childRel)
			}
		}
		if unreadable && w.opts.OnUnreadable != nil {
			w.opts.OnUnreadable(childRel, linkErr)
		}
		if descend && isDir {
			w.walk(childRel, childAbs, next)
		}
	}
}

// followsLink reports whether a symbolic link named name is read through: some
// pattern names it before its first wildcard.
func (w *globWalker) followsLink(states []globState, name string) bool {
	for _, st := range states {
		a := &w.alts[st.alt]
		if st.pos < a.lit && a.segs[st.pos].literal == name {
			return true
		}
	}
	return false
}

func (w *globWalker) prunedByExclude(rel string) bool {
	for _, p := range w.pruneExcl {
		if ok, _ := doublestar.Match(p, rel); ok {
			return true
		}
	}
	return false
}

func (w *globWalker) excluded(rel string) bool {
	for _, exc := range w.opts.Excludes {
		if ok, _ := doublestar.Match(exc, rel); ok {
			return true
		}
	}
	return false
}

// hasMeta reports whether a brace-free segment holds an unescaped wildcard.
func hasMeta(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '*', '?', '[':
			return true
		}
	}
	return false
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// expandBraces rewrites a pattern into the brace-free alternatives it
// matches: `a/{b,c/d}/*.{x,y}` becomes four patterns. Nested sets expand from
// the inside out; an escaped brace and a brace inside a character class are
// literal, and a `}` with no opening brace is literal, as doublestar.Match
// treats them.
func expandBraces(p string) []string {
	open, end := findBraceSet(p)
	if open < 0 {
		return []string{p}
	}
	prefix, body, suffix := p[:open], p[open+1:end], p[end+1:]
	var out []string
	for _, alt := range splitTopLevel(body) {
		out = append(out, expandBraces(prefix+alt+suffix)...)
	}
	return out
}

// findBraceSet returns the indexes of the first top-level `{` and its matching
// `}`, or -1, -1.
func findBraceSet(p string) (int, int) {
	depth, open := 0, -1
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '\\':
			i++
		case '[':
			if j := classEnd(p, i); j > 0 {
				i = j
			}
		case '{':
			if depth == 0 {
				open = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 {
					return open, i
				}
			}
		}
	}
	return -1, -1
}

// classEnd returns the index of the `]` closing the class that opens at i, or
// -1.
func classEnd(p string, i int) int {
	j := i + 1
	if j < len(p) && (p[j] == '^' || p[j] == '!') {
		j++
	}
	if j < len(p) && p[j] == ']' {
		j++
	}
	for ; j < len(p); j++ {
		switch p[j] {
		case '\\':
			j++
		case ']':
			return j
		}
	}
	return -1
}

// splitTopLevel splits a brace body at the commas outside any nested set.
func splitTopLevel(body string) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '\\':
			i++
		case '[':
			if j := classEnd(body, i); j > 0 {
				i = j
			}
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, body[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, body[start:])
}

// ExpandItems expands the source pattern of each item, its path with `{lang}`
// set to the item's source language (ResolvePathPattern), against one walk of
// root, and returns the matches of each in the order of items.
//
// An item whose pattern is malformed gets no matches, and err names the first
// such item, its collection and its pattern, wrapping doublestar.ErrBadPattern.
// The other items are expanded either way, so a caller that skips a bad item
// can use the result and a caller that fails on one returns err.
func ExpandItems(p *KapiProject, root string, items []IteratedItem, opts GlobOptions) ([][]string, error) {
	var badErr error
	patterns := make([]string, 0, len(items))
	index := make([]int, len(items))
	for i, it := range items {
		lang := string(it.Item.ResolvedSourceLanguage(it.Collection, p.Defaults))
		pattern := ResolvePathPattern(it.Item.Path, lang)
		if !doublestar.ValidatePattern(pattern) {
			index[i] = -1
			if badErr == nil {
				where := "content"
				if it.Collection != nil && it.Collection.Name != "" {
					where = fmt.Sprintf("content collection %q", it.Collection.Name)
				}
				badErr = fmt.Errorf("%s: pattern %q cannot be expanded, so its content would resolve to nothing. Fix the pattern in the recipe: %w",
					where, it.Item.Path, doublestar.ErrBadPattern)
			}
			continue
		}
		index[i] = len(patterns)
		patterns = append(patterns, pattern)
	}
	expanded, err := ExpandGlobs(root, patterns, opts)
	if err != nil {
		return nil, err
	}
	out := make([][]string, len(items))
	for i, k := range index {
		if k >= 0 {
			out[i] = expanded[k]
		}
	}
	return out, badErr
}
