package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/container"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/state"
)

// This file builds the change service (core/change) for a surface of the kapi
// host: the CLI, MCP and Kapi Desktop each call it, hand it a change set and
// the actor their transport vouches for, and write nothing themselves.
//
// Inside a project the service resolves a document reference the way the
// recipe does: a project-relative path names a source file, read with the
// format and the configuration its content item binds; the file of one of its
// translations (the target template's output) names that edition of the
// source; and an archive member is container!entry. An edition that lives in a
// target file is joined to the source by key, then by translation-invariant
// address, then by position, and a target file that does not exist yet is
// written from the source's skeleton, as kapi merge writes one. Outside a
// project a reference is a path under the working directory, read with the
// format detection finds.
//
// Decisions and asset operations (decide, term, memory, recipe) land through
// the host functions kapi apply and the review queue already use, after the
// content they refer to.

// ChangeServiceOptions says what a change service edits.
type ChangeServiceOptions struct {
	// Project is the recipe of the project the service edits in. Empty edits
	// the documents under Root, with no recipe.
	Project string
	// Root is the directory references resolve under outside a project;
	// empty is the working directory.
	Root string
	// Origin names the surface that applies change sets through the service
	// (apply, desktop, mcp), for the record.
	Origin string
	// Format names the format of every document, a preset included, in place
	// of what the recipe binds or detection finds.
	Format string
	// TargetLocale is the language of the translation a bilingual file holds,
	// for a format whose reader has to be told it (a PO catalog's msgstr).
	// Changes takes it from --target-lang.
	TargetLocale model.LocaleID
	// AnyPath, outside a project, resolves a reference that leaves Root as a
	// path on the file system relative to Root, as a command line names the
	// files it is given. Without it such a reference is refused.
	AnyPath bool
	// BackupSuffix keeps a copy of each file a change replaces, beside it
	// with the suffix appended (kapi apply and ksed -i.bak).
	BackupSuffix string
	// PlainText reads a file no format claims as plain text unless its bytes
	// are binary, as the toolbox reads one (ksed). Without it such a file is
	// refused: no format reads it.
	PlainText bool
	// SourceLocale is the language the documents are written in, which wins
	// over the recipe's defaults.source_language; empty takes the recipe's,
	// else DefaultSourceLang. An edition in this language is the document's
	// own. Changes takes it from --source-lang. A long-lived host (the MCP
	// server, Kapi Desktop) passes the language it resolved for the call,
	// because the App's own source language belongs to whichever project
	// resolved one last.
	SourceLocale model.LocaleID

	// revisionsOnly builds a service whose reads name no edition's basis from
	// the block history: a flow's follower reads revisions alone, and a read
	// that asks the history for every block is the larger half of its cost.
	revisionsOnly bool

	// Materialize writes each translation's file a change writes from its
	// source's skeleton (filehome.Options.Materialize): kapi merge and kapi
	// pull write whole translations this way, where the editing surfaces
	// edit a translation's file in place.
	Materialize bool
	// WriterHook, inside a project, is given every writer the service opens
	// for a source file, after the recipe configured it: kapi pull sets the
	// locale variants of a document's media on it.
	WriterHook func(format.DataFormatWriter)
	// BeforeSettle, when set, is called once a document is staged and before
	// its commit lock is taken (filehome.Options.BeforeSettle), which is where
	// a test holds one sender while another lands.
	BeforeSettle func(doc string)
}

// Changes builds the change service for the project cmd names, or for the
// working directory when it names none. origin names the surface.
func (a *App) Changes(cmd Command, origin string) (*change.Service, error) {
	recipe, err := ResolveProjectPath(cmd)
	if err != nil {
		return nil, err
	}
	return a.changeService(cmd.Context(), cmd, ChangeServiceOptions{Project: recipe, Origin: origin, Format: a.FormatFlag,
		TargetLocale: model.LocaleID(a.TargetLang), SourceLocale: model.LocaleID(a.SourceLang)})
}

// ChangeService builds the change service opts describes.
func (a *App) ChangeService(ctx context.Context, opts ChangeServiceOptions) (*change.Service, error) {
	return a.changeService(ctx, projectCommand(ctx, "change", opts.Project), opts)
}

// changeService builds the service; cmd names its project for the hooks that
// resolve one from a command.
func (a *App) changeService(ctx context.Context, cmd Command, opts ChangeServiceOptions) (*change.Service, error) {
	h, err := a.changeHome(opts)
	if err != nil {
		return nil, err
	}
	return a.serviceOver(ctx, cmd, opts, h)
}

// changeHome is where a change service finds and writes documents: the layout
// that locates them, the directory their lock files live in, what prepares
// that directory before the first lock, and the root of the project (empty
// outside one).
type changeHome struct {
	layout  filehome.Layout
	lockDir string
	prepare func() error
	root    string
}

// changeHome builds the home opts describes.
func (a *App) changeHome(opts ChangeServiceOptions) (changeHome, error) {
	a.InitRegistries()
	if opts.Project != "" {
		pl, err := a.newProjectLayout(opts)
		if err != nil {
			return changeHome{}, err
		}
		pl.plainText = opts.PlainText
		l, err := project.LayoutFor(opts.Project)
		if err != nil {
			return changeHome{}, err
		}
		// The lock files live in .kapi/work, which the layout's ignore rule
		// keeps out of a commit. The directory and the rule are written when a
		// change first takes a lock, so a read leaves the project as it was.
		return changeHome{
			layout:  pl,
			lockDir: filepath.Join(l.WorkDir(), "locks"),
			prepare: sync.OnceValue(func() error { return project.EnsureLayout(l) }),
			root:    l.Root,
		}, nil
	}
	dir := opts.Root
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return changeHome{}, err
		}
		dir = wd
	}
	return changeHome{
		layout: &dirChangeLayout{app: a, root: dir, format: opts.Format, target: opts.TargetLocale, anywhere: opts.AnyPath, plainText: opts.PlainText,
			source: model.LocaleID(ResolveSourceLocale(string(opts.SourceLocale), ""))},
		// Every kapi process of this user finds the same lock files for a
		// document outside a project, whatever its temporary directory.
		lockDir: filepath.Join(DataDir(), "locks"),
	}, nil
}

// serviceOver builds the change service that edits the documents of h.
func (a *App) serviceOver(ctx context.Context, cmd Command, opts ChangeServiceOptions, h changeHome) (*change.Service, error) {
	origin := opts.Origin
	if origin == "" {
		origin = "apply"
	}
	// The recorder opens before the home takes its first lock, after the
	// lock directory is prepared.
	recorder := a.changeRecorder(ctx, h.root)
	home := filehome.New(h.layout, filehome.Options{LockDir: h.lockDir, PrepareLocks: recorder.before(h.prepare), BackupSuffix: opts.BackupSuffix,
		Materialize: opts.Materialize, BeforeSettle: opts.BeforeSettle})
	svcOpts := []change.Option{
		change.WithOrigin(origin),
		change.WithAssets(&changeAssets{app: a, recipe: opts.Project}),
	}
	check, err := a.changeCommitCheck(changeHookCommand(ctx, cmd, opts.Project), opts.Project)
	if err != nil {
		return nil, err
	}
	if check != nil {
		svcOpts = append(svcOpts, change.WithCommitCheck(check))
	}
	policy, err := a.changePolicy(opts.Project)
	if err != nil {
		return nil, err
	}
	if policy != nil {
		svcOpts = append(svcOpts, change.WithPolicy(policy))
	}
	if recorder != nil {
		svcOpts = append(svcOpts, change.WithRecorder(recorder))
	}
	if hist := a.changeHistory(h.root); hist != nil {
		if !opts.revisionsOnly {
			svcOpts = append(svcOpts, change.WithEditionStates(hist))
		}
		svcOpts = append(svcOpts, change.WithHistories(hist))
	}
	return change.NewService(filehome.Formats{Registry: a.FormatReg}, change.OneHome(home), svcOpts...), nil
}

// changeHookCommand is the command the hooks that resolve a project from a
// command are given: cmd for a service of the project at recipe, which cmd
// names, and a command that resolves no project for a service outside one,
// so discovery from the working directory never puts an edit made outside a
// project under the governance of a project it happens to sit in. That
// command keeps the --source-lang cmd was given, the language the commit
// check reads a document outside a project in.
func changeHookCommand(ctx context.Context, cmd Command, recipe string) Command {
	switch {
	case recipe == "":
		d := detachedCommand(ctx, "change")
		if cmd != nil {
			if f := cmd.Flags().Lookup(sourceLangFlag); f != nil && f.Changed {
				d.Flags().String(sourceLangFlag, "", "")
				_ = d.Flags().Set(sourceLangFlag, f.Value.String())
			}
		}
		return d
	case cmd == nil:
		return projectCommand(ctx, "change", recipe)
	}
	return cmd
}

// projectCommand is a command that names the project at recipe with -p, for
// the functions that resolve a project from a command; "" names none.
func projectCommand(ctx context.Context, name, recipe string) Command {
	cmd := NewEnvCommand(ctx, name)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	AddProjectFlag(cmd)
	if recipe != "" {
		_ = cmd.Flags().Set(projectFlagName, recipe)
	}
	return cmd
}

// formatBinding binds a format to its reader and writer in the registry, each
// configured with cfg (the recipe's merged format configuration) and the
// writer encoding enc. A reader for a preset reference (format@preset) takes
// the preset's configuration under cfg.
func (a *App) formatBinding(name string, cfg map[string]any, enc string) filehome.Binding {
	ref := preset.ParseFormatRef(name)
	registryName := ref.RegistryName()
	id := registry.FormatID(registryName)
	return filehome.Binding{
		Name:     registryName,
		Declared: filehome.PluginDeclared(a.FormatReg, registryName),
		NewReader: func() (format.DataFormatReader, error) {
			var r format.DataFormatReader
			var err error
			if registryName != name {
				r, _, err = a.NewConfiguredReader(name)
			} else {
				r, err = a.FormatReg.NewReader(id)
			}
			if errors.Is(err, registry.ErrUnknownFormat) {
				// A format a plugin supplies: the refusal names the plugin to
				// install, as every other read of the format does.
				return nil, fmt.Errorf("%s: %w", formatInstallClause(a.discoveredPlugins(), registryName), err)
			}
			if err != nil {
				return nil, err
			}
			if err := applyFormatConfig(r, cfg); err != nil {
				return nil, fmt.Errorf("apply the %s configuration: %w", registryName, err)
			}
			return r, nil
		},
		NewWriter: func() (format.DataFormatWriter, error) {
			w, err := a.FormatReg.NewWriter(id)
			if err != nil {
				return nil, err
			}
			if enc != "" {
				w.SetEncoding(enc)
			}
			if err := applyWriterOutputConfig(w, cfg); err != nil {
				return nil, fmt.Errorf("apply the %s configuration: %w", registryName, err)
			}
			return w, nil
		},
	}
}

// editionsOf says how a format holds editions. A file of a translation
// interchange format (XLIFF, PO, TMX, Qt Linguist) keeps each translation
// beside its source, and so does a multilingual string catalog, which holds
// every language in one file; a file of any other format holds one edition.
func (a *App) editionsOf(name string) change.Editions {
	info := a.FormatReg.FormatInfo(registry.FormatID(preset.ParseFormatRef(name).RegistryName()))
	switch {
	case info == nil:
	case info.Interchange, info.Family == registry.FamilyBilingualInterchange, slices.Contains(multilingualCatalogs, string(info.Name)):
		return change.EditionsInFile
	}
	return change.EditionsPerFile
}

// multilingualCatalogs are the string-catalog formats whose one file holds
// every language, each read as an edition of the source string (an Xcode
// string catalog).
var multilingualCatalogs = []string{"xcstrings"}

// targetOf is the language a file of format name holds a translation in:
// target for a bilingual format, nothing for any other.
func (a *App) targetOf(name string, target model.LocaleID) model.LocaleID {
	if a.editionsOf(name) != change.EditionsInFile {
		return ""
	}
	return target
}

// dirChangeLayout serves the documents under a directory with no recipe: the
// format --format names, or the one detection finds, as kapi apply reads a
// file outside a project.
type dirChangeLayout struct {
	app    *App
	root   string
	format string
	source model.LocaleID
	target model.LocaleID
	// anywhere resolves a reference that leaves root as a path on the file
	// system (ChangeServiceOptions.AnyPath).
	anywhere bool
	// plainText reads a file no format claims as plain text
	// (ChangeServiceOptions.PlainText).
	plainText bool
}

func (l *dirChangeLayout) Locate(_ context.Context, doc string) (filehome.Doc, error) {
	ref, path, entry, err := filehome.ResolvePath(l.root, doc)
	if err != nil && l.anywhere {
		ref, path, entry, err = resolveAnywhere(l.root, doc, err)
	}
	if err != nil {
		return filehome.Doc{}, err
	}
	name := l.format
	if name == "" {
		if name, err = l.app.detectChangeFormat(path, entry, l.plainText); err != nil {
			return filehome.Doc{}, err
		}
	}
	enc := l.app.InputEncoding()
	return filehome.Doc{
		Ref: ref, Path: path, Entry: entry,
		Format:       l.app.formatBinding(name, nil, enc),
		SourceLocale: l.source,
		Encoding:     enc,
		Editions:     l.app.editionsOf(name),
		TargetLocale: l.app.targetOf(name, l.target),
	}, nil
}

// detectChangeFormat detects the format of a file, or of an archive member,
// by its name and then its content, as the toolbox detects one, so a format
// that claims no extension (a Qt Linguist catalog) is found by what the file
// holds. With plainText, a file no format claims is read as plain text unless
// its bytes are binary, as the toolbox reads one; without it, it is refused.
func (a *App) detectChangeFormat(path, entry string, plainText bool) (string, error) {
	if entry != "" {
		return filehome.DetectFormat(a.FormatReg, path, entry)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if name, err := a.FormatReg.Detector().Detect(path, f, ""); err == nil && name != "" {
		return name, nil
	}
	if !plainText {
		if _, code := commentProviders.For(path); code {
			return "", &change.Error{Code: change.CodeUnsupported, Capability: "comment",
				Message: filepath.Base(path) + " is source code: kapi reads its comments for checks, and the change service writes no code comment; edit the comment with kapi apply, or in the file and check the file afterwards"}
		}
		return "", &change.Error{Code: change.CodeUnsupported, Capability: "format", Message: "no format reads " + filepath.Base(path)}
	}
	if a.binaryStream(f) {
		return "", &change.Error{Code: change.CodeUnsupported, Capability: "format", Message: filepath.Base(path) + ": " + errBinaryInput().Error()}
	}
	return FallbackFormat, nil
}

// projectChangeLayout serves the documents of a project.
//
// A reference to a source file resolves on its own, by the rule that names
// the item claiming a path (ProjectContext.ResolvePaths), so reading or
// editing a document costs the same in a project of ten files and of ten
// thousand. Only a reference that names no source file, such as the file of
// a translation, expands the recipe's content patterns, once per service, to
// find the source it belongs to.
type projectChangeLayout struct {
	app    *App
	root   string
	proj   *project.KapiProject
	pctx   *project.ProjectContext
	format string
	source model.LocaleID
	target model.LocaleID
	enc    string
	// plainText reads a file the recipe does not claim and no format reads
	// as plain text (ChangeServiceOptions.PlainText).
	plainText bool
	// writerHook is given every writer opened for a source file
	// (ChangeServiceOptions.WriterHook).
	writerHook func(format.DataFormatWriter)

	once  sync.Once
	index *projectChangeIndex
	err   error
}

// projectChangeIndex is what the recipe declares, by project-relative path.
type projectChangeIndex struct {
	// sources are the files the recipe claims as content.
	sources map[string]project.ResolvedFile
	// byTarget maps a translation's file to the source and the locale it
	// holds.
	byTarget map[string]targetOfSource
}

type targetOfSource struct {
	source string
	locale model.LocaleID
}

func (a *App) newProjectLayout(opts ChangeServiceOptions) (*projectChangeLayout, error) {
	proj, err := project.LoadWithOptions(opts.Project, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	pctx := project.NewProjectContext(proj, opts.Project)
	return &projectChangeLayout{
		app: a, root: pctx.ProjectDir, proj: proj, pctx: pctx, format: opts.Format, target: opts.TargetLocale,
		source:     model.LocaleID(ResolveSourceLocale(string(opts.SourceLocale), proj.Defaults.SourceLanguage)),
		enc:        ResolveEncodingName(a.Encoding, proj.Defaults.Encoding),
		writerHook: opts.WriterHook,
	}, nil
}

// load expands the recipe's content once, for a reference that names no
// source file.
func (l *projectChangeLayout) load() (*projectChangeIndex, error) {
	l.once.Do(func() {
		ix := &projectChangeIndex{sources: map[string]project.ResolvedFile{}, byTarget: map[string]targetOfSource{}}
		files, err := l.pctx.ResolveContent(l.app.FormatReg)
		if err != nil {
			l.err = err
			return
		}
		for _, rf := range files {
			if rf.CommentsOnly() {
				continue
			}
			src := filepath.ToSlash(rf.Relative)
			ix.sources[src] = rf
			for loc, u := range l.editionUnits(rf) {
				ix.byTarget[filepath.ToSlash(u.DisplayPath)] = targetOfSource{source: src, locale: loc}
			}
		}
		l.index = ix
	})
	return l.index, l.err
}

// sourceFile resolves ref as a source file the recipe claims for its values,
// without expanding any pattern.
func (l *projectChangeLayout) sourceFile(ref string) (project.ResolvedFile, bool) {
	return claimedSource(l.app.FormatReg, l.pctx, ref)
}

// claimedSource resolves rel, a project-relative path, as the recipe claims
// it: the file, and true when an item claims its values. The format is the
// item's, or the one detection finds from the file's content, as
// ResolveContent finds it.
func claimedSource(reg *registry.FormatRegistry, pctx *project.ProjectContext, rel string) (project.ResolvedFile, bool) {
	files := pctx.ResolvePaths(reg, []string{rel}, func(rel string) (io.ReadSeeker, error) {
		return os.Open(filepath.Join(pctx.ProjectDir, filepath.FromSlash(rel)))
	})
	if len(files) != 1 || files[0].CommentsOnly() {
		return project.ResolvedFile{}, false
	}
	return files[0], true
}

// editionUnits is the file of each translation of rf the recipe declares,
// by language. A target in another format than its source is a conversion,
// and no edition of the source can be written through it.
func (l *projectChangeLayout) editionUnits(rf project.ResolvedFile) map[model.LocaleID]VerifyUnit {
	out := map[model.LocaleID]VerifyUnit{}
	for _, u := range l.app.unitsOfFile(l.proj, l.root, rf, "") {
		if u.TargetFormat == "" {
			continue
		}
		out[model.NormalizeLocale(model.LocaleID(u.Locale))] = u
	}
	return out
}

// translationFile finds the source and the language of the translation
// whose file ref names.
func (l *projectChangeLayout) translationFile(ref string) (targetOfSource, bool, error) {
	ix, err := l.load()
	if err != nil {
		return targetOfSource{}, false, err
	}
	t, ok := ix.byTarget[ref]
	return t, ok, nil
}

func (l *projectChangeLayout) Locate(_ context.Context, doc string) (filehome.Doc, error) {
	ref, path, entry, err := filehome.ResolvePath(l.root, doc)
	if err != nil {
		// The file of a translation not written yet still names its edition.
		t, ok, lerr := l.translationFile(cleanRef(doc))
		if lerr != nil {
			return filehome.Doc{}, lerr
		}
		if ok {
			return l.editionDoc(t)
		}
		return filehome.Doc{}, err
	}
	if entry != "" {
		// An archive member is read with the project's configuration of the
		// member's format.
		name := l.format
		if name == "" {
			if name, err = l.app.detectChangeFormat(path, entry, l.plainText); err != nil {
				return filehome.Doc{}, err
			}
		}
		return filehome.Doc{Ref: ref, Path: path, Entry: entry, Format: l.app.formatBinding(name, l.formatConfig(name, "", nil), l.enc),
			SourceLocale: l.source, Encoding: l.enc, Editions: l.app.editionsOf(name), TargetLocale: l.app.targetOf(name, l.target)}, nil
	}
	if rf, ok := l.sourceFile(ref); ok {
		return l.sourceDoc(ref, rf), nil
	}
	t, ok, err := l.translationFile(ref)
	if err != nil {
		return filehome.Doc{}, err
	}
	if ok {
		return l.editionDoc(t)
	}
	// A file in the project the recipe does not claim: read as detection
	// finds it, with the project's defaults for its format.
	name := l.format
	if name == "" {
		if name = l.pctx.DetectFormat(l.app.FormatReg, path); name == "" {
			if name, err = l.app.detectChangeFormat(path, "", l.plainText); err != nil {
				return filehome.Doc{}, err
			}
		}
	}
	return filehome.Doc{Ref: ref, Path: path, Format: l.app.formatBinding(name, l.formatConfig(name, "", nil), l.enc),
		SourceLocale: l.source, Encoding: l.enc, Editions: l.app.editionsOf(name), TargetLocale: l.app.targetOf(name, l.target)}, nil
}

// formatConfig is the configuration a reader and writer of format name take
// in the project: the project's defaults for the format, and the content
// item's own configuration when the item binds that format (bound). A format
// that --format puts in place of the item's takes the defaults alone, because
// the item's configuration is written for another reader.
func (l *projectChangeLayout) formatConfig(name, bound string, item *project.ContentItem) map[string]any {
	reg := func(n string) string { return preset.ParseFormatRef(n).RegistryName() }
	if item != nil && reg(name) != reg(bound) {
		item = nil
	}
	return mergedFormatConfig(l.proj, reg(name), item)
}

// hooked is b with the layout's writer hook given every writer b opens.
func (l *projectChangeLayout) hooked(b filehome.Binding) filehome.Binding {
	if l.writerHook == nil || b.NewWriter == nil {
		return b
	}
	open := b.NewWriter
	b.NewWriter = func() (format.DataFormatWriter, error) {
		w, err := open()
		if err == nil {
			l.writerHook(w)
		}
		return w, err
	}
	return b
}

// sourceDoc is a source file the recipe claims, with the file of each of its
// translations.
func (l *projectChangeLayout) sourceDoc(ref string, rf project.ResolvedFile) filehome.Doc {
	name := rf.Format
	if l.format != "" {
		name = l.format
	}
	d := filehome.Doc{
		Ref: ref, Path: rf.Path,
		Format:       l.hooked(l.app.formatBinding(name, l.formatConfig(name, rf.Format, rf.Item), l.enc)),
		SourceLocale: l.source, Encoding: l.enc, Editions: l.app.editionsOf(name), TargetLocale: l.app.targetOf(name, l.target),
	}
	targets := l.editionUnits(rf)
	if rf.Item != nil && rf.Item.Target == "" {
		d.NoEditionFile = "the collection that holds it names no target, so its translations have no file"
	}
	if len(targets) > 0 && d.Editions == change.EditionsInFile {
		// A bilingual source (a PO catalog or template, an XLIFF file, a
		// Qt Linguist or string catalog) whose translations the recipe
		// writes to files of their own keeps them there: the French of
		// po/en.po is po/fr.po's, read and written in that file, and the
		// source catalog's own msgstr holds no edition. Every surface
		// reads it the same way, whatever target language it was told.
		d.Editions, d.TargetLocale = change.EditionsPerFile, ""
	}
	d.EditionFile = func(k model.EditionKey) (filehome.EditionFile, bool) {
		if k.Tone != "" || k.Channel != "" {
			return filehome.EditionFile{}, false
		}
		u, ok := targets[model.NormalizeLocale(k.Locale)]
		if !ok {
			return filehome.EditionFile{}, false
		}
		return filehome.EditionFile{
			Ref:       filepath.ToSlash(u.DisplayPath),
			Path:      filepath.Join(l.root, u.DisplayPath),
			Format:    l.app.formatBinding(u.TargetFormat, u.TargetConfig, l.enc),
			Bilingual: l.app.editionsOf(u.TargetFormat) == change.EditionsInFile,
		}, true
	}
	for loc, u := range targets {
		if _, err := os.Stat(filepath.Join(l.root, u.DisplayPath)); err == nil {
			d.Derived = append(d.Derived, model.EditionKey{Locale: loc})
		}
	}
	slices.SortFunc(d.Derived, func(a, b model.EditionKey) int { return strings.Compare(string(a.Locale), string(b.Locale)) })
	return d
}

// editionDoc is the source a translation's file holds an edition of, with
// that edition named.
func (l *projectChangeLayout) editionDoc(t targetOfSource) (filehome.Doc, error) {
	rf, ok := l.index.sources[t.source]
	if !ok {
		return filehome.Doc{}, &change.Error{Code: change.CodeNotFound, Field: "at/doc", Message: "no document " + t.source}
	}
	d := l.sourceDoc(t.source, rf)
	k := model.EditionKey{Locale: t.locale}
	d.Edition = &k
	return d, nil
}

// resolveAnywhere resolves doc, a path relative to root that leads out of
// it, for a command line outside a project. cause is the refusal
// filehome.ResolvePath gave; anything but a reference that left root is
// returned as it was. The file's own directory becomes the root the path is
// resolved under, and the reference keeps the spelling doc gave, cleaned.
func resolveAnywhere(root, doc string, cause error) (ref, path, entry string, err error) {
	var ce *change.Error
	if !errors.As(cause, &ce) || ce.Code != change.CodeInvalid || doc == "" {
		return "", "", "", cause
	}
	file, member := doc, ""
	for i := range len(doc) {
		if doc[i] == '!' && i+1 < len(doc) && container.IsContainerPath(doc[:i]) {
			file, member = doc[:i], doc[i+1:]
			break
		}
	}
	abs := filepath.Clean(filepath.FromSlash(file))
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	inner := filepath.Base(abs)
	if member != "" {
		inner += "!" + member
	}
	if _, path, entry, err = filehome.ResolvePath(filepath.Dir(abs), inner); err != nil {
		return "", "", "", err
	}
	ref = filepath.ToSlash(filepath.Clean(filepath.FromSlash(file)))
	if entry != "" {
		ref += "!" + entry
	}
	return ref, path, entry, nil
}

// cleanRef is a reference as a project-relative, slash-separated path.
func cleanRef(doc string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(doc)))
}

// changeAssets applies decide and the asset operations through the host
// functions kapi apply and the review queue use.
type changeAssets struct {
	app    *App
	recipe string
}

// assetEntry is the kapi apply entry an asset operation lands as.
func assetEntry(set *change.Set, op change.Op) (changeEntry, *change.Error) {
	var evidence []contextop.Evidence
	if set != nil {
		for _, e := range set.Evidence {
			evidence = append(evidence, contextop.Evidence{Path: e.Path, Unit: e.Unit, Quote: e.Quote, URL: e.URL})
		}
	}
	switch body := op.Body.(type) {
	case *change.Term:
		if body.Action != "upsert" {
			return changeEntry{}, &change.Error{Code: change.CodeUnsupported, Field: "action", Capability: "term." + body.Action,
				Message: fmt.Sprintf("a term is written with upsert; %s is not supported", body.Action)}
		}
		return changeEntry{Kind: kindTerm, Op: "upsert", Term: body.Term, Locale: body.Locale, Status: body.Status,
			Replacement: body.Replacement, Replaces: body.Replaces, DoNotTranslate: body.DoNotTranslate,
			Advisory: body.Advisory, Competitor: body.Competitor, Evidence: evidence}, nil
	case *change.Memory:
		if body.Action != "add" {
			return changeEntry{}, &change.Error{Code: change.CodeUnsupported, Field: "action", Capability: "memory." + body.Action,
				Message: fmt.Sprintf("a content-memory pair is written with add; %s is not supported", body.Action)}
		}
		return changeEntry{Kind: kindMemory, Op: "add", Source: body.From.Text, Target: body.To.Text,
			SourceLocale: body.From.Edition, TargetLocale: body.To.Edition, Evidence: evidence}, nil
	case *change.Recipe:
		return changeEntry{Kind: kindRecipe, Op: "set", Path: body.Path, Value: body.Value}, nil
	}
	return changeEntry{}, &change.Error{Code: change.CodeInvalid, Message: fmt.Sprintf("%s is not an asset operation", op.Kind)}
}

// whoOf is the actor an asset entry is applied as, and the note its record
// carries: the change set's own, or what applied it.
func whoOf(actor change.Actor, set *change.Set) changeActor {
	note := "applied through the change service"
	if set != nil && set.Note != "" {
		note = set.Note
	}
	return changeActor{Actor: contextop.Actor{Kind: contextop.ActorKind(actor.Kind), Name: actor.Name, Session: actor.Session}, Note: note}
}

func (c *changeAssets) Prepare(ctx context.Context, actor change.Actor, op change.Op, target *change.DecisionTarget) *change.Error {
	if op.Kind == change.KindDecide {
		return c.prepareDecision(actor, op, target)
	}
	if c.recipe == "" {
		return &change.Error{Code: change.CodeUnsupported, Capability: string(op.Kind),
			Message: fmt.Sprintf("%s writes to a project's stores; there is no kapi project here", op.Kind)}
	}
	e, err := assetEntry(nil, op)
	if err != nil {
		return err
	}
	if refusal, refused := actorRefusal(whoOf(actor, nil), e); refused {
		return &change.Error{Code: change.CodeNotPermitted, Message: refusal.Detail}
	}
	switch e.Kind {
	case kindTerm:
		if strings.TrimSpace(e.Term) == "" {
			return &change.Error{Code: change.CodeInvalid, Field: "term", Message: "the term is empty"}
		}
	case kindMemory:
		if strings.TrimSpace(e.Source) == "" || strings.TrimSpace(e.Target) == "" || e.TargetLocale == "" {
			return &change.Error{Code: change.CodeInvalid, Field: "to", Message: "a content-memory pair needs both texts and the edition it translates to"}
		}
	case kindRecipe:
		if e.Path == "" {
			return &change.Error{Code: change.CodeInvalid, Field: "path", Message: "the recipe field is empty"}
		}
	}
	return nil
}

func (c *changeAssets) Apply(ctx context.Context, actor change.Actor, set *change.Set, op change.Op, target *change.DecisionTarget) (change.OpStatus, *change.Error) {
	if op.Kind == change.KindDecide {
		return c.applyDecision(ctx, actor, set, op, target)
	}
	e, cerr := assetEntry(set, op)
	if cerr != nil {
		return "", cerr
	}
	res := c.app.applyRecordedAssetEntry(ctx, projectCommand(ctx, "change", c.recipe), whoOf(actor, set), e)
	switch res.Status {
	case "applied":
		return change.OpApplied, nil
	case "skipped":
		return change.OpUnchanged, nil
	}
	return "", &change.Error{Code: change.CodeInvalid, Message: res.Detail}
}

// prepareDecision checks a decision before anything is written: who may make
// it, and whether the host records it.
func (c *changeAssets) prepareDecision(actor change.Actor, op change.Op, target *change.DecisionTarget) *change.Error {
	body := op.Body.(*change.Decide)
	if c.recipe == "" {
		return &change.Error{Code: change.CodeUnsupported, Capability: "decide",
			Message: "a review decision is recorded in a project; there is no kapi project here"}
	}
	if actor.Kind != change.ActorPerson && body.Outcome != change.OutcomeAdvise {
		return &change.Error{Code: change.CodeNotPermitted,
			Message: "an agent records a pre-review (decide with advise), never a decision; a person decides"}
	}
	if target == nil {
		return &change.Error{Code: change.CodeNotFound, Message: "the decision names no edition"}
	}
	if body.Outcome == change.OutcomeAdvise && body.Score == nil {
		return &change.Error{Code: change.CodeInvalid, Field: "score",
			Message: "a pre-review carries the score it gives, from 0 to 100"}
	}
	switch {
	case body.Outcome == change.OutcomeWithdraw:
		return &change.Error{Code: change.CodeUnsupported, Capability: "decide.withdraw", Message: "withdrawing a decision is not recorded by this host"}
	case target.Role == change.RoleAuthoritative && body.Outcome != change.OutcomeEstablish:
		return &change.Error{Code: change.CodeUnsupported, Capability: "decide." + string(body.Outcome),
			Message: "the document's own edition is established by a person; it takes no other decision"}
	}
	return nil
}

// applyDecision records a decision on the edition it binds to, through the
// review queue's functions. The decision binds to the content the change set
// landed, which the service read under the commit lock (target.Text and
// target.SourceText): a write that reaches the file after it makes the
// decision stale, and never takes it over.
func (c *changeAssets) applyDecision(ctx context.Context, actor change.Actor, set *change.Set, op change.Op, target *change.DecisionTarget) (change.OpStatus, *change.Error) {
	body := op.Body.(*change.Decide)
	a := c.app
	if target.Role == change.RoleAuthoritative {
		wording := target.Text
		changed, err := a.approveSourceUnit(ctx, c.recipe, "", SourceUnitRef{File: filepath.FromSlash(target.Doc.Doc), Key: target.Ref.Block}, &wording)
		return decisionOutcome(changed, err)
	}
	decided := &decidedContent{source: target.SourceText, target: target.Text, targetRev: target.Rev}
	if target.Rev == model.AbsentRevision {
		// The edition has no content in its home: a parked draft the project
		// store holds and no file carries yet. The decision binds to that
		// draft, as the review queue lists it.
		decided = nil
	}
	file := target.Place.File
	if file == "" {
		file = target.Doc.Doc
	}
	locale := string(target.Ref.Edition.Locale)
	ref := ReviewUnitRef{File: filepath.FromSlash(file), Key: target.Ref.Block, Locale: locale}
	switch body.Outcome {
	case change.OutcomeAdvise:
		review := state.AIReview{Model: reviewerName(actor), At: nowRFC3339()}
		if decided != nil {
			review.TargetHash = targetHash(decided.target)
		}
		if body.Score != nil {
			review.Score = *body.Score
		}
		for _, r := range body.Reasons {
			review.Findings = append(review.Findings, state.AIReviewFinding{Message: r})
		}
		n, err := a.RecordAIReviews(ctx, c.recipe, "", locale, ref.File, map[string]state.AIReview{ref.Key: review})
		if err == nil && n == 0 {
			// The review queue holds no unit at this edition, so the
			// pre-review has nothing to annotate.
			return "", &change.Error{Code: change.CodeNotFound, Field: "at",
				Message: fmt.Sprintf("the review queue holds no %s translation of block %s in %s to record a pre-review on", locale, ref.Key, file)}
		}
		return decisionOutcome(n > 0, err)
	case change.OutcomeReject:
		note := ""
		if set != nil {
			note = set.Note
		}
		changed, err := a.applyReviewDecision(ctx, c.recipe, "", ref, ReviewDecisionRejected, note, decided)
		return decisionOutcome(changed, err)
	default:
		changed, err := a.applyReviewDecision(ctx, c.recipe, "", ref, ReviewDecisionApproved, "", decided)
		return decisionOutcome(changed, err)
	}
}

// reviewerName is the name a pre-review is recorded under, which the review
// queue shows beside its score: agent/<client> for an agent, and the sender's
// own name for anyone else.
func reviewerName(actor change.Actor) string {
	if actor.Kind != change.ActorAgent {
		return actor.Name
	}
	if actor.Name == "" {
		return "agent"
	}
	return "agent/" + actor.Name
}

func decisionOutcome(changed bool, err error) (change.OpStatus, *change.Error) {
	if err != nil {
		return "", &change.Error{Code: change.CodeInvalid, Message: err.Error()}
	}
	if !changed {
		return change.OpUnchanged, nil
	}
	return change.OpApplied, nil
}
