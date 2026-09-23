package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/neokapi/neokapi/core/project"
)

// Wiring a project up for the coding agents that work in it.
//
// A project has a voice, terms and a check gate long before anyone tells an
// assistant they exist. The voice pointer (host/voicepointer.go) says so in
// prose; this says so in the files an agent host reads as configuration: the
// MCP server entry that starts `kapi mcp` for this project, and the short kapi
// skill in the directory the host scans for skills. The skill names four
// habits and leaves everything else to `kapi help <topic>`, which answers from
// the binary the agent is actually running.
//
// Three rules hold for every file written here.
//
// PROJECT SCOPE ONLY. Every path is under the project root. Nothing under the
// user's home directory and nothing machine-wide is read or written, because a
// project is the thing being wired and a person's own configuration is theirs.
//
// A COMMAND, AND NOTHING ELSE. An MCP entry carries the kapi binary, the `mcp`
// verb and the project it answers for. No shell, no environment, no
// credentials: these files are loaded as configuration by a program that runs
// what they say, and they are committed and shared with everyone on the
// project.
//
// AN EXISTING ENTRY IS LEFT ALONE. A config file that already names a server
// called kapi is read and not written, because someone chose what is in it.
// The one exception is an entry exactly as kapi writes it, differing only in
// its tool sets: that entry is kapi's own, and it follows the recipe. The skill
// directory is kapi's own too, so the file the binary ships is refreshed there,
// a file an earlier kapi copied there is removed, and anything else is left in
// place and reported.

// AgentHost names one coding-agent host a project can be wired for.
type AgentHost string

const (
	// AgentHostClaudeCode is Claude Code: `.mcp.json` at the project root, and
	// project skills under `.claude/skills/`.
	AgentHostClaudeCode AgentHost = "claude-code"
	// AgentHostCursor is Cursor: `.cursor/mcp.json`.
	AgentHostCursor AgentHost = "cursor"
	// AgentHostVSCode is Visual Studio Code: `.vscode/mcp.json`, whose servers
	// sit under `servers` rather than `mcpServers`.
	AgentHostVSCode AgentHost = "vscode"
	// AgentHostCodex is Codex: `.codex/config.toml`, the repository's own layer
	// of the configuration Codex reads, whose servers sit under `mcp_servers`.
	// Codex loads that layer for a repository the person has trusted, so the
	// entry starts answering the first time they open the project there.
	AgentHostCodex AgentHost = "codex"
	// AgentHostAgents is the cross-client skills convention, `.agents/skills/`,
	// which several hosts scan alongside their own directory.
	AgentHostAgents AgentHost = "agents"
)

// agentHostOrder is every host kapi knows, in the order a result lists them.
var agentHostOrder = []AgentHost{
	AgentHostClaudeCode, AgentHostCursor, AgentHostVSCode, AgentHostCodex, AgentHostAgents,
}

// AgentHosts returns every host kapi can wire a project for.
func AgentHosts() []AgentHost {
	out := make([]AgentHost, len(agentHostOrder))
	copy(out, agentHostOrder)
	return out
}

// agentHostDirs are the directories whose presence at a project root says the
// host is already in use here. Claude Code is absent on purpose: it is wired
// whether or not the project has met it, which is what makes a fresh `kapi
// init` enough on its own.
var agentHostDirs = map[AgentHost]string{
	AgentHostCursor: ".cursor",
	AgentHostVSCode: ".vscode",
	AgentHostCodex:  ".codex",
	AgentHostAgents: ".agents",
}

// AgentWiringAction is what writing one file did.
type AgentWiringAction string

const (
	// AgentWiringCreated: the file did not exist and now holds kapi's entry.
	AgentWiringCreated AgentWiringAction = "created"
	// AgentWiringUpdated: an existing file gained kapi's entry, or a file the
	// skill ships was refreshed.
	AgentWiringUpdated AgentWiringAction = "updated"
	// AgentWiringUnchanged: the file already held exactly this.
	AgentWiringUnchanged AgentWiringAction = "unchanged"
	// AgentWiringKept: the file already names a server called kapi, so it was
	// read and left as it is.
	AgentWiringKept AgentWiringAction = "kept"
)

// AgentWiringKind says what an entry is, because the two answer different
// questions for whoever reads the output: a server an agent host starts, or
// guidance it loads.
type AgentWiringKind string

const (
	// AgentWiringMCP is an MCP configuration file.
	AgentWiringMCP AgentWiringKind = "mcp"
	// AgentWiringSkill is one skill's directory.
	AgentWiringSkill AgentWiringKind = "skill"
)

// AgentWiringFile is one file, or one skill directory, the wiring touched.
type AgentWiringFile struct {
	// Kind says whether this is an MCP configuration file or a skill.
	Kind AgentWiringKind `json:"kind"`
	// Host is the agent host this file is read by.
	Host AgentHost `json:"host"`
	// Path is project-relative and slash-separated, so it reads the same on
	// every machine.
	Path string `json:"path"`
	// Action is what happened to it.
	Action AgentWiringAction `json:"action"`
	// Detail says what is in it, for a line a person reads: the server command
	// for a config file, the file count for a skill directory.
	Detail string `json:"detail,omitempty"`
	// Removed lists, for a skill directory, the files an earlier kapi copied
	// there that this run removed, relative to the skill directory.
	Removed []string `json:"removed,omitempty"`
	// Kept lists, for a skill directory, the files in it that kapi did not
	// write and left in place, relative to the skill directory.
	Kept []string `json:"kept,omitempty"`
}

// AgentWiringResult reports what WriteAgentWiring did.
type AgentWiringResult struct {
	// Hosts are the hosts that were wired, in agentHostOrder.
	Hosts []AgentHost `json:"hosts,omitempty"`
	// Files lists every file and skill directory touched, in the order they
	// were written.
	Files []AgentWiringFile `json:"files,omitempty"`
}

// AgentWiringOptions configures WriteAgentWiring.
type AgentWiringOptions struct {
	// Root is the project root. Every path written is under it.
	Root string
	// Hosts are the hosts to wire. Empty writes nothing at all.
	Hosts []AgentHost
	// Recipe is the project's recipe file. Only its name is used, because the
	// entry that carries it is committed and read relative to Root. Empty
	// means the conventional name.
	Recipe string
	// Skills is the skill tree to copy into each host's skills directory: one
	// directory per skill, each holding a SKILL.md. nil writes no skill, which
	// is what a caller with no embedded copy passes.
	Skills fs.FS
	// Retired reports whether a file found in a skill directory is one an
	// earlier kapi copied there, judged by its content. Such a file is removed;
	// any other file the binary does not ship is kept and reported. nil keeps
	// every such file.
	Retired func(body []byte) bool
	// Tools are the `kapi mcp --tools` sets the entry names. Empty writes no
	// flag, which serves the default set.
	Tools []string
}

// ErrUnknownAgentHost reports a host name kapi does not wire.
var ErrUnknownAgentHost = errors.New("unknown agent host")

// agentHostNone is the spelling that opts out, and agentHostAll the one that
// asks for every host kapi knows.
const (
	agentHostNone = "none"
	agentHostAll  = "all"
)

// ParseAgentHosts reads the `--agents` value into the hosts it names.
//
// An empty value asks for detection (DetectAgentHosts), which is what a caller
// that passed no flag gets; explicit reports which of the two happened, so the
// caller does not have to compare the result with the default to find out.
func ParseAgentHosts(spec string) (hosts []AgentHost, explicit bool, err error) {
	spec = strings.TrimSpace(spec)
	switch spec {
	case "":
		return nil, false, nil
	case agentHostNone:
		return nil, true, nil
	case agentHostAll:
		return AgentHosts(), true, nil
	}

	known := map[AgentHost]bool{}
	for _, h := range agentHostOrder {
		known[h] = true
	}
	seen := map[AgentHost]bool{}
	for part := range strings.SplitSeq(spec, ",") {
		name := AgentHost(strings.ToLower(strings.TrimSpace(part)))
		if name == "" {
			continue
		}
		if !known[name] {
			return nil, false, fmt.Errorf("%w %q: kapi wires %s, or %s for every one of them, or %s",
				ErrUnknownAgentHost, name, agentHostNames(), agentHostAll, agentHostNone)
		}
		seen[name] = true
	}
	for _, h := range agentHostOrder {
		if seen[h] {
			hosts = append(hosts, h)
		}
	}
	return hosts, true, nil
}

// agentHostNames renders the host names for a message.
func agentHostNames() string {
	names := make([]string, 0, len(agentHostOrder))
	for _, h := range agentHostOrder {
		names = append(names, string(h))
	}
	return strings.Join(names, ", ")
}

// DetectAgentHosts reports the hosts a project at root is wired for when the
// caller names none: Claude Code, plus every other host that already keeps a
// directory here.
//
// Claude Code is unconditional because its project MCP file sits at the root
// and it reads project skills from a directory kapi creates, so there is no
// prior directory to detect and a project that has never been opened in it
// would otherwise get nothing. The others are wired where they are in use,
// since writing `.vscode/` into a repository whose author does not use VS Code
// adds a directory nobody asked for.
func DetectAgentHosts(root string) []AgentHost {
	hosts := []AgentHost{AgentHostClaudeCode}
	for _, h := range agentHostOrder {
		dir, ok := agentHostDirs[h]
		if !ok {
			continue
		}
		if info, err := os.Stat(filepath.Join(root, dir)); err == nil && info.IsDir() {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

// mcpServerName is the key kapi's entry takes in an MCP configuration file.
const mcpServerName = "kapi"

// mcpServerEntry is one stdio MCP server as every host spells it: a type, a
// command and its arguments. Nothing else is written.
type mcpServerEntry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// mcpToolsFlag is the `kapi mcp` flag that names the tool sets served.
const mcpToolsFlag = "--tools"

// kapiMCPEntry is the entry kapi writes for a project: `kapi mcp` for its
// recipe, with the tool sets named when there are any.
func kapiMCPEntry(recipe string, tools []string) mcpServerEntry {
	args := []string{"mcp", "--project", recipe}
	if len(tools) > 0 {
		args = append(args, mcpToolsFlag, strings.Join(tools, ","))
	}
	return mcpServerEntry{Type: "stdio", Command: "kapi", Args: args}
}

// isKapiWrittenEntry reports whether held is an entry exactly as kapi writes
// it for this recipe, whatever tool sets it names. Such an entry is refreshed
// to the current one; any other entry called kapi is someone's choice and is
// kept.
func isKapiWrittenEntry(held mcpServerEntry, recipe string) bool {
	if held.Command != "kapi" || (held.Type != "" && held.Type != "stdio") {
		return false
	}
	base := []string{"mcp", "--project", recipe}
	if len(held.Args) < len(base) || !slices.Equal(held.Args[:len(base)], base) {
		return false
	}
	rest := held.Args[len(base):]
	return len(rest) == 0 || (len(rest) == 2 && rest[0] == mcpToolsFlag && rest[1] != "")
}

// recipeOf returns the recipe an entry of kapi's shape names, so a refresh
// compares against the entry kapi would write for that same recipe.
func recipeOf(entry mcpServerEntry) string {
	if len(entry.Args) >= 3 {
		return entry.Args[2]
	}
	return ""
}

// mcpConfigFile is one host's MCP configuration: where it lives under the
// project root, and the key its servers sit under. Claude Code and Cursor read
// `mcpServers`; VS Code reads `servers`.
type mcpConfigFile struct {
	path       string
	serversKey string
}

var agentMCPConfigs = map[AgentHost]mcpConfigFile{
	AgentHostClaudeCode: {path: ".mcp.json", serversKey: "mcpServers"},
	AgentHostCursor:     {path: ".cursor/mcp.json", serversKey: "mcpServers"},
	AgentHostVSCode:     {path: ".vscode/mcp.json", serversKey: "servers"},
}

// agentSkillDirs are the directories each host scans for project skills.
var agentSkillDirs = map[AgentHost]string{
	AgentHostClaudeCode: ".claude/skills",
	AgentHostAgents:     ".agents/skills",
}

// WriteAgentWiring puts the project's kapi wiring into the files each named
// host reads, and reports what it wrote.
//
// It is idempotent: a second run over an unchanged project writes nothing and
// reports every file as unchanged or kept.
func WriteAgentWiring(opts AgentWiringOptions) (*AgentWiringResult, error) {
	if opts.Root == "" {
		return nil, errors.New("agent wiring: name the project root")
	}
	// The recipe's own name, and never a path to it. The entry is committed
	// and shared with everyone on the project, so a directory that resolves on
	// one machine has no business in it; the recipe sits at the project root,
	// which is the directory every one of these files is read relative to.
	recipe := filepath.Base(opts.Recipe)
	if opts.Recipe == "" {
		recipe = project.RecipeFileName
	}
	entry := kapiMCPEntry(recipe, opts.Tools)

	res := &AgentWiringResult{}
	for _, host := range agentHostOrder {
		if !slices.Contains(opts.Hosts, host) {
			continue
		}
		res.Hosts = append(res.Hosts, host)

		if cfg, ok := agentMCPConfigs[host]; ok {
			file, err := upsertMCPServerEntry(filepath.Join(opts.Root, filepath.FromSlash(cfg.path)), cfg.serversKey, entry)
			if err != nil {
				return nil, err
			}
			file.Kind, file.Host, file.Path = AgentWiringMCP, host, cfg.path
			res.Files = append(res.Files, file)
		}

		if host == AgentHostCodex {
			file, err := upsertCodexMCPServerEntry(filepath.Join(opts.Root, filepath.FromSlash(codexMCPConfigPath)), entry)
			if err != nil {
				return nil, err
			}
			file.Kind, file.Host, file.Path = AgentWiringMCP, host, codexMCPConfigPath
			res.Files = append(res.Files, file)
		}

		dir, ok := agentSkillDirs[host]
		if !ok || opts.Skills == nil {
			continue
		}
		files, err := writeSkillTree(filepath.Join(opts.Root, filepath.FromSlash(dir)), opts.Skills, opts.Retired)
		if err != nil {
			return nil, err
		}
		for i := range files {
			files[i].Kind, files[i].Host = AgentWiringSkill, host
			files[i].Path = path.Join(dir, files[i].Path)
		}
		res.Files = append(res.Files, files...)
	}
	return res, nil
}

// upsertMCPServerEntry adds kapi's server to an MCP configuration file without
// disturbing anything else in it.
//
// The file is decoded as raw JSON members rather than into a struct, so every
// key kapi does not know about survives the round trip. A file that already
// names a server called kapi is left byte for byte as it is: whatever it says,
// someone put it there.
func upsertMCPServerEntry(path, serversKey string, entry mcpServerEntry) (AgentWiringFile, error) {
	out := AgentWiringFile{Detail: describeMCPEntry(entry)}

	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		doc, merr := marshalMCPConfig(map[string]json.RawMessage{}, serversKey, map[string]json.RawMessage{}, entry)
		if merr != nil {
			return out, merr
		}
		if werr := writeProjectFile(path, doc); werr != nil {
			return out, werr
		}
		out.Action = AgentWiringCreated
		return out, nil
	case err != nil:
		return out, fmt.Errorf("read %s: %w", path, err)
	}

	var doc map[string]json.RawMessage
	if jerr := json.Unmarshal(raw, &doc); jerr != nil {
		return out, fmt.Errorf("%s holds JSON kapi could not read, so it was left alone: %w", path, jerr)
	}
	servers := map[string]json.RawMessage{}
	if held, ok := doc[serversKey]; ok {
		if jerr := json.Unmarshal(held, &servers); jerr != nil {
			return out, fmt.Errorf("%s holds a %s that is not an object, so it was left alone: %w", path, serversKey, jerr)
		}
	}
	if raw, held := servers[mcpServerName]; held {
		var current mcpServerEntry
		if json.Unmarshal(raw, &current) != nil || !isKapiWrittenEntry(current, recipeOf(entry)) {
			out.Action = AgentWiringKept
			out.Detail = "already names a server called " + mcpServerName
			return out, nil
		}
		if slices.Equal(current.Args, entry.Args) && current.Type == entry.Type {
			out.Action = AgentWiringUnchanged
			return out, nil
		}
	}

	updated, merr := marshalMCPConfig(doc, serversKey, servers, entry)
	if merr != nil {
		return out, merr
	}
	if werr := writeProjectFile(path, updated); werr != nil {
		return out, werr
	}
	out.Action = AgentWiringUpdated
	return out, nil
}

// marshalMCPConfig renders the document with kapi's server added under
// serversKey.
func marshalMCPConfig(doc map[string]json.RawMessage, serversKey string, servers map[string]json.RawMessage, entry mcpServerEntry) ([]byte, error) {
	encoded, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("encode the kapi MCP server entry: %w", err)
	}
	servers[mcpServerName] = encoded
	block, err := json.Marshal(servers)
	if err != nil {
		return nil, fmt.Errorf("encode the MCP server list: %w", err)
	}
	doc[serversKey] = block
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode the MCP configuration: %w", err)
	}
	return append(out, '\n'), nil
}

// codexMCPConfigPath is the repository's own layer of the configuration Codex
// reads, and codexMCPServersTable the table its servers sit in.
const (
	codexMCPConfigPath   = ".codex/config.toml"
	codexMCPServersTable = "mcp_servers"
)

// upsertCodexMCPServerEntry adds kapi's server to the project's Codex
// configuration.
//
// The file is TOML, and it is a person's to edit: it carries their sandbox,
// hook and model settings for this repository beside its servers. So the entry
// is appended as text and the rest of the file is never rewritten, which leaves
// comments, ordering and spacing exactly as they were. Reading it back as TOML
// answers the one question that has to be settled before writing, which is
// whether a server called kapi is already there.
func upsertCodexMCPServerEntry(path string, entry mcpServerEntry) (AgentWiringFile, error) {
	out := AgentWiringFile{Detail: describeMCPEntry(entry)}
	block := codexMCPServerBlock(entry)

	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if werr := writeProjectFile(path, []byte(block)); werr != nil {
			return out, werr
		}
		out.Action = AgentWiringCreated
		return out, nil
	case err != nil:
		return out, fmt.Errorf("read %s: %w", path, err)
	}

	var doc map[string]any
	if terr := toml.Unmarshal(raw, &doc); terr != nil {
		return out, fmt.Errorf("%s holds TOML kapi could not read, so it was left alone: %w", path, terr)
	}
	if servers, ok := doc[codexMCPServersTable].(map[string]any); ok {
		if table, held := servers[mcpServerName]; held {
			current, isKapi := codexEntry(table)
			old := codexMCPServerBlock(current)
			switch {
			case !isKapi || !isKapiWrittenEntry(current, recipeOf(entry)) || !strings.Contains(string(raw), old):
				out.Action = AgentWiringKept
				out.Detail = "already names a server called " + mcpServerName
			case old == block:
				out.Action = AgentWiringUnchanged
			default:
				if werr := writeProjectFile(path, []byte(strings.Replace(string(raw), old, block, 1))); werr != nil {
					return out, werr
				}
				out.Action = AgentWiringUpdated
			}
			return out, nil
		}
	}

	held := string(raw)
	if held != "" && !strings.HasSuffix(held, "\n") {
		held += "\n"
	}
	if held != "" {
		held += "\n"
	}
	if werr := writeProjectFile(path, []byte(held+block)); werr != nil {
		return out, werr
	}
	out.Action = AgentWiringUpdated
	return out, nil
}

// codexEntry reads a Codex server table holding a command and its arguments
// and nothing else, the shape kapi writes.
func codexEntry(table any) (mcpServerEntry, bool) {
	fields, ok := table.(map[string]any)
	if !ok || len(fields) != 2 {
		return mcpServerEntry{}, false
	}
	command, ok := fields["command"].(string)
	if !ok {
		return mcpServerEntry{}, false
	}
	list, ok := fields["args"].([]any)
	if !ok {
		return mcpServerEntry{}, false
	}
	entry := mcpServerEntry{Type: "stdio", Command: command}
	for _, a := range list {
		s, ok := a.(string)
		if !ok {
			return mcpServerEntry{}, false
		}
		entry.Args = append(entry.Args, s)
	}
	return entry, true
}

// codexMCPServerBlock renders kapi's server as the table Codex reads. Codex
// takes a server with a `command` as a stdio server, so the entry carries the
// command and its arguments and nothing else.
func codexMCPServerBlock(entry mcpServerEntry) string {
	args := make([]string, 0, len(entry.Args))
	for _, arg := range entry.Args {
		args = append(args, strconv.Quote(arg))
	}
	return fmt.Sprintf("[%s.%s]\ncommand = %s\nargs = [%s]\n",
		codexMCPServersTable, mcpServerName, strconv.Quote(entry.Command), strings.Join(args, ", "))
}

// describeMCPEntry renders the entry as the command line it launches, which is
// the whole of what the file asks a host to run.
func describeMCPEntry(entry mcpServerEntry) string {
	return strings.TrimSpace(entry.Command + " " + strings.Join(entry.Args, " "))
}

// writeSkillTree copies the skill tree into a host's skills directory and
// reports one entry per skill.
//
// The directory named for a skill belongs to that skill, so the files the
// binary ships are written there whatever was in them: they are a copy of what
// this binary documents, and a copy that lags the binary names commands the
// binary may no longer have. A file an earlier kapi copied there and this one
// no longer ships is removed; any other file is left where it is.
func writeSkillTree(dir string, tree fs.FS, retired func([]byte) bool) ([]AgentWiringFile, error) {
	names, err := fs.ReadDir(tree, ".")
	if err != nil {
		return nil, fmt.Errorf("read the embedded skill tree: %w", err)
	}
	var out []AgentWiringFile
	for _, name := range names {
		if !name.IsDir() {
			continue
		}
		sub, serr := fs.Sub(tree, name.Name())
		if serr != nil {
			return nil, fmt.Errorf("read the embedded skill %s: %w", name.Name(), serr)
		}
		file, werr := writeOneSkill(filepath.Join(dir, name.Name()), sub, retired)
		if werr != nil {
			return nil, werr
		}
		file.Path = name.Name()
		out = append(out, file)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// writeOneSkill copies one skill's files into dir, clears out what an earlier
// kapi left there, and reports what changed.
func writeOneSkill(dir string, skill fs.FS, retired func([]byte) bool) (AgentWiringFile, error) {
	var (
		out     AgentWiringFile
		count   int
		created int
		updated int
	)
	shipped := map[string]bool{}
	err := fs.WalkDir(skill, ".", func(rel string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		shipped[rel] = true
		body, rerr := fs.ReadFile(skill, rel)
		if rerr != nil {
			return fmt.Errorf("read the embedded skill file %s: %w", rel, rerr)
		}
		count++
		target := filepath.Join(dir, filepath.FromSlash(rel))
		held, herr := os.ReadFile(target)
		switch {
		case errors.Is(herr, fs.ErrNotExist):
			created++
		case herr != nil:
			return fmt.Errorf("read %s: %w", target, herr)
		case string(held) == string(body):
			return nil
		default:
			updated++
		}
		return writeProjectFile(target, body)
	})
	if err != nil {
		return out, err
	}

	out.Removed, out.Kept, err = sweepSkillDir(dir, shipped, retired)
	if err != nil {
		return out, err
	}

	out.Detail = fmt.Sprintf("%d files", count)
	if count == 1 {
		out.Detail = "1 file"
	}
	switch {
	case created > 0:
		out.Action = AgentWiringCreated
	case updated > 0 || len(out.Removed) > 0:
		out.Action = AgentWiringUpdated
	default:
		out.Action = AgentWiringUnchanged
	}
	return out, nil
}

// sweepSkillDir removes the files under dir that an earlier kapi copied there
// and this binary does not ship, and lists the ones it leaves because kapi did
// not write them. Directories the sweep empties are removed too.
func sweepSkillDir(dir string, shipped map[string]bool, retired func([]byte) bool) (removed, kept []string, err error) {
	var dirs []string
	werr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." {
				dirs = append(dirs, p)
			}
			return nil
		}
		if shipped[rel] {
			return nil
		}
		body, rerr := os.ReadFile(p)
		if rerr != nil {
			return fmt.Errorf("read %s: %w", p, rerr)
		}
		if retired != nil && retired(body) {
			if rmErr := os.Remove(p); rmErr != nil {
				return fmt.Errorf("remove %s: %w", p, rmErr)
			}
			removed = append(removed, rel)
			return nil
		}
		kept = append(kept, rel)
		return nil
	})
	if werr != nil {
		return nil, nil, werr
	}
	// Deepest first, so a directory is tried after everything under it.
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		if entries, rerr := os.ReadDir(d); rerr == nil && len(entries) == 0 {
			_ = os.Remove(d)
		}
	}
	return removed, kept, nil
}

// writeProjectFile writes one file, creating the directories above it.
func writeProjectFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
