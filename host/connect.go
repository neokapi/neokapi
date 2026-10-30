package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/skills"
)

// `kapi connect`: put an assistant on a project.
//
// ConnectHosts writes the rules files (host/rulesfiles.go) and each
// assistant's MCP entry and skill (host/agentwiring.go), the recipe first when
// the folder has none, through the services `kapi init` uses. It reports each
// file as created, updated or kept. The Stop hook comes from the kapi Claude
// Code plugin, so it is reported as a step to take until the project's Claude
// Code settings enable the plugin.

// ConnectOptions configures ConnectHosts.
type ConnectOptions struct {
	// Dir is the folder to connect when the command resolves no project: the
	// recipe is written there. Empty is the working directory.
	Dir string
	// Hosts names the assistants to connect. Empty connects the ones found in
	// the folder (DetectAgentHosts).
	Hosts []AgentHost
	// DryRun reports what connecting would write and writes none of the files
	// it reports.
	DryRun bool
}

// ConnectAction is what connecting did to one file.
type ConnectAction string

const (
	// ConnectCreated: the file did not exist and now does.
	ConnectCreated ConnectAction = "created"
	// ConnectUpdated: kapi's part of an existing file was added or rewritten.
	ConnectUpdated ConnectAction = "updated"
	// ConnectKept: the file already held what kapi writes, or holds a server
	// called kapi that someone else wrote, and was left as it is.
	ConnectKept ConnectAction = "kept"
	// ConnectRemoved: kapi's section was taken out of a rules file in a folder
	// with no rules of its own, and the rest of the file kept.
	ConnectRemoved ConnectAction = "removed"
	// ConnectDeleted: such a file held nothing but kapi's section and was
	// deleted.
	ConnectDeleted ConnectAction = "deleted"
	// ConnectPending: a step a person takes outside the project's files.
	ConnectPending ConnectAction = "pending"
)

// ConnectFileKind says what a file is to the assistant that reads it.
type ConnectFileKind string

const (
	// ConnectRecipe is the project's recipe.
	ConnectRecipe ConnectFileKind = "recipe"
	// ConnectRules is a rules file, AGENTS.md or CLAUDE.md.
	ConnectRules ConnectFileKind = "rules"
	// ConnectMCP is a host's MCP configuration file.
	ConnectMCP ConnectFileKind = "mcp"
	// ConnectSkill is a skill directory.
	ConnectSkill ConnectFileKind = "skill"
	// ConnectHook is the hook that runs kapi check when the assistant
	// finishes.
	ConnectHook ConnectFileKind = "hook"
)

// ConnectFile is one file connecting wrote or considered.
type ConnectFile struct {
	// Path is project-relative and slash-separated. Empty for a pending step.
	Path   string          `json:"path,omitempty"`
	Kind   ConnectFileKind `json:"kind"`
	Action ConnectAction   `json:"action"`
	// Detail adds what a person reading the line needs: the step to take for
	// a pending hook, or why an entry was kept.
	Detail string `json:"detail,omitempty"`
}

// ConnectHost is one assistant and what connecting did for it.
type ConnectHost struct {
	Host  AgentHost `json:"host"`
	Label string    `json:"label"`
	// Connected reports that the assistant's files are in place after this
	// run, or would be after a dry run.
	Connected bool `json:"connected"`
	// Reason says why an assistant was not connected.
	Reason string `json:"reason,omitempty"`
	// Reads names what the assistant gets: rules, MCP server, skill, and the
	// check when it finishes.
	Reads []string `json:"reads,omitempty"`
	// Files are the assistant's own files: its MCP configuration, its skill
	// directory, its hook.
	Files []ConnectFile `json:"files,omitempty"`
}

// ConnectResult reports what ConnectHosts did.
type ConnectResult struct {
	// Root is the project root, and Recipe the recipe file in it.
	Root   string `json:"root"`
	Recipe string `json:"recipe"`
	DryRun bool   `json:"dry_run"`
	// Default reports that no assistant was found in the folder, so Claude
	// Code was connected, as kapi connects it in every project.
	Default bool `json:"default,omitempty"`
	// Hosts are the assistants, connected or not, in the order kapi lists
	// them.
	Hosts []ConnectHost `json:"hosts"`
	// Files are the project's files every assistant reads: the recipe and
	// the rules files.
	Files []ConnectFile `json:"files"`
}

// connectHostLabels names each assistant connect lists, in its order. The
// cross-client skills directory is listed only where a project keeps it or a
// person names it.
var connectHostLabels = []struct {
	host  AgentHost
	label string
}{
	{AgentHostClaudeCode, "Claude Code"},
	{AgentHostCursor, "Cursor"},
	{AgentHostVSCode, "VS Code"},
	{AgentHostCodex, "Codex"},
	{AgentHostAgents, "Agent skills"},
}

// connectHostNames are the names `kapi connect <host>` takes, each to the
// assistant it connects.
var connectHostNames = map[string]AgentHost{
	"claude":      AgentHostClaudeCode,
	"claude-code": AgentHostClaudeCode,
	"cursor":      AgentHostCursor,
	"vscode":      AgentHostVSCode,
	"codex":       AgentHostCodex,
	"agents":      AgentHostAgents,
}

// ConnectHostNames lists the names `kapi connect <host>` takes, in the order
// a message names them.
func ConnectHostNames() []string {
	return []string{"claude", "cursor", "vscode", "codex", "agents"}
}

// ErrConnectUnsupported reports an assistant kapi connect does not support.
var ErrConnectUnsupported = errors.New("not supported yet")

// claudeDesktopName is Claude Desktop, which reads its servers from the
// person's own configuration rather than from the project.
const claudeDesktopName = "claude-desktop"

// ParseConnectHost reads the assistant a person named.
func ParseConnectHost(name string) (AgentHost, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if h, ok := connectHostNames[key]; ok {
		return h, nil
	}
	supported := strings.Join(ConnectHostNames(), ", ")
	if key == claudeDesktopName {
		return "", fmt.Errorf("%s: %w: kapi connect writes files inside the project, and Claude Desktop reads its servers from your own configuration; connect it from the Assistant section of Kapi Desktop. kapi connect supports %s",
			name, ErrConnectUnsupported, supported)
	}
	return "", fmt.Errorf("%s: %w; kapi connect supports %s", name, ErrConnectUnsupported, supported)
}

// claudePluginSteps is how a person installs the kapi Claude Code plugin,
// whose Stop hook runs the project's check when Claude Code finishes.
const claudePluginSteps = "in Claude Code, /plugin marketplace add neokapi/claude-plugins, then /plugin install kapi@neokapi-plugins"

// claudeSettingsFiles are the project's Claude Code settings files, the
// shared one first.
var claudeSettingsFiles = []string{".claude/settings.json", ".claude/settings.local.json"}

// claudePluginEnabledIn names the project settings file that enables the kapi
// Claude Code plugin, empty when none does.
func claudePluginEnabledIn(root string) string {
	for _, rel := range claudeSettingsFiles {
		if enablesKapiPlugin(filepath.Join(root, filepath.FromSlash(rel))) {
			return rel
		}
	}
	return ""
}

// claudeUserSettings is the person's own Claude Code settings file:
// settings.json in $CLAUDE_CONFIG_DIR when it is set, else in ~/.claude.
// False when neither can be named.
func claudeUserSettings() (string, bool) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json"), true
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	return filepath.Join(home, ".claude", "settings.json"), true
}

// claudePluginEnabledForUser reports whether the person's own Claude Code
// settings enable the kapi plugin. It reads that file and writes nothing.
func claudePluginEnabledForUser() bool {
	path, ok := claudeUserSettings()
	return ok && enablesKapiPlugin(path)
}

// enablesKapiPlugin reports whether a Claude Code settings file enables a
// plugin named kapi, from any marketplace.
func enablesKapiPlugin(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return false
	}
	for name, on := range doc.EnabledPlugins {
		if on && strings.HasPrefix(name, "kapi@") {
			return true
		}
	}
	return false
}

// claudeCodeFound reports whether the folder shows Claude Code in use: its
// project folder, its MCP file or its rules file.
func claudeCodeFound(root string) bool {
	for _, rel := range []string{".claude", ".mcp.json", "CLAUDE.md"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			return true
		}
	}
	return false
}

// ConnectHosts connects the assistants opts names, or the ones found in the
// project, and reports every file it wrote or would write.
//
// The project is the one cmd resolves (-p, KAPI_PROJECT, or the upward walk
// from the working directory). When none resolves, the folder in opts.Dir is
// the project, and its recipe is written as `kapi init` writes it when the
// folder has none. A second run over an unchanged project rewrites nothing
// and reports every file as kept.
func (a *App) ConnectHosts(ctx context.Context, cmd Command, opts ConnectOptions) (*ConnectResult, error) {
	recipe, err := ResolveProjectPath(cmd)
	if err != nil {
		return nil, err
	}
	var root string
	if recipe != "" {
		root = filepath.Dir(recipe)
	} else {
		root, err = connectDir(opts.Dir, opts.DryRun)
		if err != nil {
			return nil, err
		}
		recipe = filepath.Join(root, project.RecipeFileName)
	}
	if err := connectableRecipe(recipe); err != nil {
		return nil, err
	}
	res := &ConnectResult{Root: root, Recipe: recipe, DryRun: opts.DryRun}

	// The recipe first: the MCP entry names it, and the rules files are read
	// from the context it opens.
	recipeFile, targets, haveRecipe, err := a.connectRecipe(root, recipe, opts.DryRun)
	if err != nil {
		return nil, err
	}
	res.Files = append(res.Files, recipeFile)

	hosts, named := opts.Hosts, len(opts.Hosts) > 0
	if !named {
		hosts = DetectAgentHosts(root)
		res.Default = len(hosts) == 1 && !claudeCodeFound(root)
	}

	wiring, err := WriteAgentWiring(AgentWiringOptions{
		Root:    root,
		Hosts:   hosts,
		Recipe:  filepath.Base(recipe),
		Skills:  skills.Wiring(),
		Retired: skills.Retired,
		Tools:   MCPToolSets(targets),
		DryRun:  opts.DryRun,
	})
	if err != nil {
		return nil, err
	}

	for _, h := range connectHostLabels {
		chosen := slices.Contains(hosts, h.host)
		if !chosen && (named || h.host == AgentHostAgents) {
			continue
		}
		entry := ConnectHost{Host: h.host, Label: h.label, Connected: chosen}
		if !chosen {
			entry.Reason = fmt.Sprintf("no %s/ folder here", agentHostDirs[h.host])
			res.Hosts = append(res.Hosts, entry)
			continue
		}
		entry.Reads = []string{"rules"}
		for _, f := range wiring.Files {
			if f.Host != h.host {
				continue
			}
			cf := ConnectFile{Path: f.Path, Kind: ConnectFileKind(f.Kind), Action: connectWiringAction(f.Action)}
			if f.Action == AgentWiringKept {
				cf.Detail = f.Detail
			}
			if f.Kind == AgentWiringMCP && h.host == AgentHostCodex && f.Action != AgentWiringKept {
				cf.Detail = "read once you trust this repository in Codex"
			}
			entry.Files = append(entry.Files, cf)
			switch f.Kind {
			case AgentWiringMCP:
				entry.Reads = appendOnce(entry.Reads, "MCP server")
			case AgentWiringSkill:
				entry.Reads = appendOnce(entry.Reads, "skill")
			}
		}
		if h.host == AgentHostClaudeCode {
			hook := ConnectFile{Kind: ConnectHook, Action: ConnectPending, Detail: claudePluginSteps}
			switch rel := claudePluginEnabledIn(root); {
			case rel != "":
				hook = ConnectFile{Path: rel, Kind: ConnectHook, Action: ConnectKept, Detail: "the kapi plugin is enabled"}
			case claudePluginEnabledForUser():
				hook = ConnectFile{Kind: ConnectHook, Action: ConnectKept, Detail: "enabled for your user"}
			}
			if hook.Action == ConnectKept {
				entry.Reads = append(entry.Reads, "kapi check when it finishes")
			}
			entry.Files = append(entry.Files, hook)
		}
		res.Hosts = append(res.Hosts, entry)
	}

	rules, err := a.connectRulesFiles(ctx, root, recipe, haveRecipe, opts.DryRun)
	if err != nil {
		return nil, err
	}
	res.Files = append(res.Files, rules...)
	return res, nil
}

// connectableRecipe refuses a recipe connect cannot write: one that does not
// exist under a name other than kapi.yaml, which kapi init never writes, or
// one whose folder does not exist. It runs before anything is written.
func connectableRecipe(recipe string) error {
	if _, err := os.Stat(recipe); err == nil {
		return nil
	}
	dir := filepath.Dir(recipe)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("no folder at %s: name an existing folder, or a recipe in one", dir)
	}
	if filepath.Base(recipe) != project.RecipeFileName {
		return fmt.Errorf("no recipe at %s: kapi connect writes a new recipe as %s, so name the folder or a recipe that exists",
			recipe, project.RecipeFileName)
	}
	return nil
}

// connectDir is the folder a project with no recipe is connected in. A dry
// run creates no folder.
func connectDir(dir string, dry bool) (string, error) {
	if dir == "" {
		return os.Getwd()
	}
	if dry {
		return filepath.Abs(dir)
	}
	return ResolveDir(dir)
}

// connectRecipe writes the recipe when the folder has none, as `kapi init`
// writes it, and returns the target languages it declares. haveRecipe is
// false only for a dry run over a folder with no recipe, which leaves the
// recipe unwritten.
func (a *App) connectRecipe(root, recipe string, dry bool) (file ConnectFile, targets []string, haveRecipe bool, err error) {
	file = ConnectFile{Path: relSlash(root, recipe), Kind: ConnectRecipe}
	if _, serr := os.Stat(recipe); serr == nil {
		proj, lerr := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
		if lerr != nil {
			return file, nil, false, fmt.Errorf("load project: %w", lerr)
		}
		for _, t := range proj.Defaults.TargetLanguages {
			targets = append(targets, string(t))
		}
		file.Action = ConnectKept
		return file, targets, true, nil
	}
	file.Action = ConnectCreated
	if dry {
		return file, nil, false, nil
	}
	res, ierr := InitProject(root, InitOptions{Formats: a.FormatReg})
	if ierr != nil {
		return file, nil, false, ierr
	}
	return file, res.TargetLanguages, true, nil
}

// connectRulesFiles writes the project's rules files, or reports what a write
// would do. A dry run over a folder with no recipe reads the plan of a new
// project, whose context is empty: the root's section with no rules, and no
// folder with rules of its own, so a section kapi wrote into a sub-folder's
// file is reported as the real run takes it out.
func (a *App) connectRulesFiles(ctx context.Context, root, recipe string, haveRecipe, dry bool) ([]ConnectFile, error) {
	var (
		res *RulesFilesResult
		err error
	)
	switch {
	case !haveRecipe:
		res, err = writeRulesPlan(projectWrites{dry: true}, root, rulesPlan{})
	case dry:
		res, err = a.PreviewRulesFiles(ctx, recipe)
	default:
		res, err = a.WriteRulesFiles(ctx, recipe)
	}
	if err != nil {
		return nil, fmt.Errorf("rules files: %w", err)
	}
	out := make([]ConnectFile, 0, len(res.Files))
	for _, f := range res.Files {
		out = append(out, ConnectFile{Path: f.Path, Kind: ConnectRules, Action: connectRulesAction(f.Action)})
	}
	return out, nil
}

// connectWiringAction maps what the agent wiring did onto connect's actions.
func connectWiringAction(a AgentWiringAction) ConnectAction {
	switch a {
	case AgentWiringCreated:
		return ConnectCreated
	case AgentWiringUpdated:
		return ConnectUpdated
	default:
		return ConnectKept
	}
}

// connectRulesAction maps what the rules write did onto connect's actions.
func connectRulesAction(a RulesFileAction) ConnectAction {
	switch a {
	case RulesFileCreated:
		return ConnectCreated
	case RulesFileUpdated:
		return ConnectUpdated
	case RulesFileRemoved:
		return ConnectRemoved
	case RulesFileDeleted:
		return ConnectDeleted
	default:
		return ConnectKept
	}
}

// appendOnce appends s unless list holds it.
func appendOnce(list []string, s string) []string {
	if slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// connectHint is the line that ends the report.
const connectHint = "See what an assistant reads for a file: kapi context <path> --explain"

// FormatText prints one block per assistant, then the project's files, one
// line per file with what happened to it.
func (r *ConnectResult) FormatText(w io.Writer) error {
	var b strings.Builder
	if r.Default {
		if r.DryRun {
			b.WriteString("No assistant found here; kapi would connect Claude Code.\n")
		} else {
			b.WriteString("No assistant found here; Claude Code is connected.\n")
		}
	}
	width := 0
	for _, h := range r.Hosts {
		for _, f := range h.Files {
			width = max(width, len(connectFileLabel(f)))
		}
	}
	for _, f := range r.Files {
		width = max(width, len(connectFileLabel(f)))
	}
	width = max(width, 12)

	for _, h := range r.Hosts {
		state := "connected"
		if r.DryRun {
			state = "would connect"
		}
		if !h.Connected {
			fmt.Fprintf(&b, "%-13s not connected: %s (kapi connect %s adds it)\n", h.Label, h.Reason, connectCommandName(h.Host))
			continue
		}
		summary := strings.Join(h.Reads, ", ")
		if r.DryRun && nothingToWrite(h.Files) {
			state, summary = "connected", "nothing to write"
		}
		fmt.Fprintf(&b, "%-13s %s · %s\n", h.Label, state, summary)
		for _, f := range h.Files {
			fmt.Fprintf(&b, "  %-*s  %s\n", width, connectFileLabel(f), connectActionText(f, r.DryRun))
		}
	}
	if len(r.Files) > 0 {
		b.WriteString("\nProject\n")
		for _, f := range r.Files {
			fmt.Fprintf(&b, "  %-*s  %s\n", width, connectFileLabel(f), connectActionText(f, r.DryRun))
		}
	}
	b.WriteString("\n" + connectHint + "\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// nothingToWrite reports that a dry run would leave every one of an
// assistant's files as it is.
func nothingToWrite(files []ConnectFile) bool {
	for _, f := range files {
		if f.Action != ConnectKept && f.Action != ConnectPending {
			return false
		}
	}
	return true
}

// connectCommandName is the name `kapi connect` takes for a host.
func connectCommandName(h AgentHost) string {
	if h == AgentHostClaudeCode {
		return "claude"
	}
	return string(h)
}

// connectFileLabel is the first column of a file's line.
func connectFileLabel(f ConnectFile) string {
	if f.Kind == ConnectHook {
		return "Stop hook"
	}
	return f.Path
}

// connectActionText is what happened to a file, as its line says it.
func connectActionText(f ConnectFile, dry bool) string {
	text := string(f.Action)
	if dry {
		switch f.Action {
		case ConnectCreated:
			text = "would create"
		case ConnectUpdated:
			text = "would update"
		case ConnectRemoved:
			text = "would remove the section"
		case ConnectDeleted:
			text = "would delete"
		}
	} else if f.Action == ConnectRemoved {
		text = "removed the section"
	}
	switch {
	case f.Kind == ConnectHook && f.Action == ConnectKept && f.Path == "":
		return text + " (" + f.Detail + ")"
	case f.Kind == ConnectHook && f.Action == ConnectKept:
		return text + " (" + f.Path + ": " + f.Detail + ")"
	case f.Detail != "":
		return text + ": " + f.Detail
	case f.Kind == ConnectRules && f.Action == ConnectUpdated:
		return text + " (Writing rules section)"
	}
	return text
}
