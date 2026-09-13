package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/terms"
)

func newTermsExpandCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "expand [bundle]",
		Short: "Propose the surface forms each term takes (writes the terms)",
		Long: `Ask a model for the other shapes each term takes in its own language (the
plural, the definite form, a case ending) and write them onto the term as forms.

A declared form is how kapi recognises a rendering that changes the word: the
Norwegian plural "varsler" is a use of the term "varsel" once the term lists it.

By default this asks about target terms, meaning every language except the source
language, and leaves terms that already declare forms alone. It writes the bundle
named on the command line, or the project's committed terms source, or else the
store the terms flags select. It prints each proposal and the forms the filters
dropped; review the diff before committing it.`,
		Example: "  kapi terms expand --dry-run\n  kapi terms expand --locale nb --locale de\n  kapi terms expand .kapi/terms.json --overwrite",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := a.OpenTermsFormsTarget(cmd, firstArg(args))
			if err != nil {
				return err
			}
			defer target.Close()

			ctx := cmd.Context()
			concepts, err := target.Concepts(ctx)
			if err != nil {
				return err
			}

			overwrite, _ := cmd.Flags().GetBool("overwrite")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			opts := host.TermsExpandOptions{SourceLocale: termsSourceLocale(a, cmd), Overwrite: overwrite}
			locales, _ := cmd.Flags().GetStringSlice("locale")
			for _, l := range locales {
				if l = strings.TrimSpace(l); l != "" {
					opts.Locales = append(opts.Locales, model.LocaleID(l))
				}
			}

			out := output.TermsExpandOutput{Target: target.Label, DryRun: dryRun, Terms: []output.TermsExpandEntry{}}
			if host.ExpandableTerms(concepts, opts) == 0 {
				return output.Print(cmd, out)
			}

			p, err := a.BuildVoiceProvider(cmd)
			if err != nil {
				return err
			}
			defer p.Close()

			proposals, err := host.ProposeTermForms(ctx, p, concepts, opts)
			if err != nil {
				return err
			}
			changed, count := host.ApplyFormsProposals(concepts, proposals, overwrite)
			out.Changed = count
			for _, prop := range proposals {
				entry := output.TermsExpandEntry{ConceptID: prop.ConceptID, Locale: string(prop.Locale), Term: prop.Term, Forms: prop.Forms}
				for _, r := range prop.Rejected {
					entry.Rejected = append(entry.Rejected, output.TermsRejectedForm{Form: r.Form, Reason: r.Reason})
				}
				out.Terms = append(out.Terms, entry)
			}
			if !dryRun {
				if out.Written, err = target.Save(ctx, concepts, changed); err != nil {
					return err
				}
			}
			return output.Print(cmd, out)
		},
	}
	addTermsSourceLocaleFlag(cmd)
	cmd.Flags().StringSlice("locale", nil, "only terms in these locales; a bare language matches every region (default: every language except the source language)")
	cmd.Flags().Bool("overwrite", false, "ask again about terms that already declare forms, and replace them")
	cmd.Flags().Bool("dry-run", false, "print what would be added and write nothing")
	// Not AddVoiceAIFlags: that carries `--ai`, which opts a check into using a
	// model. This command is the model call, and it needs `--model`.
	cmd.Flags().String("provider", "", "AI provider (default: anthropic)")
	cmd.Flags().String("model", "", "AI model name")
	cmd.Flags().String("api-key", "", "API key for the AI provider")
	cmd.Flags().String("credential", "", "saved credential name (see 'kapi credentials list')")
	output.AddFlags(cmd.Flags())
	return cmd
}

func newTermsValidateCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [bundle]",
		Short: "Check terms for structural problems and missing forms",
		Long: `Check a set of terms and report what is wrong with them, or what a check will
read differently from what the author meant.

Errors fail the command: a concept with no id or an id used twice, a term with no
text or no locale, a status outside the lifecycle vocabulary.

Warnings do not. A target term in a language that inflects, with no forms
declared, is recognised only where a rendering contains its exact spelling, so
the plural "varsler" is not a use of "varsel" until the term lists it (see
` + "`kapi terms expand`" + `). A form spelled the same as another term in the same
language makes one word a use of two entries.

It reads the bundle named on the command line, or the project's committed terms
source, or else the store the terms flags select.

Exit codes: 0 when there are no errors, 1 otherwise. With --json the result is
{"valid", "source", "concepts", "problems": [{"concept_id", "locale", "term",
"message", "warning"}]}.`,
		Example: "  kapi terms validate\n  kapi terms validate .kapi/terms.json --json",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := a.OpenTermsFormsTarget(cmd, firstArg(args))
			if err != nil {
				return err
			}
			defer target.Close()

			concepts, err := target.Concepts(cmd.Context())
			if err != nil {
				return err
			}
			out := output.TermsValidateOutput{Source: target.Label, Concepts: len(concepts), Problems: []output.TermsProblem{}, Valid: true}
			for _, p := range terms.ValidateConcepts(concepts, termsSourceLocale(a, cmd)) {
				out.Problems = append(out.Problems, output.TermsProblem{
					ConceptID: p.ConceptID,
					Locale:    string(p.Locale),
					Term:      p.Term,
					Message:   p.Message,
					Warning:   p.Warning,
				})
				if !p.Warning {
					out.Valid = false
				}
			}
			if err := output.Print(cmd, out); err != nil {
				return err
			}
			if !out.Valid {
				return ErrSilentExit
			}
			return nil
		},
	}
	addTermsSourceLocaleFlag(cmd)
	output.AddFlags(cmd.Flags())
	return cmd
}

func addTermsSourceLocaleFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("source-locale", "s", "", "language the source terms are written in (default: the project's source language)")
}

// termsSourceLocale is the language the source terms are in: --source-locale,
// else the run's source language.
func termsSourceLocale(a *App, cmd *cobra.Command) model.LocaleID {
	if s, _ := cmd.Flags().GetString("source-locale"); strings.TrimSpace(s) != "" {
		return model.LocaleID(strings.TrimSpace(s))
	}
	if s := a.SourceLocale(); s != "" {
		return model.LocaleID(s)
	}
	return model.LocaleEnglish
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
