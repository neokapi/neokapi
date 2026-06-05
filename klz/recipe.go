package klz

import (
	"encoding/json"
	"fmt"

	"github.com/neokapi/neokapi/core/project"
	"gopkg.in/yaml.v3"
)

// A .klz carries the project's FULL recipe — the same core/project.KapiProject
// schema a .kapi file uses — so there is one source of truth for intent and no
// parallel model (AD-025 §6). The recipe is manifest metadata, deliberately
// EXCLUDED from the content RootHash: content defines package identity, intent
// is metadata, mirroring how a workspace recipe already rides outside the hash.
//
// On the wire the recipe is stored as a JSON string holding the YAML encoding
// of the KapiProject. KapiProject is a yaml-tagged struct with an
// `Extras map[string]yaml.Node` inline catch-all (and yaml-only fields like
// flows' StepsSpec), so YAML is its faithful, lossless encoding; JSON-stringing
// that YAML keeps the manifest a single JSON document while preserving every
// recipe field, including platform Extras.

// WorkspaceMetaKey is the top-level Extras key under which a .klz stashes its
// ad-hoc workspace metadata (the merge-time output layout). It lives in the
// recipe's Extras rather than as a KapiProject field because it is a klz
// container concern, not a framework project concern — the `out` layout has no
// home in the .kapi schema (AD-025 §6).
const WorkspaceMetaKey = "klz"

// WorkspaceMeta is the klz-owned recipe extension: the small amount of
// container intent a .kapi project has no field for. Decoded from / encoded to
// the recipe's top-level Extras under WorkspaceMetaKey.
type WorkspaceMeta struct {
	// Out is the output path template `merge` writes to (placeholders
	// {name} {lang} {ext} {dir}); empty means the default per-locale layout.
	Out string `yaml:"out,omitempty" json:"out,omitempty"`
	// Workspace marks the package as an ad-hoc `.klz` workspace (created by
	// `extract -o work.klz`), as opposed to a whole-project snapshot created by
	// `pack`. `unpack` uses it to decide whether to rebuild the shadow cache (a
	// workspace) or rehydrate a `.kapi/` state dir (a project snapshot). Both
	// profiles carry a full recipe, so the recipe alone no longer
	// distinguishes them.
	Workspace bool `yaml:"workspace,omitempty" json:"workspace,omitempty"`
}

func init() {
	// Register the klz workspace-meta extension so a recipe carrying a
	// `klz:` block validates (and round-trips) through the framework loader.
	project.RegisterExtension(project.Extension{
		Name:  WorkspaceMetaKey,
		Scope: project.ScopeProject,
		Group: "klz",
		Decoder: project.ExtensionDecoderFunc(func(node yaml.Node) error {
			var m WorkspaceMeta
			return node.Decode(&m)
		}),
	})
}

// marshalRecipe encodes a KapiProject as a JSON string holding its YAML
// representation, suitable for embedding in the manifest. Returns (nil, nil)
// for a nil recipe.
func marshalRecipe(p *project.KapiProject) (json.RawMessage, error) {
	if p == nil {
		return nil, nil
	}
	yml, err := yaml.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("klz: marshal recipe yaml: %w", err)
	}
	out, err := json.Marshal(string(yml))
	if err != nil {
		return nil, fmt.Errorf("klz: encode recipe: %w", err)
	}
	return out, nil
}

// unmarshalRecipe decodes the manifest's recipe field (a JSON string holding
// YAML) back into a KapiProject. Returns (nil, nil) when no recipe is present.
func unmarshalRecipe(raw json.RawMessage) (*project.KapiProject, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var yml string
	if err := json.Unmarshal(raw, &yml); err != nil {
		return nil, fmt.Errorf("klz: decode recipe: %w", err)
	}
	if yml == "" {
		return nil, nil
	}
	var p project.KapiProject
	if err := yaml.Unmarshal([]byte(yml), &p); err != nil {
		return nil, fmt.Errorf("klz: parse recipe yaml: %w", err)
	}
	return &p, nil
}

// SanitizeRecipe returns a copy of the recipe with side-effecting top-level
// Extras keys stripped (`server`, `hooks`, `automations`), so they travel inert
// in a .klz and re-activate only when the package is adopted into a project
// with explicit re-auth / re-arming (AD-025 §6). Secrets never live in a recipe
// (they are in the OS keychain), so there is nothing else to scrub. A nil
// recipe returns nil.
func SanitizeRecipe(p *project.KapiProject) *project.KapiProject {
	if p == nil {
		return nil
	}
	clone := *p
	if len(p.Extras) > 0 {
		extras := make(map[string]yaml.Node, len(p.Extras))
		for k, v := range p.Extras {
			switch k {
			case "server", "hooks", "automations":
				// side-effecting — drop so it travels inert
			default:
				extras[k] = v
			}
		}
		clone.Extras = extras
	}
	return &clone
}

// RecipeWorkspaceMeta reads the klz workspace metadata from a recipe's Extras,
// returning the zero value when absent.
func RecipeWorkspaceMeta(p *project.KapiProject) WorkspaceMeta {
	if p == nil {
		return WorkspaceMeta{}
	}
	var m WorkspaceMeta
	if _, err := p.GetExtra(WorkspaceMetaKey, &m); err != nil {
		return WorkspaceMeta{}
	}
	return m
}

// SetRecipeWorkspaceMeta writes the klz workspace metadata onto a recipe's
// Extras (creating the recipe is the caller's job). A zero meta clears the key.
func SetRecipeWorkspaceMeta(p *project.KapiProject, m WorkspaceMeta) error {
	if p == nil {
		return nil
	}
	if m == (WorkspaceMeta{}) {
		p.DeleteExtra(WorkspaceMetaKey)
		return nil
	}
	return p.SetExtra(WorkspaceMetaKey, m)
}
