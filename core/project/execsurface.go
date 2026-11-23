package project

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/partjoin"
)

// execClassTools are the built-in tool IDs whose whole job is to run code the
// recipe names, rather than to transform content with code kapi ships:
// `external-command` spawns a subprocess from its own config, and `script`
// evaluates recipe-supplied JavaScript (and can read an arbitrary file through
// script_file).
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

// ExecSite is one place that arms code execution: in a recipe, a flow step, a
// per-tool config preset that would supply such a step's argv, or a content
// item bound to a format that shells out; and outside it, the formatter a
// project configures for comment edits (FormatterExecSite).
type ExecSite struct {
	// Where locates the site in the recipe, in the recipe's own vocabulary
	// (e.g. `flows.default.steps[2]`, `defaults.locales.nb.tools`).
	Where string
	// Kind is "tool", "format" or "formatter".
	Kind string
	// Name is the tool ID or format name.
	Name string
	// Detail summarises what would run (the command line, or the head of the
	// script), so a person can answer the prompt on the facts rather than on
	// the tool's name. It is derived from untrusted recipe text and is
	// sanitised for terminal display; see sanitizeForDisplay.
	Detail string
	// config is the canonical form of the site's configuration, used for the
	// digest so that changing an argv invalidates a previous decision.
	config map[string]any
}

// FormatterExecSite is the exec site of a formatter a project configures, which
// a comment edit runs on the project's files. The file that selected the
// formatter, configFile, locates the site, and command is what would run under
// name. The digest also holds facts, such as hashes of the executable and of
// configFile, so a change to either gives a different digest.
func FormatterExecSite(configFile, name string, command []string, facts map[string]any) ExecSite {
	return ExecSite{
		Where:  sanitizeForDisplay(configFile),
		Kind:   "formatter",
		Name:   sanitizeForDisplay(name),
		Detail: sanitizeForDisplay(clip(strings.Join(command, " "), 320)),
		config: map[string]any{"command": command},
	}.WithFacts(facts)
}

// WithFacts returns the site with facts added to what its digest covers:
// hashes of the files it runs, the project it belongs to. A recipe's sites
// and a formatter's carry their facts the same way, so one digest covers
// both the configuration and what it names.
func (s ExecSite) WithFacts(facts map[string]any) ExecSite {
	config := make(map[string]any, len(s.config)+len(facts))
	maps.Copy(config, s.config)
	maps.Copy(config, facts)
	s.config = config
	return s
}

// NamedPaths lists what a site's configuration names that may be a file the
// command runs: external-command's command when it is a path rather than a
// name looked up on PATH, each of its arguments (an interpreter's script
// among them), and the script tool's script_file. Whether one is a file in
// the project is for the caller to decide.
func (s ExecSite) NamedPaths() []string {
	var out []string
	switch s.Name {
	case "external-command":
		if c, ok := s.config["command"].(string); ok && strings.ContainsAny(c, `/\`) {
			out = append(out, c)
		}
		switch args := s.config["args"].(type) {
		case []any:
			for _, a := range args {
				if v, ok := a.(string); ok && v != "" {
					out = append(out, v)
				}
			}
		case []string:
			for _, v := range args {
				if v != "" {
					out = append(out, v)
				}
			}
		}
	case "script":
		if f, ok := s.config["script_file"].(string); ok && f != "" {
			out = append(out, f)
		}
	}
	return out
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
//
// ExecSurface reads the recipe alone. ExecSurfaceIn adds what the project's
// files declare beside it, which is what a decision must cover.
func ExecSurface(p *KapiProject) []ExecSite {
	if p == nil {
		return nil
	}
	var sites []ExecSite

	// Flows, in name order, since Go map iteration is randomised.
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

// ExecSurfaceIn enumerates everything the project at root can make kapi
// execute: the recipe's own sites (ExecSurface) and the steps of every flow
// file in its `flows_dir:`, each located by the file's path relative to root.
// ExecSurfaceSources lists the places this walk reads, and a test holds the
// recipe's fields to it, so a new place a step can be declared joins the walk.
// A flow file that does not load is skipped: it runs nothing.
func ExecSurfaceIn(p *KapiProject, root string) []ExecSite {
	sites := ExecSurface(p)
	if p == nil {
		return sites
	}
	for _, def := range ListDirFlows(p.FlowsDirIn(root)) {
		if def.Err != nil || def.Spec == nil {
			continue
		}
		where := def.Path
		if rel, err := filepath.Rel(root, def.Path); err == nil {
			where = filepath.ToSlash(rel)
		}
		sites = append(sites, execSitesInSteps(def.Spec.Steps, where+" steps")...)
		sites = append(sites, execSitesInSteps(def.Spec.SourceTransforms, where+" source_transforms")...)
	}
	return sites
}

// ExecSurfaceSources names the recipe fields ExecSurfaceIn reads steps,
// presets and formats from. Every other field of KapiProject is listed in
// ExecSurfaceNonSources with why it declares nothing kapi executes.
var ExecSurfaceSources = []string{"Flows", "FlowsDir", "Defaults", "Collections"}

// ExecSurfaceNonSources names the recipe fields that declare no step, preset
// or format, so the walk passes them over.
var ExecSurfaceNonSources = map[string]string{
	"Version":          "the recipe format version",
	"ID":               "the project's identity",
	"Name":             "a label",
	"Plugins":          "which installed plugins the project needs; a plugin is installed code, decided at install",
	"Preset":           "a built-in preset name, which brings kapi's own configuration",
	"Profiles":         "voice, terms and concept bindings, which run nothing",
	"Places":           "where content is published, read by venue plugins",
	"Axes":             "context coordinates",
	"ShipGate":         "a check gate over findings",
	"ShipGates":        "check gates over findings",
	"Gates":            "check gates over findings",
	"EstablishedGate":  "a check gate over findings",
	"EstablishedGates": "check gates over findings",
	"SourceGate":       "a check gate over findings",
	"Requires":         "the kapi and plugin versions the recipe needs",
	"Context":          "where the context is kept, decided by the context-store approval",
	"Extras":           "keys a registered recipe extension decodes, such as a venue's settings; an extension is installed code",
}

// ExecSurfaceDefaultsSources names the `defaults:` fields ExecSurface reads:
// the tool presets that would arm a step.
var ExecSurfaceDefaultsSources = []string{"Tools", "Locales"}

// inlineCodeFlags are the arguments that hand a program code as a string
// rather than a file: a shell's `-c`, an interpreter's `-e` or `-c` (node,
// python, perl, ruby), `--eval`, PowerShell's `-Command`, cmd's `/c`. They
// count for any program, a program kapi does not know included: an unknown
// program with one of them is reported as running inline code, because a
// step wrongly reported as unchecked costs a clearer question, and one
// wrongly passed over in silence reads as checked.
var inlineCodeFlags = map[string]bool{
	"-c": true, "-e": true, "-E": true, "--eval": true, "--exec": true,
	"-command": true, "/c": true,
}

// inlineCodeForms are a known program's own ways of taking code inline, read
// beside inlineCodeFlags: node's `-p`, deno's `eval`, php's `-r`, ruby's
// `-r` (which loads a library the recipe does not name).
var inlineCodeForms = map[string]map[string]bool{
	"node":   {"-p": true, "--print": true},
	"nodejs": {"-p": true, "--print": true},
	"bun":    {"-p": true, "--print": true},
	"deno":   {"eval": true},
	"php":    {"-r": true},
	"ruby":   {"-r": true},
}

// InlineCode reports how the site hands a program code as a string, such as
// "sh -c" or "node -e", or "" when it does not. Code given inline can load
// any file, and kapi cannot see which, so a question about the step says so.
// It reads the shape of the invocation: a program, then a flag that takes
// code.
func (s ExecSite) InlineCode() string {
	if s.Name != "external-command" {
		return ""
	}
	command, _ := s.config["command"].(string)
	words := []string{command}
	switch args := s.config["args"].(type) {
	case []any:
		for _, a := range args {
			if v, ok := a.(string); ok {
				words = append(words, v)
			}
		}
	case []string:
		words = append(words, args...)
	}
	// The program is the command, or, behind env, the first word after it
	// that is not a flag.
	program := ""
	for i, w := range words {
		if i == 0 || program == "env" && !strings.HasPrefix(w, "-") {
			program = strings.TrimSuffix(strings.ToLower(filepath.Base(w)), ".exe")
			continue
		}
		flag, _, _ := strings.Cut(w, "=")
		lower := strings.ToLower(flag)
		caseFree := lower == "/c" || lower == "-command"
		if inlineCodeFlags[flag] || caseFree || inlineCodeForms[program][flag] {
			return program + " " + flag
		}
	}
	return ""
}

// kapiOwnedKeys are configuration keys kapi sets on a recipe's step itself.
var kapiOwnedKeys = []string{"working_dir", "invoke_dir"}

// KapiOwnedKeys lists the keys in the site's configuration that kapi sets on
// a recipe's step itself, and a recipe may not.
func (s ExecSite) KapiOwnedKeys() []string {
	var out []string
	for _, k := range kapiOwnedKeys {
		if _, ok := s.config[k]; ok {
			out = append(out, k)
		}
	}
	return out
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
// `script_file` and `script` are what the script tool would evaluate.
var execDetailKeys = []string{"command", "args", "script_file", "script"}

// summariseExecConfig renders the part of a config that says what would run.
// Values come from an untrusted recipe, so the result is sanitised and clipped:
// it is evidence for a decision, not a faithful echo of the file.
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
// canonical configuration. encoding/json sorts map keys, so a reordered YAML
// mapping digests identically while a changed value does not.
//
// An empty surface digests to the empty string; there is nothing to decide.
func ExecSurfaceDigest(sites []ExecSite) string {
	if len(sites) == 0 {
		return ""
	}
	h := sha256.New()
	for _, s := range sites {
		config := ""
		if enc, err := json.Marshal(s.config); err == nil {
			config = string(enc)
		} else {
			config = fmt.Sprintf("unencodable:%v", err)
		}
		// A config that cannot be encoded (it came from YAML, so this should
		// not happen) contributes a marker rather than nothing, so two sites
		// differing only in an unencodable config never collide.
		partjoin.WriteNUL(h, s.Where, s.Kind, s.Name, config)
	}
	return hex.EncodeToString(h.Sum(nil))
}
