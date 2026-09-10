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
	seen := map[string]bool{}
	for _, rf := range resolved {
		hasTargets := rf.Item != nil && rf.Item.Target != "" && len(rf.Item.ResolvedTargetLanguages(nil, proj.Defaults)) > 0
		if hasTargets || seen[rf.Path] {
			continue
		}
		seen[rf.Path] = true
		relative, err := filepath.Rel(root, rf.Path)
		if err != nil {
			return nil, err
		}
		units = append(units, VerifyUnit{
			SourcePath: rf.Path, Locale: a.SourceLocale(), Collection: rf.Collection,
			DisplayPath: relative, ProjectRoot: root, SourceFormat: rf.Format,
			SourceConfig: mergedFormatConfig(proj, rf.Format, rf.Item),
		})
	}
	return units, nil
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

func (a *App) verifySourceChecks(ctx context.Context, cmd Command, u VerifyUnit, gate *verifyGateResult) error {
	execution := newCheckExecution()
	extractionStart := time.Now()
	blocks, err := a.readSource(ctx, u)
	if err != nil {
		return err
	}
	execution.Timings.ExtractionMS = elapsedMS(extractionStart)
	contextStart := time.Now()
	voice, err := a.newCheckVoice(cmd)
	if err != nil {
		return err
	}
	defer voice.close()
	profile, err := voice.forFile(ctx, u.SourcePath)
	if err != nil {
		return err
	}
	vocab, err := a.ProjectTermsForFile(ctx, cmd, u.SourcePath)
	if err != nil {
		return err
	}
	execution.Timings.ContextMS = elapsedMS(contextStart)
	opts := checkRunOptions{profile: profile, terms: vocab, execution: execution}
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
	if gate.Execution == nil {
		gate.Execution = &check.Execution{Analyzers: []check.AnalyzerExecution{}}
	}
	gate.Execution.Analyzers = append(gate.Execution.Analyzers, execution.Analyzers...)
	gate.Execution.Timings.AnalyzersMS += execution.Timings.AnalyzersMS
	gate.Execution.Timings.ExtractionMS += execution.Timings.ExtractionMS
	gate.Execution.Timings.ContextMS += execution.Timings.ContextMS
	gate.Execution.Timings.TotalMS += elapsedMS(execution.started)
	gate.Coverage.Files++
	gate.Coverage.Blocks += len(blocks)
	for _, d := range diagnostics {
		failing := d.Severity == check.SeverityMajor || d.Severity == check.SeverityCritical
		severity := verifySeverity(d.Severity)
		if failing {
			gate.Pass = false
			severity = "error"
		}
		gate.Findings = append(gate.Findings, verifyFinding{Gate: gateChecks, File: u.DisplayPath,
			Block: d.Location.Block, Locale: u.Locale, Severity: severity, Message: d.Message, Suggestion: d.Suggestion})
	}
	return nil
}

// Source terminology judges each occurrence in its own language using the same
// scoped vocabulary matcher as file checks. No synthetic target is introduced.
func (a *App) verifySourceTerminology(ctx context.Context, vocab *checkTerms, u VerifyUnit, gate *verifyGateResult) error {
	store, err := vocab.forFile(ctx, u.SourcePath)
	if err != nil {
		return err
	}
	if store == nil {
		return nil
	}
	blocks, err := a.readSource(ctx, u)
	if err != nil {
		return err
	}
	checker := coretools.NewVoiceVocabCheckTool(nil, store).InSourceLocale(model.LocaleID(a.SourceLocale()))
	gate.Coverage.Files++
	gate.Coverage.Blocks += len(blocks)
	for _, b := range blocks {
		findings, err := runVoiceVocabOnBlock(ctx, checker, b)
		if err != nil {
			return fmt.Errorf("source terminology %s: %w", u.DisplayPath, err)
		}
		for _, f := range findings {
			finding := voiceFindingToVerify(u.DisplayPath, blockKey(b), f)
			finding.Gate = gateTerms
			finding.Locale = u.Locale
			if finding.Severity == "error" {
				gate.Pass = false
			}
			gate.Findings = append(gate.Findings, finding)
		}
	}
	return nil
}
