package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/skills"
)

// What an assistant reads about a project.
//
// kapi wires a project for the coding assistants that work in it: an MCP
// server entry in each host's project configuration and the short kapi skill
// in the directory the host scans (host/agentwiring.go), plus the rules files
// an assistant loads as it starts (host/rulesfiles.go). The Assistant section
// reads both and writes them the way `kapi init` does, so a person with no
// terminal can connect an assistant and see what it is told.
//
// Claude Desktop is the one host configured outside the project, in a file of
// the person's own. The host layer never writes there; this file does, on the
// person's request, keeping every other key in it.

// agentWiringTimeout caps one read or write of a project's wiring and rules
// files.
const agentWiringTimeout = 30 * time.Second

// agentHostClaudeDesktop is Claude Desktop, which reads the MCP servers of the
// person's own configuration rather than a project's.
const agentHostClaudeDesktop = "claude-desktop"

// agentHostLabels names each host the Assistant section lists, in its order.
var agentHostLabels = []struct {
	host  string
	label string
}{
	{string(host.AgentHostClaudeCode), "Claude Code"},
	{string(host.AgentHostCursor), "Cursor"},
	{string(host.AgentHostVSCode), "VS Code"},
	{string(host.AgentHostCodex), "Codex"},
	{agentHostClaudeDesktop, "Claude Desktop"},
}

// AgentHostStatus is one assistant the project can be connected to.
type AgentHostStatus struct {
	// Host is claude-code, cursor, vscode, codex or claude-desktop, and
	// Label what to call it on screen.
	Host  string `json:"host"`
	Label string `json:"label"`
	// Detected reports that the assistant is in use here: the project keeps
	// its folder (Claude Code always counts, as kapi init counts it), or, for
	// Claude Desktop, its configuration folder exists on this machine.
	Detected bool `json:"detected"`
	// Wired reports that the assistant's configuration starts kapi's MCP
	// server for this project.
	Wired bool `json:"wired"`
	// Files are the files that carry the connection, relative to the
	// project, for a project host that is connected. For Claude Desktop it is
	// the absolute path of the person's own configuration file whether or not
	// it is connected yet, so a confirmation can name the file Connect writes.
	Files []string `json:"files,omitempty"`
}

// RulesFileView is one rules file kapi writes for a project.
type RulesFileView struct {
	// Path is the file, relative to the project root.
	Path string `json:"path"`
	// Folder is the folder whose rules it states, empty for the root.
	Folder string `json:"folder"`
	// Section is kapi's section of the file as it would be written now.
	Section string `json:"section"`
	// Exists reports that the file is on disk.
	Exists bool `json:"exists"`
}

// checkoutOf is a tab's recipe and project root, false for a tab with no
// checkout.
func (a *App) checkoutOf(tabID string) (*openProject, string, string, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, "", "", fmt.Errorf("project tab %q not found", tabID)
	}
	if op.contextOnly {
		return op, "", "", nil
	}
	a.mu.RLock()
	path := op.Path
	a.mu.RUnlock()
	root, ok := projectRoot(op)
	if !ok {
		return op, "", "", nil
	}
	return op, path, root, nil
}

// AgentHosts lists the assistants a tab's project can be connected to, and
// whether each is. A tab holding the context alone lists none: the
// connection lives in a checkout's files.
func (a *App) AgentHosts(tabID string) ([]AgentHostStatus, error) {
	_, recipe, root, err := a.checkoutOf(tabID)
	if err != nil {
		return nil, err
	}
	out := []AgentHostStatus{}
	if root == "" {
		return out, nil
	}
	detected := host.DetectAgentHosts(root)
	wiring := host.ReadAgentWiring(root, recipe, skills.Wiring())
	for _, h := range agentHostLabels {
		if h.host == agentHostClaudeDesktop {
			if st, ok := claudeDesktopStatus(recipe); ok {
				out = append(out, st)
			}
			continue
		}
		w := wiring[host.AgentHost(h.host)]
		out = append(out, AgentHostStatus{
			Host:     h.host,
			Label:    h.label,
			Detected: slices.Contains(detected, host.AgentHost(h.host)),
			Wired:    w.Wired,
			Files:    w.Files,
		})
	}
	return out, nil
}

// ConnectAgentHost connects one assistant to a tab's project and reports the
// files it wrote. A project host gets the files `kapi init` writes for it;
// Claude Desktop gets kapi's server in the person's own configuration, with
// every other key kept.
func (a *App) ConnectAgentHost(tabID, hostName string) (*host.AgentWiringResult, error) {
	op, recipe, root, err := a.checkoutOf(tabID)
	if err != nil {
		return nil, err
	}
	if root == "" {
		return nil, errors.New("this tab holds the project's context and no checkout of it; open the project's folder to connect an assistant")
	}
	var targets []string
	a.mu.RLock()
	if op.Project != nil {
		for _, l := range op.Project.Defaults.TargetLanguages {
			targets = append(targets, string(l))
		}
	}
	a.mu.RUnlock()
	tools := host.MCPToolSets(targets)

	if hostName == agentHostClaudeDesktop {
		return connectClaudeDesktop(recipe, tools)
	}
	hosts, _, err := host.ParseAgentHosts(hostName)
	if err != nil {
		return nil, err
	}
	if len(hosts) != 1 {
		return nil, fmt.Errorf("name one assistant to connect, not %q", hostName)
	}
	return host.WriteAgentWiring(host.AgentWiringOptions{
		Root:    root,
		Hosts:   hosts,
		Recipe:  filepath.Base(recipe),
		Skills:  skills.Wiring(),
		Retired: skills.Retired,
		Tools:   tools,
	})
}

// RulesFiles lists the rules files kapi writes for a tab's project, each with
// the section it would write now and whether the file is on disk. A tab
// holding the context alone lists none.
func (a *App) RulesFiles(tabID string) ([]RulesFileView, error) {
	_, recipe, root, err := a.checkoutOf(tabID)
	if err != nil {
		return nil, err
	}
	out := []RulesFileView{}
	if root == "" {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentWiringTimeout)
	defer cancel()
	plan, err := a.hostEngine().PlanRulesFiles(ctx, recipe)
	if err != nil {
		return nil, err
	}
	for _, f := range plan {
		out = append(out, RulesFileView(f))
	}
	return out, nil
}

// WriteRulesFiles writes a tab's project's rules files, as `kapi context sync
// --files-only` does, and reports what it wrote.
func (a *App) WriteRulesFiles(tabID string) (*host.RulesFilesResult, error) {
	_, recipe, root, err := a.checkoutOf(tabID)
	if err != nil {
		return nil, err
	}
	if root == "" {
		return nil, errors.New("this tab holds the project's context and no checkout of it; open the project's folder to write its rules files")
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentWiringTimeout)
	defer cancel()
	return a.hostEngine().WriteRulesFiles(ctx, recipe)
}

// claudeDesktopConfigDir is the folder Claude Desktop keeps its configuration
// in, on the platforms whose location its documentation states: macOS and
// Windows (modelcontextprotocol.io, "Connect to local MCP servers"). False
// elsewhere, where the row is left out rather than guessed at. A variable so a
// test can point it at a folder of its own.
var claudeDesktopConfigDir = func() (string, bool) {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		return filepath.Join(home, "Library", "Application Support", "Claude"), true
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", false
		}
		return filepath.Join(appData, "Claude"), true
	}
	return "", false
}

// claudeDesktopConfigFile is the configuration file inside that folder.
const claudeDesktopConfigFile = "claude_desktop_config.json"

// claudeDesktopServersKey is the key its MCP servers sit under.
const claudeDesktopServersKey = "mcpServers"

// claudeDesktopStatus reads whether Claude Desktop starts kapi's server for
// the recipe, and names the file it reads. False when the platform's location
// is not known.
func claudeDesktopStatus(recipe string) (AgentHostStatus, bool) {
	dir, ok := claudeDesktopConfigDir()
	if !ok {
		return AgentHostStatus{}, false
	}
	path := filepath.Join(dir, claudeDesktopConfigFile)
	st := AgentHostStatus{Host: agentHostClaudeDesktop, Label: "Claude Desktop", Files: []string{path}}
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		st.Detected = true
	}
	abs, err := filepath.Abs(recipe)
	if err != nil {
		return st, true
	}
	if entry, ok := readClaudeDesktopEntry(path); ok && host.ServesRecipe(entry.Args, abs, abs) {
		st.Wired = true
	}
	return st, true
}

// claudeDesktopEntry is a stdio server as Claude Desktop's configuration
// spells it.
type claudeDesktopEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// readClaudeDesktopEntry reads the server called kapi, false when the file
// names none or cannot be read.
func readClaudeDesktopEntry(path string) (claudeDesktopEntry, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return claudeDesktopEntry{}, false
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(raw, &doc) != nil {
		return claudeDesktopEntry{}, false
	}
	var servers map[string]json.RawMessage
	if json.Unmarshal(doc[claudeDesktopServersKey], &servers) != nil {
		return claudeDesktopEntry{}, false
	}
	held, ok := servers["kapi"]
	if !ok {
		return claudeDesktopEntry{}, false
	}
	var entry claudeDesktopEntry
	if json.Unmarshal(held, &entry) != nil {
		return claudeDesktopEntry{}, false
	}
	return entry, true
}

// connectClaudeDesktop writes kapi's server for the recipe into Claude
// Desktop's configuration.
//
// Claude Desktop starts the server outside any project and with the
// environment of an application rather than a shell, so the entry names the
// kapi binary and the recipe by absolute path. An entry called kapi that
// starts `kapi mcp` for another project is the desktop's own and is moved to
// this one; an entry someone wrote by hand (other arguments, an environment)
// is kept as it is.
func connectClaudeDesktop(recipe string, tools []string) (*host.AgentWiringResult, error) {
	dir, ok := claudeDesktopConfigDir()
	if !ok {
		return nil, fmt.Errorf("kapi knows where Claude Desktop keeps its configuration on macOS and Windows, not on %s", runtime.GOOS)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("this machine has no Claude Desktop configuration folder at %s: open Claude Desktop once, then connect again", dir)
	}
	abs, err := filepath.Abs(recipe)
	if err != nil {
		return nil, err
	}
	bin, err := kapiBinary()
	if err != nil {
		return nil, err
	}
	args := []string{"mcp", "--project", abs}
	if len(tools) > 0 {
		args = append(args, "--tools", strings.Join(tools, ","))
	}
	entry := claudeDesktopEntry{Command: bin, Args: args}
	path := filepath.Join(dir, claudeDesktopConfigFile)
	file := host.AgentWiringFile{
		Kind:   host.AgentWiringMCP,
		Host:   agentHostClaudeDesktop,
		Path:   path,
		Detail: strings.Join(append([]string{bin}, args...), " "),
	}
	res := &host.AgentWiringResult{Hosts: []host.AgentHost{agentHostClaudeDesktop}}

	doc := map[string]json.RawMessage{}
	servers := map[string]json.RawMessage{}
	mode := fs.FileMode(0o600)
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		file.Action = host.AgentWiringCreated
	case err != nil:
		return nil, fmt.Errorf("read %s: %w", path, err)
	default:
		if info, serr := os.Stat(path); serr == nil {
			mode = info.Mode().Perm()
		}
		if jerr := json.Unmarshal(raw, &doc); jerr != nil {
			return nil, fmt.Errorf("%s holds JSON kapi could not read, so it was left alone: %w", path, jerr)
		}
		if held, ok := doc[claudeDesktopServersKey]; ok {
			if jerr := json.Unmarshal(held, &servers); jerr != nil {
				return nil, fmt.Errorf("%s holds a %s that is not an object, so it was left alone: %w", path, claudeDesktopServersKey, jerr)
			}
		}
		file.Action = host.AgentWiringUpdated
		if held, ok := servers["kapi"]; ok {
			current, mine := desktopWrittenEntry(held)
			switch {
			case !mine:
				file.Action, file.Detail = host.AgentWiringKept, "already names a server called kapi"
				res.Files = append(res.Files, file)
				return res, nil
			case current.Command == entry.Command && slices.Equal(current.Args, entry.Args):
				file.Action = host.AgentWiringUnchanged
				res.Files = append(res.Files, file)
				return res, nil
			case len(current.Args) > 2 && current.Args[2] != abs:
				file.Detail += " (in place of the entry for " + current.Args[2] + ")"
			}
		}
	}

	encoded, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	servers["kapi"] = encoded
	block, err := json.Marshal(servers)
	if err != nil {
		return nil, err
	}
	doc[claudeDesktopServersKey] = block
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeFileAtomic(path, append(out, '\n'), mode); err != nil {
		return nil, fmt.Errorf("write %s: %w", path, err)
	}
	res.Files = append(res.Files, file)
	return res, nil
}

// desktopWrittenEntry reports whether a server entry is one this app writes:
// a command and its arguments and nothing else, starting `kapi mcp --project`
// on an absolute recipe path.
func desktopWrittenEntry(raw json.RawMessage) (claudeDesktopEntry, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return claudeDesktopEntry{}, false
	}
	for k := range fields {
		if k != "command" && k != "args" {
			return claudeDesktopEntry{}, false
		}
	}
	var entry claudeDesktopEntry
	if json.Unmarshal(raw, &entry) != nil {
		return claudeDesktopEntry{}, false
	}
	name := strings.TrimSuffix(filepath.Base(entry.Command), ".exe")
	if name != "kapi" || len(entry.Args) < 3 || entry.Args[0] != "mcp" || entry.Args[1] != "--project" || !filepath.IsAbs(entry.Args[2]) {
		return claudeDesktopEntry{}, false
	}
	rest := entry.Args[3:]
	if len(rest) != 0 && (len(rest) != 2 || rest[0] != "--tools") {
		return claudeDesktopEntry{}, false
	}
	return entry, true
}

// kapiBinaryCandidates are where an install puts the kapi command when the
// application's PATH does not reach it: an app started from the Finder sees
// the system's PATH, which leaves out Homebrew. A variable so a test can name
// its own.
var kapiBinaryCandidates = []string{"/opt/homebrew/bin/kapi", "/usr/local/bin/kapi"}

// kapiBinary is the absolute path of the kapi command Claude Desktop is to
// start: the one on PATH, else the first an install put in a known place.
func kapiBinary() (string, error) {
	if p, err := exec.LookPath("kapi"); err == nil {
		if abs, aerr := filepath.Abs(p); aerr == nil {
			return abs, nil
		}
	}
	for _, p := range kapiBinaryCandidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("the kapi command Claude Desktop starts is not installed: install it with `brew install neokapi/tap/kapi-cli`, then connect again")
}

// writeFileAtomic replaces a file through a temporary file beside it, so a
// reader never sees half of it.
func writeFileAtomic(path string, data []byte, mode fs.FileMode) error {
	tmp := path + ".kapi-tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
