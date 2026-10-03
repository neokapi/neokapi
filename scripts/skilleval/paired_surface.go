package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const pairedSurfaceTimeout = 45 * time.Second

// PairedSurface is what an agent host reports it would give the model in one
// prepared cell, read through the same executable, arguments, environment and
// working directory the attempt uses, with no model call:
//
//   - Claude: the init event of `claude --print`, with the model endpoint set
//     to a closed local port and a placeholder credential, so the session ends
//     before any request leaves the machine.
//   - Codex: `codex debug prompt-input`, which renders the model-visible input
//     (its skill list and skill roots) without a turn, and `codex mcp list`.
//
// Problems lists every way the cell differs from its condition's arm. A cell
// with a problem is not run.
type PairedSurface struct {
	Host      string   `json:"host"`
	Condition string   `json:"condition"`
	Evidence  string   `json:"evidence"`
	Skills    []string `json:"skills"`
	// BundledSkills are the skills the host installs itself, where the host
	// says so (Codex's system skills).
	BundledSkills []string `json:"bundled_skills,omitempty"`
	SkillRoots    []string `json:"skill_roots,omitempty"`
	MCPServers    []string `json:"mcp_servers"`
	MCPTools      []string `json:"mcp_tools"`
	Plugins       []string `json:"plugins,omitempty"`
	Executables   []string `json:"executables"`
	Problems      []string `json:"problems"`
	Error         string   `json:"error,omitempty"`
}

// probePairedSurface reads one prepared cell's surface and checks it against
// the condition. It needs no subscription and spends nothing.
func probePairedSurface(ctx context.Context, p PairedPrepared) PairedSurface {
	surface := PairedSurface{
		Host: p.Launch.Agent.Host, Condition: p.Launch.Condition,
		Skills: []string{}, MCPServers: []string{}, MCPTools: []string{}, Executables: []string{}, Problems: []string{},
	}
	ctx, cancel := context.WithTimeout(ctx, pairedSurfaceTimeout)
	defer cancel()
	var err error
	switch p.Launch.Agent.Host {
	case "claude":
		err = probePairedClaudeSurface(ctx, p, &surface)
	case "codex":
		err = probePairedCodexSurface(ctx, p, &surface)
	default:
		err = errors.New("unknown host")
	}
	if err != nil {
		surface.Error = err.Error()
	}
	surface.Executables = pairedCellExecutables(filepath.Join(p.Launch.StateDir, "bin"))
	checkPairedSurface(&surface, p.Launch, pairedDeveloperSkills(p.Launch.RepoRoot))
	return surface
}

func probePairedClaudeSurface(ctx context.Context, p PairedPrepared, surface *PairedSurface) error {
	surface.Evidence = "claude init event; model endpoint closed, placeholder credential"
	port, err := pairedClosedPort(ctx)
	if err != nil {
		return err
	}
	env := []string{}
	for _, pair := range p.Env {
		key, _, _ := strings.Cut(pair, "=")
		if key == "CLAUDE_CODE_OAUTH_TOKEN" || key == "ANTHROPIC_BASE_URL" {
			continue
		}
		env = append(env, pair)
	}
	env = append(env, "ANTHROPIC_BASE_URL=http://127.0.0.1:"+port, "CLAUDE_CODE_OAUTH_TOKEN=surface-probe-placeholder")
	command := exec.CommandContext(ctx, p.Executable, p.Args...)
	command.Dir = p.Launch.Workspace
	command.Env = env
	command.Stdin = strings.NewReader("Reply with the word ready.\n")
	command.Stderr = io.Discard
	command.WaitDelay = time.Second
	pairedConfigureProcess(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	defer func() {
		_ = pairedStopProcess(command)
		_ = stdout.Close()
		_ = command.Wait()
	}()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		event := map[string]any{}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		if pairedString(event, "type") != "system" || pairedString(event, "subtype") != "init" {
			continue
		}
		surface.Skills = pairedStringList(event["skills"])
		for _, raw := range pairedList(event["mcp_servers"]) {
			server, _ := raw.(map[string]any)
			name := pairedString(server, "name")
			if status := pairedString(server, "status"); status != "connected" {
				name += " (" + status + ")"
			}
			surface.MCPServers = append(surface.MCPServers, name)
		}
		for _, tool := range pairedStringList(event["tools"]) {
			if strings.HasPrefix(tool, "mcp__") {
				surface.MCPTools = append(surface.MCPTools, tool)
			}
		}
		for _, raw := range pairedList(event["plugins"]) {
			plugin, _ := raw.(map[string]any)
			surface.Plugins = append(surface.Plugins, pairedString(plugin, "name")+"@"+pairedString(plugin, "path"))
		}
		slices.Sort(surface.Skills)
		slices.Sort(surface.MCPServers)
		slices.Sort(surface.MCPTools)
		return nil
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return errors.New("claude ended without an init event")
}

var (
	pairedCodexSkillRoot = regexp.MustCompile("(?m)^- `(r\\d+)` = `([^`]+)`$")
	pairedCodexSkill     = regexp.MustCompile(`(?m)^- ([^:\n]+): .*\(file: (r\d+)/[^)]*\)$`)
)

func probePairedCodexSurface(ctx context.Context, p PairedPrepared, surface *PairedSurface) error {
	surface.Evidence = "codex debug prompt-input and codex mcp list --json"
	run := func(args ...string) ([]byte, error) {
		command := exec.CommandContext(ctx, p.Executable, args...)
		command.Dir = p.Launch.Workspace
		command.Env = p.Env
		command.Stderr = io.Discard
		command.WaitDelay = time.Second
		pairedConfigureProcess(command)
		return command.Output()
	}
	input, err := run("debug", "prompt-input", "Reply with the word ready.")
	if err != nil {
		return fmt.Errorf("codex debug prompt-input: %w", err)
	}
	var items []map[string]any
	if err := json.Unmarshal(input, &items); err != nil {
		return fmt.Errorf("codex prompt input: %w", err)
	}
	roots := map[string]string{}
	for _, item := range items {
		for _, text := range pairedStrings(item["content"]) {
			if !strings.Contains(text, "<skills_instructions>") {
				continue
			}
			for _, match := range pairedCodexSkillRoot.FindAllStringSubmatch(text, -1) {
				roots[match[1]] = match[2]
			}
			for _, match := range pairedCodexSkill.FindAllStringSubmatch(text, -1) {
				name, root := strings.TrimSpace(match[1]), roots[match[2]]
				surface.Skills = append(surface.Skills, name)
				surface.SkillRoots = pairedUnique(surface.SkillRoots, root)
				// Codex installs its own system skills into the cell's fresh
				// CODEX_HOME; they belong to the host, whatever a developer
				// also keeps under the same name.
				if pairedWithin(root, filepath.Join(p.Launch.StateDir, "codex", "skills", ".system")) {
					surface.BundledSkills = append(surface.BundledSkills, name)
				}
			}
		}
	}
	listing, err := run("mcp", "list", "--json")
	if err != nil {
		return fmt.Errorf("codex mcp list: %w", err)
	}
	var servers []map[string]any
	if err := json.Unmarshal(listing, &servers); err != nil {
		return fmt.Errorf("codex mcp list: %w", err)
	}
	for _, server := range servers {
		name := pairedString(server, "name")
		if enabled, ok := server["enabled"].(bool); ok && !enabled {
			name += " (disabled)"
		}
		surface.MCPServers = append(surface.MCPServers, name)
	}
	if p.MCPReadiness != nil {
		for _, tool := range p.MCPReadiness.Tools {
			surface.MCPTools = append(surface.MCPTools, "mcp__kapi__"+tool)
		}
	}
	slices.Sort(surface.Skills)
	slices.Sort(surface.MCPServers)
	return nil
}

// checkPairedSurface lists every way a cell's surface differs from its arm:
// the arm's skill and no other kapi skill, the MCP server only in the MCP arm,
// exactly the arm's kapi names on PATH, and nothing of the developer's own.
func checkPairedSurface(surface *PairedSurface, launch PairedLaunch, developer []string) {
	problem := func(format string, args ...any) {
		surface.Problems = append(surface.Problems, fmt.Sprintf(format, args...))
	}
	if surface.Error != "" {
		problem("surface unreadable: %s", surface.Error)
		return
	}
	arm, err := pairedArmFor(launch.Condition)
	if err != nil {
		problem("%v", err)
		return
	}
	for _, skill := range []string{"kapi", pairedFilesAlias} {
		has := slices.Contains(surface.Skills, skill)
		if want := arm.Skill == skill; has != want {
			problem("skill %s visible=%t, want %t", skill, has, want)
		}
	}
	wantServers := []string{}
	if arm.MCP {
		wantServers = []string{"kapi"}
	}
	if !slices.Equal(surface.MCPServers, wantServers) {
		problem("MCP servers %v, want %v", surface.MCPServers, wantServers)
	}
	for _, tool := range surface.MCPTools {
		if !arm.MCP || !strings.HasPrefix(tool, "mcp__kapi__") {
			problem("MCP tool %s outside the MCP arm", tool)
		}
	}
	if arm.MCP {
		for _, tool := range pairedMCPRequiredTools {
			if launch.Agent.Host == "claude" && !slices.Contains(surface.MCPTools, "mcp__kapi__"+tool) {
				problem("MCP tool %s not exposed to the model", tool)
			}
		}
	}
	if !slices.Equal(surface.Executables, arm.Executables) {
		problem("kapi names on PATH %v, want %v", surface.Executables, arm.Executables)
	}
	for _, skill := range surface.Skills {
		if slices.Contains(developer, skill) && !slices.Contains(surface.BundledSkills, skill) {
			problem("the developer's skill %s is visible", skill)
		}
	}
	for _, plugin := range surface.Plugins {
		if !strings.HasSuffix(plugin, "@builtin") {
			problem("plugin %s is not built into the host", plugin)
		}
	}
	for _, root := range surface.SkillRoots {
		if !pairedWithin(root, launch.StateDir) && !pairedWithin(root, launch.Workspace) {
			problem("skill root %s is outside the cell", root)
		}
	}
}

// pairedCellExecutables lists the kapi names in a cell's private PATH.
func pairedCellExecutables(bin string) []string {
	out := []string{}
	entries, err := os.ReadDir(bin)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if pairedKapiExecutable(entry.Name()) {
			out = append(out, entry.Name())
		}
	}
	slices.Sort(out)
	return out
}

// pairedDeveloperSkills names the skills and plugins the developer running the
// study has installed for either host, and the repository's own skills. None
// of them may be visible in a cell.
func pairedDeveloperSkills(repoRoot string) []string {
	var names []string
	home, _ := os.UserHomeDir()
	dirs := []string{}
	if home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".claude", "skills"),
			filepath.Join(home, ".agents", "skills"),
			filepath.Join(home, ".codex", "skills"))
	}
	if repoRoot != "" {
		dirs = append(dirs, filepath.Join(repoRoot, ".claude", "skills"), filepath.Join(repoRoot, ".agents", "skills"))
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".") {
				names = pairedUnique(names, entry.Name())
			}
		}
	}
	if home != "" {
		var installed struct {
			Plugins map[string]any `json:"plugins"`
		}
		if data, err := os.ReadFile(filepath.Join(home, ".claude", "plugins", "installed_plugins.json")); err == nil &&
			json.Unmarshal(data, &installed) == nil {
			for key := range installed.Plugins {
				name, _, _ := strings.Cut(key, "@")
				names = pairedUnique(names, name)
			}
		}
	}
	// The host's own skills are part of the host, wherever a developer also
	// keeps a copy; Codex's system skills are told apart by their root.
	return slices.DeleteFunc(names, func(name string) bool { return name == "kapi" || name == pairedFilesAlias })
}

func pairedWithin(child, parent string) bool {
	if parent == "" {
		return false
	}
	rel, err := filepath.Rel(pairedResolve(parent), pairedResolve(child))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// pairedResolve resolves the symlinks in the longest existing prefix of name,
// so /var and /private/var compare equal on macOS whether or not the rest of
// the path exists yet.
func pairedResolve(name string) string {
	name = filepath.Clean(name)
	rest := ""
	for current := name; ; current = filepath.Dir(current) {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(resolved, rest)
		}
		if parent := filepath.Dir(current); parent == current {
			return name
		}
		rest = filepath.Join(filepath.Base(current), rest)
	}
}

// pairedClosedPort returns a local port nothing listens on.
func pairedClosedPort(ctx context.Context) (string, error) {
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	_ = listener.Close()
	return port, err
}

func pairedList(value any) []any {
	list, _ := value.([]any)
	return list
}

func pairedStringList(value any) []string {
	out := []string{}
	for _, item := range pairedList(value) {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}
