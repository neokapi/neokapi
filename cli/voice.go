package cli

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	coretools "github.com/neokapi/neokapi/core/tools"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/profile/packs"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewVoiceCmd creates the `kapi voice` command group: a text-first, JSON-first
// surface for keeping AI-generated content in voice. It works fully offline
// against a local voice profile (a starter pack, a standalone YAML file, or the
// local SQLite voice store).
func NewVoiceCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "voice",
		Short:   "Keep AI-generated content in voice (tone, style, terminology)",
		GroupID: "assets",
		Long: `Check, rewrite, and govern content against a voice profile.

Profile source (mutually exclusive):
  --profile <name>       Profile in the local voice store (see 'kapi voice profiles')
  --profile-file <path>  Standalone profile YAML (git-shareable, no store needed)
  --pack <name>          Built-in starter pack (professional-b2b, friendly-dtc,
                         technical-docs, marketing-blog, customer-support)

Text input for check/rewrite is read from --text, or from stdin when --text is
omitted or set to "-".`,
	}

	cmd.AddCommand(
		newVoiceNewCmd(a),
		newVoiceGuideCmd(a),
		newVoiceCheckCmd(a),
		newVoiceRewriteCmd(a),
		newVoiceValidateCmd(a),
		newVoiceExpandCmd(a),
		newVoiceProfilesCmd(a),
		newVoiceShowCmd(a),
		newVoiceImportCmd(a),
		newVoicePackCmd(a),
	)
	return cmd
}

func newVoiceGuideCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "guide",
		Short: "Print the voice guide (inject into your assistant's context)",
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _, err := a.ResolveVoiceProfileCmd(cmd, args...)
			if err != nil {
				return err
			}
			return output.Print(cmd, output.VoiceGuideOutput{
				Profile: profile.Name,
				Guide:   coreprofile.RenderVoiceGuide(profile),
			})
		},
	}
	AddProfileFlags(cmd)
	output.AddFlags(cmd.Flags())
	return cmd
}

func newVoiceCheckCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Score text against a voice profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _, err := a.ResolveVoiceProfileCmd(cmd, args...)
			if err != nil {
				return err
			}
			// A profile this command cannot score against must not be scored
			// against. Every field is optional, so an empty file and
			// `hello: world` both load, contribute no rules, and came back
			// 100/100 "on brand" — the most misleading answer this command can
			// give, and reachable from a scaffold nobody filled in or a path
			// typo that resolved to some other YAML. `kapi voice validate`
			// already refuses both; check asks it the same question. See #2224.
			if probs := coreprofile.Blocking(coreprofile.ValidateProfile(profile)); len(probs) > 0 {
				return fmt.Errorf("%s is not a usable voice profile:%s\n"+
					"run `kapi voice validate` for the full report",
					VoiceProfileLabel(profile), problemLines(probs))
			}
			text, err := ReadSubjectText(cmd, args)
			if err != nil {
				return err
			}
			useAI, _ := cmd.Flags().GetBool("ai")

			var findings []coreprofile.VoiceFinding

			// Rule-based vocabulary check always runs (fast, offline).
			vocab := coretools.NewVoiceVocabCheckTool(profile, nil)
			vf, err := RunBlockTool(cmd.Context(), vocab, text)
			if err != nil {
				return err
			}
			findings = append(findings, vf...)

			// Optional LLM-based check for tone/style/clarity.
			if useAI {
				p, perr := a.BuildVoiceProvider(cmd)
				if perr != nil {
					return perr
				}
				ai := aitools.NewVoiceCheckTool(p, profile)
				af, aerr := RunBlockTool(cmd.Context(), ai, text)
				if aerr != nil {
					return aerr
				}
				findings = append(findings, af...)
			}

			score := coreprofile.CalculateScore(findings)
			score.ProfileID = profile.ID

			out := output.VoiceCheckOutput{
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
	AddVoiceAIFlags(cmd)
	// --input-text avoids colliding with the persistent --text bool (output-format)
	// flag registered by AddPersistentFlags on the root command. The old --text
	// String flag shadowed the persistent Bool so GetBool("text") silently broke
	// output-format resolution on all voice subcommands.
	cmd.Flags().String("input-text", "", `text to check (use "-" or omit to read stdin)`)
	cmd.Flags().Int("min-score", 0, "fail (non-zero exit) when the score is below this threshold")
	// Only --json here (not output.AddFlags) to avoid colliding with --input-text.
	cmd.Flags().Bool("json", false, "output results as JSON")
	return cmd
}

// VoiceProfileLabel names a profile for an error message, falling back to a
// description of what was resolved when it has no name — which is the case the
// error is usually about.
func VoiceProfileLabel(p *coreprofile.VoiceProfile) string {
	if p == nil {
		return "the profile"
	}
	if name := strings.TrimSpace(p.Name); name != "" {
		return name
	}
	return "the resolved profile"
}

// problemLines renders validation problems one per line, indented.
func problemLines(probs []coreprofile.ProfileProblem) string {
	var b strings.Builder
	for _, p := range probs {
		b.WriteString("\n  ")
		if p.Field != "" {
			b.WriteString(p.Field)
			b.WriteString(": ")
		}
		b.WriteString(p.Message)
	}
	return b.String()
}

func newVoiceRewriteCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rewrite",
		Short: "Substitute forbidden/competitor terms for their approved wording (offline)",
		Long: `Rewrite content against a voice profile by substituting forbidden and
competitor terms with their approved replacements. This is the deterministic,
offline path: it changes only the terms the profile defines and reports each
change; it does not call a model.

Text is read from --input-text or stdin and the rewrite is printed. To fix tone,
style, or phrasing in voice, rewrite the text yourself with the voice guide as
context ('kapi voice guide') and apply the edit through 'kapi apply'. kapi does
not send content to a model to rewrite it.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _, err := a.ResolveVoiceProfileCmd(cmd, args...)
			if err != nil {
				return err
			}
			text, err := ReadSubjectText(cmd, args)
			if err != nil {
				return err
			}
			rewritten, changes := RuleRewrite(profile, text)
			return output.Print(cmd, output.VoiceRewriteOutput{
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

func newVoiceValidateCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <file.yaml|->",
		Short: "Validate a voice profile YAML (structure, enums, regex, terms)",
		Long: `Validate a voice profile YAML and report structural problems.

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

			out := output.VoiceValidateOutput{
				Source: ValidateSourceLabel(src),
				Errors: []coreprofile.ProfileProblem{},
			}

			// 1. Lenient parse catches YAML syntax and type errors. A document
			// that does not parse cannot be checked further.
			profile, perr := coreprofile.LoadProfileYAML(bytes.NewReader(data))
			if perr != nil {
				out.Errors = append(out.Errors, coreprofile.ProfileProblem{Message: perr.Error()})
				return EmitValidate(cmd, out)
			}
			out.Profile = profile.Name

			// 2. Strict parse catches unknown/typo'd fields that the lenient
			// loader silently ignores.
			if _, serr := coreprofile.DecodeProfileStrict(bytes.NewReader(data)); serr != nil {
				out.Errors = append(out.Errors, StrictDecodeProblems(serr)...)
			}

			// 3. Semantic validation (required fields, enums, regex, terms).
			out.Errors = append(out.Errors, coreprofile.ValidateProfile(profile)...)

			return EmitValidate(cmd, out)
		},
	}
	output.AddFlags(cmd.Flags())
	return cmd
}

func newVoiceProfilesCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "List voice profiles (local store + built-in packs)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var summaries []output.VoiceProfileSummary

			store, _, release, err := a.OpenVoiceStore(cmd)
			if err == nil {
				defer release()
				profiles, lerr := store.ListProfiles(cmd.Context(), LocalScope)
				if lerr == nil {
					for _, p := range profiles {
						summaries = append(summaries, output.VoiceProfileSummary{
							ID: p.ID, Name: p.Name, Description: p.Description, Source: "store",
						})
					}
				}
			}

			names, _ := packs.List()
			for _, n := range names {
				summaries = append(summaries, output.VoiceProfileSummary{
					ID: n, Name: n, Source: "pack",
				})
			}

			return output.Print(cmd, output.VoiceProfilesOutput{Profiles: summaries, Total: len(summaries)})
		},
	}
	AddResourceFlags(cmd)
	output.AddFlags(cmd.Flags())
	return cmd
}

func newVoiceNewCmd(a *App) *cobra.Command {
	var pack, out string
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Scaffold a voice profile YAML to fill in (optionally seeded from a starter pack)",
		Long: `Write a voice profile YAML to fill in.

With no flags, emits a commented template. With --pack, emits an existing
starter pack as an editable base. An AI assistant can fill this in from what it
already knows about the product, from sample content, or from a linked website,
then ` + "`kapi voice import`" + ` it.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			data := []byte(VoiceProfileTemplate)
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
				data = append([]byte("# Seeded from the "+pack+" starter pack. Edit to taste, then `kapi voice import`.\n"), b...)
			}
			if out != "" {
				if err := os.WriteFile(out, data, 0o644); err != nil {
					return fmt.Errorf("write %s: %w", out, err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Wrote %s. Fill it in, then: kapi voice import %s\n", out, out)
				return nil
			}
			_, err := cmd.OutOrStdout().Write(data)
			return err
		},
	}
	cmd.Flags().StringVar(&pack, "pack", "", "seed from a starter pack (see 'kapi voice profiles')")
	cmd.Flags().StringVarP(&out, "out", "o", "", "write to a file instead of stdout")
	return cmd
}

func newVoiceShowCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a voice profile as a guide",
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _, err := a.ResolveVoiceProfileCmd(cmd, args...)
			if err != nil {
				return err
			}
			return output.Print(cmd, output.VoiceGuideOutput{
				Profile: profile.Name,
				Guide:   coreprofile.RenderVoiceGuide(profile),
			})
		},
	}
	AddProfileFlags(cmd)
	output.AddFlags(cmd.Flags())
	return cmd
}

func newVoiceImportCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <file.yaml>",
		Short: "Import a profile YAML into the local voice store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := os.Open(args[0])
			if err != nil {
				return fmt.Errorf("open profile: %w", err)
			}
			defer f.Close()
			profile, err := coreprofile.LoadProfileYAML(f)
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

func newVoicePackCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pack <name>",
		Short: "Install a built-in starter pack into the local voice store",
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
