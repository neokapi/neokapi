package host

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// A node_modules/.bin launcher is the POSIX shell script a package manager's
// cmd-shim writes in place of a package's bin. It computes $basedir from its
// own location and execs node on the package's script. kapi reads two layouts:
//
//   - npm's cmd-shim and pnpm 11 set basedir to the directory of $0 as
//     invoked, and exec node on "$basedir/<relative path>" or on an absolute
//     path.
//   - pnpm 12 resolves $0 through symbolic links, sets basedir_abs to the
//     physical directory (`cd -P`), and execs node on
//     "$basedir_abs"/'<relative path>'. It also carries Windows branches that
//     use $basedir_win and node.exe.
//
// Anything else in a file that sets basedir is refused rather than guessed.

// The basedir assignments of the layouts kapi reads. Each must appear as a line
// of its own.
const (
	launcherBasedirNPM     = `basedir=$(dirname "$(echo "$0" | sed -e 's,\\,/,g')")`
	launcherBasedirPlain   = `basedir=$(dirname "$0")`
	launcherBasedirAbsPnpm = `basedir_abs=$(CDPATH= cd -P -- "$basedir" && pwd -P) || exit $?`
	launcherResolveLinks   = `while [ -L "$link" ] && [ "$hops" -lt 40 ]; do`
)

// isLauncher reports whether data is a cmd-shim launcher: a script with a line
// that assigns basedir.
func isLauncher(data []byte) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.HasPrefix(bytes.TrimSpace(line), []byte("basedir=")) {
			return true
		}
	}
	return false
}

// launcherWord is one shell word of an exec line, as a sequence of literal
// text and variable references.
type launcherWord []launcherSegment

type launcherSegment struct {
	literal string
	// variable names a variable the segment expands; literal is then empty.
	variable string
}

// readLauncher reads the target and interpreter of the launcher at path whose
// contents are data. Every exec line must have a form readLauncher knows, and
// every POSIX exec line must name the same script; otherwise it returns an
// error naming what it could not read.
func readLauncher(path string, data []byte) (formatterRuntime, error) {
	lines := strings.Split(string(data), "\n")
	has := func(want string) bool {
		for _, line := range lines {
			if strings.TrimSpace(line) == want {
				return true
			}
		}
		return false
	}

	// What $basedir and $basedir_abs expand to when the launcher reaches its
	// exec lines.
	var basedir, basedirAbs string
	switch {
	case has(launcherBasedirAbsPnpm):
		if !has(launcherResolveLinks) {
			return formatterRuntime{}, errors.New("it sets basedir_abs without resolving $0 through symbolic links")
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return formatterRuntime{}, fmt.Errorf("resolve its location: %w", err)
		}
		physical, err := filepath.EvalSymlinks(filepath.Dir(resolved))
		if err != nil {
			return formatterRuntime{}, fmt.Errorf("resolve its directory: %w", err)
		}
		if !has(`basedir="$basedir_abs"`) {
			return formatterRuntime{}, errors.New("it does not set basedir to basedir_abs")
		}
		basedir, basedirAbs = physical, physical
	case has(launcherBasedirNPM), has(launcherBasedirPlain):
		basedir = filepath.Dir(path)
	default:
		return formatterRuntime{}, errors.New("its basedir assignment is not one kapi knows")
	}

	var script string
	localNode := false
	posix := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "exec" && !strings.HasPrefix(trimmed, "exec ") {
			continue
		}
		words, err := splitLauncherWords(strings.TrimPrefix(trimmed, "exec"))
		if err != nil {
			return formatterRuntime{}, fmt.Errorf("line %d: %w", i+1, err)
		}
		if len(words) != 3 || !words[2].is(launcherSegment{variable: "@"}) {
			return formatterRuntime{}, fmt.Errorf("line %d: an exec line that is not `exec <node> <script> \"$@\"`", i+1)
		}
		node, target := words[0], words[1]
		var windows bool
		switch {
		case node.is(launcherSegment{literal: "node"}):
		case node.is(launcherSegment{variable: "basedir"}, launcherSegment{literal: "/node"}):
			localNode = true
		case node.is(launcherSegment{literal: "node.exe"}),
			node.is(launcherSegment{variable: "basedir"}, launcherSegment{literal: "/node.exe"}):
			windows = true
		default:
			return formatterRuntime{}, fmt.Errorf("line %d: it runs something other than node", i+1)
		}
		resolved, targetWindows, err := resolveLauncherTarget(target, basedir, basedirAbs)
		if err != nil {
			return formatterRuntime{}, fmt.Errorf("line %d: %w", i+1, err)
		}
		if windows || targetWindows {
			continue
		}
		posix++
		if script == "" {
			script = resolved
		} else if script != resolved {
			return formatterRuntime{}, fmt.Errorf("its exec lines name two scripts, %s and %s", script, resolved)
		}
	}
	if posix == 0 {
		return formatterRuntime{}, errors.New("it has no exec line that runs node on a script")
	}

	rt := formatterRuntime{script: script}
	if node := filepath.Join(basedir, "node"); localNode && isExecutableFile(node) {
		rt.interpreter = node
	} else if node, ok := lookPathAbsolute("node"); ok {
		rt.interpreter = node
	}
	return rt, nil
}

// resolveLauncherTarget resolves the script word of an exec line. It reports
// windows for a target under $basedir_win, which only the Windows branches use.
func resolveLauncherTarget(word launcherWord, basedir, basedirAbs string) (path string, windows bool, err error) {
	if len(word) == 0 {
		return "", false, errors.New("an empty script")
	}
	var base string
	rest := word
	if v := word[0].variable; v != "" {
		switch {
		case v == "basedir":
			base = basedir
		case v == "basedir_abs" && basedirAbs != "":
			base = basedirAbs
		case v == "basedir_win" && basedirAbs != "":
			windows = true
		default:
			return "", false, fmt.Errorf("its script starts with $%s, which kapi does not resolve", v)
		}
		rest = word[1:]
	}
	var b strings.Builder
	for _, seg := range rest {
		if seg.variable != "" {
			return "", false, fmt.Errorf("its script expands $%s after its start", seg.variable)
		}
		b.WriteString(seg.literal)
	}
	tail := b.String()
	if windows {
		return "", true, nil
	}
	if base == "" {
		if !filepath.IsAbs(tail) {
			return "", false, fmt.Errorf("its script %q is neither absolute nor under $basedir", tail)
		}
		return filepath.Clean(tail), false, nil
	}
	if !strings.HasPrefix(tail, "/") {
		return "", false, fmt.Errorf("its script %q does not continue $basedir with a path", tail)
	}
	return filepath.Clean(filepath.Join(base, filepath.FromSlash(tail))), false, nil
}

// is reports whether w consists of exactly the given segments.
func (w launcherWord) is(segs ...launcherSegment) bool {
	if len(w) != len(segs) {
		return false
	}
	for i := range w {
		if w[i] != segs[i] {
			return false
		}
	}
	return true
}

// splitLauncherWords splits the arguments of an exec line into words. It reads
// unquoted text, single quotes and double quotes holding text and plain $name
// references, and nothing else: an escape, a command substitution, a ${...}
// expansion or an unquoted $ is an error.
func splitLauncherWords(s string) ([]launcherWord, error) {
	var words []launcherWord
	var cur launcherWord
	inWord := false
	lit := func(text string) {
		if text == "" {
			return
		}
		if n := len(cur); n > 0 && cur[n-1].variable == "" {
			cur[n-1].literal += text
			return
		}
		cur = append(cur, launcherSegment{literal: text})
	}
	end := func() {
		if inWord {
			words = append(words, cur)
		}
		cur, inWord = nil, false
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			end()
			i++
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, errors.New("an unterminated single quote")
			}
			inWord = true
			lit(s[i+1 : i+1+j])
			i += j + 2
		case c == '"':
			j := strings.IndexByte(s[i+1:], '"')
			if j < 0 {
				return nil, errors.New("an unterminated double quote")
			}
			inWord = true
			if err := splitDoubleQuoted(s[i+1:i+1+j], &cur, lit); err != nil {
				return nil, err
			}
			i += j + 2
		case strings.IndexByte("\\`$;&|<>(){}*?[]~#", c) >= 0:
			return nil, fmt.Errorf("unquoted %q, which kapi does not interpret", c)
		default:
			inWord = true
			j := i
			for j < len(s) && strings.IndexByte(" \t'\"\\`$;&|<>(){}*?[]~#", s[j]) < 0 {
				j++
			}
			lit(s[i:j])
			i = j
		}
	}
	end()
	return words, nil
}

// splitDoubleQuoted appends the contents of a double-quoted string to cur:
// literal text and $name references, where name is a run of letters, digits
// and underscores, or @.
func splitDoubleQuoted(s string, cur *launcherWord, lit func(string)) error {
	for i := 0; i < len(s); {
		switch s[i] {
		case '\\', '`':
			return fmt.Errorf("%q inside double quotes, which kapi does not interpret", s[i])
		case '$':
			j := i + 1
			if j < len(s) && s[j] == '@' {
				j++
			} else {
				for j < len(s) && (s[j] == '_' || s[j] >= 'a' && s[j] <= 'z' || s[j] >= 'A' && s[j] <= 'Z' || s[j] >= '0' && s[j] <= '9') {
					j++
				}
			}
			if j == i+1 {
				return errors.New("a $ inside double quotes that is not a plain variable reference")
			}
			*cur = append(*cur, launcherSegment{variable: s[i+1 : j]})
			i = j
		default:
			j := i
			for j < len(s) && s[j] != '$' && s[j] != '\\' && s[j] != '`' {
				j++
			}
			lit(s[i:j])
			i = j
		}
	}
	return nil
}
