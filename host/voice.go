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
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/profile/packs"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host/credentials"
	"github.com/neokapi/neokapi/host/output"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	voicestore "github.com/neokapi/neokapi/voice"
)

// LocalScope is the profile scope for the local CLI voice store: a single-owner
// store keeps everything under the empty partition key.
const LocalScope = ""

// AddProfileFlags adds the mutually-exclusive profile-source flags plus the
// voice-store resource flags.
func AddProfileFlags(cmd Command) {
	cmd.Flags().String("profile", "", "voice profile name in the local store")
	cmd.Flags().String("profile-file", "", "path to a standalone profile YAML")
	cmd.Flags().String("pack", "", "built-in starter pack name")
	cmd.Flags().String("locale", "", "apply locale-specific overrides")
	cmd.Flags().String("channel", "", "apply channel-specific overrides")
	cmd.Flags().String("persona", "", "apply an author persona's overrides (within the profile guardrails)")
	AddProjectFlag(cmd)
	AddResourceFlags(cmd)
}

func AddVoiceAIFlags(cmd Command) {
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
func EmitValidate(cmd Command, out output.VoiceValidateOutput) error {
	// Valid means usable, not unremarked. A profile carrying only advisories —
	// a tone this does not recognise, which the guide renders as written — is
	// a working profile, and reporting it INVALID is what made kapi refuse the
	// register it had inferred from a project's own documentation.
	out.Valid = len(coreprofile.Blocking(out.Errors)) == 0
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
func StrictDecodeProblems(err error) []coreprofile.ProfileProblem {
	if err == nil {
		return nil
	}
	var probs []coreprofile.ProfileProblem
	for line := range strings.SplitSeq(err.Error(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "yaml: unmarshal errors:") {
			continue
		}
		if m := unknownFieldRe.FindStringSubmatch(line); m != nil {
			probs = append(probs, coreprofile.ProfileProblem{
				Field:   m[2],
				Message: fmt.Sprintf("unknown field %q (line %s)", m[2], m[1]),
			})
			continue
		}
		probs = append(probs, coreprofile.ProfileProblem{Message: line})
	}
	return probs
}

// ---------------------------------------------------------------------------
// profiles / show / import / pack
// ---------------------------------------------------------------------------

// VoiceProfileTemplate is a commented, schema-valid VoiceProfile starting point
// emitted by `kapi voice new`. It parses via coreprofile.LoadProfileYAML (guarded by a
// test), so an AI assistant or a human can fill it in and import it directly.
const VoiceProfileTemplate = `# Voice profile. Fill in the fields, then:
#   kapi voice import voice.yaml                     # save to the local store
#   kapi voice guide --profile-file voice.yaml       # render the guide
#   echo "draft" | kapi voice check --profile-file voice.yaml --json
# Only 'name' is required; every other field is optional. The English source
# text always stays the key — do not invent message IDs.

name: My Voice
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

// openVoiceStoreAt opens (creating if needed) the local SQLite voice store at
// dbPath — the cobra-free leg of OpenVoiceStore.
func openVoiceStoreAt(dbPath string) (*voicestore.SQLiteStore, error) {
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open voice store: %w", err)
	}
	store, err := voicestore.NewSQLiteStore(db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// SaveProfileToStore creates or updates a profile in the local store, returning
// a typed import result.
func (a *App) SaveProfileToStore(cmd Command, profile *coreprofile.VoiceProfile, srcPath string) error {
	store, _, release, err := a.OpenVoiceStore(cmd)
	if err != nil {
		return err
	}
	defer release()

	if profile.ID == "" {
		profile.ID = slugify(profile.Name)
	}
	profile.Scope = LocalScope

	action := "created"
	if _, gerr := store.GetProfile(cmd.Context(), profile.ID); gerr == nil {
		if uerr := store.UpdateProfile(cmd.Context(), profile); uerr != nil {
			return uerr
		}
		action = "updated"
	} else if cerr := store.CreateProfile(cmd.Context(), profile); cerr != nil {
		return cerr
	}

	return output.Print(cmd, output.VoiceImportOutput{
		ID: profile.ID, Name: profile.Name, Action: action, Path: srcPath,
	})
}

// ResolveVoiceProfileCmd resolves the effective profile from --profile-file,
// --pack, or --profile (local store), then applies locale/channel overrides.
// paths, when given, are the files the caller is about to act on. The first
// one decides which content collection's context governs the answer: a repo
// holding two products binds a different voice per collection, so resolving
// `kapi voice guide` against the project default would hand back the wrong
// register for half the tree.
func (a *App) ResolveVoiceProfileCmd(cmd Command, paths ...string) (*coreprofile.VoiceProfile, string, error) {
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
		// No explicit flag — fall back to the project's bound voice profile
		// (defaults.voice) or a convention file at the project root.
		// This makes `kapi voice check DRAFT.md` work flag-free inside a
		// project directory.
		profile, src, ok, perr := a.resolveProjectVoiceProfile(cmd, locale, channel, persona, paths...)
		if perr != nil {
			return nil, "", perr
		}
		if ok {
			return profile, src, nil
		}
		return nil, "", errors.New("specify a profile with --profile, --profile-file, or --pack (or bind one in your kapi.yaml under defaults.voice)")
	}

	var profile *coreprofile.VoiceProfile
	var src string
	switch {
	case file != "":
		f, err := os.Open(file)
		if err != nil {
			return nil, "", fmt.Errorf("open profile: %w", err)
		}
		defer f.Close()
		profile, err = coreprofile.LoadProfileYAML(f)
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
		profile = coreprofile.ResolveProfile(profile, model.LocaleID(locale), channel, persona)
	}
	return profile, src, nil
}

// resolveProjectVoiceProfile resolves a voice profile from the .kapi
// project in scope, with no profile flag — the cobra adapter over
// ResolveVoiceProfile: it discovers and loads the project, resolves the local
// voice store from the standard resource flags (when the command carries
// them), and delegates the resolution ladder.
//
// Returns (profile, source, found, error). found is false (with nil error)
// when no project is in scope or the project carries no voice binding and no
// convention file — letting the caller surface the "specify a profile" error.
func (a *App) resolveProjectVoiceProfile(cmd Command, locale, channel, persona string, paths ...string) (*coreprofile.VoiceProfile, string, bool, error) {
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil {
		return nil, "", false, err
	}
	if projectPath == "" {
		return nil, "", false, nil
	}

	root := filepath.Dir(projectPath)

	// Load the recipe to read defaults.voice. Skip the
	// requires-extension check so voice resolution does not demand plugins
	// that voice check/rewrite/guide do not actually use.
	proj, lerr := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if lerr != nil {
		return nil, "", false, fmt.Errorf("load project for voice profile: %w", lerr)
	}

	// The voice store the flags select: the project's own pool when none are
	// given, a standalone file when one is.
	store, release, err := a.VoiceLookupStore(cmd)
	if err != nil {
		return nil, "", false, err
	}
	defer release()

	// Where the first named file sits, if any. A path outside every declared
	// glob resolves the project defaults — the same answer as before this
	// lookup existed.
	point := a.GovernancePointFor("", "")
	for _, path := range paths {
		if path == "" {
			continue
		}
		abs := path
		if !filepath.IsAbs(abs) {
			if cwd, cerr := os.Getwd(); cerr == nil {
				abs = filepath.Join(cwd, path)
			}
		}
		if rel, rerr := filepath.Rel(root, abs); rerr == nil && !strings.HasPrefix(rel, "..") {
			point = a.GovernancePointFor("", filepath.ToSlash(rel))
		}
		break
	}

	return a.ResolveVoiceProfile(CmdContext(cmd), proj, root, VoiceResolveOptions{
		Locale: locale, Channel: channel, Persona: persona, Store: store,
		Point: point,
	})
}

// VoiceResolveOptions configures ResolveVoiceProfile.
type VoiceResolveOptions struct {
	// Locale, Channel, and Persona apply per-audience overrides to the resolved
	// profile (coreprofile.ResolveProfile). Empty means the base profile. Persona is
	// an author voice layered inside the profile guardrails.
	Locale  string
	Channel string
	Persona string
	// Store is the voice store consulted when the recipe binds
	// defaults.voice.profile by name. Inside a project it is the shared pool's
	// (App.ProjectVoiceStore); a --name/--local/--file flag selects a
	// standalone file instead (App.VoiceLookupStore). nil means there is
	// nowhere to look, and a name-bound profile is reported as not found.
	Store coreprofile.Store
	// Point is the place in the context space whose governance to resolve: a
	// content collection, or the finer point one file sits at. Its coordinates
	// select the profile (hence the voice) and the channel; the zero point, and
	// a place that declares none, resolve the project's own default point —
	// defaults.voice, no channel. A non-zero At applies profile validity, so an
	// expired profile stops selecting its voice.
	Point project.GovernancePoint
}

// ResolveVoiceProfile resolves the voice profile bound to a loaded
// project — the cobra-free resolution ladder shared by the CLI's flag-free
// voice/check/verify paths and the Kapi Desktop. Resolution order:
//
//  1. The `voice:` of the profile whose `when:` matches the collection's point
//     most closely (profile_file → YAML, pack → built-in starter pack, profile
//     → local voice store). profile_file is resolved relative to the project
//     root.
//  2. That profile's conventional file, <root>/.kapi/profiles/<name>/voice.yaml.
//  3. defaults.voice, in the same three forms.
//  4. A convention file at <root>/.kapi/voice.yaml, then <root>/voice.yaml.
//
// What the recipe binds is *loaded* here and then handed to the framework's one
// resolution chain (coreprofile.ResolveProfileFromContext) at the collection tier,
// where a connector- or editor-created project's `CollectionConfig` binding
// sits. Loading has to happen here because a recipe binds a profile file or a
// starter pack, which the store cannot name — but ranking does not: an explicit
// per-call profile still wins over the recipe, stream/project/workspace
// bindings still sit under it, and the locale/channel/persona composition is
// the same ResolveProfile every surface uses. The collection's channel enters
// at the same tier; an explicit Channel (a `--channel` flag) outranks it there.
//
// Returns (profile, source, found, error). found is false (with nil error)
// when the project carries no voice binding and no convention file.
func (a *App) ResolveVoiceProfile(ctx context.Context, proj *project.KapiProject, root string, opts VoiceResolveOptions) (*coreprofile.VoiceProfile, string, bool, error) {
	profile, _, src, found, err := a.ResolveVoiceAtPoint(ctx, proj, root, opts)
	return profile, src, found, err
}

// ResolveVoiceAtPoint is ResolveVoiceProfile plus the governance that selected
// the voice. A caller reporting what applies at a location needs both halves —
// the composed profile it would write against, and the point, channel and
// validity window that chose it — and resolving them twice is how the two come
// to disagree.
func (a *App) ResolveVoiceAtPoint(ctx context.Context, proj *project.KapiProject, root string, opts VoiceResolveOptions) (*coreprofile.VoiceProfile, *project.ResolvedGovernance, string, bool, error) {
	if proj == nil {
		return nil, nil, "", false, nil
	}
	rc, err := proj.ResolveGovernanceFor(opts.Point)
	if err != nil {
		return nil, nil, "", false, err
	}
	profile, src, found, err := a.resolveVoiceForGovernance(ctx, root, opts.Store, rc, opts)
	return profile, rc, src, found, err
}

// resolveVoiceForGovernance loads the voice an already-resolved governance binds
// and composes the locale, channel and persona overrides onto it.
//
// The governance is passed in rather than resolved here because a caller that
// reports the point AND the voice must resolve the point exactly once: two
// resolutions of the same request is how a reported coordinate and the guidance
// under it come to describe different places.
func (a *App) resolveVoiceForGovernance(ctx context.Context, root string, store coreprofile.Store, rc *project.ResolvedGovernance, opts VoiceResolveOptions) (*coreprofile.VoiceProfile, string, bool, error) {
	profile, src, found, err := a.loadVoiceAtGovernance(ctx, root, store, rc)
	if err != nil || !found {
		return nil, "", false, err
	}

	// The recipe's match enters the shared chain at the collection tier. No
	// store is passed: every tier this caller binds carries an already-loaded
	// profile, because a recipe's `profile:` name was resolved against the local
	// store by path above (matching id, then slug, then name — which
	// Store.GetProfile does not do).
	resolved, err := coreprofile.ResolveProfileFromContext(ctx, coreprofile.ResolveContext{
		CollectionProfile: profile,
		CollectionConfig:  map[string]string{coreprofile.PropertyChannel: rc.Channel},
		Locale:            model.LocaleID(opts.Locale),
		Channel:           opts.Channel,
		Persona:           opts.Persona,
	}, nil)
	if err != nil {
		return nil, "", false, err
	}
	return resolved, src, true, nil
}

// LoadCollectionVoice loads the voice profile bound to a collection's point AS
// AUTHORED — before any locale, channel or persona override is layered on —
// together with the governance that point resolved to.
//
// ResolveVoiceProfile is this plus the override composition, and calls it. The
// split exists because a caller that is going to CARRY the profile somewhere
// else needs the authored content, not a resolution of it: a profile whose
// base tone has already been replaced by one channel's override no longer
// describes how the voice sounds on any other channel, so storing it under the
// profile's name would quietly collapse the variants into whichever collection
// happened to be resolved last.
//
// Returns (profile, governance, source, found, error). found is false (with a
// nil error) when the project carries no voice binding and no convention file;
// the governance is still returned, since its channel and terms are resolved
// whether or not a voice was bound.
func (a *App) LoadCollectionVoice(ctx context.Context, proj *project.KapiProject, root string, opts VoiceResolveOptions) (*coreprofile.VoiceProfile, *project.ResolvedGovernance, string, bool, error) {
	if proj == nil {
		return nil, nil, "", false, nil
	}
	rc, err := proj.ResolveGovernanceFor(opts.Point)
	if err != nil {
		return nil, nil, "", false, err
	}
	profile, src, found, lerr := a.loadVoiceAtGovernance(ctx, root, opts.Store, rc)
	return profile, rc, src, found, lerr
}

// loadVoiceAtGovernance loads the voice profile an already-resolved governance
// binds, AS AUTHORED — the ladder LoadCollectionVoice documents, once the point
// itself has been resolved.
func (a *App) loadVoiceAtGovernance(ctx context.Context, root string, store coreprofile.Store, rc *project.ResolvedGovernance) (*coreprofile.VoiceProfile, string, bool, error) {
	if rc == nil {
		return nil, "", false, nil
	}
	// A profile that binds no `voice:` of its own is answered by its own
	// directory before the project default is: `.kapi/profiles/<name>/voice.yaml`
	// is that profile's voice by convention, and a project that keeps its
	// overrides there should not have to bind every one of them by hand.
	if rc.Profile != "" && rc.VoiceField == project.DefaultVoiceField {
		conv := filepath.Join(root, project.RelStatePath(project.ProfilesDirName, rc.Profile, VoiceConventionalName))
		p, lerr := loadProfileFile(conv)
		if lerr != nil {
			return nil, "", false, lerr
		}
		if p != nil {
			return p, conv, true, nil
		}
	}
	profile, src, found, err := a.loadBoundVoiceProfile(ctx, rc.Voice, root, store, rc.VoiceField)
	if err != nil {
		return nil, "", false, err
	}
	if !found {
		for _, conv := range VoiceProfileConventions(root) {
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
	return profile, src, found, nil
}

// VoiceConventionalName is the voice profile's filename at a conventional
// location, matching kmb/ktb's ConventionalName for the other two committed
// sources.
//
// Just `voice.yaml`, because the directory already says whose voice it is:
// `.kapi/voice.yaml` is the project's, `.kapi/profiles/bowrain/voice.yaml` is
// that profile's. A per-profile scope belongs in the path, not in the filename.
const VoiceConventionalName = "voice.yaml"

// VoiceProfileConventions lists the well-known profile locations, in the order
// an unbound project is searched.
//
// `.kapi/` comes first: it is committed, and it is where a project's authored
// sources live — beside the terms bundle and the memory bundles, which is where
// a reader looks for the voice. The root spelling is second; a project that
// keeps its profile there is not wrong.
func VoiceProfileConventions(root string) []string {
	return []string{
		filepath.Join(root, project.RelStatePath(VoiceConventionalName)),
		filepath.Join(root, VoiceConventionalName),
	}
}

// loadBoundVoiceProfile turns a resolved voice binding into a VoiceProfile.
// Returns found=false when the binding is nil (nothing bound at this point, nor
// project-wide). field names the recipe key the binding came from,
// so a missing file names the line to fix. profile_file paths are resolved
// relative to the project root; a profile name is looked up in the local voice
// store.
func (a *App) loadBoundVoiceProfile(ctx context.Context, bv *project.VoiceBinding, root string, store coreprofile.Store, field string) (*coreprofile.VoiceProfile, string, bool, error) {
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
			return nil, "", false, fmt.Errorf("voice profile file %q (from %s.profile_file) not found", path, field)
		}
		return p, path, true, nil
	case bv.Pack != "":
		p, err := packs.Load(bv.Pack)
		if err != nil {
			return nil, "", false, err
		}
		return p, "pack:" + bv.Pack, true, nil
	case bv.Profile != "":
		p, err := lookupProfileIn(ctx, store, bv.Profile)
		if err != nil {
			return nil, "", false, err
		}
		return p, "store:" + bv.Profile, true, nil
	}
	return nil, "", false, nil
}

// loadProfileFile loads a VoiceProfile YAML from path. Returns (nil, nil) when
// the file does not exist so callers can fall through to other sources.
func loadProfileFile(path string) (*coreprofile.VoiceProfile, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open profile %q: %w", path, err)
	}
	defer f.Close()
	p, err := coreprofile.LoadProfileYAML(f)
	if err != nil {
		return nil, fmt.Errorf("load profile %q: %w", path, err)
	}
	return p, nil
}

// lookupStoreProfile finds a profile in the voice store this command resolves,
// by ID or by name.
func (a *App) lookupStoreProfile(cmd Command, name string) (*coreprofile.VoiceProfile, error) {
	store, release, err := a.VoiceLookupStore(cmd)
	if err != nil {
		return nil, err
	}
	defer release()
	return lookupProfileIn(CmdContext(cmd), store, name)
}

// lookupProfileIn finds a profile in a voice store by ID, slugged name, then
// case-insensitive name. A nil store — no project, and no standalone file on
// disk — reads as profile-not-found.
func lookupProfileIn(ctx context.Context, store coreprofile.Store, name string) (*coreprofile.VoiceProfile, error) {
	notFound := fmt.Errorf("voice profile %q not found in local store (try 'kapi voice pack %s' or 'kapi voice profiles')", name, name)
	if store == nil {
		return nil, notFound
	}

	if p, gerr := store.GetProfile(ctx, name); gerr == nil {
		return p, nil
	}
	if p, gerr := store.GetProfile(ctx, slugify(name)); gerr == nil {
		return p, nil
	}
	profiles, lerr := store.ListProfiles(ctx, LocalScope)
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

// BuildVoiceProvider resolves an LLM provider from --provider/--api-key/
// --credential plus the saved credential store.
func (a *App) BuildVoiceProvider(cmd Command) (aiprovider.LLMProvider, error) {
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

// RunBlockTool runs a single-block tool over text and returns the voice
// findings it produced (read from the block annotation).
func RunBlockTool(ctx context.Context, t tool.Tool, text string) ([]coreprofile.VoiceFinding, error) {
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

	if ann, ok := model.AnnoAs[*coreprofile.VoiceAnnotation](block, "voice"); ok {
		return ann.Findings, nil
	}
	// Fallback: read findings from properties (rule-based or AI key).
	for _, key := range []string{"voice-vocab-findings", "voice-findings"} {
		if raw, ok := block.Properties[key]; ok && raw != "" {
			var fs []coreprofile.VoiceFinding
			if json.Unmarshal([]byte(raw), &fs) == nil {
				return fs, nil
			}
		}
	}
	return nil, nil
}

// RuleRewrite applies forbidden/competitor term replacements from the profile,
// preserving surrounding text. Returns the rewritten text and the changes made.
func RuleRewrite(profile *coreprofile.VoiceProfile, text string) (string, []output.VoiceChange) {
	var changes []output.VoiceChange
	result := text
	apply := func(rules []coreprofile.TermRule) {
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
			changes = append(changes, output.VoiceChange{From: rule.Term, To: rule.Replacement, Count: n})
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
