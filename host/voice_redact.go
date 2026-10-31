package host

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
)

// `kapi voice check --ai` and `kapi terms expand` ask a model outside a run of
// a registry tool, so each applies the project's redaction policy here. The
// voice check runs its tool between redact and unredact, as `kapi exec` does.
// Terms expansion holds no block, so it redacts its requests under
// `defaults.redaction`.

// VoiceCheckTools returns the tools `kapi voice check --ai` applies to its
// text. Under a policy that covers the text, t runs between redact and
// unredact; otherwise t runs alone. file is the path the text was read from,
// which selects the policy. Text given inline or on stdin ("") reads under
// `defaults.redaction`.
func (a *App) VoiceCheckTools(cmd Command, file string, t tool.Tool) ([]tool.Tool, error) {
	recipe, err := ResolveProjectPath(cmd)
	if err != nil || recipe == "" {
		return []tool.Tool{t}, err
	}
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project for redaction: %w", err)
	}
	return a.RedactionChain(recipe, proj, file, "voice-check", map[string]any{}, t)
}

// RedactionChain returns the tools a run of the registry tool toolName (built
// as t with config) applies to the blocks of file in the project at recipe.
// Under the policy that covers the file, t runs between redact and unredact,
// as `kapi exec` places them. With no policy, or for a tool that sends no
// block out, t runs alone. file is an absolute path, or "" for content in no
// file, which reads under `defaults.redaction`. A surface that runs one tool
// over a block it holds, such as the desktop's review actions, builds its run
// with it.
func (a *App) RedactionChain(recipe string, proj *project.KapiProject, file, toolName string, config map[string]any, t tool.Tool) ([]tool.Tool, error) {
	if recipe == "" || proj == nil {
		return []tool.Tool{t}, nil
	}
	a.InitRegistries()
	pol, err := a.toolRunPolicyFor(recipe, proj, file, toolName, config)
	if err != nil {
		return nil, err
	}
	chain, err := a.chainFor(pol, toolName, config, t)
	if err != nil {
		return nil, err
	}
	return chain.tools(t), nil
}

// RunTools runs each tool of a chain in turn over one block, as RedactionChain
// composes it.
func RunTools(ctx context.Context, chain []tool.Tool, b *model.Block) error {
	_, err := runChain(ctx, chain, []*model.Part{{Type: model.PartBlock, Resource: b}})
	return err
}

// TermsExpandRedaction returns ctx carrying the redaction `kapi terms expand`
// applies to the terms it sends: the rules of `defaults.redaction`. A term
// sits in no file, and no redact step has read it. ctx is returned as it is
// when the command resolves no project, or the project declares no default
// policy. A policy that detects entities is refused, as it is for a run of
// one tool, since no extraction step finds them in a term list.
//
// A term the policy withholds goes out as its token. The model's forms for a
// token name no term that was asked about, so no form is written for it.
func (a *App) TermsExpandRedaction(ctx context.Context, cmd Command) (context.Context, error) {
	recipe, err := ResolveProjectPath(cmd)
	if err != nil || recipe == "" {
		return ctx, err
	}
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return ctx, fmt.Errorf("load project for redaction: %w", err)
	}
	resolved := proj.RedactionForPath("")
	if resolved.Spec == nil {
		return ctx, nil
	}
	if hasDetector(resolved.Spec.Detectors, "entities") {
		return ctx, fmt.Errorf("the project's redaction policy (%s) detects entities, which needs an entity-extract step that `kapi terms expand` does not have, so the terms would go out with the values the policy withholds. Set %s.detectors to [rules]", resolved.At, resolved.At)
	}
	if err := checkRedactionRules(resolved.Spec); err != nil {
		return ctx, err
	}
	locale := proj.Defaults.SourceLanguage
	if locale.IsEmpty() {
		locale = model.LocaleID(a.SourceLocale())
	}
	rt, err := tools.NewRedactTool(redactToolConfig(resolved.Spec, filepath.Dir(recipe), "", locale))
	if err != nil {
		return ctx, fmt.Errorf("redaction: %w", err)
	}
	return redaction.WithEgress(ctx, redaction.NewEgress(rt.Policy())), nil
}
