package project

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/flow"
)

// execClassTools are the built-in tool IDs whose whole job is to run code the
// recipe names, rather than to transform content with code kapi ships:
// `external-command` spawns a subprocess from its own config, and `script`
// evaluates recipe-supplied JavaScript (and can read an arbitrary file through
// scriptFile).
//
// The same classification is applied on two other surfaces, for the same
// reason. `host/mcp_tools.go` withholds both from the agent-facing MCP surface
// even under --all-tools (AD-037), and `kpz.SanitizeRecipe` strips both out of
// a recipe arriving inside a package. This map is the recipe arm: a recipe read
// off disk is authored by whoever wrote the directory, which is not necessarily
// whoever is running kapi.
var execClassTools = map[string]bool{
	"external-command": true,
	"script":           true,
}

// execClassFormats are format names that spawn a subprocess to read content, so
// naming one in a recipe arms execution exactly as a step does.
var execClassFormats = map[string]bool{"exec": true}

// IsExecClassTool reports whether a tool ID runs recipe-supplied code. Exported
// so the host layer can refuse to construct one without an established trust
// decision, without duplicating the list.
func IsExecClassTool(id string) bool { return execClassTools[id] }

// IsExecClassFormat reports whether a format name runs recipe-supplied code.
func IsExecClassFormat(name string) bool { return execClassFormats[name] }

// ExecSite is one place in a recipe that arms code execution: a flow step, a
// per-tool config preset that would supply such a step's argv, or a content
// item bound to a format that shells out.
type ExecSite struct {
	// Where locates the site in the recipe, in the recipe's own vocabulary
	// (e.g. `flows.default.steps[2]`, `defaults.locales.nb.tools`).
	Where string
	// Kind is "tool" or "format".
	Kind string
	// Name is the tool ID or format name.
	Name string
	// Detail summarises what would run — the command line, or the head of the
	// script — so a person can answer the prompt on the facts rather than on
	// the tool's name. It is derived from untrusted recipe text and is
	// sanitised for terminal display; see sanitizeForDisplay.
	Detail string
	// config is the canonical form of the site's configuration, used for the
	// digest so that changing an argv invalidates a previous decision.
	config map[string]any
}

// ExecSurface enumerates every site in a recipe that would run code the recipe
// itself names. An empty result means the recipe cannot execute anything, which
// is the case for every recipe in this repository and for the overwhelming
// majority of real ones.
//
// The walk mirrors kpz.SanitizeRecipe's: flow steps including nested `parallel`
// branches and the retired `source_transforms` stage, the `defaults.tools` and
// `defaults.locales.*.tools` presets that would arm a step, and format bindings
// on content collections and their items. A site that only *configures* an
// exec-class tool still counts: the config is the argv a step would use, so it
// is part of what a person is being asked to approve.
//
// Sites come back in a deterministic order so the digest is stable across runs.
func ExecSurface(p *KapiProject) []ExecSite {
	if p == nil {
		return nil
	}
	var sites []ExecSite

	// Flows, in name order — Go map iteration is randomised.
	flowNames := make([]string, 0, len(p.Flows))
	for name := range p.Flows {
		flowNames = append(flowNames, name)
	}
	sort.Strings(flowNames)
	for _, name := range flowNames {
		spec := p.Flows[name]
		if spec == nil {
			continue
		}
		sites = append(sites, execSitesInSteps(spec.Steps, fmt.Sprintf("flows.%s.steps", name))...)
		sites = append(sites, execSitesInSteps(spec.SourceTransforms, fmt.Sprintf("flows.%s.source_transforms", name))...)
	}

	sites = append(sites, execSitesInToolConfig(p.Defaults.Tools, "defaults.tools")...)
	localeNames := make([]string, 0, len(p.Defaults.Locales))
	for loc := range p.Defaults.Locales {
		localeNames = append(localeNames, loc)
	}
	sort.Strings(localeNames)
	for _, loc := range localeNames {
		sites = append(sites, execSitesInToolConfig(p.Defaults.Locales[loc].Tools, "defaults.locales."+loc+".tools")...)
	}

	for i, coll := range p.Collections {
		where := fmt.Sprintf("collections[%d]", i)
		if coll.Format != nil && execClassFormats[coll.Format.Name] {
			sites = append(sites, ExecSite{
				Where:  where + ".format",
				Kind:   "format",
				Name:   coll.Format.Name,
				Detail: summariseExecConfig(coll.Format.Config),
				config: coll.Format.Config,
			})
		}
		for j, item := range coll.Content {
			if item.Format != nil && execClassFormats[item.Format.Name] {
				sites = append(sites, ExecSite{
					Where:  fmt.Sprintf("%s.content[%d].format", where, j),
					Kind:   "format",
					Name:   item.Format.Name,
					Detail: summariseExecConfig(item.Format.Config),
					config: item.Format.Config,
				})
			}
		}
	}

	return sites
}

// execSitesInSteps collects exec-class steps, recursing into `parallel`
// branches so a step cannot hide one level down.
func execSitesInSteps(steps []flow.FlowStep, where string) []ExecSite {
	var sites []ExecSite
	for i, step := range steps {
		at := fmt.Sprintf("%s[%d]", where, i)
		if execClassTools[step.Tool] {
			sites = append(sites, ExecSite{
				Where:  at,
				Kind:   "tool",
				Name:   step.Tool,
				Detail: summariseExecConfig(step.Config),
				config: step.Config,
			})
		}
		sites = append(sites, execSitesInSteps(step.Parallel, at+".parallel")...)
	}
	return sites
}

// execSitesInToolConfig collects per-tool presets for exec-class tools.
func execSitesInToolConfig(in map[string]map[string]any, where string) []ExecSite {
	ids := make([]string, 0, len(in))
	for id := range in {
		if execClassTools[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	sites := make([]ExecSite, 0, len(ids))
	for _, id := range ids {
		sites = append(sites, ExecSite{
			Where:  where + "." + id,
			Kind:   "tool",
			Name:   id,
			Detail: summariseExecConfig(in[id]),
			config: in[id],
		})
	}
	return sites
}

// execDetailKeys are the config keys worth showing in a consent prompt, in the
// order they read best. `command`/`args` are external-command's argv;
// `scriptFile` and `script` are what the script tool would evaluate.
var execDetailKeys = []string{"command", "args", "scriptFile", "script"}

// summariseExecConfig renders the part of a config that says what would run.
// Values come from an untrusted recipe, so the result is sanitised and clipped
// — it is evidence for a decision, not a faithful echo of the file.
func summariseExecConfig(config map[string]any) string {
	if len(config) == 0 {
		return ""
	}
	var parts []string
	for _, key := range execDetailKeys {
		v, ok := config[key]
		if !ok {
			continue
		}
		parts = append(parts, key+"="+clip(fmt.Sprintf("%v", v), 160))
	}
	if len(parts) == 0 {
		return ""
	}
	return sanitizeForDisplay(strings.Join(parts, " "))
}

// clip truncates s to at most max runes, marking the cut.
func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// sanitizeForDisplay strips control characters from recipe-derived text before
// it is written to a terminal. A consent prompt that renders attacker-authored
// bytes verbatim can be rewritten by those bytes: ANSI escapes reposition the
// cursor, recolour, or erase, which is enough to hide the command being
// approved or to forge a reassuring line above it. Replacing every control
// character (escape included) with a space keeps the summary readable and
// keeps it inside the line it was printed on.
func sanitizeForDisplay(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

// ExecSurfaceDigest returns a stable fingerprint of what a recipe would run.
//
// The digest deliberately covers the exec sites alone, not the whole recipe: a
// decision is about the argv a person approved, so adding a target language or
// a content glob must not invalidate it, while adding an argument to an
// approved command must. Each site contributes its location, kind, name and
// canonical configuration — encoding/json sorts map keys, so a reordered YAML
// mapping digests identically while a changed value does not.
//
// An empty surface digests to the empty string; there is nothing to decide.
func ExecSurfaceDigest(sites []ExecSite) string {
	if len(sites) == 0 {
		return ""
	}
	h := sha256.New()
	for _, s := range sites {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00", s.Where, s.Kind, s.Name)
		// A config that cannot be encoded (it came from YAML, so this should
		// not happen) contributes a marker rather than nothing, so two sites
		// differing only in an unencodable config never collide.
		if enc, err := json.Marshal(s.config); err == nil {
			h.Write(enc)
		} else {
			fmt.Fprintf(h, "unencodable:%v", err)
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
