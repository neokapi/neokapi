package host

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
)

// UnitRef names one unit of content for a config assembly: where it sits in the
// project, and the locale a tool would run into for it.
//
// It is the point a run resolves governance at, in the form a per-unit surface
// already has it. Collection may be empty, which is what a flow run passes: the
// content item claiming Path decides the collection anyway.
type UnitRef struct {
	// Collection is the recipe collection the unit's file belongs to.
	Collection string
	// Path is the unit's source file relative to the project root,
	// slash-separated.
	Path string
	// TargetLang is the locale a tool would produce or judge for this unit.
	TargetLang string
}

// ToolConfigForUnit returns the config a flow run would hand toolName for one
// unit of content: the project's tool preset and the preset for the unit's
// locale, the voice profile and term rules governing the unit's point, the
// point itself, and the content memory. base carries the caller's own keys and
// wins over everything the project supplies, exactly as a flow step's config
// wins over its preset.
//
// It exists so a per-unit surface (a Review action, a pre-review judge) governs
// its model calls the way a run does. Building the same tool by hand is how the
// two diverged: a Review retranslation was off-voice by construction while the
// checks beside it judged that unit against the voice. So this calls the flow
// runner's own assembly (resolveBindingsFor, applyBindingsFor, grantMemory)
// rather than restating it. Two copies drift.
//
// The returned cleanup releases any store the memory grant opened and is always
// safe to call. proj and recipePath may be zero for a caller with no project in
// scope, which yields base with only the memory grant applied.
func (a *App) ToolConfigForUnit(
	ctx context.Context,
	proj *project.KapiProject,
	recipePath, toolName string,
	unit UnitRef,
	base map[string]any,
) (map[string]any, func(), error) {
	noop := func() {}
	if a.ToolReg == nil {
		return nil, noop, errors.New("tool config: no tool registry")
	}
	if toolName == "" {
		return nil, noop, errors.New("tool config: no tool named")
	}

	// Cloned up front: base belongs to the caller, and the assembly below adds
	// a live voice profile and content-memory handle to what it is given.
	config := make(map[string]any, len(base)+6)
	maps.Copy(config, base)

	cmd, err := a.unitCommand(ctx, recipePath)
	if err != nil {
		return nil, noop, err
	}

	if proj != nil && recipePath != "" {
		// The recipe's source language, for the length of the assembly and no
		// longer: term rules are keyed by (source, target), and an App shared
		// across projects must not carry one recipe's answer into the next.
		defer a.scopeSourceLang()()
		a.ResolveSourceLang(proj.Defaults.SourceLanguage)

		point := a.GovernancePointFor(unit.Collection, unit.Path)
		b, berr := a.resolveBindingsFor(cmd, proj, recipePath, point, unit.TargetLang)
		if berr != nil {
			return nil, noop, fmt.Errorf("resolve bindings for %s: %w", toolName, berr)
		}
		config = a.applyBindingsFor(b, toolName, a.ToolReg.Schema(registry.ToolID(toolName)), config, unit.TargetLang)
	}

	return a.grantMemory(toolName, config, cmd)
}

// unitCommand builds the project-bound Command the resolvers read their scope
// from: the recipe path, and the resource flags whose absence means "the
// project's own store" (OpenToolMemory, ResolveTermsStore). It is the synthetic
// command an embedded run already uses, so the resolution ladder is the one a
// `kapi` invocation walks.
func (a *App) unitCommand(ctx context.Context, recipePath string) (Command, error) {
	cmd := NewEnvCommand(ctx, "tool-config")
	AddProjectFlag(cmd)
	cmd.Flags().String("memory", "", "named content memory")
	cmd.Flags().String("termstore", "", "named terms store")
	if recipePath != "" {
		if err := cmd.Flags().Set(projectFlagName, recipePath); err != nil {
			return nil, fmt.Errorf("bind project: %w", err)
		}
	}
	return cmd, nil
}

// stepConfigForUnit is ToolConfigForUnit expressed as the flow step it stands
// in for: what toolFromStep would assemble for a step running toolName with
// base as its config, at the unit's point and locale. Tests use it to hold the
// two paths against each other.
func (a *App) stepConfigForUnit(
	ctx context.Context,
	proj *project.KapiProject,
	recipePath, toolName string,
	unit UnitRef,
	base map[string]any,
) (map[string]any, func(), error) {
	cmd, err := a.unitCommand(ctx, recipePath)
	if err != nil {
		return nil, func() {}, err
	}
	savedTarget, savedBindings := a.TargetLang, a.ProjectBindings
	defer func() { a.TargetLang, a.ProjectBindings = savedTarget, savedBindings }()
	a.TargetLang = unit.TargetLang

	if proj != nil && recipePath != "" {
		defer a.scopeSourceLang()()
		a.ResolveSourceLang(proj.Defaults.SourceLanguage)

		point := a.GovernancePointFor(unit.Collection, unit.Path)
		b, berr := a.resolveProjectBindings(cmd, proj, recipePath, point)
		if berr != nil {
			return nil, func() {}, berr
		}
		a.ProjectBindings = b
	}
	return a.stepToolConfig(flow.FlowStep{Tool: toolName, Config: base}, cmd, nil)
}
