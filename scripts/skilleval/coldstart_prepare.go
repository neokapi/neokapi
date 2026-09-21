package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ColdStartPaths names every directory one cell owns. The repository sits
// under the cell root and kapi's roots sit beside it, which is where a person's
// workspace sits relative to their checkout.
type ColdStartPaths struct {
	Root    string `json:"root"`
	Repo    string `json:"repo"`
	Data    string `json:"data"`
	Config  string `json:"config"`
	Cache   string `json:"cache"`
	Plugins string `json:"plugins"`
	Home    string `json:"home"`
	Bin     string `json:"bin"`
	State   string `json:"state"`
}

func coldStartPaths(root string) ColdStartPaths {
	return ColdStartPaths{
		Root:    root,
		Repo:    filepath.Join(root, coldStartRepoName),
		Data:    filepath.Join(root, "kapi-data"),
		Config:  filepath.Join(root, "kapi-config"),
		Cache:   filepath.Join(root, "kapi-cache"),
		Plugins: filepath.Join(root, "kapi-plugins"),
		Home:    filepath.Join(root, "home"),
		Bin:     filepath.Join(root, "bin"),
		State:   filepath.Join(root, "state"),
	}
}

// ColdStartWiring records what the shipped `kapi init` wrote, and every place
// the harness had to complete or change it. A gap here is a product finding,
// reported rather than repaired.
type ColdStartWiring struct {
	InitArgs   []string `json:"init_args"`
	InitOutput string   `json:"init_output"`
	Files      []string `json:"files"`
	Harness    []string `json:"harness"`
	// ContextAnswer is what the shipped surface says about a location in a
	// project that has recorded nothing, read once during preparation. It is the
	// baseline the first session starts from.
	ContextAnswer string `json:"context_answer"`
}

// ColdStartServer is the MCP server an agent host starts from the wiring, as
// the server itself answers.
type ColdStartServer struct {
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Resolved  string   `json:"resolved"`
	UnderTest bool     `json:"under_test"`
	Name      string   `json:"server_name"`
	Version   string   `json:"server_version"`
	Tools     []string `json:"tools"`
	GrowTools []string `json:"grow_tools"`
}

// ColdStartIsolation holds what the drill establishes by measurement.
type ColdStartIsolation struct {
	// Ancestors are discoverable project or assistant files above the fixture.
	// Empty is the passing result.
	Ancestors []string `json:"ancestors"`
	// WorkspaceFiles are the workspace files found under this cell's data root,
	// which is where kapi wrote instead of the person's own.
	WorkspaceFiles []string `json:"workspace_files"`
	// UserDataRoot is the per-user data root this machine resolves without
	// KAPI_DATA_DIR, and UserDataWitness a digest of its listing.
	UserDataRoot    string `json:"user_data_root"`
	UserDataWitness string `json:"user_data_witness"`
	// EnvNames are the variable names handed to the agent host, never a value.
	EnvNames []string `json:"env_names"`
}

// ColdStartPrepared is one session's launch description, prepared without any
// model call. Blockers prohibit inference.
type ColdStartPrepared struct {
	Session ColdStartSession `json:"session"`
	Paths   ColdStartPaths   `json:"paths"`
	Wiring  ColdStartWiring  `json:"wiring"`
	Server  *ColdStartServer `json:"server,omitempty"`
	// Codex is what Codex itself says about this cell's wiring, on a Codex cell.
	Codex      *ColdStartCodexWiring `json:"codex,omitempty"`
	Isolation  ColdStartIsolation    `json:"isolation"`
	Executable string                `json:"executable"`
	Args       []string              `json:"args"`
	Version    string                `json:"version"`
	AuthMode   string                `json:"auth_mode"`
	KapiBin    string                `json:"kapi_bin"`
	Timeout    time.Duration         `json:"timeout"`
	MaxTurns   int                   `json:"max_turns"`
	Blockers   []string              `json:"blockers"`
	Notes      []string              `json:"notes"`
	Env        []string              `json:"-"`
	Prompt     string                `json:"-"`
}

// coldStartEnv is the environment every process in a cell runs under: the agent
// host, the kapi it starts over MCP, and the kapi the agent runs from a shell.
//
// KAPI_NO_PROJECT is absent on purpose. The fixture is generated outside this
// repository with nothing discoverable above it, so the upward walk finds the
// fixture's own recipe and that discovery is part of what the drill measures.
// Every other variable of the isolation contract is set, to this cell's own
// throwaway directories.
func coldStartEnv(paths ColdStartPaths) []string {
	return []string{
		"PATH=" + paths.Bin,
		"HOME=" + paths.Home,
		"SHELL=/bin/sh",
		"LANG=en_US.UTF-8",
		"TERM=dumb",
		"NO_COLOR=1",
		"KAPI_CONFIG_DIR=" + paths.Config,
		"KAPI_DATA_DIR=" + paths.Data,
		"XDG_DATA_HOME=" + filepath.Join(paths.Root, "xdg-data"),
		"XDG_CACHE_HOME=" + paths.Cache,
		"KAPI_PLUGINS_DIR_ONLY=1",
		"KAPI_PLUGINS_DIR=" + paths.Plugins,
		"CLAUDE_CONFIG_DIR=" + filepath.Join(paths.State, "claude"),
		"CODEX_HOME=" + filepath.Join(paths.State, "codex"),
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"ENABLE_CLAUDEAI_MCP_SERVERS=false",
		"CLAUDE_CODE_DISABLE_AUTO_MEMORY=1",
		"DISABLE_AUTOUPDATER=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_SYSTEM=" + os.DevNull,
		"GIT_AUTHOR_NAME=Fernwell docs",
		"GIT_AUTHOR_EMAIL=docs@fernwell.invalid",
		"GIT_COMMITTER_NAME=Fernwell docs",
		"GIT_COMMITTER_EMAIL=docs@fernwell.invalid",
	}
}

func coldStartEnvNames(env []string) []string {
	names := make([]string, 0, len(env))
	for _, pair := range env {
		key, _, _ := strings.Cut(pair, "=")
		names = append(names, key)
	}
	sort.Strings(names)
	return names
}

// coldStartTools are the ordinary editing tools a private PATH keeps, so the
// agent works the way it would in any repository without inheriting the
// developer's own executables.
var coldStartTools = []string{
	"bash", "sh", "cat", "cp", "mv", "rm", "mkdir", "ls", "pwd", "find", "sed",
	"awk", "grep", "head", "tail", "wc", "sort", "uniq", "cut", "tr", "diff",
	"git", "rg", "python3", "jq", "file", "which", "env", "printf", "touch",
	"date", "node", "perl", "xargs", "tee", "basename", "dirname",
}

// coldStartKapiNames are the commands the shipped skill drives, each of them
// this checkout's binary under a different name.
var coldStartKapiNames = []string{"kapi", "kcat", "kgrep", "ksed", "kdiff"}

// prepareColdStartCell brings one cell to the state a first agent session finds:
// the fixture repository under git, `kapi init` run over it by the binary under
// test, the content mapping its scaffold leaves for the person, and a private
// PATH whose kapi is this checkout's build.
//
// It is idempotent. A cell that has been prepared is reused, which is how
// session two runs over what session one recorded.
func prepareColdStartCell(ctx context.Context, opts ColdStartOptions, cell ColdStartCell) (ColdStartPaths, ColdStartWiring, error) {
	paths := coldStartPaths(filepath.Join(opts.SandboxRoot, cell.ID))
	var wiring ColdStartWiring
	readyPath := filepath.Join(paths.Root, "wiring.json")
	if err := readPairedJSON(readyPath, &wiring); err == nil {
		return paths, wiring, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return paths, wiring, err
	}
	for _, dir := range []string{paths.Root, paths.Data, paths.Config, paths.Cache, paths.Plugins, paths.Home, paths.Bin, paths.State, filepath.Join(paths.State, "claude"), filepath.Join(paths.State, "codex")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return paths, wiring, err
		}
	}
	if err := coldStartToolPath(paths, opts.KapiBin); err != nil {
		return paths, wiring, err
	}
	if err := materializeColdStartRepo(paths.Repo); err != nil {
		return paths, wiring, err
	}
	if err := coldStartGitInit(ctx, paths); err != nil {
		return paths, wiring, err
	}
	wiring.InitArgs = []string{"init", "--agents", opts.Manifest.Agents}
	out, err := coldStartRunKapi(ctx, paths, wiring.InitArgs...)
	wiring.InitOutput = out
	if err != nil {
		return paths, wiring, fmt.Errorf("kapi init: %w", err)
	}
	wiring.Files, err = coldStartWiringFiles(paths.Repo)
	if err != nil {
		return paths, wiring, err
	}
	recipe := filepath.Join(paths.Repo, "kapi.yaml")
	scaffolded, err := os.ReadFile(recipe)
	if err != nil {
		return paths, wiring, err
	}
	mapped, err := coldStartRecipeMapping(scaffolded)
	if err != nil {
		return paths, wiring, err
	}
	if err := os.WriteFile(recipe, mapped, 0o600); err != nil {
		return paths, wiring, err
	}
	wiring.Harness = append(wiring.Harness,
		"kapi.yaml: collections point at the fixture's prose, completing the content mapping the scaffold leaves for the person",
		"kapi.yaml: the scaffold's starter voice pack is removed, so the drill starts from a context that holds nothing")
	if err := coldStartGitCommit(ctx, paths, "Wire the project for kapi"); err != nil {
		return paths, wiring, err
	}
	// One read through the shipped surface, which both records what an empty
	// context answers and opens this cell's workspace, so the isolation check
	// has a workspace file to find.
	wiring.ContextAnswer, err = coldStartRunKapi(ctx, paths, "context", "README.md")
	if err != nil {
		return paths, wiring, fmt.Errorf("read the empty context: %w", err)
	}
	if err := writePairedJSON(readyPath, wiring); err != nil {
		return paths, wiring, err
	}
	return paths, wiring, nil
}

// coldStartToolPath fills the cell's private bin directory. The kapi names come
// first because they are written into it directly: the `.mcp.json` that
// `kapi init` wrote names a bare `kapi`, so what that name resolves to on this
// PATH is what an agent host starts.
func coldStartToolPath(paths ColdStartPaths, kapiBin string) error {
	if kapiBin == "" {
		return errors.New("the cold-start drill needs this checkout's kapi binary; run `make build` first")
	}
	for _, name := range coldStartKapiNames {
		destination := filepath.Join(paths.Bin, name)
		if _, err := os.Lstat(destination); err == nil {
			continue
		}
		if err := os.Symlink(kapiBin, destination); err != nil {
			return err
		}
	}
	for _, name := range coldStartTools {
		source, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		destination := filepath.Join(paths.Bin, name)
		if _, err := os.Lstat(destination); err == nil {
			continue
		}
		if err := os.Symlink(source, destination); err != nil {
			return err
		}
	}
	return nil
}

// coldStartWiringFiles lists what the wiring put in the repository, so the
// record says which hosts `kapi init` reached.
func coldStartWiringFiles(repo string) ([]string, error) {
	candidates := []string{
		".mcp.json", ".cursor/mcp.json", ".vscode/mcp.json", ".codex/config.toml",
		".claude/skills/kapi/SKILL.md", ".agents/skills/kapi/SKILL.md",
		"CLAUDE.md", "AGENTS.md", "kapi.yaml",
	}
	found := []string{}
	for _, name := range candidates {
		if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(name))); err == nil {
			found = append(found, name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return found, nil
}

func coldStartRunKapi(ctx context.Context, paths ColdStartPaths, args ...string) (string, error) {
	command := exec.CommandContext(ctx, filepath.Join(paths.Bin, "kapi"), args...)
	command.Dir = paths.Repo
	command.Env = coldStartEnv(paths)
	out, err := command.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func coldStartRunGit(ctx context.Context, paths ColdStartPaths, args ...string) error {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = paths.Repo
	command.Env = coldStartEnv(paths)
	out, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// coldStartGitInit puts the fixture under version control, because the shipped
// skill names a change by diffing it against HEAD and a repository with no
// history has nothing to name.
func coldStartGitInit(ctx context.Context, paths ColdStartPaths) error {
	if err := coldStartRunGit(ctx, paths, "init", "--initial-branch=main", "--quiet"); err != nil {
		return err
	}
	return coldStartGitCommit(ctx, paths, "The docs as they stand")
}

func coldStartGitCommit(ctx context.Context, paths ColdStartPaths, message string) error {
	if err := coldStartRunGit(ctx, paths, "add", "-A"); err != nil {
		return err
	}
	return coldStartRunGit(ctx, paths, "commit", "--quiet", "--allow-empty", "-m", message)
}

// coldStartMCPEntry reads the server entry out of the `.mcp.json` that
// `kapi init` wrote, so the probe starts what an agent host would start.
func coldStartMCPEntry(repo string) (string, []string, error) {
	data, err := os.ReadFile(filepath.Join(repo, ".mcp.json"))
	if err != nil {
		return "", nil, err
	}
	var doc struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", nil, fmt.Errorf("read the wired MCP configuration: %w", err)
	}
	entry, ok := doc.Servers["kapi"]
	if !ok || entry.Command == "" {
		return "", nil, errors.New("the wired MCP configuration names no kapi server")
	}
	return entry.Command, entry.Args, nil
}

// coldStartGrowToolNames are the write tools the drill measures, and
// check_file is what the fourth habit calls. A server missing any of them
// cannot answer the question the drill asks.
var coldStartGrowToolNames = []string{"context_observe", "context_propose", "context_correct", "context_session_summary", "check_file"}

// probeColdStartServer starts the wired server exactly as an agent host would,
// and records what answers: the binary the wired command resolves to, the
// server's own name and version, and its tool inventory.
func probeColdStartServer(ctx context.Context, paths ColdStartPaths, kapiBin string) (*ColdStartServer, error) {
	command, args, err := coldStartMCPEntry(paths.Repo)
	if err != nil {
		return nil, err
	}
	server := &ColdStartServer{Command: command, Args: args, Tools: []string{}, GrowTools: []string{}}
	resolved, err := coldStartResolveOnPath(command, paths.Bin)
	if err != nil {
		return server, err
	}
	server.Resolved = resolved
	server.UnderTest = resolved == kapiBin
	readiness := PairedMCPReadiness{Capabilities: []string{}, Tools: []string{}, Resources: []string{}, ResourceTemplates: []string{}}
	probe := exec.CommandContext(ctx, resolved, args...)
	probe.Dir = paths.Repo
	probe.Env = coldStartEnv(paths)
	if err := coldStartDiscover(ctx, probe, &readiness); err != nil {
		return server, err
	}
	server.Name, server.Version, server.Tools = readiness.ServerName, readiness.ServerVersion, readiness.Tools
	for _, name := range coldStartGrowToolNames {
		if slices.Contains(readiness.Tools, name) {
			server.GrowTools = append(server.GrowTools, name)
		}
	}
	if len(server.GrowTools) != len(coldStartGrowToolNames) {
		return server, fmt.Errorf("the wired server offers %d of the %d tools the drill measures", len(server.GrowTools), len(coldStartGrowToolNames))
	}
	return server, nil
}

// coldStartResolveOnPath answers what a bare command name resolves to on the
// cell's private PATH, which is the question `.mcp.json` leaves open.
func coldStartResolveOnPath(command, bin string) (string, error) {
	if strings.ContainsRune(command, filepath.Separator) {
		return filepath.EvalSymlinks(command)
	}
	return filepath.EvalSymlinks(filepath.Join(bin, command))
}

// coldStartDiscover runs an initialize and tools/list handshake against an
// already-configured command, with no tools/call and no model anywhere.
func coldStartDiscover(ctx context.Context, command *exec.Cmd, readiness *PairedMCPReadiness) error {
	ctx, cancel := context.WithTimeout(ctx, pairedMCPProbeTimeout)
	defer cancel()
	command.Stderr = nil
	pairedConfigureProcess(command)
	input, err := command.StdinPipe()
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	defer output.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = input.Close(); _ = output.Close() })
	defer stopClose()
	if err := command.Start(); err != nil {
		return fmt.Errorf("start the wired server: %w", err)
	}
	defer func() {
		_ = input.Close()
		_ = pairedStopProcess(command)
		_ = output.Close()
		_ = command.Wait()
	}()
	client := newPairedMCPDiscoveryClient(input, output)
	if err := client.initialize(readiness); err != nil {
		return err
	}
	return client.list("tools/list", "tools", "name", &readiness.Tools)
}

// coldStartIsolation establishes the properties the drill refuses to assume.
func coldStartIsolation(paths ColdStartPaths, env []string) (ColdStartIsolation, error) {
	isolation := ColdStartIsolation{Ancestors: []string{}, WorkspaceFiles: []string{}, EnvNames: coldStartEnvNames(env)}
	ancestors, err := coldStartAncestorFindings(paths.Repo)
	if err != nil {
		return isolation, err
	}
	isolation.Ancestors = ancestors
	workspaces, err := filepath.Glob(filepath.Join(paths.Data, "workspaces", "*", "workspace.db"))
	if err != nil {
		return isolation, err
	}
	for _, path := range workspaces {
		relative, relErr := filepath.Rel(paths.Data, path)
		if relErr != nil {
			return isolation, relErr
		}
		isolation.WorkspaceFiles = append(isolation.WorkspaceFiles, filepath.ToSlash(relative))
	}
	isolation.UserDataRoot = coldStartUserDataRoot()
	isolation.UserDataWitness, err = coldStartWitness(isolation.UserDataRoot)
	return isolation, err
}

// coldStartWitnessChanged compares two witnesses of the person's own data root.
// A difference means something in this phase reached it, which invalidates the
// attempt rather than being noted after the fact.
func coldStartWitnessChanged(before, after string) bool {
	return before != "" && after != "" && before != after
}

// coldStartUserDataRoot is the per-user data root kapi resolves on this machine
// without KAPI_DATA_DIR: the one holding the developer's own terms, voice
// profiles, content memory and recorded decisions.
func coldStartUserDataRoot() string {
	if dir := os.Getenv("KAPI_DATA_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "kapi")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch runtime.GOOS {
	case "windows":
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "kapi")
		}
		return filepath.Join(home, "AppData", "Local", "kapi")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "kapi")
	default:
		return filepath.Join(home, ".local", "share", "kapi")
	}
}

// coldStartWitness digests a directory's shape: each entry's path, size and
// modification time, and nothing of its contents. Comparing the witness taken
// before a phase with the one taken after is how the drill establishes that
// nothing reached the person's own data root.
func coldStartWitness(dir string) (string, error) {
	if dir == "" {
		return "absent", nil
	}
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return "absent", nil
	} else if err != nil {
		return "", err
	}
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) || errors.Is(walkErr, os.ErrPermission) {
				return nil
			}
			return walkErr
		}
		relative, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			if errors.Is(infoErr, os.ErrNotExist) {
				return nil
			}
			return infoErr
		}
		return encoder.Encode(struct {
			Name string
			Size int64
			Mod  int64
		}{Name: filepath.ToSlash(relative), Size: info.Size(), Mod: info.ModTime().UnixNano()})
	})
	if errors.Is(err, os.ErrNotExist) {
		return "absent", nil
	}
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// prepareColdStartSession prepares one live session without any model call.
func prepareColdStartSession(ctx context.Context, opts ColdStartOptions, session ColdStartSession) (ColdStartPrepared, error) {
	prepared := ColdStartPrepared{
		Session: session, Args: []string{}, Blockers: []string{}, Notes: []string{},
		KapiBin: opts.KapiBin, Timeout: opts.Manifest.attemptTimeout(), MaxTurns: opts.Manifest.MaxTurns,
	}
	task, err := findColdStartTask(session.Task)
	if err != nil {
		return prepared, err
	}
	prepared.Prompt = task.Prompt
	cell := ColdStartCell{ID: session.Cell, Host: session.Host, Task: session.Task}
	paths, wiring, err := prepareColdStartCell(ctx, opts, cell)
	prepared.Paths, prepared.Wiring = paths, wiring
	if err != nil {
		return prepared, err
	}
	prepared.Env = coldStartEnv(paths)
	server, serverErr := probeColdStartServer(ctx, paths, opts.KapiBin)
	prepared.Server = server
	switch {
	case serverErr != nil:
		prepared.Blockers = append(prepared.Blockers, "wired MCP server: "+serverErr.Error())
	case !server.UnderTest:
		prepared.Blockers = append(prepared.Blockers,
			fmt.Sprintf("the wired command %q resolves to %s, which is not this checkout's build", server.Command, server.Resolved))
	}
	prepared.Isolation, err = coldStartIsolation(paths, prepared.Env)
	if err != nil {
		return prepared, err
	}
	if len(prepared.Isolation.Ancestors) != 0 {
		prepared.Blockers = append(prepared.Blockers,
			"the fixture has discoverable project or assistant files above it: "+strings.Join(prepared.Isolation.Ancestors, ", "))
	}
	if len(prepared.Isolation.WorkspaceFiles) == 0 {
		prepared.Blockers = append(prepared.Blockers, "no workspace file appeared under this cell's data root")
	}
	executable, err := exec.LookPath(session.Host.Host)
	if err != nil {
		prepared.Blockers = append(prepared.Blockers, "agent executable unavailable: "+session.Host.Host)
		return prepared, nil
	}
	prepared.Executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return prepared, err
	}
	version, err := exec.CommandContext(ctx, prepared.Executable, "--version").Output()
	if err != nil {
		prepared.Blockers = append(prepared.Blockers, "agent version probe failed")
		return prepared, nil
	}
	prepared.Version = strings.TrimSpace(string(version))
	switch session.Host.Host {
	case "claude":
		err = prepareColdStartClaude(ctx, &prepared)
	case "codex":
		err = prepareColdStartCodex(ctx, &prepared)
	default:
		err = fmt.Errorf("unsupported host %q", session.Host.Host)
	}
	if err != nil {
		return prepared, err
	}
	prepared.Notes = append(prepared.Notes,
		"The kapi on this PATH is this checkout's build, which is what the wired `.mcp.json` command name resolves to.",
		"Process-level confinement is not claimed. The controls are a fresh HOME, a private PATH, this cell's own kapi roots, and evidence kept in ignored local output.",
		"Subscription quota is not inferred from token counts; the runner pauses after its authorized batch.")
	return prepared, nil
}

func (p ColdStartPrepared) blocked() bool { return len(p.Blockers) != 0 }

// prepareColdStartClaude lets Claude Code discover the project wiring rather
// than replacing it: project settings are the only source, which is what makes
// the `.mcp.json` and the skill directory `kapi init` wrote the thing under
// test.
func prepareColdStartClaude(ctx context.Context, p *ColdStartPrepared) error {
	token, err := pairedClaudeSubscriptionToken(ctx)
	if err != nil {
		p.Blockers = append(p.Blockers, err.Error())
	} else {
		p.Env = append(p.Env, "CLAUDE_CODE_OAUTH_TOKEN="+token)
		p.AuthMode = "claude.ai subscription"
	}
	settings := map[string]any{
		"autoMemoryEnabled":          false,
		"enableAllProjectMcpServers": true,
		"permissions": map[string]any{
			"defaultMode": "acceptEdits",
			"allow":       []string{"Bash", "Read", "Edit", "Write", "Glob", "Grep", "Skill", "mcp__kapi__*"},
		},
		"sandbox": map[string]any{
			"enabled": false,
			"network": map[string]any{"allowedDomains": []string{}, "strictAllowlist": true},
		},
	}
	settingsPath := filepath.Join(p.Paths.State, "claude-settings.json")
	if err := pairedWriteJSON(settingsPath, settings); err != nil {
		return err
	}
	p.Args = []string{
		"--print", "--output-format", "stream-json", "--verbose",
		"--model", p.Session.Host.Model, "--effort", p.Session.Host.Effort,
		"--setting-sources", "project", "--settings", settingsPath,
		"--no-session-persistence", "--permission-mode", "acceptEdits",
		"--tools", "Bash,Read,Edit,Write,Glob,Grep,Skill,ToolSearch",
		"--max-turns", strconv.Itoa(p.MaxTurns),
	}
	return nil
}

// prepareColdStartCodex brings this cell to the state a person's machine is in
// after `kapi init --agents all`.
//
// `kapi init` writes the kapi server into the repository's own `.codex/
// config.toml`, and Codex reads that file for a repository the person has
// trusted. The harness marks the fixture as trusted in this cell's own
// CODEX_HOME, which is the prompt a person accepts the first time they open the
// repository there, and the server entry itself stays as the product wrote it.
//
// Codex starts a stdio MCP server with a filtered environment: HOME, PATH and a
// few locale variables reach it and everything else is dropped. So this cell's
// kapi roots are named on the launch itself, through the `env_vars` list Codex
// forwards from its own environment.
func prepareColdStartCodex(ctx context.Context, p *ColdStartPrepared) error {
	originalHome, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	source := os.Getenv("CODEX_HOME")
	if source == "" {
		source = filepath.Join(originalHome, ".codex")
	}
	auth := filepath.Join(source, "auth.json")
	if _, err := os.Stat(auth); err != nil {
		p.Blockers = append(p.Blockers, "Codex file-backed subscription login unavailable; authenticate an isolated account profile")
	} else {
		destination := filepath.Join(p.Paths.State, "codex", "auth.json")
		if _, err := os.Lstat(destination); errors.Is(err, os.ErrNotExist) {
			if err := os.Symlink(auth, destination); err != nil {
				return err
			}
		}
		probe := exec.CommandContext(ctx, p.Executable, "login", "status")
		probe.Env = p.Env
		output, err := probe.CombinedOutput()
		if err != nil || !strings.Contains(string(output), "Logged in using ChatGPT") {
			p.Blockers = append(p.Blockers, "Codex subscription login was not verified")
		} else {
			p.AuthMode = "ChatGPT subscription"
		}
	}
	if err := os.WriteFile(filepath.Join(p.Paths.State, "codex", "config.toml"),
		[]byte(coldStartCodexConfig(*p)), 0o600); err != nil {
		return err
	}
	p.Wiring.Harness = append(p.Wiring.Harness,
		"codex config.toml: this cell's own CODEX_HOME marks the fixture as a trusted project, which stands for the trust prompt a person accepts, and is what makes Codex read the .codex/config.toml `kapi init` wrote",
		"codex mcp_servers.kapi.env_vars: the cell's kapi roots are forwarded to the server on the launch, because Codex hands a stdio MCP server HOME, PATH and locale variables and drops the rest")
	p.Args = append([]string{"exec", "--strict-config", "--ignore-rules", "--json", "--skip-git-repo-check",
		"-c", coldStartCodexForwardedEnv(p.Env)},
		"--model", p.Session.Host.Model, "--cd", p.Paths.Repo, "-")
	codex, err := probeColdStartCodexWiring(ctx, *p)
	p.Codex = codex
	switch {
	case err != nil:
		p.Blockers = append(p.Blockers, "Codex MCP wiring: "+err.Error())
	case !codex.UnderTest:
		p.Blockers = append(p.Blockers,
			fmt.Sprintf("Codex starts %q from %s, which is not this checkout's build", codex.Command, codex.Resolved))
	}
	return nil
}

// coldStartCodexConfig is this cell's own Codex configuration: how a session
// runs, the environment its shell tool sees, and the fixture marked as a
// trusted project. It names no MCP server, because the repository's own
// `.codex/config.toml` does.
func coldStartCodexConfig(p ColdStartPrepared) string {
	var config strings.Builder
	config.WriteString("forced_login_method = \"chatgpt\"\napproval_policy = \"never\"\nsandbox_mode = \"danger-full-access\"\nweb_search = \"disabled\"\nallow_login_shell = false\nmodel_reasoning_effort = " + strconv.Quote(p.Session.Host.Effort) + "\n[features]\napps = false\nplugins = false\nhooks = false\nmulti_agent = false\nbrowser_use = false\ncomputer_use = false\nimage_generation = false\nshell_snapshot = false\n[shell_environment_policy]\ninherit = \"none\"\n[shell_environment_policy.set]\n")
	for _, pair := range p.Env {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			config.WriteString(strconv.Quote(key) + " = " + strconv.Quote(value) + "\n")
		}
	}
	for _, path := range coldStartTrustedPaths(p.Paths.Repo) {
		config.WriteString("[projects." + strconv.Quote(path) + "]\ntrust_level = \"trusted\"\n")
	}
	return config.String()
}

// coldStartTrustedPaths are the spellings of the fixture a trust entry has to
// carry. Codex matches a project by the path it resolves, and a cells
// directory reached through a symlink (`/tmp` on macOS) resolves to another
// name, so both are named when they differ.
func coldStartTrustedPaths(repo string) []string {
	paths := []string{repo}
	if resolved, err := filepath.EvalSymlinks(repo); err == nil && resolved != repo {
		paths = append(paths, resolved)
	}
	return paths
}

// coldStartCodexForwardedEnv renders the `-c` override naming the cell
// variables Codex forwards to the kapi server it starts.
func coldStartCodexForwardedEnv(env []string) string {
	names := []string{}
	for _, pair := range env {
		key, _, ok := strings.Cut(pair, "=")
		if ok && (strings.HasPrefix(key, "KAPI_") || strings.HasPrefix(key, "XDG_")) {
			names = pairedUnique(names, key)
		}
	}
	sort.Strings(names)
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, strconv.Quote(name))
	}
	return "mcp_servers.kapi.env_vars=[" + strings.Join(quoted, ",") + "]"
}

// ColdStartCodexWiring is what Codex answers about this cell, read from its own
// configuration with no model call.
type ColdStartCodexWiring struct {
	// Trusted are the fixture paths this cell's CODEX_HOME marks as trusted.
	Trusted []string `json:"trusted"`
	// Servers are the MCP servers Codex sees in the fixture.
	Servers []string `json:"servers"`
	// Command is what the kapi server entry names, and Resolved what that name
	// resolves to on the cell's PATH.
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Resolved  string   `json:"resolved"`
	UnderTest bool     `json:"under_test"`
	// EnvVars are the cell variables Codex forwards to the server.
	EnvVars []string `json:"env_vars"`
}

// probeColdStartCodexWiring asks Codex what it sees in this cell, with the
// launch's own configuration and no model call. It answers the question the
// trust entry opens: whether the repository's `.codex/config.toml` is read at
// all, and whether the kapi it names is the build under test.
func probeColdStartCodexWiring(ctx context.Context, p ColdStartPrepared) (*ColdStartCodexWiring, error) {
	wiring := &ColdStartCodexWiring{
		Trusted: coldStartTrustedPaths(p.Paths.Repo), Servers: []string{}, Args: []string{}, EnvVars: []string{},
	}
	probe := exec.CommandContext(ctx, p.Executable, "mcp", "-c", coldStartCodexForwardedEnv(p.Env), "list", "--json")
	probe.Dir = p.Paths.Repo
	probe.Env = p.Env
	out, err := probe.Output()
	if err != nil {
		return wiring, fmt.Errorf("codex mcp list: %w", err)
	}
	var servers []struct {
		Name      string `json:"name"`
		Enabled   bool   `json:"enabled"`
		Transport struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
			EnvVars []string `json:"env_vars"`
		} `json:"transport"`
	}
	if err := json.Unmarshal(out, &servers); err != nil {
		return wiring, fmt.Errorf("read what Codex sees: %w", err)
	}
	for _, server := range servers {
		wiring.Servers = append(wiring.Servers, server.Name)
		if server.Name != "kapi" {
			continue
		}
		if !server.Enabled {
			return wiring, errors.New("Codex holds the kapi server disabled")
		}
		wiring.Command, wiring.Args, wiring.EnvVars = server.Transport.Command, server.Transport.Args, server.Transport.EnvVars
	}
	if wiring.Command == "" {
		return wiring, errors.New("Codex names no kapi server in the fixture, so the project configuration was not read")
	}
	resolved, err := coldStartResolveOnPath(wiring.Command, p.Paths.Bin)
	if err != nil {
		return wiring, err
	}
	wiring.Resolved = resolved
	wiring.UnderTest = resolved == p.KapiBin
	for _, name := range []string{"KAPI_DATA_DIR", "KAPI_CONFIG_DIR", "KAPI_PLUGINS_DIR_ONLY", "XDG_DATA_HOME"} {
		if !slices.Contains(wiring.EnvVars, name) {
			return wiring, fmt.Errorf("Codex forwards no %s to the server, so the cell's kapi roots would not reach it", name)
		}
	}
	return wiring, nil
}
