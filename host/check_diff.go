package host

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/diffscope"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
)

// readScopedSource reads a file a diff-scoped check covers. A test replaces it
// to show that a file the diff does not name is never read.
var readScopedSource = os.ReadFile

// diffSource is a parsed diff and the directory its paths are relative to.
type diffSource struct {
	// label says where the diff came from, for the report.
	label string
	root  string
	files []diffscope.File
}

// diffSourceFromFlags reads the diff named by --diff-file or --diff-against, or
// returns nil when the check is not scoped to a diff.
func (a *App) diffSourceFromFlags(cmd Command) (*diffSource, error) {
	file, _ := cmd.Flags().GetString("diff-file")
	against, _ := cmd.Flags().GetString("diff-against")
	ctx := CmdContext(cmd)
	switch {
	case file != "" && against != "":
		return nil, errors.New("--diff-file and --diff-against each name a diff; pass one")
	case file != "":
		var data []byte
		var err error
		if file == StdinName {
			data, err = io.ReadAll(cmd.InOrStdin())
		} else {
			data, err = os.ReadFile(file)
		}
		if err != nil {
			return nil, fmt.Errorf("read the diff %s: %w", DisplayName(file), err)
		}
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		return parsedDiff(ctx, file, data, cwd)
	case against != "":
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		return gitDiffAgainst(ctx, cwd, against)
	}
	return nil, nil
}

// parsedDiff parses a supplied diff. git writes paths relative to the top of
// the work tree whatever directory it ran in, so inside a work tree they resolve
// against its top, and elsewhere against dir.
func parsedDiff(ctx context.Context, label string, data []byte, dir string) (*diffSource, error) {
	files, err := diffscope.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse the diff %s: %w", DisplayName(label), err)
	}
	root := dir
	if top, err := gitOutput(ctx, dir, "rev-parse", "--show-toplevel"); err == nil {
		root = strings.TrimSpace(string(top))
	}
	return &diffSource{label: label, root: root, files: files}, nil
}

// gitDiffAgainst diffs the working tree in dir against rev, read-only, and adds
// every untracked file as wholly added: an untracked file is part of the change
// the working tree holds, and git diff does not show it.
func gitDiffAgainst(ctx context.Context, dir, rev string) (*diffSource, error) {
	if strings.HasPrefix(rev, "-") {
		return nil, fmt.Errorf("%q is not a revision", rev)
	}
	top, err := gitOutput(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("--diff-against needs a git work tree: %w", err)
	}
	root := strings.TrimSpace(string(top))
	// Every option that user configuration could change about the output is
	// pinned: colour, external diff drivers, text conversion and path prefixes.
	out, err := gitOutput(ctx, root, "diff", "--no-color", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/", "-M", "--end-of-options", rev, "--")
	if err != nil {
		return nil, fmt.Errorf("git diff %s: %w", rev, err)
	}
	files, err := diffscope.Parse(out)
	if err != nil {
		return nil, fmt.Errorf("parse git diff %s: %w", rev, err)
	}
	untracked, err := gitOutput(ctx, root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("list untracked files: %w", err)
	}
	for path := range strings.SplitSeq(string(untracked), "\x00") {
		if path == "" {
			continue
		}
		files = append(files, diffscope.File{
			NewPath: path,
			Status:  diffscope.Added,
			Changes: []diffscope.Change{{Lines: format.LineRange{First: 1, Last: math.MaxInt}}},
		})
	}
	return &diffSource{label: "git diff " + rev, root: root, files: files}, nil
}

func gitOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	return out, nil
}

// diffCheckRun is what one diff-scoped check resolves before it reads a file.
type diffCheckRun struct {
	src *diffSource
	// named narrows the check to these files when non-empty.
	named []string
	cmd   Command
	opts  checkRunOptions
	// voice and vocab resolve governance per file. voice is nil when a profile
	// was named explicitly and opts already holds it.
	voice *checkVoice
	vocab *checkTerms
	gate  check.Gate
}

// runDiffCheck checks the content blocks a diff touches, each block whole, and
// reports every file the diff names with what became of it.
//
// Inside a project only declared content is checked, and a changed file outside
// it is listed as out of scope. Outside a project every file a format reads is
// checked. A file whose touched content cannot be located did not run, and so
// does the check as a whole. A file the diff does not name is never read.
func (a *App) runDiffCheck(ctx context.Context, run diffCheckRun) (check.Report, error) {
	declared, recipe, err := a.declaredContent(run.cmd)
	if err != nil {
		return check.Report{}, err
	}
	var named map[string]bool
	if len(run.named) > 0 {
		named = map[string]bool{}
		for _, n := range run.named {
			named[scopeKey(n)] = true
		}
	}

	files := slices.Clone(run.src.files)
	slices.SortStableFunc(files, func(x, y diffscope.File) int { return strings.Compare(x.Path(), y.Path()) })
	scope := &check.Scope{Diff: run.src.label, Files: []check.ScopeFile{}}
	var diags []check.Diagnostic
	checked := 0
	for _, f := range files {
		abs := filepath.Join(run.src.root, filepath.FromSlash(f.Path()))
		entry := check.ScopeFile{Path: displayRelative(abs)}
		key := scopeKey(abs)
		switch {
		case named != nil && !named[key]:
			entry.Status, entry.Reason = check.ScopeOutOfScope, "not among the files named"
		case f.Status == diffscope.Deleted:
			entry.Status = check.ScopeDeleted
		case declared != nil && declared[key] == "":
			entry.Status, entry.Reason = check.ScopeOutOfScope, "not content "+recipe+" declares"
		default:
			// The recipe binds a format under the path as it resolved it.
			path := abs
			if declared != nil {
				path = declared[key]
			}
			fileDiags, blocks, err := a.checkDiffFile(ctx, run, f, path, &entry)
			if err != nil {
				return check.Report{}, err
			}
			diags = append(diags, fileDiags...)
			checked += blocks
		}
		scope.Files = append(scope.Files, entry)
	}

	report := run.opts.execution.report(check.Target{Kind: "diff", File: run.src.label, Blocks: checked}, diags, run.gate)
	report.Scope = scope
	report.Decide()
	return report, nil
}

// checkDiffFile reads one changed file once, locates its blocks, and checks the
// ones the change touched. It fills entry with the outcome and returns the
// findings and how many blocks it checked.
func (a *App) checkDiffFile(ctx context.Context, run diffCheckRun, f diffscope.File, abs string, entry *check.ScopeFile) ([]check.Diagnostic, int, error) {
	fmtName, cfg := run.opts.formats.forFile(a, abs)
	if fmtName == "" {
		detected, err := a.FormatReg.Detect(abs, registry.DetectOptions{ExtensionOnly: true})
		if err != nil || detected == "" {
			entry.Status, entry.Reason = check.ScopeNoReader, "no format reads this file"
			return nil, 0, nil
		}
		fmtName = string(detected)
	}
	content, err := readScopedSource(abs)
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", entry.Path, err)
	}
	read, err := a.readWithExtents(ctx, abs, content, fmtName, cfg)
	if err != nil {
		return nil, 0, err
	}
	if len(read.blocks) == 0 {
		entry.Status = check.ScopeNoContent
		return nil, 0, nil
	}
	notRun := func(reason string) ([]check.Diagnostic, int, error) {
		entry.Status, entry.Reason = check.ScopeDidNotRun, reason
		return nil, 0, nil
	}
	switch {
	case f.Binary:
		return notRun("the diff reports a binary change, which names no lines")
	case read.unlocated != nil:
		return notRun(read.unlocated.Error())
	}
	if err := verifyPostImage(f, content); err != nil {
		return nil, 0, fmt.Errorf("%s: %w", entry.Path, err)
	}
	located := map[string]bool{}
	for _, x := range read.extents {
		located[x.Block] = true
	}
	for _, b := range read.blocks {
		if !located[b.ID] {
			return notRun(fmt.Sprintf("block %s has no position in the file", blockKey(b)))
		}
	}

	lines := map[string]format.LineRange{}
	for _, x := range diffscope.Touched(read.extents, f.Changes) {
		if _, seen := lines[x.Block]; !seen {
			lines[x.Block] = x.Lines
		}
	}
	var touched []*model.Block
	byKey := map[string]format.LineRange{}
	for _, b := range read.blocks {
		if l, ok := lines[b.ID]; ok {
			touched = append(touched, b)
			byKey[blockKey(b)] = l
			entry.Blocks = append(entry.Blocks, check.ScopeBlock{Block: blockKey(b), Lines: l})
		}
	}
	if len(touched) == 0 {
		entry.Status = check.ScopeUntouched
		return nil, 0, nil
	}
	entry.Status = check.ScopeChecked

	opts := run.opts
	if run.voice != nil {
		if opts.profile, opts.voiceContext, err = run.voice.forFile(ctx, abs); err != nil {
			return nil, 0, err
		}
	}
	if run.vocab != nil {
		if opts.terms, err = run.vocab.forFile(ctx, abs); err != nil {
			return nil, 0, err
		}
	}
	opts.execution.recordContext(entry.Path, "", opts)
	// Document-scope rules read the whole file the change touched.
	opts.documentBlocks = read.blocks
	diags, err := a.collectFileDiagnostics(ctx, touched, entry.Path, opts)
	if err != nil {
		return nil, 0, err
	}
	for i := range diags {
		if l, ok := byKey[diags[i].Location.Block]; ok && diags[i].Location.Block != "" {
			diags[i].Location.Lines = &l
		}
	}
	return diags, len(touched), nil
}

// scopedRead is one file read for a diff-scoped check.
type scopedRead struct {
	blocks  []*model.Block
	extents []format.Extent
	// unlocated says why the blocks could not be placed in the file, and is nil
	// when extents locates them.
	unlocated error
}

// readWithExtents reads content once with a skeleton store wired, returning the
// translatable blocks and where each sits in content.
func (a *App) readWithExtents(ctx context.Context, path string, content []byte, fmtName string, cfg map[string]any) (scopedRead, error) {
	reader, err := a.FormatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return scopedRead{}, fmt.Errorf("no reader for %q: %w", fmtName, err)
	}
	defer reader.Close()
	if err := applyFormatConfig(reader, cfg); err != nil {
		return scopedRead{}, fmt.Errorf("apply format config for %q: %w", filepath.Base(path), err)
	}
	var store *format.SkeletonStore
	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		store = format.NewMemorySkeletonStore()
		emitter.SetSkeletonStore(store)
	}
	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleID(a.SourceLocale()),
		Encoding:     a.InputEncoding(),
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return scopedRead{}, fmt.Errorf("open %q: %w", filepath.Base(path), err)
	}
	var out scopedRead
	var readErr error
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			readErr = fmt.Errorf("read %q: %w", filepath.Base(path), result.Error)
			break
		}
		if result.Part == nil || result.Part.Type != model.PartBlock {
			continue
		}
		if b, ok := result.Part.Resource.(*model.Block); ok && b.Translatable {
			out.blocks = append(out.blocks, b)
		}
	}
	if readErr != nil {
		return scopedRead{}, readErr
	}
	if store == nil {
		out.unlocated = fmt.Errorf("the %s format keeps no record of where its content sits in the file", fmtName)
		return out, nil
	}
	if err := store.Flush(); err != nil {
		return scopedRead{}, err
	}
	out.extents, out.unlocated = format.ExtentsFromSkeleton(content, store)
	return out, nil
}

// verifyPostImage confirms that every line the diff shows is the file's line at
// that number. A diff taken from another tree would otherwise scope the wrong
// blocks with nothing to show for it.
func verifyPostImage(f diffscope.File, content []byte) error {
	if len(f.PostLines) == 0 {
		return nil
	}
	lines := strings.Split(string(content), "\n")
	numbers := make([]int, 0, len(f.PostLines))
	for n := range f.PostLines {
		numbers = append(numbers, n)
	}
	slices.Sort(numbers)
	for _, n := range numbers {
		want := f.PostLines[n]
		got := ""
		if n >= 1 && n <= len(lines) {
			got = lines[n-1]
		}
		if n < 1 || n > len(lines) || got != want {
			return fmt.Errorf("the diff does not match the file: line %d is %q in the diff and %q on disk. Take the diff from the working tree being checked", n, want, got)
		}
	}
	return nil
}

// declaredContent maps each file of the project's declared content from its
// scope key to the path the recipe resolved, and returns the recipe's name.
// Both are empty outside a project.
func (a *App) declaredContent(cmd Command) (map[string]string, string, error) {
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		return nil, "", err
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, "", fmt.Errorf("load project: %w", err)
	}
	files, err := a.projectSourceFiles(proj, filepath.Dir(projectPath))
	if err != nil {
		return nil, "", err
	}
	declared := make(map[string]string, len(files))
	for _, f := range files {
		declared[scopeKey(f)] = f
	}
	return declared, filepath.Base(projectPath), nil
}

// scopeKey is a path in absolute, symlink-free form, so a path git reports and
// a path a recipe resolves compare equal whichever spelling of one directory
// each used (macOS /var and /private/var).
func scopeKey(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return canonicalPath(path)
}

// displayRelative names a path the way a user in the working directory would.
func displayRelative(abs string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return abs
	}
	if rel, err := filepath.Rel(scopeKey(cwd), scopeKey(abs)); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return abs
}
