package host

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host/output"
)

// InitOptions configures InitProject.
type InitOptions struct {
	// Name is the project id written into the recipe's name: field. Empty
	// defaults to the root directory's basename.
	Name string
	// SourceLocale is the BCP-47 source language. Empty defaults to "en".
	SourceLocale string
	// TargetLocales are written under defaults.target_languages. A project
	// with none keeps its source content and translates nothing.
	TargetLocales []string
	// Framework names a framework preset whose catalog layout is proposed
	// whether or not its files exist yet.
	Framework string
	// MintID asks for a stable project id on a recipe that is already there
	// and carries none. A recipe InitProject writes itself always gets one, so
	// this only governs the adopt path.
	MintID bool
	// Formats recognises the files a proposal reads. Nil uses a registry of
	// the built-in formats.
	Formats *registry.FormatRegistry
}

// InitResult reports what InitProject did.
type InitResult struct {
	Name       string
	RecipePath string
	// ID is the project's stable id, empty when the adopted recipe has none
	// and none was asked for.
	ID string
	// IDMinted is true when this call wrote ID into the recipe.
	IDMinted bool
	// AlreadyInitialized is true when a recipe was already present; InitProject
	// adopts it and leaves its collections as they are (init is idempotent).
	AlreadyInitialized bool
	// Collections are the collections written into a new recipe, proposed from
	// the files in the tree.
	Collections []ProposedCollection
	// Uncovered, on a project that already had a recipe, are the collections
	// kapi would propose for content no existing collection reads. Nothing is
	// written for them; the caller prints them for a person to add.
	Uncovered []ProposedCollection
	// TargetLanguages are the target languages the recipe declares, read back
	// from it whichever way it arrived.
	TargetLanguages []string
}

// InitProject scaffolds (or adopts) a kapi project at root: it proposes
// collections from the files in the tree, writes the recipe (unless one is
// already present), and creates the `.kapi/` cache directory. It is idempotent:
// re-running on an initialized project adopts the existing recipe, leaves its
// collections alone and reports what content no collection reads yet.
func InitProject(root string, opts InitOptions) (*InitResult, error) {
	name := opts.Name
	if name == "" {
		name = filepath.Base(root)
	}
	sourceLocale := opts.SourceLocale
	if sourceLocale == "" {
		sourceLocale = "en"
	}

	recipeExists, err := RecipeExists(root)
	if err != nil {
		return nil, fmt.Errorf("check for existing project: %w", err)
	}
	recipePath := filepath.Join(root, project.RecipeFileName)

	res := &InitResult{
		Name:               name,
		RecipePath:         recipePath,
		AlreadyInitialized: recipeExists,
	}

	formats := opts.Formats
	if formats == nil {
		a := &App{}
		a.InitRegistries()
		formats = a.FormatReg
	}

	switch {
	case !recipeExists:
		proposals, perr := ProposeCollections(root, ProposeOptions{
			Formats:      formats,
			SourceLocale: sourceLocale,
			Targets:      len(opts.TargetLocales) > 0,
			Framework:    opts.Framework,
		})
		if perr != nil {
			return nil, perr
		}
		res.Collections = proposals
		res.TargetLanguages = opts.TargetLocales
		// Every project kapi scaffolds is born with a stable identity, so
		// nothing it later records is keyed on a label the user is free to
		// edit.
		res.ID, res.IDMinted = project.NewID(), true
		recipe := ScaffoldRecipe(name, res.ID, sourceLocale, opts.TargetLocales, proposals)
		if err := os.WriteFile(recipePath, recipe, 0o644); err != nil {
			return nil, fmt.Errorf("write recipe: %w", err)
		}
	default:
		if opts.MintID {
			id, minted, err := MintProjectID(recipePath)
			if err != nil {
				return nil, err
			}
			res.ID, res.IDMinted = id, minted
		}
		proj, lerr := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
		if lerr != nil {
			return nil, fmt.Errorf("load project: %w", lerr)
		}
		for _, t := range proj.Defaults.TargetLanguages {
			res.TargetLanguages = append(res.TargetLanguages, string(t))
		}
		src := sourceLocale
		if proj.Defaults.SourceLanguage != "" {
			src = string(proj.Defaults.SourceLanguage)
		}
		proposals, perr := ProposeCollections(root, ProposeOptions{
			Formats:      formats,
			SourceLocale: src,
			Targets:      len(res.TargetLanguages) > 0,
			Framework:    opts.Framework,
		})
		if perr != nil {
			return nil, perr
		}
		res.Uncovered = uncoveredProposals(proj, proposals)
	}

	if err := project.EnsureLayout(project.LayoutAt(root)); err != nil {
		return nil, fmt.Errorf("create %s: %w", project.StateDirName, err)
	}
	return res, nil
}

// uncoveredProposals keeps the proposals whose files no collection in the
// recipe reads yet. A proposal with a file an existing collection covers is
// dropped whole: a person who chose a narrower glob for that content has
// already decided about it.
func uncoveredProposals(proj *project.KapiProject, proposals []ProposedCollection) []ProposedCollection {
	var globs []string
	for i := range proj.Collections {
		for _, item := range proj.Collections[i].EffectiveItems() {
			if item.Path != "" {
				globs = append(globs, item.Path)
			}
		}
	}
	covered := func(file string) bool {
		for _, g := range globs {
			if project.MatchGlob(g, file) {
				return true
			}
		}
		return false
	}
	var out []ProposedCollection
	for _, p := range proposals {
		if len(p.Files) == 0 {
			continue
		}
		if !slices.ContainsFunc(p.Files, covered) {
			out = append(out, p)
		}
	}
	return out
}

// MintProjectID gives a recipe the stable project id it has none of, and
// reports the id the recipe carries either way along with whether this call
// wrote it.
//
// The write goes through the recipe setter every other pending recipe change
// takes (project.SetField, then project.Save over core/yamledit), so the file
// keeps its comments, its blank lines, its key order and the spelling of every
// value already in it. A recipe that already carries an id is read and left
// alone: SetField refuses to re-key a project, and this path does not ask it to.
func MintProjectID(recipePath string) (string, bool, error) {
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return "", false, fmt.Errorf("load project: %w", err)
	}
	if proj.ID != "" {
		return proj.ID, false, nil
	}
	id := project.NewID()
	raw, err := json.Marshal(id)
	if err != nil {
		return "", false, fmt.Errorf("encode project id: %w", err)
	}
	if _, err := project.SetField(proj, "id", raw); err != nil {
		return "", false, err
	}
	if err := project.Save(recipePath, proj); err != nil {
		return "", false, err
	}
	return id, true, nil
}

// writeScaffoldID writes the project's stable id under `version:`, with the one
// line of explanation a reader of a fresh recipe needs for a value they did not
// type. An empty id writes nothing, which is what a caller building a recipe
// for comparison rather than for a project asks for.
func writeScaffoldID(b *strings.Builder, id string) {
	if id == "" {
		return
	}
	b.WriteString("# This project's stable identity. It survives a rename, a move and a clone,\n")
	b.WriteString("# and everything kapi records about the project is keyed on it. Keep it.\n")
	b.WriteString("id: ")
	b.WriteString(id)
	b.WriteByte('\n')
}

// PrintPresetList emits the full preset catalog (framework scaffolds plus
// per-format parsing presets), the `kapi init --list-presets` surface.
func PrintPresetList(cmd Command) error {
	reg := preset.NewPresetRegistry()
	preset.RegisterBuiltins(reg)
	entries := CollectAllPresets(reg)
	return output.Print(cmd, output.PresetsListOutput{Presets: entries, Total: len(entries)})
}

// ─── helpers ────────────────────────────────────────────────────

func ResolveDir(flag string) (string, error) {
	if flag == "" {
		return os.Getwd()
	}
	abs, err := filepath.Abs(flag)
	if err != nil {
		return "", fmt.Errorf("resolve --dir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", abs, err)
	}
	return abs, nil
}

// RecipeExists reports whether dir already holds a kapi.yaml recipe. Used to
// detect an already-initialized project so `kapi init` is idempotent.
func RecipeExists(dir string) (bool, error) {
	_, err := os.Stat(filepath.Join(dir, project.RecipeFileName))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// ScaffoldRecipe renders a new project's recipe: its identity, its languages,
// and the collections proposed from the tree, each under a comment saying what
// it matched. With no collection it writes an empty list under a comment that
// shows the shape of one.
func ScaffoldRecipe(name, id, sourceLocale string, targetLocales []string, collections []ProposedCollection) []byte {
	var b strings.Builder
	b.WriteString("version: v1\n")
	writeScaffoldID(&b, id)
	b.WriteString("name: ")
	b.WriteString(name)
	b.WriteString("\ndefaults:\n")
	b.WriteString("  source_language: ")
	b.WriteString(sourceLocale)
	b.WriteByte('\n')
	if len(targetLocales) > 0 {
		b.WriteString("  target_languages:\n")
		for _, t := range targetLocales {
			b.WriteString("    - ")
			b.WriteString(t)
			b.WriteByte('\n')
		}
	}

	b.WriteString("\n# Collections are the files kapi reads as content.")
	if len(collections) == 0 {
		b.WriteString(` kapi init found none it
# recognised here. Add one with 'kapi add <pattern>', or write it here:
#
#   - path: "docs/**/*.md"
#     format: markdown
#     target: "i18n/{lang}/{path}.md"   # where translations go, if any
collections: []
`)
		return []byte(b.String())
	}
	b.WriteString(` kapi init proposed these
# from the files it found. Edit them freely; 'kapi add <pattern>' adds more.
collections:
`)
	b.WriteString(RenderCollections(collections))
	return []byte(b.String())
}

// RenderCollections renders proposals as entries of a recipe's `collections:`
// list, each under its one-line comment. It is what a new recipe carries and
// what a re-run prints for a person to paste.
func RenderCollections(collections []ProposedCollection) string {
	var b strings.Builder
	for _, c := range collections {
		fmt.Fprintf(&b, "  # %s\n", c.Reason)
		fmt.Fprintf(&b, "  - path: %s\n", yamlScalar(c.Path))
		fmt.Fprintf(&b, "    format: %s\n", c.Format)
		if c.Target != "" {
			fmt.Fprintf(&b, "    target: %s\n", yamlScalar(c.Target))
		}
	}
	return b.String()
}

// plainScalar matches a path YAML reads back as the same string unquoted.
var plainScalar = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./-]*$`)

// yamlScalar writes a path plain when it can be, and double-quoted otherwise,
// which every glob needs because `*` opens an alias in YAML.
func yamlScalar(s string) string {
	if plainScalar.MatchString(s) {
		return s
	}
	return fmt.Sprintf("%q", s)
}
