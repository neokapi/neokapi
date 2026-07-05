package cli

import (
	"bytes"
	"fmt"
	"os"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	coretools "github.com/neokapi/neokapi/core/tools"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/brand/packs"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewBrandCmd creates the `kapi brand` command group: a text-first, JSON-first
// surface for keeping AI-generated content on-brand. It works fully offline
// against a local brand voice profile (a starter pack, a standalone YAML file,
// or the local SQLite brand store).
func NewBrandCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "brand",
		Short:   "Keep AI-generated content on brand (voice, tone, terminology)",
		GroupID: "assets",
		Long: `Check, rewrite, and govern content against a brand voice profile.

Profile source (mutually exclusive):
  --profile <name>       Profile in the local brand store (see 'kapi brand profiles')
  --profile-file <path>  Standalone profile YAML (git-shareable, no store needed)
  --pack <name>          Built-in starter pack (professional-b2b, friendly-dtc,
                         technical-docs, marketing-blog, customer-support)

Text input for check/rewrite is read from --text, or from stdin when --text is
omitted or set to "-".`,
	}

	cmd.AddCommand(
		newBrandNewCmd(a),
		newBrandGuideCmd(a),
		newBrandCheckCmd(a),
		newBrandRewriteCmd(a),
		newBrandValidateCmd(a),
		newBrandProfilesCmd(a),
		newBrandShowCmd(a),
		newBrandImportCmd(a),
		newBrandPackCmd(a),
	)
	return cmd
}

func newBrandGuideCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "guide",
		Short: "Print the brand voice guide (inject into your assistant's context)",
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _, err := a.ResolveBrandProfileCmd(cmd)
			if err != nil {
				return err
			}
			return output.Print(cmd, output.BrandGuideOutput{
				Profile: profile.Name,
				Guide:   brand.RenderVoiceGuide(profile),
			})
		},
	}
	AddProfileFlags(cmd)
	output.AddFlags(cmd.Flags())
	return cmd
}

func newBrandCheckCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Score text against a brand voice profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _, err := a.ResolveBrandProfileCmd(cmd)
			if err != nil {
				return err
			}
			text, err := ReadSubjectText(cmd, args)
			if err != nil {
				return err
			}
			useAI, _ := cmd.Flags().GetBool("ai")

			var findings []brand.BrandVoiceFinding

			// Rule-based vocabulary check always runs (fast, offline).
			vocab := coretools.NewBrandVocabCheckTool(profile, nil)
			vf, err := RunBlockTool(cmd.Context(), vocab, text)
			if err != nil {
				return err
			}
			findings = append(findings, vf...)

			// Optional LLM-based check for tone/style/clarity.
			if useAI {
				p, perr := a.BuildBrandProvider(cmd)
				if perr != nil {
					return perr
				}
				ai := aitools.NewBrandVoiceCheckTool(p, profile)
				af, aerr := RunBlockTool(cmd.Context(), ai, text)
				if aerr != nil {
					return aerr
				}
				findings = append(findings, af...)
			}

			score := brand.CalculateScore(findings)
			score.ProfileID = profile.ID

			out := output.BrandCheckOutput{
				Profile:    profile.Name,
				Score:      score.Overall,
				Passed:     true,
				AIChecked:  useAI,
				Dimensions: score.Dimensions,
				Findings:   findings,
			}
			if cmd.Flags().Changed("min-score") {
				min, _ := cmd.Flags().GetInt("min-score")
				out.MinScore = &min
				out.Passed = score.Overall >= min
			}
			if err := output.Print(cmd, out); err != nil {
				return err
			}
			if !out.Passed {
				return ErrQualityGate
			}
			return nil
		},
	}
	AddProfileFlags(cmd)
	AddBrandAIFlags(cmd)
	// --input-text avoids colliding with the persistent --text bool (output-format)
	// flag registered by AddPersistentFlags on the root command. The old --text
	// String flag shadowed the persistent Bool so GetBool("text") silently broke
	// output-format resolution on all brand subcommands.
	cmd.Flags().String("input-text", "", `text to check (use "-" or omit to read stdin)`)
	cmd.Flags().Int("min-score", 0, "fail (non-zero exit) when the score is below this threshold")
	// Only --json here (not output.AddFlags) to avoid colliding with --input-text.
	cmd.Flags().Bool("json", false, "output results as JSON")
	return cmd
}

func newBrandRewriteCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rewrite",
		Short: "Substitute forbidden/competitor terms for their approved wording (offline)",
		Long: `Rewrite content against a brand voice profile by substituting forbidden and
competitor terms with their approved replacements. This is the deterministic,
offline path — it changes only the terms the profile defines and reports each
change; it does not call a model.

Text is read from --input-text or stdin and the rewrite is printed. To fix tone,
style, or phrasing on-brand, rewrite the text yourself with the voice guide as
context ('kapi brand guide') and apply the edit through 'kapi apply' — kapi does
not send content to a model to rewrite it.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _, err := a.ResolveBrandProfileCmd(cmd)
			if err != nil {
				return err
			}
			text, err := ReadSubjectText(cmd, args)
			if err != nil {
				return err
			}
			rewritten, changes := RuleRewrite(profile, text)
			return output.Print(cmd, output.BrandRewriteOutput{
				Profile:   profile.Name,
				Original:  text,
				Rewritten: rewritten,
				Changes:   changes,
			})
		},
	}
	AddProfileFlags(cmd)
	// --input-text avoids colliding with the persistent --text bool output-format flag.
	cmd.Flags().String("input-text", "", `text to rewrite (use "-" or omit to read stdin)`)
	// Only --json here (not output.AddFlags) to avoid colliding with --input-text.
	cmd.Flags().Bool("json", false, "output results as JSON")
	return cmd
}

func newBrandValidateCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <file.yaml|->",
		Short: "Validate a brand voice profile YAML (structure, enums, regex, terms)",
		Long: `Validate a brand voice profile YAML and report structural problems.

Pass a file path, or "-" to read the profile from stdin. Validation reports:

  - YAML syntax or type errors that stop the profile from parsing
  - unknown fields (typo'd or unsupported keys)
  - missing required fields (only 'name' is required)
  - invalid enum values (tone formality/emotion/humor, style sentence_length/
    person_pov/contractions, example category, rule severity)
  - regex in style prohibited_patterns/required_patterns that does not compile
  - vocabulary term rules with an empty term

Exit codes: 0 when the profile is valid, 1 when it has any problem. With --json
the result is {"valid": bool, "errors": [{"field", "message"}]}.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := args[0]
			data, err := ReadProfileInput(src)
			if err != nil {
				// Operational: the file could not be opened/read.
				return err
			}

			out := output.BrandValidateOutput{
				Source: ValidateSourceLabel(src),
				Errors: []brand.ProfileProblem{},
			}

			// 1. Lenient parse catches YAML syntax and type errors. A document
			// that does not parse cannot be checked further.
			profile, perr := brand.LoadProfileYAML(bytes.NewReader(data))
			if perr != nil {
				out.Errors = append(out.Errors, brand.ProfileProblem{Message: perr.Error()})
				return EmitValidate(cmd, out)
			}
			out.Profile = profile.Name

			// 2. Strict parse catches unknown/typo'd fields that the lenient
			// loader silently ignores.
			if _, serr := brand.DecodeProfileStrict(bytes.NewReader(data)); serr != nil {
				out.Errors = append(out.Errors, StrictDecodeProblems(serr)...)
			}

			// 3. Semantic validation (required fields, enums, regex, terms).
			out.Errors = append(out.Errors, brand.ValidateProfile(profile)...)

			return EmitValidate(cmd, out)
		},
	}
	output.AddFlags(cmd.Flags())
	return cmd
}

func newBrandProfilesCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "List brand voice profiles (local store + built-in packs)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var summaries []output.BrandProfileSummary

			store, _, err := a.OpenBrandStore(cmd)
			if err == nil {
				defer store.Close()
				profiles, lerr := store.ListProfiles(cmd.Context(), LocalWorkspace)
				if lerr == nil {
					for _, p := range profiles {
						summaries = append(summaries, output.BrandProfileSummary{
							ID: p.ID, Name: p.Name, Description: p.Description, Source: "store",
						})
					}
				}
			}

			names, _ := packs.List()
			for _, n := range names {
				summaries = append(summaries, output.BrandProfileSummary{
					ID: n, Name: n, Source: "pack",
				})
			}

			return output.Print(cmd, output.BrandProfilesOutput{Profiles: summaries, Total: len(summaries)})
		},
	}
	AddResourceFlags(cmd)
	output.AddFlags(cmd.Flags())
	return cmd
}

func newBrandNewCmd(a *App) *cobra.Command {
	var pack, out string
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Scaffold a brand voice profile YAML to fill in (optionally seeded from a starter pack)",
		Long: `Write a brand voice profile YAML to fill in.

With no flags, emits a commented template. With --pack, emits an existing
starter pack as an editable base. An AI assistant can fill this in from what it
already knows about the product, from sample content, or from a linked website,
then ` + "`kapi brand import`" + ` it.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			data := []byte(BrandProfileTemplate)
			if pack != "" {
				p, err := packs.Load(pack)
				if err != nil {
					return err
				}
				p.ID = "" // let import derive the id from the (edited) name
				b, err := yaml.Marshal(p)
				if err != nil {
					return fmt.Errorf("marshal pack %q: %w", pack, err)
				}
				data = append([]byte("# Seeded from the "+pack+" starter pack — edit to taste, then `kapi brand import`.\n"), b...)
			}
			if out != "" {
				if err := os.WriteFile(out, data, 0o644); err != nil {
					return fmt.Errorf("write %s: %w", out, err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Wrote %s — fill it in, then: kapi brand import %s\n", out, out)
				return nil
			}
			_, err := cmd.OutOrStdout().Write(data)
			return err
		},
	}
	cmd.Flags().StringVar(&pack, "pack", "", "seed from a starter pack (see 'kapi brand profiles')")
	cmd.Flags().StringVarP(&out, "out", "o", "", "write to a file instead of stdout")
	return cmd
}

func newBrandShowCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a brand voice profile as a guide",
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _, err := a.ResolveBrandProfileCmd(cmd)
			if err != nil {
				return err
			}
			return output.Print(cmd, output.BrandGuideOutput{
				Profile: profile.Name,
				Guide:   brand.RenderVoiceGuide(profile),
			})
		},
	}
	AddProfileFlags(cmd)
	output.AddFlags(cmd.Flags())
	return cmd
}

func newBrandImportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <file.yaml>",
		Short: "Import a profile YAML into the local brand store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := os.Open(args[0])
			if err != nil {
				return fmt.Errorf("open profile: %w", err)
			}
			defer f.Close()
			profile, err := brand.LoadProfileYAML(f)
			if err != nil {
				return err
			}
			return a.SaveProfileToStore(cmd, profile, args[0])
		},
	}
	AddResourceFlags(cmd)
	output.AddFlags(cmd.Flags())
	return cmd
}

func newBrandPackCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pack <name>",
		Short: "Install a built-in starter pack into the local brand store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, err := packs.Load(args[0])
			if err != nil {
				return err
			}
			return a.SaveProfileToStore(cmd, profile, "")
		},
	}
	AddResourceFlags(cmd)
	output.AddFlags(cmd.Flags())
	return cmd
}
