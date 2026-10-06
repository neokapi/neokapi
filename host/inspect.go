package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	yamlv3 "gopkg.in/yaml.v3"
)

// inspectRecord is one block as kapi inspect prints it: the read record of
// the change service (the reference to copy into an operation, the revision
// to send as if_match, the text in placeholder form, its inline codes with
// the attributes an edit may change, its plurals and selects, its other
// editions and the operations it accepts), with the block's structural role
// and nesting level, and its rendering in other formats when --project asks
// for it.
type inspectRecord struct {
	change.BlockRead
	Role      string            `json:"role,omitempty"`
	Level     int               `json:"level,omitempty"`
	Projected map[string]string `json:"projected,omitempty"`
}

// RunInspect prints a read record for each block of the files args name. It
// reads through the change service, the read kapi apply writes through, so
// every reference and revision it prints is one kapi apply resolves.
func (a *App) RunInspect(ctx context.Context, cmd Command, args []string, outFormat string, render []string) error {
	hadError := false
	report := func(path string, err error) {
		hadError = true
		fmt.Fprintf(cmd.ErrOrStderr(), "kapi inspect: %s: %v\n", path, err)
	}
	// A source a .kpz carries (work.kpz!guide.md) is read through the KPZ's
	// workspace home; every other argument names files.
	var kpzDocs, rest []string
	for _, arg := range args {
		if isKpzDocRef(arg) {
			kpzDocs = append(kpzDocs, arg)
			continue
		}
		rest = append(rest, arg)
	}
	files := kpzDocs
	if len(rest) > 0 || len(kpzDocs) == 0 {
		resolved, err := a.ResolveInputs(cmd, rest, InputOptions{Command: "kapi inspect", OnSkip: report})
		if err != nil {
			return err
		}
		files = append(files, resolved...)
	}
	a.InitRegistries()
	recipe, err := ResolveProjectPath(cmd)
	if err != nil {
		return err
	}
	changes := a.newCommandChanges(cmd, recipe, ChangeServiceOptions{Origin: "inspect", Format: a.FormatFlag, TargetLocale: model.LocaleID(a.TargetLang), SourceLocale: model.LocaleID(a.SourceLang)})
	src := inspectSources{changes: changes, editions: a.projectEditions(recipe), comments: a.newCommentDocs(recipe)}

	streaming := outFormat == "jsonl"
	enc := json.NewEncoder(cmd.OutOrStdout())
	// The <x id="…"/> tokens in a record's text stay readable.
	enc.SetEscapeHTML(false)
	var recs []inspectRecord
	emit := func(rec inspectRecord) error {
		if streaming {
			return enc.Encode(rec)
		}
		recs = append(recs, rec)
		return nil
	}

	prog := a.NewProgress(cmd, "reading", len(files))
	defer prog.Done()
	for _, file := range files {
		prog.Step(DisplayName(file))
		docs := []string{file}
		if isContainer(file) {
			if docs, err = a.containerMembers(file); err != nil {
				report(DisplayName(file), err)
				continue
			}
		}
		for _, doc := range docs {
			ferr := a.inspectDocument(ctx, cmd, src, doc, render, emit)
			if errors.Is(ferr, context.Canceled) {
				return ferr
			}
			if ferr != nil {
				report(DisplayName(doc), ferr)
			}
		}
	}

	if !streaming {
		if recs == nil {
			recs = []inspectRecord{}
		}
		out, mErr := marshalRecords(recs, outFormat == "yaml")
		if mErr != nil {
			return mErr
		}
		if _, wErr := cmd.OutOrStdout().Write(out); wErr != nil {
			return wErr
		}
	}
	if hadError {
		return WithExitCode(ExitUsage, ErrSilentExit)
	}
	return nil
}

// inspectSources is what a kapi inspect run reads files through: the change
// services, the editions the project declares for each of its documents, and
// which files are read for their comments.
type inspectSources struct {
	changes  *commandChanges
	editions func(doc string) []model.EditionKey
	comments *commentDocs
}

// inspectDocument reads one document, or one archive member, and emits a
// record for each block that holds content, with its rendering in each format
// render names.
func (a *App) inspectDocument(ctx context.Context, cmd Command, src inspectSources, file string, render []string, emit func(inspectRecord) error) error {
	svc, doc, label := (*change.Service)(nil), "", ""
	var want []model.EditionKey
	// A ref names the document's own edition by its language where the
	// project or --source-lang gives it.
	own := a.SourceLang != ""
	switch {
	case file == StdinName:
		path, cleanup, err := stdinDocument(ctx)
		if err != nil {
			return err
		}
		defer cleanup()
		if svc, err = a.changeService(ctx, cmd, ChangeServiceOptions{Origin: "inspect", Root: filepath.Dir(path), Format: a.FormatFlag, TargetLocale: model.LocaleID(a.TargetLang), SourceLocale: model.LocaleID(a.SourceLang)}); err != nil {
			return err
		}
		doc, label = filepath.Base(path), StdinName
	case src.comments.is(file):
		return a.inspectComments(cmd, src.changes, file, emit)
	default:
		cs, ref, project, err := src.changes.forFile(ctx, file)
		if err != nil {
			return err
		}
		svc, doc = cs.svc, ref
		if project {
			want = src.editions(doc)
			own = true
		}
	}
	_, err := svc.ReadEach(ctx, change.ReadRequest{Doc: doc, Editions: want, OwnEdition: own}, func(b *model.Block, r change.BlockRead) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		// A block with no content is left out. The predicate is the text with
		// inline codes kept, so a block whose only run is a code is listed.
		if ed, _ := b.Edition(r.Ref.Edition); model.RunsHygieneText(ed.Runs) == "" {
			return nil
		}
		if label != "" {
			r.Ref.Doc = label
		}
		rec := inspectRecord{BlockRead: r}
		if s, ok := b.Structure(); ok {
			rec.Role, rec.Level = s.Role, s.Level
		}
		if len(render) > 0 {
			rec.Projected = make(map[string]string, len(render))
			for _, p := range render {
				if frag, ok := formats.RenderBlockFragment(b, p); ok {
					rec.Projected[p] = frag
				}
			}
		}
		return emit(rec)
	})
	return err
}

// inspectComments emits a record for each comment of a source file whose
// comments are what kapi edits in it. A comment's revision is the one a
// set_content that rewrites it sends as if_match, and its text is its prose
// without comment markers.
func (a *App) inspectComments(cmd Command, changes *commandChanges, file string, emit func(inspectRecord) error) error {
	formats, err := a.newCheckFormats(cmd)
	if err != nil {
		return err
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	p, ok := a.commentProviderForEdit(file, formats)
	if !ok {
		if hint, plugin := commentPluginHintFor(file); plugin {
			return &noCommentReaderError{file: file, hint: hint}
		}
		return fmt.Errorf("no comment layer reads %s", DisplayName(file))
	}
	located, err := comment.Locate(p, file, src, formats.directivesFor(file))
	if err != nil {
		return err
	}
	doc := commentRef(changes, file)
	r, writes := p.(comment.Rewriter)
	for i, b := range located.Blocks() {
		c := located.Comments[i]
		rec := inspectRecord{
			Ref:  change.Ref{Doc: doc, Block: b.ID},
			Rev:  commentRevision(comment.Fingerprint(src, c)),
			Text: model.RunsEditText(b.SourceRuns()),
			Ops:  []change.Kind{},
			Role: "comment",
		}
		if writes {
			if prose, perr := r.Prose(src, c); perr == nil {
				rec.Text = prose
				rec.Ops = []change.Kind{change.KindSetContent}
			}
		}
		if err := emit(rec); err != nil {
			return err
		}
	}
	return nil
}

// commentRef is the document reference of a source file, as kapi apply
// resolves one: project-relative inside the project, its absolute path for a
// file outside the project, else from the working directory.
func commentRef(changes *commandChanges, file string) string {
	abs, err := filepath.Abs(file)
	if err != nil {
		return filepath.ToSlash(file)
	}
	if changes.recipe != "" {
		if rel, rerr := filepath.Rel(filepath.Dir(changes.recipe), abs); rerr == nil && filepath.IsLocal(rel) {
			return filepath.ToSlash(rel)
		}
		return filepath.ToSlash(abs)
	}
	wd, err := os.Getwd()
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return relRef(wd, abs)
}

// projectEditions returns, for a document of the project at recipe, the
// editions the recipe declares for it in files of their own, so a read lists
// each translation beside the source. Each document is resolved on its own,
// as the change service resolves a source reference, so no read expands the
// whole recipe. Outside a project, and for a document the recipe does not
// claim, it returns none.
func (a *App) projectEditions(recipe string) func(doc string) []model.EditionKey {
	if recipe == "" {
		return func(string) []model.EditionKey { return nil }
	}
	layout := a.lazyProjectLayout(recipe)
	return func(doc string) []model.EditionKey {
		l := layout()
		if l == nil {
			return nil
		}
		rf, ok := l.sourceFile(doc)
		if !ok {
			return nil
		}
		var out []model.EditionKey
		for loc := range l.editionUnits(rf) {
			out = append(out, model.EditionKey{Locale: loc})
		}
		slices.SortFunc(out, func(x, y model.EditionKey) int { return strings.Compare(string(x.Locale), string(y.Locale)) })
		return out
	}
}

// lazyProjectLayout returns the change layout of the project at recipe, as
// the command line reads it, built on the first call; nil when the recipe
// does not load.
func (a *App) lazyProjectLayout(recipe string) func() *projectChangeLayout {
	return sync.OnceValue(func() *projectChangeLayout {
		l, err := a.newProjectLayout(ChangeServiceOptions{Project: recipe, Format: a.FormatFlag, SourceLocale: model.LocaleID(a.SourceLang)})
		if err != nil {
			return nil
		}
		return l
	})
}

// marshalRecords renders records as an indented JSON array, or as a YAML
// sequence with the same keys in the same order.
func marshalRecords(recs []inspectRecord, asYAML bool) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(recs); err != nil {
		return nil, err
	}
	if !asYAML {
		return buf.Bytes(), nil
	}
	if len(recs) == 0 {
		return []byte("[]\n"), nil
	}
	var node yamlv3.Node
	if err := yamlv3.Unmarshal(buf.Bytes(), &node); err != nil {
		return nil, err
	}
	blockStyle(&node)
	var out bytes.Buffer
	yenc := yamlv3.NewEncoder(&out)
	yenc.SetIndent(2)
	if err := yenc.Encode(&node); err != nil {
		return nil, err
	}
	if err := yenc.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// blockStyle clears the flow style a JSON document parses into, so the YAML
// encoder writes block mappings and plain scalars where it can.
func blockStyle(n *yamlv3.Node) {
	n.Style = 0
	for _, c := range n.Content {
		blockStyle(c)
	}
}
