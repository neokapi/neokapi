package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/brand/packs"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host/credentials"
	"github.com/neokapi/neokapi/host/output"
	brandstore "github.com/neokapi/neokapi/host/storage/brand"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// LocalWorkspace is the workspace ID used for profiles in the local CLI brand store.
const LocalWorkspace = "local"

// AddProfileFlags adds the mutually-exclusive profile-source flags plus the
// brand-store resource flags.
func AddProfileFlags(cmd Command) {
	cmd.Flags().String("profile", "", "brand voice profile name in the local store")
	cmd.Flags().String("profile-file", "", "path to a standalone profile YAML")
	cmd.Flags().String("pack", "", "built-in starter pack name")
	cmd.Flags().String("locale", "", "apply locale-specific overrides")
	cmd.Flags().String("channel", "", "apply channel-specific overrides")
	cmd.Flags().String("persona", "", "apply an author persona's overrides (within brand guardrails)")
	AddProjectFlag(cmd)
	AddResourceFlags(cmd)
}

func AddBrandAIFlags(cmd Command) {
	cmd.Flags().Bool("ai", false, "use an LLM provider in addition to rule-based checks")
	cmd.Flags().String("provider", "", "AI provider (default: anthropic)")
	cmd.Flags().String("api-key", "", "API key for the AI provider")
	cmd.Flags().String("credential", "", "saved credential name (see 'kapi credentials list')")
}

// ---------------------------------------------------------------------------
// guide
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// check
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// rewrite
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// validate
// ---------------------------------------------------------------------------

// EmitValidate finalizes the validation verdict, prints it, and maps an invalid
// profile to a non-zero (silent) exit so CI fails on a misconfigured profile
// while the structured output stays the result channel.
func EmitValidate(cmd Command, out output.BrandValidateOutput) error {
	out.Valid = len(out.Errors) == 0
	if err := output.Print(cmd, out); err != nil {
		return err
	}
	if !out.Valid {
		return ErrSilentExit
	}
	return nil
}

// ReadProfileInput reads the profile bytes from a file path, or from stdin when
// src is "-".
func ReadProfileInput(src string) ([]byte, error) {
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

// ValidateSourceLabel is the human/JSON label for the validated source.
func ValidateSourceLabel(src string) string {
	if src == "-" {
		return "stdin"
	}
	return src
}

// unknownFieldRe extracts the line number and field name from a yaml.v3
// KnownFields(true) "field X not found in type ..." error line.
var unknownFieldRe = regexp.MustCompile(`line (\d+): field (\S+) not found in type`)

// StrictDecodeProblems turns a strict-decode error (from DecodeProfileStrict)
// into per-field problems. It recognises yaml.v3's unknown-field lines and
// rewrites them without the leaking Go type name; any unrecognised remainder is
// surfaced verbatim so no decode error is swallowed.
func StrictDecodeProblems(err error) []brand.ProfileProblem {
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

// BrandProfileTemplate is a commented, schema-valid VoiceProfile starting point
// emitted by `kapi brand new`. It parses via brand.LoadProfileYAML (guarded by a
// test), so an AI assistant or a human can fill it in and import it directly.
const BrandProfileTemplate = `# Brand voice profile. Fill in the fields, then:
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

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// OpenBrandStore opens the local SQLite brand store using the standard
// --name/--local/--file resource flags (default ./brand.db), mirroring the
// terms/tm pattern.
func (a *App) OpenBrandStore(cmd Command) (*brandstore.SQLiteBrandStore, string, error) {
	dbPath, err := resolveResourcePath(cmd, "brands", "brand.db")
	if err != nil {
		return nil, "", err
	}
	store, err := openBrandStoreAt(dbPath)
	if err != nil {
		return nil, dbPath, err
	}
	return store, dbPath, nil
}

// openBrandStoreAt opens (creating if needed) the local SQLite brand store at
// dbPath — the cobra-free leg of OpenBrandStore.
func openBrandStoreAt(dbPath string) (*brandstore.SQLiteBrandStore, error) {
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open brand store: %w", err)
	}
	store, err := brandstore.NewSQLiteBrandStore(db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// SaveProfileToStore creates or updates a profile in the local store, returning
// a typed import result.
func (a *App) SaveProfileToStore(cmd Command, profile *brand.VoiceProfile, srcPath string) error {
	store, _, err := a.OpenBrandStore(cmd)
	if err != nil {
		return err
	}
	defer store.Close()

	if profile.ID == "" {
		profile.ID = slugify(profile.Name)
	}
	profile.WorkspaceID = LocalWorkspace

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

// ResolveBrandProfileCmd resolves the effective profile from --profile-file,
// --pack, or --profile (local store), then applies locale/channel overrides.
func (a *App) ResolveBrandProfileCmd(cmd Command) (*brand.VoiceProfile, string, error) {
	file, _ := cmd.Flags().GetString("profile-file")
	pack, _ := cmd.Flags().GetString("pack")
	name, _ := cmd.Flags().GetString("profile")
	locale, _ := cmd.Flags().GetString("locale")
	channel, _ := cmd.Flags().GetString("channel")
	persona, _ := cmd.Flags().GetString("persona")

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
		profile, src, ok, perr := a.resolveProjectBrandProfile(cmd, locale, channel, persona)
		if perr != nil {
			return nil, "", perr
		}
		if ok {
			return profile, src, nil
		}
		return nil, "", errors.New("specify a profile with --profile, --profile-file, or --pack (or bind one in your kapi.yaml under defaults.brand_voice)")
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

	if locale != "" || channel != "" || persona != "" {
		profile = brand.ResolveProfile(profile, model.LocaleID(locale), channel, persona)
	}
	return profile, src, nil
}

// resolveProjectBrandProfile resolves a brand voice profile from the .kapi
// project in scope, with no profile flag — the cobra adapter over
// ResolveBrandProfile: it discovers and loads the project, resolves the local
// brand store from the standard resource flags (when the command carries
// them), and delegates the resolution ladder.
//
// Returns (profile, source, found, error). found is false (with nil error)
// when no project is in scope or the project carries no brand binding and no
// convention file — letting the caller surface the "specify a profile" error.
func (a *App) resolveProjectBrandProfile(cmd Command, locale, channel, persona string) (*brand.VoiceProfile, string, bool, error) {
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

	// The local brand store from the standard --name/--local/--file resource
	// flags; commands without those flags fall through to the ./brand.db
	// default, exactly as before.
	storePath, err := resolveResourcePath(cmd, "brands", "brand.db")
	if err != nil {
		return nil, "", false, err
	}

	return a.ResolveBrandProfile(CmdContext(cmd), proj, root, BrandResolveOptions{
		Locale: locale, Channel: channel, Persona: persona, StorePath: storePath,
	})
}

// BrandResolveOptions configures ResolveBrandProfile.
type BrandResolveOptions struct {
	// Locale, Channel, and Persona apply per-audience overrides to the resolved
	// profile (brand.ResolveProfile). Empty means the base profile. Persona is
	// an author voice layered inside the brand's guardrails.
	Locale  string
	Channel string
	Persona string
	// StorePath is the local SQLite brand store consulted when the recipe
	// binds defaults.brand_voice.profile. Empty means "brand.db" relative to
	// the project root (the CLI's flag-free default resolves against the
	// working directory instead, via its resource flags).
	StorePath string
}

// ResolveBrandProfile resolves the brand voice profile bound to a loaded
// project — the cobra-free resolution ladder shared by the CLI's flag-free
// brand/check/verify paths and the Kapi Desktop. Resolution order:
//
//  1. defaults.brand_voice in the recipe (profile_file → YAML, pack →
//     built-in starter pack, profile → local brand store). profile_file is
//     resolved relative to the project root.
//  2. A convention file at <root>/brand.yaml, then <root>/.kapi/brand.yaml.
//
// Returns (profile, source, found, error). found is false (with nil error)
// when the project carries no brand binding and no convention file.
func (a *App) ResolveBrandProfile(ctx context.Context, proj *project.KapiProject, root string, opts BrandResolveOptions) (*brand.VoiceProfile, string, bool, error) {
	if proj == nil {
		return nil, "", false, nil
	}
	storePath := opts.StorePath
	if storePath == "" {
		storePath = filepath.Join(root, "brand.db")
	}

	profile, src, found, err := a.loadBoundBrandProfile(ctx, proj, root, storePath)
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

	if opts.Locale != "" || opts.Channel != "" || opts.Persona != "" {
		profile = brand.ResolveProfile(profile, model.LocaleID(opts.Locale), opts.Channel, opts.Persona)
	}
	return profile, src, true, nil
}

// loadBoundBrandProfile resolves the recipe's defaults.brand_voice binding
// into a VoiceProfile. Returns found=false when the recipe has no binding.
// profile_file paths are resolved relative to the project root; a profile
// name is looked up in the local brand store at storePath.
func (a *App) loadBoundBrandProfile(ctx context.Context, proj *project.KapiProject, root, storePath string) (*brand.VoiceProfile, string, bool, error) {
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
		p, err := lookupStoreProfileAt(ctx, storePath, bv.Profile)
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

// lookupStoreProfile finds a profile in the local store by ID or by name,
// resolving the store from the standard resource flags.
func (a *App) lookupStoreProfile(cmd Command, name string) (*brand.VoiceProfile, error) {
	dbPath, err := resolveResourcePath(cmd, "brands", "brand.db")
	if err != nil {
		return nil, err
	}
	return lookupStoreProfileAt(CmdContext(cmd), dbPath, name)
}

// lookupStoreProfileAt finds a profile in the local brand store at dbPath by
// ID, slugged name, then case-insensitive name — the cobra-free leg of
// lookupStoreProfile. A store file that does not exist yet reads as
// profile-not-found (without creating an empty database as a side effect).
func lookupStoreProfileAt(ctx context.Context, dbPath, name string) (*brand.VoiceProfile, error) {
	notFound := fmt.Errorf("brand voice profile %q not found in local store (try 'kapi brand pack %s' or 'kapi brand profiles')", name, name)
	if _, err := os.Stat(dbPath); err != nil {
		return nil, notFound
	}
	store, err := openBrandStoreAt(dbPath)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	if p, gerr := store.GetProfile(ctx, name); gerr == nil {
		return p, nil
	}
	if p, gerr := store.GetProfile(ctx, slugify(name)); gerr == nil {
		return p, nil
	}
	profiles, lerr := store.ListProfiles(ctx, LocalWorkspace)
	if lerr != nil {
		return nil, lerr
	}
	for _, p := range profiles {
		if strings.EqualFold(p.Name, name) {
			return p, nil
		}
	}
	return nil, notFound
}

// BuildBrandProvider resolves an LLM provider from --provider/--api-key/
// --credential plus the saved credential store.
func (a *App) BuildBrandProvider(cmd Command) (aiprovider.LLMProvider, error) {
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

// ReadSubjectText reads the text to check/rewrite from --input-text, a positional
// file argument, or stdin (when --input-text is empty or "-").
func ReadSubjectText(cmd Command, args []string) (string, error) {
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

// RunBlockTool runs a single-block tool over text and returns the brand voice
// findings it produced (read from the block annotation).
func RunBlockTool(ctx context.Context, t tool.Tool, text string) ([]brand.BrandVoiceFinding, error) {
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

// RuleRewrite applies forbidden/competitor term replacements from the profile,
// preserving surrounding text. Returns the rewritten text and the changes made.
func RuleRewrite(profile *brand.VoiceProfile, text string) (string, []output.BrandChange) {
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

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a profile name to a stable, filename-safe ID.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
