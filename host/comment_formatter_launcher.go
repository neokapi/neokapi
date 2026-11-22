package host

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// A node_modules/.bin launcher is the shell script pnpm writes in place of a
// package's bin; it execs node on the package's script. On POSIX systems npm
// and Yarn link the bin to the script itself, which has a `#!` line instead.
//
// kapi reads a launcher only when the whole file equals a template in
// launchers/, one per pnpm release range, where only the script path, the
// NODE_PATH value and the cmd-shim-target comment may differ. Any other
// difference, of one line or one byte, means its formatter is not run.

//go:embed launchers/*.sh.tmpl
var launcherTemplateFS embed.FS

// launcherLayout is one template and how the launcher it matches finds its
// directory.
type launcherLayout struct {
	// file is the template's name under launchers/.
	file string
	// resolvesLinks is set when the launcher resolves $0 through symbolic
	// links before taking its directory, as pnpm's launchers do from 11.28
	// and 12.0. Older ones take the directory of $0 as invoked.
	resolvesLinks bool
}

var launcherLayouts = []launcherLayout{
	{file: "cmdshim-pnpm11.5.sh.tmpl"},
	{file: "cmdshim-pnpm11.13.sh.tmpl"},
	{file: "cmdshim-pnpm11.28.sh.tmpl", resolvesLinks: true},
	{file: "cmdshim-pnpm12.0.sh.tmpl", resolvesLinks: true},
	{file: "cmdshim-pnpm12.11.sh.tmpl", resolvesLinks: true},
	{file: "cmdshim-pnpm12.19.sh.tmpl", resolvesLinks: true},
}

// The parts of a template that differ between launchers, and what each may
// hold.
const (
	// The script, relative to the launcher's directory: one or more leading
	// `..` and then names. The characters exclude everything a shell would
	// expand inside single or double quotes.
	launcherTargetPattern = `([A-Za-z0-9@._+/-]+)`
	// The NODE_PATH value: one shell word of single-quoted text, double-quoted
	// text expanding only $basedir_abs or $NODE_PATH, and unquoted path
	// characters. It names where the script's modules resolve and not what
	// runs. It cannot hold a command substitution or end the assignment.
	launcherNodePathPattern = `((?:'[^'\n]*'|"(?:[A-Za-z0-9@._+/:-]|\$basedir_abs|\$NODE_PATH)*"|[A-Za-z0-9@._+/:-])+)`
	// The cmd-shim-target comment, which nothing executes.
	launcherShimTargetPattern = `[A-Za-z0-9@._+/-]+`
)

type compiledLayout struct {
	launcherLayout
	re *regexp.Regexp
	// targets is the index of each TARGET group in re.
	targets []int
}

var (
	compileLayoutsOnce sync.Once
	compiledLayouts    []compiledLayout
	compileLayoutsErr  error
)

// layouts compiles the templates once.
func layouts() ([]compiledLayout, error) {
	compileLayoutsOnce.Do(func() {
		placeholder := regexp.MustCompile(`\{\{(TARGET|NODE_PATH|SHIM_TARGET)\}\}`)
		for _, l := range launcherLayouts {
			text, err := launcherTemplateFS.ReadFile("launchers/" + l.file)
			if err != nil {
				compileLayoutsErr = err
				return
			}
			var b strings.Builder
			var targets []int
			group := 0
			b.WriteString(`\A`)
			last := 0
			for _, m := range placeholder.FindAllSubmatchIndex(text, -1) {
				b.WriteString(regexp.QuoteMeta(string(text[last:m[0]])))
				switch string(text[m[2]:m[3]]) {
				case "TARGET":
					group++
					targets = append(targets, group)
					b.WriteString(launcherTargetPattern)
				case "NODE_PATH":
					group++
					b.WriteString(launcherNodePathPattern)
				case "SHIM_TARGET":
					b.WriteString(launcherShimTargetPattern)
				}
				last = m[1]
			}
			b.WriteString(regexp.QuoteMeta(string(text[last:])))
			b.WriteString(`\z`)
			if len(targets) == 0 {
				compileLayoutsErr = fmt.Errorf("launcher template %s names no script", l.file)
				return
			}
			compiledLayouts = append(compiledLayouts, compiledLayout{launcherLayout: l, re: regexp.MustCompile(b.String()), targets: targets})
		}
	})
	return compiledLayouts, compileLayoutsErr
}

// isLauncher reports whether data looks like a cmd-shim launcher, which kapi
// then reads only by recognising all of it.
func isLauncher(data []byte) bool {
	return bytes.Contains(data, []byte("basedir")) || bytes.Contains(data, []byte("cmd-shim")) || bytes.Contains(data, []byte("NODE_PATH"))
}

// readLauncher reads the script and interpreter of the launcher at path whose
// whole contents are data. It returns an error unless data matches a template
// exactly and names one script.
func readLauncher(path string, data []byte) (formatterRuntime, error) {
	all, err := layouts()
	if err != nil {
		return formatterRuntime{}, err
	}
	for _, l := range all {
		m := l.re.FindSubmatch(data)
		if m == nil {
			continue
		}
		target := string(m[l.targets[0]])
		for _, i := range l.targets[1:] {
			if string(m[i]) != target {
				return formatterRuntime{}, fmt.Errorf("it matches %s but names two scripts, %s and %s", l.file, target, m[i])
			}
		}
		if err := checkLauncherTarget(target); err != nil {
			return formatterRuntime{}, err
		}
		// The launcher's directory as the shell reaches it. A path the shell
		// resolves as <dir>/../<name> leaves dir through its physical parent,
		// so the directory is taken with every link resolved.
		from := path
		if l.resolvesLinks {
			if from, err = filepath.EvalSymlinks(path); err != nil {
				return formatterRuntime{}, fmt.Errorf("resolve its location: %w", err)
			}
		}
		dir, err := filepath.EvalSymlinks(filepath.Dir(from))
		if err != nil {
			return formatterRuntime{}, fmt.Errorf("resolve its directory: %w", err)
		}
		rt := formatterRuntime{script: filepath.Join(dir, filepath.FromSlash(target))}
		if node := filepath.Join(dir, "node"); isExecutableFile(node) {
			rt.interpreter = node
		} else if node, ok := lookPathAbsolute("node"); ok {
			rt.interpreter = node
		}
		return rt, nil
	}
	return formatterRuntime{}, errors.New("it is not the launcher of any pnpm release kapi knows, so kapi cannot tell what it runs")
}

// checkLauncherTarget accepts a script path of one or more leading `..`
// followed by names, none of them `.` or `..`.
func checkLauncherTarget(target string) error {
	parts := strings.Split(target, "/")
	i := 0
	for i < len(parts) && parts[i] == ".." {
		i++
	}
	if i == 0 || i == len(parts) {
		return fmt.Errorf("its script %q does not leave the launcher's directory and name a file", target)
	}
	for _, p := range parts[i:] {
		if p == "" || p == "." || p == ".." {
			return fmt.Errorf("its script %q has a %q component after its leading \"..\" components", target, p)
		}
	}
	return nil
}
