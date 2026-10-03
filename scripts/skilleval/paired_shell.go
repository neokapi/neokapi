package main

import (
	"path"
	"slices"
	"strings"
)

// The route audit asks which programs a shell command runs, not which words
// it mentions: `ls .agents/skills/kapi` lists the arm's own skill and runs
// ls. pairedExecutedWords reads a command the way a POSIX shell splits it,
// closely enough for that question. It is a reading aid for an audit, not a
// shell: a command built at run time (a variable holding a program name, a
// script written to a file and run) escapes it.

// pairedShellWrappers run the command that follows them. The value lists the
// options that take an argument, which is skipped with the option.
var pairedShellWrappers = map[string][]string{
	"env":        {"-u", "-C", "--unset", "--chdir"},
	"exec":       {"-a"},
	"command":    {},
	"builtin":    {},
	"nohup":      {},
	"time":       {"-o", "-f"},
	"nice":       {"-n"},
	"sudo":       {"-u", "-g", "-C", "-D", "-h", "-p", "-r", "-t", "-U"},
	"xargs":      {"-n", "-I", "-L", "-P", "-s", "-E", "-d", "-a", "-J", "-R", "-S"},
	"timeout":    {"-s", "-k", "--signal", "--kill-after"},
	"stdbuf":     {"-i", "-o", "-e"},
	"caffeinate": {"-t", "-w"},
}

// pairedShellNames run the script a -c option carries.
var pairedShellNames = []string{"sh", "bash", "zsh", "dash", "ksh"}

// pairedShellKeywords keep the next word in command position.
var pairedShellKeywords = []string{"if", "then", "else", "elif", "do", "while", "until", "!", "{"}

// pairedShellNonCommands start or end a compound command; the words after
// them are not programs until an operator starts another command.
var pairedShellNonCommands = []string{"fi", "done", "esac", "}", "for", "select", "case", "function"}

// pairedFindExec are find's options whose next word is a program.
var pairedFindExec = []string{"-exec", "-execdir", "-ok", "-okdir"}

type pairedShellToken struct {
	op   string // an operator, "redirect" for a redirection; "" for a word
	word string // a word with its quotes removed
}

// pairedExecutedWords lists the words a shell command runs as programs: the
// first word of each simple command, after variable assignments and wrappers
// such as env or xargs, in pipelines, lists, subshells, command and process
// substitutions, the scripts `sh -c` and `eval` run, and `find -exec`.
// Arguments, quoted text, redirection targets and here-document bodies are
// not programs.
func pairedExecutedWords(command string) []string {
	return pairedExecutedWordsDepth(command, 0)
}

func pairedExecutedWordsDepth(command string, depth int) []string {
	if depth > 8 {
		return nil
	}
	tokens, nested := pairedShellTokens(command)
	var out []string
	for _, script := range nested {
		out = append(out, pairedExecutedWordsDepth(script, depth+1)...)
	}
	var (
		commandPosition = true
		skipTarget      bool   // the next word is a redirection's target
		program         string // the base name of the simple command's program
		wrapper         string // the wrapper whose options are being read
		skipWords       int    // option arguments still to skip
		pendingDuration bool   // timeout's duration comes before its command
		runNext         bool   // the next word is a program (find -exec)
		scriptNext      bool   // the next word is a script (sh -c)
		inEval          bool
		evalWords       []string
	)
	reset := func() {
		if inEval && len(evalWords) > 0 {
			out = append(out, pairedExecutedWordsDepth(strings.Join(evalWords, " "), depth+1)...)
		}
		commandPosition, program, wrapper, skipWords, pendingDuration = true, "", "", 0, false
		runNext, scriptNext, inEval, evalWords = false, false, false, nil
	}
	for _, token := range tokens {
		if token.op == "redirect" {
			skipTarget = true
			continue
		}
		if token.op != "" {
			reset()
			continue
		}
		word := token.word
		switch {
		case skipTarget:
			skipTarget = false
			continue
		case inEval:
			evalWords = append(evalWords, word)
			continue
		case scriptNext:
			scriptNext = false
			out = append(out, pairedExecutedWordsDepth(word, depth+1)...)
			continue
		case runNext:
			runNext = false
			out = append(out, word)
			continue
		case !commandPosition:
			if program == "find" && slices.Contains(pairedFindExec, word) {
				runNext = true
			} else if pairedShellScriptOption(program, word) {
				scriptNext = true
			}
			continue
		case skipWords > 0:
			skipWords--
			continue
		case wrapper != "" && strings.HasPrefix(word, "-") && word != "-":
			if wrapper == "command" && (word == "-v" || word == "-V") {
				// command -v names a program without running it.
				commandPosition = false
			} else if slices.Contains(pairedShellWrappers[wrapper], word) {
				skipWords = 1
			}
			continue
		case pendingDuration:
			pendingDuration = false
			continue
		case pairedShellAssignment(word), slices.Contains(pairedShellKeywords, word):
			continue
		case slices.Contains(pairedShellNonCommands, word):
			commandPosition = false
			continue
		}
		out = append(out, word)
		base := path.Base(word)
		if _, ok := pairedShellWrappers[base]; ok {
			wrapper = base
			pendingDuration = base == "timeout"
			continue
		}
		if base == "eval" {
			inEval = true
			continue
		}
		program, wrapper, commandPosition = base, "", false
	}
	reset()
	return out
}

// pairedShellScriptOption reports whether word is the option of a shell that
// takes the script to run, -c or a cluster holding c such as -lc.
func pairedShellScriptOption(program, word string) bool {
	if !slices.Contains(pairedShellNames, program) || !strings.HasPrefix(word, "-") || strings.HasPrefix(word, "--") {
		return false
	}
	return strings.ContainsRune(word[1:], 'c')
}

// pairedShellAssignment reports whether word assigns a variable, NAME=value.
func pairedShellAssignment(word string) bool {
	name, _, ok := strings.Cut(word, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		letter := r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
		if !letter && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// pairedShellTokens splits a command into words and operators, and returns
// separately the commands that substitutions inside it run. Quotes are
// removed from words; a here-document's body is dropped.
func pairedShellTokens(command string) ([]pairedShellToken, []string) {
	s := &pairedShellScanner{runes: []rune(command)}
	s.scan()
	return s.tokens, s.nested
}

type pairedShellScanner struct {
	runes         []rune
	tokens        []pairedShellToken
	nested        []string
	word          strings.Builder
	inWord        bool
	heredocs      []pairedHeredoc
	wantDelimiter bool
	stripTabs     bool
}

type pairedHeredoc struct {
	delimiter string
	stripTabs bool
}

func (s *pairedShellScanner) endWord() {
	if !s.inWord {
		return
	}
	text := s.word.String()
	s.word.Reset()
	s.inWord = false
	if s.wantDelimiter {
		s.heredocs = append(s.heredocs, pairedHeredoc{delimiter: text, stripTabs: s.stripTabs})
		s.wantDelimiter = false
	}
	s.tokens = append(s.tokens, pairedShellToken{word: text})
}

func (s *pairedShellScanner) emit(op string) {
	s.endWord()
	s.tokens = append(s.tokens, pairedShellToken{op: op})
}

// substitution records the command a substitution runs and leaves a
// placeholder in the word.
func (s *pairedShellScanner) substitution(from, to int) {
	s.nested = append(s.nested, string(s.runes[from:to]))
	s.word.WriteString("_")
	s.inWord = true
}

func (s *pairedShellScanner) scan() {
	runes := s.runes
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		next := func(offset int) rune {
			if i+offset < len(runes) {
				return runes[i+offset]
			}
			return 0
		}
		switch {
		case r == '\\':
			if i+1 < len(runes) {
				i++
				if runes[i] != '\n' {
					s.word.WriteRune(runes[i])
					s.inWord = true
				}
			}
		case r == '\'':
			end := pairedIndexRune(runes, i+1, '\'')
			s.word.WriteString(string(runes[i+1 : end]))
			s.inWord = true
			i = end
		case r == '"':
			i = s.doubleQuoted(i + 1)
			s.inWord = true
		case r == '`':
			end := pairedIndexRune(runes, i+1, '`')
			s.substitution(i+1, end)
			i = end
		case r == '$' && next(1) == '(' && next(2) == '(':
			// Arithmetic: $(( … )) runs nothing.
			i = pairedMatchParen(runes, i+1)
			s.word.WriteString("0")
			s.inWord = true
		case r == '$' && next(1) == '(':
			end := pairedMatchParen(runes, i+1)
			s.substitution(i+2, end)
			i = end
		case (r == '<' || r == '>') && next(1) == '(':
			end := pairedMatchParen(runes, i+1)
			s.endWord()
			s.substitution(i+2, end)
			i = end
		case r == '#' && !s.inWord:
			for i+1 < len(runes) && runes[i+1] != '\n' {
				i++
			}
		case r == ' ' || r == '\t':
			s.endWord()
		case r == '\n':
			s.emit("\n")
			if len(s.heredocs) > 0 {
				i = pairedSkipHeredocs(runes, i+1, s.heredocs) - 1
				s.heredocs = nil
			}
		case r == ';' || r == '(' || r == ')':
			s.emit(string(r))
		case r == '&' && next(1) == '>':
			// &> and &>> send both streams to a file.
			s.endWord()
			for next(1) == '>' {
				i++
			}
			s.tokens = append(s.tokens, pairedShellToken{op: "redirect"})
		case r == '&' || r == '|':
			if next(1) == r || (r == '|' && next(1) == '&') {
				i++
			}
			s.emit(string(r))
		case r == '<' || r == '>':
			i = s.redirection(i)
		default:
			s.word.WriteRune(r)
			s.inWord = true
		}
	}
	s.endWord()
}

// redirection reads the redirection operator at i and returns the index of
// its last character. A here-document records its delimiter; a duplication
// such as 2>&1 takes no target.
func (s *pairedShellScanner) redirection(i int) int {
	runes := s.runes
	// A word of digits right before is the descriptor, not an argument.
	if s.inWord && pairedAllDigits(s.word.String()) {
		s.word.Reset()
		s.inWord = false
	}
	s.endWord()
	if runes[i] == '<' && i+2 < len(runes) && runes[i+1] == '<' && runes[i+2] == '<' {
		// A here-string: the next word is text.
		s.tokens = append(s.tokens, pairedShellToken{op: "redirect"})
		return i + 2
	}
	if runes[i] == '<' && i+1 < len(runes) && runes[i+1] == '<' {
		i++
		s.stripTabs = i+1 < len(runes) && runes[i+1] == '-'
		if s.stripTabs {
			i++
		}
		s.wantDelimiter = true
		return i
	}
	for i+1 < len(runes) && (runes[i+1] == '>' || runes[i+1] == '|' || runes[i+1] == '&') {
		i++
		if runes[i] != '&' {
			continue
		}
		// >&2 and >&- name a descriptor.
		if i+1 < len(runes) && (runes[i+1] == '-' || (runes[i+1] >= '0' && runes[i+1] <= '9')) {
			for i+1 < len(runes) && (runes[i+1] == '-' || (runes[i+1] >= '0' && runes[i+1] <= '9')) {
				i++
			}
			return i
		}
	}
	s.tokens = append(s.tokens, pairedShellToken{op: "redirect"})
	return i
}

// doubleQuoted copies a double-quoted string starting at start into the
// word, collecting the commands its substitutions run, and returns the index
// of the closing quote.
func (s *pairedShellScanner) doubleQuoted(start int) int {
	runes := s.runes
	for i := start; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '"':
			return i
		case r == '\\' && i+1 < len(runes):
			i++
			if runes[i] != '\n' {
				s.word.WriteRune(runes[i])
			}
		case r == '`':
			end := pairedIndexRune(runes, i+1, '`')
			s.substitution(i+1, end)
			i = end
		case r == '$' && i+1 < len(runes) && runes[i+1] == '(' && (i+2 >= len(runes) || runes[i+2] != '('):
			end := pairedMatchParen(runes, i+1)
			s.substitution(i+2, end)
			i = end
		default:
			s.word.WriteRune(r)
		}
	}
	return len(runes)
}

// pairedSkipHeredocs returns the index after the bodies of the pending
// here-documents that start at start.
func pairedSkipHeredocs(runes []rune, start int, heredocs []pairedHeredoc) int {
	i := start
	for _, doc := range heredocs {
		for i < len(runes) {
			end := pairedIndexRune(runes, i, '\n')
			line := string(runes[i:end])
			i = end + 1
			if doc.stripTabs {
				line = strings.TrimLeft(line, "\t")
			}
			if line == doc.delimiter {
				break
			}
		}
	}
	return min(i, len(runes))
}

// pairedMatchParen returns the index of the parenthesis that closes the one
// at open, skipping quoted text, or the length when none does.
func pairedMatchParen(runes []rune, open int) int {
	depth := 0
	for i := open; i < len(runes); i++ {
		switch runes[i] {
		case '\\':
			i++
		case '\'':
			i = pairedIndexRune(runes, i+1, '\'')
		case '"':
			inner := &pairedShellScanner{runes: runes}
			i = inner.doubleQuoted(i + 1)
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(runes)
}

// pairedIndexRune returns the index of the first r at or after start, or the
// length when there is none, so an unterminated quote ends the command.
func pairedIndexRune(runes []rune, start int, r rune) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == r {
			return i
		}
	}
	return len(runes)
}

func pairedAllDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
