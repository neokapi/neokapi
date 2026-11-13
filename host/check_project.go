package host

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
)

// ProjectCheckRequest asks for `kapi check` over files a project declares, and
// over the translations of each file in the languages it names.
type ProjectCheckRequest struct {
	// Recipe is the kapi.yaml the files are declared in. Empty checks them as
	// files outside any project: by the formats their extensions name, with no
	// voice and no terms.
	Recipe string
	// Root is the project directory. A file the check leaves unread is named
	// relative to it in the report's warnings. Empty takes the recipe's
	// directory.
	Root string
	// SourceLang is the language the sources are read in. Empty takes the
	// project's.
	SourceLang string
	// Files are the declared source files, with the translations of each to
	// check beside it.
	Files []ProjectCheckFileRef
}

// ProjectCheckFileRef is one declared source file and its translations.
type ProjectCheckFileRef struct {
	// Path is the source file, absolute.
	Path string
	// Targets are the translations to check against the source. A target whose
	// file does not exist is skipped: nothing is translated there yet.
	Targets []ProjectCheckTargetRef
}

// ProjectCheckTargetRef names one translation of a source file.
type ProjectCheckTargetRef struct {
	// Locale is the translation's language, canonical BCP-47.
	Locale string
	// Path is the translation's file, absolute: the target path the project
	// resolves for the source and the locale (project.ProjectContext.TargetFile).
	Path string
}

// ProjectCheck is a project check's report with what it read, grouped by
// source file, for a surface that shows each finding in its block.
type ProjectCheck struct {
	// Report is the canonical report, as `kapi check` builds it. Its findings
	// are every finding in Files.
	Report check.Report
	// Files are the source files the check met, in the order requested. A
	// file it could not read holds no blocks and the failing finding that
	// says so. A file no installed reader opens is absent, and named in the
	// report's warnings.
	Files []ProjectCheckFile
}

// ProjectCheckFile is one source file a project check read.
type ProjectCheckFile struct {
	// Path is the source file as requested.
	Path string
	// Blocks are the blocks the check read from the file: the reader's, then
	// the comment layer's when the recipe declares the file's comments.
	Blocks []*model.Block
	// Findings are the findings on the source.
	Findings []check.Diagnostic
	// Targets are the translations the check read, in the order requested.
	Targets []ProjectCheckTarget
}

// ProjectCheckTarget is one translation a project check read.
type ProjectCheckTarget struct {
	Locale string
	Path   string
	// Blocks are the source file's blocks with this translation overlaid.
	Blocks []*model.Block
	// Findings are the findings on the translation: placeholder integrity, the
	// terms in force for its language (protected terms among them), and the
	// rules that say a unit is not translated.
	Findings []check.Diagnostic
}

// ComputeProjectCheck runs the check `kapi check` runs over a project's
// declared content (ComputeDeclaredCheck) on the files req names, and holds
// each file's translations to the checks `kapi check SOURCE --target TARGET`
// runs. Every finding a surface shows for the project comes from here, so the
// surface and the command line agree about the same content.
//
// A declared file no installed reader opens is left out and named in the
// report's warnings, as a check of the whole project leaves it out.
func (a *App) ComputeProjectCheck(ctx context.Context, req ProjectCheckRequest) (ProjectCheck, error) {
	var cmd Command
	if req.Recipe == "" {
		cmd = detachedCommand(ctx, "check")
	} else {
		cmd = NewEnvCommand(ctx, "check")
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		AddProjectFlag(cmd)
		if err := cmd.Flags().Set(projectFlagName, req.Recipe); err != nil {
			return ProjectCheck{}, err
		}
	}
	if req.SourceLang != "" {
		defer a.scopeSourceLang()()
		a.SourceLang = ResolveSourceLocale(req.SourceLang, "")
	}
	result := &ProjectCheck{}
	if len(req.Files) == 0 {
		// Nothing declared is selected: the check read nothing, which its
		// verdict says.
		report := check.BuildReport(check.Target{Kind: "project"}, nil)
		report.Execution = &check.Execution{}
		report.Decide()
		result.Report = report
		return *result, nil
	}
	files := make([]string, len(req.Files))
	targets := make(map[string][]ProjectCheckTargetRef, len(req.Files))
	for i, f := range req.Files {
		files[i] = f.Path
		targets[f.Path] = f.Targets
	}
	root := req.Root
	if root == "" && req.Recipe != "" {
		root = filepath.Dir(req.Recipe)
	}
	pass := &projectPass{targets: targets, result: result, root: root}
	report, err := a.computeCheck(cmd, files, checkRequest{declared: true, project: pass}) //nolint:contextcheck // ctx travels on cmd, which computeCheck reads through CmdContext
	if err != nil {
		return ProjectCheck{}, err
	}
	report.Target.Kind = "project"
	result.Report = report
	return *result, nil
}

// checkRequest is what computeCheck is asked to check besides the files it is
// handed.
type checkRequest struct {
	// declared reports files the project declares rather than files a caller
	// named (ComputeDeclaredCheck).
	declared bool
	// project, when set, collects what the check read per file and checks each
	// file's translations (ComputeProjectCheck).
	project *projectPass
}

// projectPass is the part of a project check computeCheck runs per file.
type projectPass struct {
	targets map[string][]ProjectCheckTargetRef
	result  *ProjectCheck
	// root is the project directory unread files are named relative to.
	root string
}

// unreadName is how the report names a file the check leaves unread: relative
// to the project directory in a project check, and as given otherwise.
func (p *projectPass) unreadName(file string) string {
	if p == nil || p.root == "" {
		return file
	}
	if rel, err := filepath.Rel(p.root, file); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return file
}

// unreadable records a source file the check met and could not read, with the
// failing finding that says so.
func (p *projectPass) unreadable(file string, d check.Diagnostic) {
	if p == nil {
		return
	}
	p.result.Files = append(p.result.Files, ProjectCheckFile{Path: file, Findings: []check.Diagnostic{d}})
}

// checkFile records one source file the check read, and checks its
// translations, returning the findings on them.
func (p *projectPass) checkFile(ctx context.Context, a *App, cmd Command, file string, blocks []*model.Block, sourceDiags []check.Diagnostic, vocab *checkTerms, opts checkRunOptions) ([]check.Diagnostic, error) {
	pf := ProjectCheckFile{Path: file, Blocks: blocks, Findings: sourceDiags}
	var all []check.Diagnostic
	for _, t := range p.targets[file] {
		if _, err := os.Stat(t.Path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		fmtName, fmtCfg := opts.formats.forFile(a, file)
		// The target is the translated rendering of the source file, so both
		// carry the source's reader binding.
		unit := VerifyUnit{
			SourcePath:   file,
			TargetPath:   t.Path,
			Locale:       t.Locale,
			DisplayPath:  t.Path,
			SourceFormat: fmtName,
			SourceConfig: fmtCfg,
			TargetFormat: fmtName,
			TargetConfig: fmtCfg,
		}
		tblocks, missing, err := a.bilingualBlocks(ctx, unit)
		if err != nil {
			return nil, NoReaderError(err, DisplayName(file), fmtName, a.discoveredPlugins()...)
		}
		if missing {
			continue
		}
		termRules, err := vocab.rulesFor(file, t.Locale)
		if err != nil {
			return nil, err
		}
		// A translated rendering holds no comment layer, so its blocks sit at
		// the source file's own point.
		topts := opts
		topts.comments = nil
		diags, err := a.checkTranslation(ctx, cmd, tblocks, file, model.LocaleID(t.Locale), nil, termRules, topts)
		if err != nil {
			return nil, err
		}
		pf.Targets = append(pf.Targets, ProjectCheckTarget{Locale: t.Locale, Path: t.Path, Blocks: tblocks, Findings: diags})
		all = append(all, diags...)
	}
	p.result.Files = append(p.result.Files, pf)
	return all, nil
}

// checkTranslation runs the checks a translation is held to against its
// source, over blocks that carry the source with the translation in loc
// overlaid: placeholder integrity, the term rules for the language, the
// protected terms dnt names, and the rules that say a unit is not translated.
// Each finding names the point its block sits at.
func (a *App) checkTranslation(ctx context.Context, cmd Command, blocks []*model.Block, sourcePath string, loc model.LocaleID, dnt []string, termRules []profile.TermRule, opts checkRunOptions) ([]check.Diagnostic, error) {
	diags, err := a.collectBilingualDiagnostics(ctx, blocks, sourcePath, loc, dnt, termRules, opts)
	if err != nil {
		return nil, err
	}
	ruleDiags, err := a.collectTargetRuleDiagnostics(ctx, cmd, blocks, sourcePath, loc, opts.execution, opts.identity)
	if err != nil {
		return nil, err
	}
	diags = append(diags, ruleDiags...)
	opts.stampPoints(diags, blocks)
	return diags, nil
}
