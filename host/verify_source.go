package host

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// resolveVerifyCheckUnits includes declared source-only content beside real
// source/target pairs. A target locale filter narrows only the bilingual units.
// The declared comments of a file with targets get a source unit of their own:
// they are source-language content the bilingual units never read. A file
// declared for its comments alone gets a unit narrowed to them as well.
func (a *App) resolveVerifyCheckUnits(
	cmd Command,
	proj *project.KapiProject,
	root string,
	args []string,
	locale string,
) ([]VerifyUnit, error) {
	if len(args) > 0 {
		return a.unitsFromArgs(proj, root, args, locale)
	}
	units, err := a.UnitsFromProject(proj, root, locale)
	if err != nil {
		return nil, err
	}
	pctx := project.NewProjectContext(proj, filepath.Join(root, "kapi.yaml"))
	resolved, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return nil, err
	}
	seen, seenComments := map[string]bool{}, map[string]bool{}
	for _, rf := range resolved {
		hasTargets := rf.Item != nil && rf.Item.Target != "" && len(rf.Item.ResolvedTargetLanguages(nil, proj.Defaults)) > 0
		comments := rf.CommentItem != nil
		switch {
		case hasTargets && (!comments || seenComments[rf.Path]):
			continue
		case hasTargets:
			seenComments[rf.Path] = true
		case seen[rf.Path]:
			continue
		default:
			seen[rf.Path] = true
		}
		relative, err := filepath.Rel(root, rf.Path)
		if err != nil {
			return nil, err
		}
		units = append(units, VerifyUnit{
			SourcePath: rf.Path, Locale: a.SourceLocale(), Collection: rf.Collection,
			DisplayPath: relative, ProjectRoot: root, SourceFormat: rf.Format,
			SourceConfig: mergedFormatConfig(proj, rf.Format, rf.Item),
			Comments:     comments, OnlyComments: hasTargets || narrowedToComments(rf), Directives: commentDirectives(proj, rf.CommentItem),
		})
	}
	return units, nil
}

// narrowedToComments reports a file declared for its comments alone that a
// format names, whose unit reads the comments that format supplies and no
// value. A comments-only file no format names is read through its language's
// comment provider, or reported unread when no provider reads it.
func narrowedToComments(rf project.ResolvedFile) bool {
	return rf.CommentsOnly() && rf.Format != ""
}

func targetVerifyUnits(units []VerifyUnit) []VerifyUnit {
	targets := make([]VerifyUnit, 0, len(units))
	for _, u := range units {
		if u.TargetPath != "" {
			targets = append(targets, u)
		}
	}
	return targets
}

func (a *App) verifySourceChecks(ctx context.Context, cmd Command, u VerifyUnit, gate *verifyGateResult, warnings *voiceWarnings) error {
	execution := newCheckExecution()
	extractionStart := time.Now()
	blocks, formatterDiags, err := a.readSourceForCheck(ctx, u, execution)
	if err != nil {
		return err
	}
	execution.Timings.ExtractionMS = elapsedMS(extractionStart)
	contextStart := time.Now()
	voice, err := a.newCheckVoice(cmd, warnings)
	if err != nil {
		return err
	}
	defer voice.close()
	vocab, err := a.newCheckTerms(cmd)
	if err != nil {
		return err
	}
	g, err := a.governFile(ctx, voice, vocab, u.SourcePath, atPoint{})
	if err != nil {
		return err
	}
	execution.Timings.ContextMS = elapsedMS(contextStart)
	opts := checkRunOptions{execution: execution}.govern(g)
	opts.maxChars, _ = cmd.Flags().GetInt("max-chars")
	opts.maxWords, _ = cmd.Flags().GetInt("max-words")
	opts.forbid, _ = cmd.Flags().GetStringSlice("forbid")
	opts.require, _ = cmd.Flags().GetStringSlice("require")
	opts.voice, _ = cmd.Flags().GetBool("voice")
	opts.voiceMin, _ = cmd.Flags().GetFloat64("voice-min")
	diagnostics, err := a.collectFileDiagnostics(ctx, blocks, u.DisplayPath, opts)
	if err != nil {
		return err
	}
	diagnostics = append(formatterDiags, diagnostics...)
	opts.stampPoints(diagnostics, blocks)
	if gate.Execution == nil {
		gate.Execution = &check.Execution{Analyzers: []check.AnalyzerExecution{}}
	}
	execution.recordContexts(u.DisplayPath, "", opts, blocks)
	gate.Execution.Contexts = append(gate.Execution.Contexts, execution.Contexts...)
	gate.Execution.Analyzers = append(gate.Execution.Analyzers, execution.Analyzers...)
	gate.Execution.Timings.AnalyzersMS += execution.Timings.AnalyzersMS
	gate.Execution.Timings.ExtractionMS += execution.Timings.ExtractionMS
	gate.Execution.Timings.ContextMS += execution.Timings.ContextMS
	gate.Execution.Timings.TotalMS += elapsedMS(execution.started)
	gate.Coverage.Files++
	gate.Coverage.Blocks += len(blocks)
	for _, run := range execution.Analyzers {
		if run.Status != check.AnalyzerInvalid {
			continue
		}
		gate.Pass = false
		gate.Findings = append(gate.Findings, verifyFinding{Gate: gateChecks, File: u.DisplayPath, Locale: u.Locale, Severity: "error",
			Message: fmt.Sprintf("The %s check missed its canary, so its result for this file cannot be trusted. %s", run.ID, run.Reason)})
	}
	for _, d := range diagnostics {
		failing := d.Severity == check.SeverityMajor || d.Severity == check.SeverityCritical
		severity := verifySeverity(d.Severity)
		if failing {
			gate.Pass = false
			severity = "error"
		}
		gate.Findings = append(gate.Findings, verifyFinding{Gate: gateChecks, File: u.DisplayPath,
			Block: d.Location.Block, Locale: u.Locale, Severity: severity, Message: d.Message, Suggestion: d.Suggestion, Point: d.Point})
	}
	return nil
}

// Source terminology judges each occurrence in its own language using the same
// scoped vocabulary matcher as file checks. No synthetic target is introduced.
// Each block is judged against the vocabulary at the point it sits at. governed
// reports whether terms govern either of the file's points, decided before the
// file is read.
func (a *App) verifySourceTerminology(ctx context.Context, vocab *checkTerms, u VerifyUnit, gate *verifyGateResult, execution *checkExecution) (governed bool, err error) {
	g, err := a.governFile(ctx, nil, vocab, u.SourcePath, atPoint{})
	if err != nil {
		return false, err
	}
	if g.content.terms == nil && g.comments.terms == nil {
		return false, nil
	}
	blocks, _, err := a.readSourceForCheck(ctx, u, nil)
	if err != nil {
		return true, err
	}
	gate.Coverage.Files++
	for _, group := range (checkRunOptions{}).govern(g).pointGroups(blocks, blocks) {
		if group.at.terms == nil {
			continue
		}
		checker := coretools.NewVoiceVocabCheckTool(nil, group.at.terms).InSourceLocale(model.LocaleID(a.SourceLocale()))
		gate.Coverage.Blocks += len(group.blocks)
		start, before := time.Now(), len(gate.Findings)
		for _, b := range group.blocks {
			findings, err := runVoiceVocabOnBlock(ctx, checker, b)
			if err != nil {
				return true, fmt.Errorf("source terminology %s: %w", u.DisplayPath, err)
			}
			for _, f := range findings {
				finding := voiceFindingToVerify(u.DisplayPath, blockKey(b), f)
				finding.Gate = gateTerms
				finding.Locale = u.Locale
				finding.Point = clonePoint(group.at.point)
				if finding.Severity == "error" {
					gate.Pass = false
				}
				gate.Findings = append(gate.Findings, finding)
			}
		}
		canary, err := probeVoiceRules(ctx, checker, nil)
		if err != nil {
			return true, fmt.Errorf("source terminology %s: %w", u.DisplayPath, err)
		}
		mark := execution.analyzerCount()
		execution.completed("terms.source", u.DisplayPath, len(gate.Findings)-before, start, canary, false)
		if group.apart {
			execution.pointAnalyzers(mark, group.at.point)
		}
	}
	return true, nil
}
