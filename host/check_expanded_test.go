package host

import (
	"io"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// noReaderSources returns the files a report lists as not read.
func noReaderSources(report check.Report) []string {
	var out []string
	for _, w := range report.Warnings {
		if w.Code == check.WarningFormatNoReader {
			out = append(out, filepath.Base(w.Source))
		}
	}
	slices.Sort(out)
	return out
}

// rulesOnFile returns the rule of each finding on the file named base.
func rulesOnFile(report check.Report, base string) []string {
	var out []string
	for _, d := range report.Findings {
		if filepath.Base(d.Location.File) == base {
			out = append(out, d.Rule)
		}
	}
	return out
}

// TestCheckExpandedInputs checks that a directory or a glob argument reports a
// file it found and cannot read, and goes on to check the others, while a file
// named exactly still fails the check.
func TestCheckExpandedInputs(t *testing.T) {
	const good = `{"title":"TODO write this"}`
	const brokenYAML = "title: [unclosed\n  - nested: {\n"

	tests := []struct {
		name  string
		files map[string]string
		// args are slash paths relative to the tree's root.
		args []string
		// wantErr is set when the check ends with an error.
		wantErr bool
		// wantNoReader are the files reported under format.no_reader.
		wantNoReader []string
		// wantUnreadable are the files with a structure.unreadable finding.
		wantUnreadable []string
		// wantChecked is a file whose own findings the report holds.
		wantChecked string
	}{
		{
			name:         "directory with a file no format claims",
			files:        map[string]string{"src/app.json": good, "src/go.mod": "module x\n", "src/LICENSE": "text"},
			args:         []string{"src"},
			wantNoReader: []string{"LICENSE", "go.mod"},
			wantChecked:  "app.json",
		},
		{
			name:         "directory with a file whose comment reader is not installed",
			files:        map[string]string{"src/app.json": good, "src/run.sh": "# Run it.\necho hi\n"},
			args:         []string{"src"},
			wantNoReader: []string{"run.sh"},
			wantChecked:  "app.json",
		},
		{
			name:         "glob with a file no format claims",
			files:        map[string]string{"src/app.json": good, "src/sub/go.mod": "module x\n"},
			args:         []string{"src/**"},
			wantNoReader: []string{"go.mod"},
			wantChecked:  "app.json",
		},
		{
			name:    "file named exactly with no reader",
			files:   map[string]string{"src/go.mod": "module x\n"},
			args:    []string{"src/go.mod"},
			wantErr: true,
		},
		{
			name:    "file named exactly beside a directory",
			files:   map[string]string{"src/app.json": good, "go.mod": "module x\n"},
			args:    []string{"go.mod", "src"},
			wantErr: true,
		},
		{
			name:    "file named exactly that the directory also holds",
			files:   map[string]string{"src/app.json": good, "src/go.mod": "module x\n"},
			args:    []string{"src", "src/go.mod"},
			wantErr: true,
		},
		{
			name:           "directory with a malformed file",
			files:          map[string]string{"src/app.json": good, "src/broken.yaml": brokenYAML},
			args:           []string{"src"},
			wantUnreadable: []string{"broken.yaml"},
			wantChecked:    "app.json",
		},
		{
			name:    "malformed file named exactly",
			files:   map[string]string{"src/broken.yaml": brokenYAML},
			args:    []string{"src/broken.yaml"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateCheckExecution(t)
			root := t.TempDir()
			writeTree(t, root, tt.files)
			t.Chdir(root)
			app := &App{SourceLang: "en"}
			cmd := executionCommand(t)
			cmd.Flags().StringSlice("forbid", []string{"TODO"}, "")
			cmd.SetErr(io.Discard)

			args := make([]string, 0, len(tt.args))
			for _, a := range tt.args {
				args = append(args, filepath.FromSlash(a))
			}
			report, err := app.ComputeCheck(cmd, args)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantNoReader, noReaderSources(report))
			for _, base := range tt.wantUnreadable {
				assert.Equal(t, []string{check.RuleUnreadable}, rulesOnFile(report, base))
			}
			if tt.wantChecked != "" {
				assert.NotEmpty(t, rulesOnFile(report, tt.wantChecked), "the readable file is checked")
			}
			assert.Equal(t, check.VerdictFailed, report.Verdict)
			for _, base := range tt.wantNoReader {
				assert.Empty(t, rulesOnFile(report, base), "a file not read has no finding")
			}
		})
	}
}

// TestCheckExpandedMalformedFileFailsTheGate checks that a malformed file a
// directory holds fails the run, which still reports the other files.
func TestCheckExpandedMalformedFileFailsTheGate(t *testing.T) {
	isolateCheckExecution(t)
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"src/app.json":    `{"title":"Hello world"}`,
		"src/broken.yaml": "title: [unclosed\n  - nested: {\n",
	})
	t.Chdir(root)
	app := &App{SourceLang: "en"}

	cmd := executionCommand(t)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	report, err := app.ComputeCheck(cmd, []string{"src"})
	require.NoError(t, err)
	assert.Equal(t, check.VerdictFailed, report.Verdict)
	require.Len(t, report.Findings, 1)
	assert.Equal(t, check.RuleUnreadable, report.Findings[0].Rule)
	assert.True(t, report.Findings[0].Fails)
	assert.Contains(t, report.Findings[0].Message, "broken.yaml")
	assert.Positive(t, report.Target.Blocks, "the readable file is checked")

	cmd = executionCommand(t)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err = app.RunCheck(cmd, []string{"src"})
	assert.NotEqual(t, ExitOK, ExitCode(cmd, err))
}

// TestExpandArgsHonoursIgnoreFiles checks that a directory or a glob argument
// leaves out what a .gitignore ignores, including the rules of a .gitignore
// above the argument in the same repository.
func TestExpandArgsHonoursIgnoreFiles(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		arg   string
		want  []string
	}{
		{
			name: "ignore file in the directory",
			files: map[string]string{
				"pkg/.gitignore":                   "node_modules/\n",
				"pkg/app.json":                     "{}",
				"pkg/node_modules/dep/index.json":  "{}",
				"pkg/sub/node_modules/x/info.json": "{}",
			},
			arg:  "pkg",
			want: []string{"pkg/.gitignore", "pkg/app.json"},
		},
		{
			name: "ignore file at the top of the repository",
			files: map[string]string{
				".git/HEAD":                             "ref: refs/heads/main\n",
				".gitignore":                            "node_modules/\n*.log\n",
				"packages/ui/app.json":                  "{}",
				"packages/ui/debug.log":                 "x",
				"packages/ui/node_modules/dep/pkg.json": "{}",
			},
			arg:  "packages",
			want: []string{"packages/ui/app.json"},
		},
		{
			name: "nested ignore file",
			files: map[string]string{
				"pkg/app.json":       "{}",
				"pkg/web/.gitignore": "/build\n",
				"pkg/web/build/a.md": "x",
				"pkg/web/page.md":    "x",
			},
			arg:  "pkg",
			want: []string{"pkg/app.json", "pkg/web/.gitignore", "pkg/web/page.md"},
		},
		{
			name: "glob",
			files: map[string]string{
				".git/HEAD":                       "ref: refs/heads/main\n",
				".gitignore":                      "node_modules/\n",
				"packages/ui/app.json":            "{}",
				"packages/ui/node_modules/d.json": "{}",
			},
			arg:  "packages/**/*.json",
			want: []string{"packages/ui/app.json"},
		},
		{
			name: "ignored directory named before the wildcard",
			files: map[string]string{
				".git/HEAD":                    "ref: refs/heads/main\n",
				".gitignore":                   "node_modules/\n",
				"ui/node_modules/dep/pkg.json": "{}",
			},
			arg:  "ui/node_modules/**/*.json",
			want: []string{"ui/node_modules/dep/pkg.json"},
		},
		{
			name: "ignored directory named as the argument",
			files: map[string]string{
				".git/HEAD":                    "ref: refs/heads/main\n",
				".gitignore":                   "node_modules/\n",
				"ui/node_modules/dep/pkg.json": "{}",
			},
			arg:  "ui/node_modules",
			want: []string{"ui/node_modules/dep/pkg.json"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, tt.files)
			t.Chdir(root)
			got, err := expandArgs([]string{filepath.FromSlash(tt.arg)}, InputOptions{})
			require.NoError(t, err)
			for i := range got {
				got[i] = filepath.ToSlash(got[i])
			}
			slices.Sort(got)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExpandArgsOnFound checks that OnFound receives the files an expansion
// selected and never one an argument names exactly.
func TestExpandArgsOnFound(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"src/a.json": "{}",
		"src/b.json": "{}",
		"c.json":     "{}",
	})
	t.Chdir(root)
	var found []string
	files, err := expandArgs([]string{"c.json", "src", filepath.Join("src", "b.json")}, InputOptions{
		OnFound: func(path string) { found = append(found, filepath.ToSlash(path)) },
	})
	require.NoError(t, err)
	assert.Len(t, files, 3)
	assert.Equal(t, []string{"src/a.json"}, found)
}

// TestCheckProjectMalformedFileIsAFinding checks that a check over the whole
// project reports a declared file that does not parse as a failing finding and
// checks the project's other files.
func TestCheckProjectMalformedFileIsAFinding(t *testing.T) {
	isolateCheckExecution(t)
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"kapi.yaml": `version: v1
name: malformed
defaults:
  source_language: en
collections:
  - name: content
    source_only: true
    content:
      - path: "content/*.json"
      - path: "content/*.yaml"
`,
		"content/app.json":    `{"title":"TODO write this"}`,
		"content/broken.yaml": "title: [unclosed\n  - nested: {\n",
	})
	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
	cmd.Flags().StringSlice("forbid", []string{"TODO"}, "")
	cmd.SetErr(io.Discard)
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, check.VerdictFailed, report.Verdict)
	assert.Equal(t, []string{check.RuleUnreadable}, rulesOnFile(report, "broken.yaml"))
	assert.NotEmpty(t, rulesOnFile(report, "app.json"), "the other file is checked")
}
