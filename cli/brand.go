package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/cli/credentials"
	"github.com/neokapi/neokapi/cli/output"
	brandstore "github.com/neokapi/neokapi/cli/storage/brand"
	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/brand/packs"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// localWorkspace is the workspace ID used for profiles in the local CLI brand store.
const localWorkspace = "local"

// NewBrandCmd creates the `kapi brand` command group: a text-first, JSON-first
// surface for keeping AI-generated content on-brand. It works fully offline
// against a local brand voice profile (a starter pack, a standalone YAML file,
// or the local SQLite brand store).
func (a *App) NewBrandCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "brand",
		Short:   "Keep AI-generated content on brand (voice, tone, terminology)",
		GroupID: "management",
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
		a.newBrandNewCmd(),
		a.newBrandGuideCmd(),
		a.newBrandCheckCmd(),
		a.newBrandRewriteCmd(),
		a.newBrandValidateCmd(),
		a.newBrandProfilesCmd(),
		a.newBrandShowCmd(),
		a.newBrandImportCmd(),
		a.newBrandPackCmd(),
	)
	return cmd
}

// addProfileFlags adds the mutually-exclusive profile-source flags plus the
// brand-store resource flags.
func addProfileFlags(cmd *cobra.Command) {
	cmd.Flags().String("profile", "", "brand voice profile name in the local store")
	cmd.Flags().String("profile-file", "", "path to a standalone profile YAML")
	cmd.Flags().String("pack", "", "built-in starter pack name")
	cmd.Flags().String("locale", "", "apply locale-specific overrides")
	cmd.Flags().String("channel", "", "apply channel-specific overrides")
	AddProjectFlag(cmd)
	AddResourceFlags(cmd)
}

func addBrandAIFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("ai", false, "use an LLM provider in addition to rule-based checks")
	cmd.Flags().String("provider", "", "AI provider (default: anthropic)")
	cmd.Flags().String("api-key", "", "API key for the AI provider")
	cmd.Flags().String("credential", "", "saved credential name (see 'kapi credentials list')")
}

// ---------------------------------------------------------------------------
// guide
// ---------------------------------------------------------------------------

func (a *App) newBrandGuideCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "guide",
		Short: "Print the brand voice guide (inject into your assistant's context)",
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _, err := a.resolveBrandProfile(cmd)
			if err != nil {
				return err
			}
			return output.Print(cmd, output.BrandGuideOutput{
				Profile: profile.Name,
				Guide:   brand.RenderVoiceGuide(profile),
			})
		},
	}
	addProfileFlags(cmd)
	output.AddFlags(cmd)
	return cmd
}

// ---------------------------------------------------------------------------
// check
// ---------------------------------------------------------------------------

func (a *App) newBrandCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Score text against a brand voice profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _, err := a.resolveBrandProfile(cmd)
			if err != nil {
				return err
			}
			text, err := readSubjectText(cmd, args)
			if err != nil {
				return err
			}
			useAI, _ := cmd.Flags().GetBool("ai")

			var findings []brand.BrandVoiceFinding

			// Rule-based vocabulary check always runs (fast, offline).
			vocab := coretools.NewBrandVocabCheckTool(profile, nil)
			vf, err := runBlockTool(cmd.Context(), vocab, text)
			if err != nil {
				return err
			}
			findings = append(findings, vf...)

			// Optional LLM-based check for tone/style/clarity.
			if useAI {
				p, perr := a.buildBrandProvider(cmd)
				if perr != nil {
					return perr
				}
				ai := aitools.NewBrandVoiceCheckTool(p, profile)
				af, aerr := runBlockTool(cmd.Context(), ai, text)
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
	addProfileFlags(cmd)
	addBrandAIFlags(cmd)
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

// ---------------------------------------------------------------------------
// rewrite
// ---------------------------------------------------------------------------

func (a *App) newBrandRewriteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rewrite",
		Short: "Rewrite text to comply with a brand voice profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _, err := a.resolveBrandProfile(cmd)
			if err != nil {
				return err
			}
			text, err := readSubjectText(cmd, args)
			if err != nil {
				return err
			}
			useAI, _ := cmd.Flags().GetBool("ai")

			if useAI {
				p, perr := a.buildBrandProvider(cmd)
				if perr != nil {
					return perr
				}
				rewritten, aerr := aiRewrite(cmd.Context(), p, profile, text)
				if aerr != nil {
					return aerr
				}
				return output.Print(cmd, output.BrandRewriteOutput{
					Profile:   profile.Name,
					AIRewrite: true,
					Original:  text,
					Rewritten: rewritten,
				})
			}

			rewritten, changes := ruleRewrite(profile, text)
			return output.Print(cmd, output.BrandRewriteOutput{
				Profile:   profile.Name,
				Original:  text,
				Rewritten: rewritten,
				Changes:   changes,
			})
		},
	}
	addProfileFlags(cmd)
	addBrandAIFlags(cmd)
	// --input-text avoids colliding with the persistent --text bool output-format flag.
	cmd.Flags().String("input-text", "", `text to rewrite (use "-" or omit to read stdin)`)
	// Only --json here (not output.AddFlags) to avoid colliding with --input-text.
	cmd.Flags().Bool("json", false, "output results as JSON")
	return cmd
}

// ---------------------------------------------------------------------------
// validate
// ---------------------------------------------------------------------------

func (a *App) newBrandValidateCmd() *cobra.Command {
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
			data, err := readProfileInput(src)
			if err != nil {
				// Operational: the file could not be opened/read.
				return err
			}

			out := output.BrandValidateOutput{
				Source: validateSourceLabel(src),
				Errors: []brand.ProfileProblem{},
			}

			// 1. Lenient parse catches YAML syntax and type errors. A document
			// that does not parse cannot be checked further.
			profile, perr := brand.LoadProfileYAML(bytes.NewReader(data))
			if perr != nil {
				out.Errors = append(out.Errors, brand.ProfileProblem{Message: perr.Error()})
				return emitValidate(cmd, out)
			}
			out.Profile = profile.Name

			// 2. Strict parse catches unknown/typo'd fields that the lenient
			// loader silently ignores.
			if _, serr := brand.DecodeProfileStrict(bytes.NewReader(data)); serr != nil {
				out.Errors = append(out.Errors, strictDecodeProblems(serr)...)
			}

			// 3. Semantic validation (required fields, enums, regex, terms).
			out.Errors = append(out.Errors, brand.ValidateProfile(profile)...)

			return emitValidate(cmd, out)
		},
	}
	output.AddFlags(cmd)
	return cmd
}

// emitValidate finalizes the validation verdict, prints it, and maps an invalid
// profile to a non-zero (silent) exit so CI fails on a misconfigured profile
// while the structured output stays the result channel.
func emitValidate(cmd *cobra.Command, out output.BrandValidateOutput) error {
	out.Valid = len(out.Errors) == 0
	if err := output.Print(cmd, out); err != nil {
		return err
	}
	if !out.Valid {
		return ErrSilentExit
	}
	return nil
}

// readProfileInput reads the profile bytes from a file path, or from stdin when
// src is "-".
func readProfileInput(src string) ([]byte, error) {
	if src == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	return data, nil
}

// validateSourceLabel is the human/JSON label for the validated source.
func validateSourceLabel(src string) string {
	if src == "-" {
		return "stdin"
	}
	return src
}

// unknownFieldRe extracts the line number and field name from a yaml.v3
// KnownFields(true) "field X not found in type ..." error line.
var unknownFieldRe = regexp.MustCompile(`line (\d+): field (\S+) not found in type`)

// strictDecodeProblems turns a strict-decode error (from DecodeProfileStrict)
// into per-field problems. It recognises yaml.v3's unknown-field lines and
// rewrites them without the leaking Go type name; any unrecognised remainder is
// surfaced verbatim so no decode error is swallowed.
func strictDecodeProblems(err error) []brand.ProfileProblem {
	if err == nil {
		return nil
	}
	var probs []brand.ProfileProblem
	for line := range strings.SplitSeq(err.Error(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "yaml: unmarshal errors:") {
			continue
		}
		if m := unknownFieldRe.FindStringSubmatch(line); m != nil {
			probs = append(probs, brand.ProfileProblem{
				Field:   m[2],
				Message: fmt.Sprintf("unknown field %q (line %s)", m[2], m[1]),
			})
			continue
		}
		probs = append(probs, brand.ProfileProblem{Message: line})
	}
	return probs
}

// ---------------------------------------------------------------------------
// profiles / show / import / pack
// ---------------------------------------------------------------------------

func (a *App) newBrandProfilesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "List brand voice profiles (local store + built-in packs)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var summaries []output.BrandProfileSummary

			store, _, err := a.openBrandStore(cmd)
			if err == nil {
				defer store.Close()
				profiles, lerr := store.ListProfiles(cmd.Context(), localWorkspace)
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
	output.AddFlags(cmd)
	return cmd
}

// brandProfileTemplate is a commented, schema-valid VoiceProfile starting point
// emitted by `kapi brand new`. It parses via brand.LoadProfileYAML (guarded by a
// test), so an AI assistant or a human can fill it in and import it directly.
const brandProfileTemplate = `# Brand voice profile. Fill in the fields, then:
#   kapi brand import brand.yaml                     # save to the local store
#   kapi brand guide --profile-file brand.yaml       # render the guide
#   echo "draft" | kapi brand check --profile-file brand.yaml --json
# Only 'name' is required; every other field is optional. The English source
# text always stays the key — do not invent message IDs.

name: My Brand
description: One line on who this voice is for and the impression it should leave.

tone:
  personality: [clear, confident, friendly]   # 2-4 adjectives
  formality: neutral        # casual | neutral | formal | technical
  emotion: warm             # warm | neutral | authoritative
  humor: light              # none | light | frequent
  guidelines: Address the reader as "you". Lead with the benefit.

style:
  active_voice: true
  sentence_length: varied   # short | medium | varied
  person_pov: second        # first_plural | second | third
  contractions: always      # always | sometimes | never
  prohibited_patterns:
    - regex: "\\b(synergy|leverage)\\b"
      description: Corporate jargon
      severity: minor        # minor | major | critical

vocabulary:
  preferred_terms:
    - term: sign in
      note: not "log in"
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: minor
  competitor_terms:
    - term: Globex
      replacement: our platform
      severity: major

examples:
  - before: We utilize cutting-edge technology to facilitate outcomes.
    after: We help you ship faster.
    explanation: Cut the jargon; speak to the reader.
    category: tone           # tone | style | vocabulary
`

func (a *App) newBrandNewCmd() *cobra.Command {
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
			data := []byte(brandProfileTemplate)
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

func (a *App) newBrandShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a brand voice profile as a guide",
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _, err := a.resolveBrandProfile(cmd)
			if err != nil {
				return err
			}
			return output.Print(cmd, output.BrandGuideOutput{
				Profile: profile.Name,
				Guide:   brand.RenderVoiceGuide(profile),
			})
		},
	}
	addProfileFlags(cmd)
	output.AddFlags(cmd)
	return cmd
}

func (a *App) newBrandImportCmd() *cobra.Command {
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
			return a.saveProfileToStore(cmd, profile, args[0])
		},
	}
	AddResourceFlags(cmd)
	output.AddFlags(cmd)
	return cmd
}

func (a *App) newBrandPackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pack <name>",
		Short: "Install a built-in starter pack into the local brand store",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, err := packs.Load(args[0])
			if err != nil {
				return err
			}
			return a.saveProfileToStore(cmd, profile, "")
		},
	}
	AddResourceFlags(cmd)
	output.AddFlags(cmd)
	return cmd
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// openBrandStore opens the local SQLite brand store using the standard
// --name/--local/--file resource flags (default ./brand.db), mirroring the
// termbase/tm pattern.
func (a *App) openBrandStore(cmd *cobra.Command) (*brandstore.SQLiteBrandStore, string, error) {
	dbPath, err := ResolveResourcePath(cmd, "brands", "brand.db")
	if err != nil {
		return nil, "", err
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, dbPath, fmt.Errorf("open brand store: %w", err)
	}
	store, err := brandstore.NewSQLiteBrandStore(db)
	if err != nil {
		return nil, dbPath, err
	}
	return store, dbPath, nil
}

// saveProfileToStore creates or updates a profile in the local store, returning
// a typed import result.
func (a *App) saveProfileToStore(cmd *cobra.Command, profile *brand.VoiceProfile, srcPath string) error {
	store, _, err := a.openBrandStore(cmd)
	if err != nil {
		return err
	}
	defer store.Close()

	if profile.ID == "" {
		profile.ID = slugify(profile.Name)
	}
	profile.WorkspaceID = localWorkspace

	action := "created"
	if _, gerr := store.GetProfile(cmd.Context(), profile.ID); gerr == nil {
		if uerr := store.UpdateProfile(cmd.Context(), profile); uerr != nil {
			return uerr
		}
		action = "updated"
	} else if cerr := store.CreateProfile(cmd.Context(), profile); cerr != nil {
		return cerr
	}

	return output.Print(cmd, output.BrandImportOutput{
		ID: profile.ID, Name: profile.Name, Action: action, Path: srcPath,
	})
}

// resolveBrandProfile resolves the effective profile from --profile-file,
// --pack, or --profile (local store), then applies locale/channel overrides.
func (a *App) resolveBrandProfile(cmd *cobra.Command) (*brand.VoiceProfile, string, error) {
	file, _ := cmd.Flags().GetString("profile-file")
	pack, _ := cmd.Flags().GetString("pack")
	name, _ := cmd.Flags().GetString("profile")
	locale, _ := cmd.Flags().GetString("locale")
	channel, _ := cmd.Flags().GetString("channel")

	count := 0
	for _, v := range []string{file, pack, name} {
		if v != "" {
			count++
		}
	}
	if count > 1 {
		return nil, "", errors.New("--profile, --profile-file, and --pack are mutually exclusive")
	}
	if count == 0 {
		// No explicit flag — fall back to the project's bound brand voice
		// (defaults.brand_voice) or a convention file at the project root.
		// This makes `kapi brand check DRAFT.md` work flag-free inside a
		// project directory.
		profile, src, ok, perr := a.resolveProjectBrandProfile(cmd, locale, channel)
		if perr != nil {
			return nil, "", perr
		}
		if ok {
			return profile, src, nil
		}
		return nil, "", errors.New("specify a profile with --profile, --profile-file, or --pack (or bind one in your .kapi project under defaults.brand_voice)")
	}

	var profile *brand.VoiceProfile
	var src string
	switch {
	case file != "":
		f, err := os.Open(file)
		if err != nil {
			return nil, "", fmt.Errorf("open profile: %w", err)
		}
		defer f.Close()
		profile, err = brand.LoadProfileYAML(f)
		if err != nil {
			return nil, "", err
		}
		src = file
	case pack != "":
		p, err := packs.Load(pack)
		if err != nil {
			return nil, "", err
		}
		profile, src = p, "pack:"+pack
	default:
		p, err := a.lookupStoreProfile(cmd, name)
		if err != nil {
			return nil, "", err
		}
		profile, src = p, "store:"+name
	}

	if locale != "" || channel != "" {
		profile = brand.ResolveProfile(profile, model.LocaleID(locale), channel)
	}
	return profile, src, nil
}

// resolveProjectBrandProfile resolves a brand voice profile from the .kapi
// project in scope, with no profile flag. Resolution order:
//
//  1. defaults.brand_voice in the recipe (profile_file → YAML, profile →
//     local store, pack → built-in starter pack). profile_file is resolved
//     relative to the project root.
//  2. A convention file at <projectRoot>/brand.yaml, then
//     <projectRoot>/.kapi/brand.yaml.
//
// Returns (profile, source, found, error). found is false (with nil error)
// when no project is in scope or the project carries no brand binding and no
// convention file — letting the caller surface the "specify a profile" error.
func (a *App) resolveProjectBrandProfile(cmd *cobra.Command, locale, channel string) (*brand.VoiceProfile, string, bool, error) {
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil {
		return nil, "", false, err
	}
	if projectPath == "" {
		return nil, "", false, nil
	}

	root := filepath.Dir(projectPath)

	// Load the recipe to read defaults.brand_voice. Skip the
	// requires-extension check so brand resolution does not demand plugins
	// that brand check/rewrite/guide don't actually use.
	proj, lerr := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if lerr != nil {
		return nil, "", false, fmt.Errorf("load project for brand voice: %w", lerr)
	}

	profile, src, found, err := a.loadBoundBrandProfile(cmd, proj, root)
	if err != nil {
		return nil, "", false, err
	}
	if !found {
		// Convention files at the project root.
		for _, conv := range []string{
			filepath.Join(root, "brand.yaml"),
			filepath.Join(root, project.StateDirName, "brand.yaml"),
		} {
			p, lerr := loadProfileFile(conv)
			if lerr != nil {
				return nil, "", false, lerr
			}
			if p != nil {
				profile, src, found = p, conv, true
				break
			}
		}
	}
	if !found {
		return nil, "", false, nil
	}

	if locale != "" || channel != "" {
		profile = brand.ResolveProfile(profile, model.LocaleID(locale), channel)
	}
	return profile, src, true, nil
}

// loadBoundBrandProfile resolves the recipe's defaults.brand_voice binding
// into a VoiceProfile. Returns found=false when the recipe has no binding.
// profile_file paths are resolved relative to the project root.
func (a *App) loadBoundBrandProfile(cmd *cobra.Command, proj *project.KapiProject, root string) (*brand.VoiceProfile, string, bool, error) {
	bv := proj.Defaults.BrandVoice
	if bv == nil {
		return nil, "", false, nil
	}
	switch {
	case bv.ProfileFile != "":
		path := bv.ProfileFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		p, err := loadProfileFile(path)
		if err != nil {
			return nil, "", false, err
		}
		if p == nil {
			return nil, "", false, fmt.Errorf("brand voice profile file %q (from defaults.brand_voice.profile_file) not found", path)
		}
		return p, path, true, nil
	case bv.Pack != "":
		p, err := packs.Load(bv.Pack)
		if err != nil {
			return nil, "", false, err
		}
		return p, "pack:" + bv.Pack, true, nil
	case bv.Profile != "":
		p, err := a.lookupStoreProfile(cmd, bv.Profile)
		if err != nil {
			return nil, "", false, err
		}
		return p, "store:" + bv.Profile, true, nil
	}
	return nil, "", false, nil
}

// loadProfileFile loads a VoiceProfile YAML from path. Returns (nil, nil) when
// the file does not exist so callers can fall through to other sources.
func loadProfileFile(path string) (*brand.VoiceProfile, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open profile %q: %w", path, err)
	}
	defer f.Close()
	p, err := brand.LoadProfileYAML(f)
	if err != nil {
		return nil, fmt.Errorf("load profile %q: %w", path, err)
	}
	return p, nil
}

// lookupStoreProfile finds a profile in the local store by ID or by name.
func (a *App) lookupStoreProfile(cmd *cobra.Command, name string) (*brand.VoiceProfile, error) {
	store, _, err := a.openBrandStore(cmd)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	if p, gerr := store.GetProfile(cmd.Context(), name); gerr == nil {
		return p, nil
	}
	if p, gerr := store.GetProfile(cmd.Context(), slugify(name)); gerr == nil {
		return p, nil
	}
	profiles, lerr := store.ListProfiles(cmd.Context(), localWorkspace)
	if lerr != nil {
		return nil, lerr
	}
	for _, p := range profiles {
		if strings.EqualFold(p.Name, name) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("brand voice profile %q not found in local store (try 'kapi brand pack %s' or 'kapi brand profiles')", name, name)
}

// buildBrandProvider resolves an LLM provider from --provider/--api-key/
// --credential plus the saved credential store.
func (a *App) buildBrandProvider(cmd *cobra.Command) (aiprovider.LLMProvider, error) {
	provider, _ := cmd.Flags().GetString("provider")
	apiKey, _ := cmd.Flags().GetString("api-key")
	cred, _ := cmd.Flags().GetString("credential")

	config := map[string]any{}
	if provider != "" {
		config["provider"] = provider
	}
	if apiKey != "" {
		config["apiKey"] = apiKey
	}
	if cred != "" {
		config["credential"] = cred
	}
	resolved, err := credentials.ResolveCredentials(a.Credentials, "", []string{"credentials"}, config)
	if err != nil {
		return nil, err
	}
	pName, _ := resolved["provider"].(string)
	key, _ := resolved["apiKey"].(string)
	mdl, _ := resolved["model"].(string)
	return aitools.ProviderFromConfig(pName, aiprovider.Config{APIKey: key, Model: mdl})
}

// readSubjectText reads the text to check/rewrite from --input-text, a positional
// file argument, or stdin (when --input-text is empty or "-").
func readSubjectText(cmd *cobra.Command, args []string) (string, error) {
	text, _ := cmd.Flags().GetString("input-text")
	if text != "" && text != "-" {
		return text, nil
	}
	if text == "" && len(args) == 1 && args[0] != "-" {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return "", fmt.Errorf("read input: %w", err)
		}
		return string(data), nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", errors.New("no text provided (use --input-text or pipe via stdin)")
	}
	return string(data), nil
}

// runBlockTool runs a single-block tool over text and returns the brand voice
// findings it produced (read from the block annotation).
func runBlockTool(ctx context.Context, t tool.Tool, text string) ([]brand.BrandVoiceFinding, error) {
	block := model.NewBlock("stdin", text)
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	errc := make(chan error, 1)
	go func() {
		defer close(out)
		errc <- t.Process(ctx, in, out)
	}()
	for range out { //nolint:revive // drain the pipeline
	}
	if err := <-errc; err != nil {
		return nil, err
	}

	if ann, ok := model.AnnoAs[*brand.BrandVoiceAnnotation](block, "brand-voice"); ok {
		return ann.Findings, nil
	}
	// Fallback: read findings from properties (rule-based or AI key).
	for _, key := range []string{"brand-vocab-findings", "brand-voice-findings"} {
		if raw, ok := block.Properties[key]; ok && raw != "" {
			var fs []brand.BrandVoiceFinding
			if json.Unmarshal([]byte(raw), &fs) == nil {
				return fs, nil
			}
		}
	}
	return nil, nil
}

// ruleRewrite applies forbidden/competitor term replacements from the profile,
// preserving surrounding text. Returns the rewritten text and the changes made.
func ruleRewrite(profile *brand.VoiceProfile, text string) (string, []output.BrandChange) {
	var changes []output.BrandChange
	result := text
	apply := func(rules []brand.TermRule) {
		for _, rule := range rules {
			if rule.Replacement == "" || rule.Term == "" {
				continue
			}
			re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(rule.Term))
			n := len(re.FindAllString(result, -1))
			if n == 0 {
				continue
			}
			result = re.ReplaceAllString(result, rule.Replacement)
			changes = append(changes, output.BrandChange{From: rule.Term, To: rule.Replacement, Count: n})
		}
	}
	if profile != nil {
		apply(profile.Vocabulary.CompetitorTerms)
		apply(profile.Vocabulary.ForbiddenTerms)
	}
	return result, changes
}

// aiRewrite asks the LLM to rewrite the text in the brand voice.
func aiRewrite(ctx context.Context, p aiprovider.LLMProvider, profile *brand.VoiceProfile, text string) (string, error) {
	var b strings.Builder
	b.WriteString("Rewrite the following text so it complies with the brand voice guide below. ")
	b.WriteString("Preserve meaning and any placeholders or markup. Return ONLY the rewritten text.\n\n")
	b.WriteString(brand.RenderVoiceGuide(profile))
	b.WriteString("\n## Text to rewrite\n\n")
	b.WriteString(text)

	resp, err := p.Chat(ctx, []aiprovider.Message{{Role: "user", Content: b.String()}})
	if err != nil {
		return "", fmt.Errorf("brand rewrite: %w", err)
	}
	return strings.TrimSpace(resp.Content), nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a profile name to a stable, filename-safe ID.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
