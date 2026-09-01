package host

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/state"
)

// Source review: the author's half of the loop.
//
// The target ladder (draft→translated→reviewed→signed-off) has had a queue and a
// decision path since review shipped. The source ladder (authored→checked→
// approved) had neither, so `approved` was a rung nothing could reach: the
// settle derivation stamps `authored` or `checked` from the checks alone, and
// the branch in check.NewSourceReadinessTool that preserves a human sign-off was
// waiting on a sign-off no code path could record. A project asking for
// `source_gate: approved` therefore held its fan-out forever.
//
// This file is the missing half, in the shape the target side already has: a
// derived queue, and a decision recorded in the project state store bound to the
// wording it blessed.

// SourceUnitRef addresses one source unit: the source file as the queue lists it
// and the block key within it. There is no locale — a source unit is the same
// content for every target, which is the whole reason the gate holds the fan-out
// rather than one language of it.
type SourceUnitRef struct {
	File string
	Key  string
}

// sourceVariant is the variant a source decision is recorded under: the
// project's source locale.
//
// A target decision is keyed by the locale it judges, and a source decision is
// about the source locale, so the two cannot collide in the store unless a
// recipe lists its own source language among its targets — which is a recipe
// error, not a case to design for.
func sourceVariant(sourceLang string) model.VariantKey {
	return model.Variant(model.LocaleID(sourceLang))
}

// sourceApprovals maps a unit (document + block identity) to the source wording
// a human approved, loaded from the project state store.
//
// Only the basis is carried. The approval applies while the source still hashes
// to it, and a source edit is exactly what should drop it: an approval of a
// sentence is not an approval of the sentence that replaced it.
type sourceApprovals map[string]string

func (s sourceApprovals) approves(scope, unit, sourceText string) bool {
	basis, ok := s[sourceUnitKey(scope, unit)]
	return ok && basis != "" && basis == state.SourceHash(sourceText)
}

func sourceUnitKey(scope, unit string) string { return scope + "\x00" + unit }

// loadSourceApprovals reads the committed source approvals for a project.
func (a *App) loadSourceApprovals(ctx context.Context, root, sourceLang string) (sourceApprovals, error) {
	out := sourceApprovals{}
	if root == "" {
		return out, nil
	}
	st, err := a.OpenProjectState(ctx, root)
	if err != nil {
		return out, err
	}
	all, err := st.All(ctx)
	if err != nil {
		return out, err
	}
	want := sourceVariant(sourceLang)
	for _, u := range all {
		if u.SourceStatus != model.SourceStatusApproved || u.Variant != want {
			continue
		}
		out[sourceUnitKey(u.Scope, u.Unit)] = u.ContentHash
	}
	return out, nil
}

// SourceQueueItem is one source unit a person is being asked to look at: it sits
// below the project's source gate, or below `approved` when the gate asks for a
// human.
type SourceQueueItem struct {
	// File is the source file as the project names it, and Relative its
	// project-relative path. For source content the two are the same file, unlike
	// the target queue where they are a target and its source.
	File     string `json:"file"`
	Relative string `json:"relative,omitempty"`
	Key      string `json:"key"`

	Collection   string `json:"collection,omitempty"`
	SourceLocale string `json:"sourceLocale,omitempty"`
	Source       string `json:"source"`

	// Status is the settled source rung (authored|checked|approved).
	Status string `json:"status"`
	// Held reports that this unit ranks below the project's source gate, so the
	// loop is holding its translations. An unheld item is in the queue because
	// the gate asks for approval and it has not been approved, which is work
	// rather than a blockage.
	Held bool `json:"held"`
	// Approved reports a committed human approval that still blesses this exact
	// wording.
	Approved bool `json:"approved"`
}

// computeSourceQueue lists the source units awaiting authoring attention: every
// translatable source block that has not reached the project's source gate, plus
// (when the gate asks for `approved`) those that clear the checks but nobody has
// signed off.
//
// It settles exactly the way the coverage path and the in-flow gate settle,
// through the shared check.SettleSourceStatus, with one addition the others do
// not yet make: a committed approval is seeded onto the block first, so the
// "a clean re-check never undoes a human sign-off" branch can actually fire.
func (a *App) computeSourceQueue(ctx context.Context, proj *project.KapiProject, root string, units []VerifyUnit) ([]SourceQueueItem, error) {
	sourceLang := string(proj.Defaults.SourceLanguage)
	approvals, err := a.loadSourceApprovals(ctx, root, sourceLang)
	if err != nil {
		return nil, err
	}
	gateLevel, _ := convergeSourceGate(proj)
	docs := a.documentIndexOrEmpty(ctx, root)

	seen := map[string]bool{}
	var items []SourceQueueItem
	for _, u := range units {
		if seen[u.SourcePath] {
			continue // source content is shared across locales; judge it once
		}
		seen[u.SourcePath] = true

		blocks, berr := a.readSource(ctx, u)
		if berr != nil {
			if errors.Is(berr, registry.ErrUnknownFormat) {
				continue // no reader on this machine — reported by the coverage path
			}
			return nil, berr
		}
		scope := docs.Scope(root, u.SourcePath)
		display := relativeToRoot(root, u.SourcePath)

		for _, b := range blocks {
			if !b.Translatable || !model.RunsHaveContent(b.SourceRuns()) {
				continue
			}
			text := b.SourceText()
			approved := approvals.approves(scope, blockKey(b), text)
			if approved {
				b.SourceStatus = model.SourceStatusApproved
			}
			check.SettleSourceStatus(ctx, b)

			held := gateLevel != model.SourceGateNone && !gateLevel.Admits(b.SourceStatus)
			needsSignOff := gateLevel == model.SourceGateApproved &&
				b.SourceStatus != model.SourceStatusApproved
			if !held && !needsSignOff {
				continue
			}
			items = append(items, SourceQueueItem{
				File:         display,
				Relative:     display,
				Key:          blockKey(b),
				Collection:   u.Collection,
				SourceLocale: sourceLang,
				Source:       preview(text),
				Status:       string(b.SourceStatus),
				Held:         held,
				Approved:     b.SourceStatus == model.SourceStatusApproved,
			})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].File != items[j].File {
			return items[i].File < items[j].File
		}
		return items[i].Key < items[j].Key
	})
	return items, nil
}

// SourceQueue is the public entry point onto computeSourceQueue: it loads the
// project, resolves its source content and returns the units awaiting authoring
// attention.
func (a *App) SourceQueue(ctx context.Context, projectPath, sourceLang string) ([]SourceQueueItem, error) {
	a.InitRegistries()
	ctx = ctxOrBackground(ctx)

	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)
	a.SourceLang = ResolveSourceLocale(sourceLang, proj.Defaults.SourceLanguage)

	// The source gate measures the author's content, so it reads the project's
	// source directly: a monolingual project has a source to judge even though
	// it resolves no per-locale unit.
	units, err := a.SourceUnitsFromProject(proj, root)
	if err != nil {
		return nil, fmt.Errorf("resolve source content: %w", err)
	}
	return a.computeSourceQueue(ctx, proj, root, units)
}

// ApproveSourceUnit records a human approval of one source unit, bound to the
// wording in front of the approver: the record carries that wording's hash, so
// an edit to the source drops the approval rather than letting it outlive the
// sentence it blessed.
//
// It returns whether anything changed — an approval already recorded for this
// exact wording is not rewritten.
func (a *App) ApproveSourceUnit(ctx context.Context, projectPath, sourceLang string, ref SourceUnitRef) (bool, error) {
	a.InitRegistries()
	ctx = ctxOrBackground(ctx)

	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return false, fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)
	a.SourceLang = ResolveSourceLocale(sourceLang, proj.Defaults.SourceLanguage)

	units, err := a.UnitsFromProject(proj, root, "")
	if err != nil {
		return false, fmt.Errorf("resolve content: %w", err)
	}

	seen := map[string]bool{}
	for _, u := range units {
		if seen[u.SourcePath] || relativeToRoot(root, u.SourcePath) != ref.File {
			continue
		}
		seen[u.SourcePath] = true

		blocks, berr := a.readSource(ctx, u)
		if berr != nil {
			return false, berr
		}
		for _, b := range blocks {
			if !b.Translatable || blockKey(b) != ref.Key {
				continue
			}
			text := b.SourceText()
			if !model.RunsHaveContent(b.SourceRuns()) {
				return false, fmt.Errorf("source unit %s is empty — nothing to approve", ref.Key)
			}
			scope := a.documentIndexOrEmpty(ctx, root).Scope(root, u.SourcePath)
			return a.recordSourceApproval(ctx, root, scope, blockKey(b), text, string(a.SourceLang))
		}
	}
	return false, fmt.Errorf("source unit %q not found in %s", ref.Key, ref.File)
}

// recordSourceApproval writes the approval to the project state store, keyed by
// (document, unit, source locale) and bound to the source wording's hash.
func (a *App) recordSourceApproval(ctx context.Context, root, scope, unit, sourceText, sourceLang string) (bool, error) {
	st, err := a.OpenProjectState(ctx, root)
	if err != nil {
		return false, err
	}
	k := state.Key{Scope: scope, Unit: unit, Variant: sourceVariant(sourceLang)}
	basis := state.SourceHash(sourceText)

	prev, hadPrev := st.Get(ctx, k)
	if hadPrev && prev.SourceStatus == model.SourceStatusApproved && prev.ContentHash == basis {
		return false, nil // already approved, for this exact wording
	}
	now := nowRFC3339()
	next := state.UnitState{
		Unit:         unit,
		Variant:      sourceVariant(sourceLang),
		SourceStatus: model.SourceStatusApproved,
		ContentHash:  basis,
		Decision:     state.Decision{ReviewState: "approved", At: now},
		Updated:      now,
		Scope:        scope,
	}
	if hadPrev {
		// A source record and a target record never share a key, so anything
		// already here is a previous source decision: keep its advisory fields
		// and replace only the decision.
		next.Origin = prev.Origin
		next.Status = prev.Status
		next.TargetHash = prev.TargetHash
		next.AIReview = prev.AIReview
		next.ContextHash = prev.ContextHash
	}
	if err := st.Put(ctx, next); err != nil {
		return false, err
	}
	return true, nil
}
