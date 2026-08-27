package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/yamledit"
)

// `kapi voice expand` fills in the surface forms a profile's terms take.
//
// A rule names a word and prose uses that word inflected, so a profile
// forbidding `utilize` passed "the platform utilizes your data" and one
// forbidding `løsning` passed "løsningen", "løsninger" and "løsningene" (#2226).
//
// Deriving the forms was tried and belongs to nobody: English suffix rules gave
// English the right answer, Norwegian a set of non-words, and every other
// language nothing. Morphology is per-language knowledge and the tools that
// hold it, LanguageTool and Acrolinx, ship a linguistic pack per language.
//
// A model has that knowledge for every language, so this asks once, at
// authoring time, and writes the answer into the profile. What the check then
// consumes is a list a person reviewed in a diff, matched exactly: free,
// deterministic, and as good in Norwegian as in English.

func newVoiceExpandCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "expand",
		Short: "Fill in the surface forms a profile's terms take (writes the profile)",
		Long: `Ask a model for the other shapes each vocabulary term takes (inflections,
declensions, conjugations) and write them into the profile as ` + "`forms:`" + `.

The check matches those forms exactly, so this is authoring-time work whose
result you review in a diff. Run it again after adding terms; rules that already
carry forms are left alone unless --overwrite is given.

  kapi voice expand --profile-file voice.yaml --language nb
  kapi voice expand --profile-file voice.yaml --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, src, err := a.ResolveVoiceProfileCmd(cmd, args...)
			if err != nil {
				return err
			}
			// The profile has to be one kapi can read before it is rewritten.
			if probs := coreprofile.ValidateProfile(profile); len(probs) > 0 {
				return fmt.Errorf("%s is not a usable voice profile:%s",
					VoiceProfileLabel(profile), problemLines(probs))
			}

			lang, _ := cmd.Flags().GetString("language")
			if strings.TrimSpace(lang) == "" {
				lang = a.SourceLocale()
			}
			if strings.TrimSpace(lang) == "" {
				return errors.New("no language: pass --language, or run inside a project that sets source_language.\n" +
					"The forms of a word depend on which language it is in, and guessing from the term's spelling " +
					"is how a borrowed word gets the wrong ones")
			}

			overwrite, _ := cmd.Flags().GetBool("overwrite")
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			targets := expandTargets(profile, overwrite)
			if len(targets) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Every rule already carries forms. Use --overwrite to redo them.")
				return nil
			}

			p, err := a.BuildVoiceProvider(cmd)
			if err != nil {
				return err
			}
			defer p.Close()

			expansions, err := aitools.ExpandTermForms(cmd.Context(), p, targets, lang)
			if err != nil {
				return err
			}
			applied := applyExpansions(profile, expansions, overwrite)

			out := cmd.OutOrStdout()
			for _, e := range expansions {
				if len(e.Forms) == 0 {
					fmt.Fprintf(out, "  %-24s no forms%s\n", e.Term, rejectedNote(e))
					continue
				}
				fmt.Fprintf(out, "  %-24s + %s%s\n", e.Term, strings.Join(e.Forms, ", "), rejectedNote(e))
			}
			if applied == 0 {
				fmt.Fprintln(out, "\nNothing to write.")
				return nil
			}
			if dryRun {
				fmt.Fprintf(out, "\n%d rule(s) would gain forms in %s. Re-run without --dry-run to write them.\n", applied, src)
				return nil
			}
			if src == "" {
				return fmt.Errorf("expanded %d rule(s) but there is no file to write: "+
					"--profile-file names one, while --profile and --pack resolve a profile this command cannot save over", applied)
			}
			if _, err := yamledit.WriteFile(src, profile, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(out, "\n%d rule(s) expanded in %s. Review the diff before committing.\n", applied, src)
			return nil
		},
	}
	AddProfileFlags(cmd)
	// Not AddVoiceAIFlags: that carries `--ai`, which opts a check into using a
	// model. This command is the model, so opting in has no meaning, and it
	// needs `--model` that the shared set does not offer.
	cmd.Flags().String("provider", "", "AI provider (default: anthropic)")
	cmd.Flags().String("model", "", "AI model name")
	cmd.Flags().String("api-key", "", "API key for the AI provider")
	cmd.Flags().String("credential", "", "saved credential name (see 'kapi credentials list')")
	cmd.Flags().String("language", "", "language the terms are in (defaults to the project's source language)")
	cmd.Flags().Bool("overwrite", false, "redo rules that already carry forms")
	cmd.Flags().Bool("dry-run", false, "print what would be added and write nothing")
	return cmd
}

// rejectedNote reports what the filter dropped, so a short list reads as a
// decision rather than as a model that answered thinly.
func rejectedNote(e aitools.TermExpansion) string {
	if len(e.Rejected) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e.Rejected))
	for _, r := range e.Rejected {
		parts = append(parts, fmt.Sprintf("%s (%s)", r.Form, r.Reason))
	}
	return "   dropped: " + strings.Join(parts, ", ")
}

// expandTargets is the terms to ask about: every rule, or only those with no
// forms yet.
func expandTargets(p *coreprofile.VoiceProfile, overwrite bool) []string {
	var out []string
	forEachRule(p, func(r *coreprofile.TermRule) {
		if overwrite || len(r.Forms) == 0 {
			out = append(out, r.Term)
		}
	})
	return out
}

// applyExpansions writes the forms onto their rules and reports how many rules
// changed.
func applyExpansions(p *coreprofile.VoiceProfile, expansions []aitools.TermExpansion, overwrite bool) int {
	byTerm := make(map[string][]string, len(expansions))
	for _, e := range expansions {
		if len(e.Forms) > 0 {
			byTerm[strings.ToLower(e.Term)] = e.Forms
		}
	}
	applied := 0
	forEachRule(p, func(r *coreprofile.TermRule) {
		forms, ok := byTerm[strings.ToLower(r.Term)]
		if !ok || (len(r.Forms) > 0 && !overwrite) {
			return
		}
		r.Forms = forms
		applied++
	})
	return applied
}

// forEachRule visits every vocabulary rule in the profile, by pointer.
func forEachRule(p *coreprofile.VoiceProfile, fn func(*coreprofile.TermRule)) {
	if p == nil {
		return
	}
	for _, set := range []*[]coreprofile.TermRule{
		&p.Vocabulary.ForbiddenTerms, &p.Vocabulary.CompetitorTerms, &p.Vocabulary.PreferredTerms,
	} {
		for i := range *set {
			fn(&(*set)[i])
		}
	}
}
