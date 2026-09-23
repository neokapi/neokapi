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

// EvalPaths names every directory one cell owns. The repository sits
// under the cell root and kapi's roots sit beside it, which is where a person's
// workspace sits relative to their checkout.
type EvalPaths struct {
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

func evalPaths(root string) EvalPaths {
	return EvalPaths{
		Root:    root,
		Repo:    filepath.Join(root, evalRepoName),
		Data:    filepath.Join(root, "kapi-data"),
		Config:  filepath.Join(root, "kapi-config"),
		Cache:   filepath.Join(root, "kapi-cache"),
		Plugins: filepath.Join(root, "kapi-plugins"),
		Home:    filepath.Join(root, "home"),
		Bin:     filepath.Join(root, "bin"),
		State:   filepath.Join(root, "state"),
	}
}

// EvalWiring records what the shipped `kapi init` wrote, and every place
// the harness had to complete or change it. A gap here is a product finding,
// reported rather than repaired.
type EvalWiring struct {
	InitArgs   []string `json:"init_args"`
	InitOutput string   `json:"init_output"`
	Files      []string `json:"files"`
	Harness    []string `json:"harness"`
	// Held are the rule terms a person put in force before the session, on a
	// Measure 1 cell. Empty on a Measure 2 cell, whose context starts empty.
	Held []string `json:"held"`
	// Baseline is the commit the session starts from: the fixture, the wiring
	// and, on a Measure 1 cell, the held rules. A check names the session's
	// work by diffing against it.
	Baseline string `json:"baseline"`
	// ContextAnswer is what the shipped surface says about a location in the
	// project before the session, read once during preparation.
	ContextAnswer string `json:"context_answer"`
}

// EvalServer is the MCP server an agent host starts from the wiring, as
// the server itself answers.
type EvalServer struct {
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Resolved  string   `json:"resolved"`
	UnderTest bool     `json:"under_test"`
	Name      string   `json:"server_name"`
	Version   string   `json:"server_version"`
	Tools     []string `json:"tools"`
	// Missing are the tools the transcript reader classifies as recording or
	// checking that this server does not offer, and Unclassified the tools it
	// offers that the reader has no kind for. Both are reported rather than
	// blocking a run: they say the reader and the product have moved apart.
	Missing      []string `json:"missing"`
	Unclassified []string `json:"unclassified"`
}

// EvalIsolation holds what the evaluation establishes by measurement.
type EvalIsolation struct {
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

// EvalPrepared is one session's launch description, prepared without any
// model call. Blockers prohibit inference.
type EvalPrepared struct {
	Session EvalSession `json:"session"`
	Paths   EvalPaths   `json:"paths"`
	Wiring  EvalWiring  `json:"wiring"`
	Server  *EvalServer `json:"server,omitempty"`
	// Codex is what Codex itself says about this cell's wiring, on a Codex cell.
	Codex      *EvalCodexWiring `json:"codex,omitempty"`
	Isolation  EvalIsolation    `json:"isolation"`
	Executable string           `json:"executable"`
	Args       []string         `json:"args"`
	Version    string           `json:"version"`
	AuthMode   string           `json:"auth_mode"`
	KapiBin    string           `json:"kapi_bin"`
	Timeout    time.Duration    `json:"timeout"`
	MaxTurns   int              `json:"max_turns"`
	Blockers   []string         `json:"blockers"`
	Notes      []string         `json:"notes"`
	Env        []string         `json:"-"`
	Prompt     string           `json:"-"`
}

// evalEnv is the environment every process in a cell runs under: the agent
// host, the kapi it starts over MCP, and the kapi the agent runs from a shell.
//
// KAPI_NO_PROJECT is absent on purpose. The fixture is generated outside this
// repository with nothing discoverable above it, so the upward walk finds the
// fixture's own recipe and that discovery is part of what is measured.
// Every other variable of the isolation contract is set, to this cell's own
// throwaway directories.
func evalEnv(paths EvalPaths) []string {
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
		"GIT_AUTHOR_NAME=Loomwise docs",
		"GIT_AUTHOR_EMAIL=docs@loomwise.invalid",
		"GIT_COMMITTER_NAME=Loomwise docs",
		"GIT_COMMITTER_EMAIL=docs@loomwise.invalid",
	}
}

func evalEnvNames(env []string) []string {
	names := make([]string, 0, len(env))
	for _, pair := range env {
		key, _, _ := strings.Cut(pair, "=")
		names = append(names, key)
	}
	sort.Strings(names)
	return names
}

// evalTools are the ordinary editing tools a private PATH keeps, so the
// agent works the way it would in any repository without inheriting the
// developer's own executables.
var evalTools = []string{
	"bash", "sh", "cat", "cp", "mv", "rm", "mkdir", "ls", "pwd", "find", "sed",
	"awk", "grep", "head", "tail", "wc", "sort", "uniq", "cut", "tr", "diff",
	"git", "rg", "python3", "jq", "file", "which", "env", "printf", "touch",
	"date", "node", "perl", "xargs", "tee", "basename", "dirname",
}

// evalKapiNames are the commands the shipped skill drives, each of them
// this checkout's binary under a different name.
var evalKapiNames = []string{"kapi", "kcat", "kgrep", "ksed", "kdiff"}

// prepareEvalCell brings one cell to the state its session finds: the fixture
// repository under git, `kapi init` run over it by the binary under test, the
// content mapping its scaffold leaves for the person, a private PATH whose kapi
// is this checkout's build and, on a Measure 1 cell, the planted conventions
// held as rules a person put in force. The commit it ends on is the baseline a
// check diffs the session's work against.
//
// It is idempotent. A cell that has been prepared is reused, so preflight
// prepares the cells a live phase then runs.
func prepareEvalCell(ctx context.Context, opts EvalOptions, session EvalSession) (EvalPaths, EvalWiring, error) {
	paths := evalPaths(filepath.Join(opts.SandboxRoot, session.ID))
	wiring := EvalWiring{Harness: []string{}, Held: []string{}}
	readyPath := filepath.Join(paths.Root, "wiring.json")
	if err := readPairedJSON(readyPath, &wiring); err == nil {
		return paths, wiring, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return paths, wiring, err
	}
	// A cell that failed part-way is rebuilt from nothing rather than finished,
	// because what it holds is unknown.
	if err := os.RemoveAll(paths.Root); err != nil {
		return paths, wiring, err
	}
	for _, dir := range []string{paths.Root, paths.Data, paths.Config, paths.Cache, paths.Plugins, paths.Home, paths.Bin, paths.State, filepath.Join(paths.State, "claude"), filepath.Join(paths.State, "codex")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return paths, wiring, err
		}
	}
	if err := evalToolPath(paths, opts.KapiBin); err != nil {
		return paths, wiring, err
	}
	if err := materializeEvalRepo(paths.Repo, opts.Fixture); err != nil {
		return paths, wiring, err
	}
	if err := evalGitInit(ctx, paths); err != nil {
		return paths, wiring, err
	}
	wiring.InitArgs = []string{"init", "--agents", opts.Manifest.Agents}
	out, err := evalRunKapiAs(ctx, paths, evalActorPerson, wiring.InitArgs...)
	wiring.InitOutput = out
	if err != nil {
		return paths, wiring, fmt.Errorf("kapi init: %w", err)
	}
	wiring.Files, err = evalWiringFiles(paths.Repo)
	if err != nil {
		return paths, wiring, err
	}
	recipe := filepath.Join(paths.Repo, "kapi.yaml")
	scaffolded, err := os.ReadFile(recipe)
	if err != nil {
		return paths, wiring, err
	}
	mapped, err := evalRecipeMapping(scaffolded)
	if err != nil {
		return paths, wiring, err
	}
	if err := os.WriteFile(recipe, mapped, 0o600); err != nil {
		return paths, wiring, err
	}
	wiring.Harness = append(wiring.Harness,
		"kapi.yaml: collections point at the fixture's prose, completing the content mapping the scaffold leaves for the person")
	if err := evalGitCommit(ctx, paths, "Wire the project for kapi"); err != nil {
		return paths, wiring, err
	}
	if session.Measure == evalMeasureApply {
		if err := evalHoldRules(ctx, paths, opts.Fixture.Key, &wiring); err != nil {
			return paths, wiring, err
		}
		if err := evalGitCommit(ctx, paths, "Hold the project's rules"); err != nil {
			return paths, wiring, err
		}
	}
	if wiring.Baseline, err = evalGitHead(ctx, paths); err != nil {
		return paths, wiring, err
	}
	// One read through the shipped surface, which records what the context
	// answers before the session and opens this cell's workspace, so the
	// isolation check has a workspace file to find.
	wiring.ContextAnswer, err = evalRunKapi(ctx, paths, "context", "README.md")
	if err != nil {
		return paths, wiring, fmt.Errorf("read the context before the session: %w", err)
	}
	if err := writePairedJSON(readyPath, wiring); err != nil {
		return paths, wiring, err
	}
	return paths, wiring, nil
}

// evalHoldRules loads the planted conventions as a person would, then reads the
// context log back and refuses a cell in which any rule did not reach the held
// status. A Measure 1 run over rules that silently failed to load would score
// agents on rules they were never given.
func evalHoldRules(ctx context.Context, paths EvalPaths, key EvalKey, wiring *EvalWiring) error {
	loaded, err := evalLoadHeldRules(ctx, paths, key)
	if err != nil {
		return err
	}
	store, err := evalReadStore(ctx, paths)
	if err != nil {
		return err
	}
	wiring.Held = evalHeldTerms(store)
	missing := []string{}
	for _, term := range loaded {
		if !slices.Contains(wiring.Held, strings.ToLower(term)) {
			missing = append(missing, term)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("%d of %d rules did not reach the %q status: %s",
			len(missing), len(loaded), evalHeldStatus, strings.Join(missing, ", "))
	}
	wiring.Harness = append(wiring.Harness, fmt.Sprintf(
		"held rules: a person proposed and confirmed %d rules for the eleven planted conventions before each Measure 1 session", len(loaded)))
	return nil
}

// evalToolPath fills the cell's private bin directory. The kapi names come
// first because they are written into it directly: the `.mcp.json` that
// `kapi init` wrote names a bare `kapi`, so what that name resolves to on this
// PATH is what an agent host starts.
func evalToolPath(paths EvalPaths, kapiBin string) error {
	if kapiBin == "" {
		return errors.New("the evaluation needs this checkout's kapi binary; run `make build` first")
	}
	for _, name := range evalKapiNames {
		destination := filepath.Join(paths.Bin, name)
		if _, err := os.Lstat(destination); err == nil {
			continue
		}
		if err := os.Symlink(kapiBin, destination); err != nil {
			return err
		}
	}
	for _, name := range evalTools {
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

// evalWiringFiles lists what the wiring put in the repository, so the
// record says which hosts `kapi init` reached.
func evalWiringFiles(repo string) ([]string, error) {
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

func evalRunKapi(ctx context.Context, paths EvalPaths, args ...string) (string, error) {
	return evalRunKapiAs(ctx, paths, "", args...)
}

// evalRunKapiAs runs the cell's kapi with the actor it should record under.
// Empty leaves kapi to work the actor out. Standard error is kept apart from
// the answer, so a warning never corrupts a JSON reply, and joins the error
// when the command fails.
func evalRunKapiAs(ctx context.Context, paths EvalPaths, actor string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, filepath.Join(paths.Bin, "kapi"), args...)
	command.Dir = paths.Repo
	command.Env = evalEnv(paths)
	if actor != "" {
		command.Env = append(command.Env, "KAPI_ACTOR="+actor)
	}
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()+" "+stdout.String()))
	}
	return stdout.String(), nil
}

func evalRunGit(ctx context.Context, paths EvalPaths, args ...string) error {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = paths.Repo
	command.Env = evalEnv(paths)
	out, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// evalGitInit puts the fixture under version control, because the shipped
// skill names a change by diffing it against HEAD and a repository with no
// history has nothing to name.
func evalGitInit(ctx context.Context, paths EvalPaths) error {
	if err := evalRunGit(ctx, paths, "init", "--initial-branch=main", "--quiet"); err != nil {
		return err
	}
	return evalGitCommit(ctx, paths, "The docs as they stand")
}

// evalGitHead names the commit a cell's working tree sits on.
func evalGitHead(ctx context.Context, paths EvalPaths) (string, error) {
	command := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	command.Dir = paths.Repo
	command.Env = evalEnv(paths)
	out, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func evalGitCommit(ctx context.Context, paths EvalPaths, message string) error {
	if err := evalRunGit(ctx, paths, "add", "-A"); err != nil {
		return err
	}
	return evalRunGit(ctx, paths, "commit", "--quiet", "--allow-empty", "-m", message)
}

// evalMCPEntry reads the server entry out of the `.mcp.json` that
// `kapi init` wrote, so the probe starts what an agent host would start.
func evalMCPEntry(repo string) (string, []string, error) {
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

// probeEvalServer starts the wired server exactly as an agent host would,
// and records what answers: the binary the wired command resolves to, the
// server's own name and version, and its tool inventory.
func probeEvalServer(ctx context.Context, paths EvalPaths, kapiBin string) (*EvalServer, error) {
	command, args, err := evalMCPEntry(paths.Repo)
	if err != nil {
		return nil, err
	}
	server := &EvalServer{Command: command, Args: args, Tools: []string{}, Missing: []string{}, Unclassified: []string{}}
	resolved, err := evalResolveOnPath(command, paths.Bin)
	if err != nil {
		return server, err
	}
	server.Resolved = resolved
	server.UnderTest = resolved == kapiBin
	readiness := PairedMCPReadiness{Capabilities: []string{}, Tools: []string{}, Resources: []string{}, ResourceTemplates: []string{}}
	probe := exec.CommandContext(ctx, resolved, args...)
	probe.Dir = paths.Repo
	probe.Env = evalEnv(paths)
	if err := evalDiscover(ctx, probe, &readiness); err != nil {
		return server, err
	}
	server.Name, server.Version, server.Tools = readiness.ServerName, readiness.ServerVersion, readiness.Tools
	server.Missing, server.Unclassified = evalToolCoverage(readiness.Tools)
	return server, nil
}

// evalResolveOnPath answers what a bare command name resolves to on the
// cell's private PATH, which is the question `.mcp.json` leaves open.
func evalResolveOnPath(command, bin string) (string, error) {
	if strings.ContainsRune(command, filepath.Separator) {
		return filepath.EvalSymlinks(command)
	}
	return filepath.EvalSymlinks(filepath.Join(bin, command))
}

// evalDiscover runs an initialize and tools/list handshake against an
// already-configured command, with no tools/call and no model anywhere.
func evalDiscover(ctx context.Context, command *exec.Cmd, readiness *PairedMCPReadiness) error {
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

// evalIsolation establishes the properties the evaluation refuses to assume.
func evalIsolation(paths EvalPaths, env []string) (EvalIsolation, error) {
	isolation := EvalIsolation{Ancestors: []string{}, WorkspaceFiles: []string{}, EnvNames: evalEnvNames(env)}
	ancestors, err := evalAncestorFindings(paths.Repo)
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
	isolation.UserDataRoot = evalUserDataRoot()
	isolation.UserDataWitness, err = evalWitness(isolation.UserDataRoot)
	return isolation, err
}

// evalWitnessChanged compares two witnesses of the person's own data root.
// A difference means something in this phase reached it, which invalidates the
// attempt rather than being noted after the fact.
func evalWitnessChanged(before, after string) bool {
	return before != "" && after != "" && before != after
}

// evalUserDataRoot is the per-user data root kapi resolves on this machine
// without KAPI_DATA_DIR: the one holding the developer's own terms, voice
// profiles, content memory and recorded decisions.
func evalUserDataRoot() string {
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

// evalWitness digests a directory's shape: each entry's path, size and
// modification time, and nothing of its contents. Comparing the witness taken
// before a phase with the one taken after is how the evaluation establishes that
// nothing reached the person's own data root.
func evalWitness(dir string) (string, error) {
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

// prepareEvalSession prepares one live session without any model call.
func prepareEvalSession(ctx context.Context, opts EvalOptions, session EvalSession) (EvalPrepared, error) {
	prepared := EvalPrepared{
		Session: session, Args: []string{}, Blockers: []string{}, Notes: []string{},
		KapiBin: opts.KapiBin, Timeout: opts.Manifest.attemptTimeout(), MaxTurns: opts.Manifest.MaxTurns,
	}
	task, err := findEvalTask(session.Task)
	if err != nil {
		return prepared, err
	}
	prepared.Prompt = task.Prompt
	paths, wiring, err := prepareEvalCell(ctx, opts, session)
	prepared.Paths, prepared.Wiring = paths, wiring
	if err != nil {
		return prepared, err
	}
	prepared.Env = evalEnv(paths)
	server, serverErr := probeEvalServer(ctx, paths, opts.KapiBin)
	prepared.Server = server
	switch {
	case serverErr != nil:
		prepared.Blockers = append(prepared.Blockers, "wired MCP server: "+serverErr.Error())
	case !server.UnderTest:
		prepared.Blockers = append(prepared.Blockers,
			fmt.Sprintf("the wired command %q resolves to %s, which is not this checkout's build", server.Command, server.Resolved))
	case len(server.Tools) == 0:
		prepared.Blockers = append(prepared.Blockers, "the wired MCP server offers no tools")
	}
	if server != nil && len(server.Missing) != 0 {
		prepared.Notes = append(prepared.Notes, "the wired server offers none of "+strings.Join(server.Missing, ", ")+
			", which the transcript reader counts as recording or checking")
	}
	if server != nil && len(server.Unclassified) != 0 {
		prepared.Notes = append(prepared.Notes, "the transcript reader has no kind for "+strings.Join(server.Unclassified, ", "))
	}
	prepared.Isolation, err = evalIsolation(paths, prepared.Env)
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
		err = prepareEvalClaude(ctx, &prepared)
	case "codex":
		err = prepareEvalCodex(ctx, &prepared)
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

func (p EvalPrepared) blocked() bool { return len(p.Blockers) != 0 }

// prepareEvalClaude lets Claude Code discover the project wiring rather
// than replacing it: project settings are the only source, which is what makes
// the `.mcp.json` and the skill directory `kapi init` wrote the thing under
// test.
func prepareEvalClaude(ctx context.Context, p *EvalPrepared) error {
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

// prepareEvalCodex brings this cell to the state a person's machine is in
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
func prepareEvalCodex(ctx context.Context, p *EvalPrepared) error {
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
		[]byte(evalCodexConfig(*p)), 0o600); err != nil {
		return err
	}
	p.Wiring.Harness = append(p.Wiring.Harness,
		"codex config.toml: this cell's own CODEX_HOME marks the fixture as a trusted project, which stands for the trust prompt a person accepts, and is what makes Codex read the .codex/config.toml `kapi init` wrote",
		"codex mcp_servers.kapi.env_vars: the cell's kapi roots are forwarded to the server on the launch, because Codex hands a stdio MCP server HOME, PATH and locale variables and drops the rest")
	p.Args = append([]string{"exec", "--strict-config", "--ignore-rules", "--json", "--skip-git-repo-check",
		"-c", evalCodexForwardedEnv(p.Env)},
		"--model", p.Session.Host.Model, "--cd", p.Paths.Repo, "-")
	codex, err := probeEvalCodexWiring(ctx, *p)
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

// evalCodexConfig is this cell's own Codex configuration: how a session
// runs, the environment its shell tool sees, and the fixture marked as a
// trusted project. It names no MCP server, because the repository's own
// `.codex/config.toml` does.
func evalCodexConfig(p EvalPrepared) string {
	var config strings.Builder
	config.WriteString("forced_login_method = \"chatgpt\"\napproval_policy = \"never\"\nsandbox_mode = \"danger-full-access\"\nweb_search = \"disabled\"\nallow_login_shell = false\nmodel_reasoning_effort = " + strconv.Quote(p.Session.Host.Effort) + "\n[features]\napps = false\nplugins = false\nhooks = false\nmulti_agent = false\nbrowser_use = false\ncomputer_use = false\nimage_generation = false\nshell_snapshot = false\n[shell_environment_policy]\ninherit = \"none\"\n[shell_environment_policy.set]\n")
	for _, pair := range p.Env {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			config.WriteString(strconv.Quote(key) + " = " + strconv.Quote(value) + "\n")
		}
	}
	for _, path := range evalTrustedPaths(p.Paths.Repo) {
		config.WriteString("[projects." + strconv.Quote(path) + "]\ntrust_level = \"trusted\"\n")
	}
	return config.String()
}

// evalTrustedPaths are the spellings of the fixture a trust entry has to
// carry. Codex matches a project by the path it resolves, and a cells
// directory reached through a symlink (`/tmp` on macOS) resolves to another
// name, so both are named when they differ.
func evalTrustedPaths(repo string) []string {
	paths := []string{repo}
	if resolved, err := filepath.EvalSymlinks(repo); err == nil && resolved != repo {
		paths = append(paths, resolved)
	}
	return paths
}

// evalCodexForwardedEnv renders the `-c` override naming the cell
// variables Codex forwards to the kapi server it starts.
func evalCodexForwardedEnv(env []string) string {
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

// EvalCodexWiring is what Codex answers about this cell, read from its own
// configuration with no model call.
type EvalCodexWiring struct {
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

// probeEvalCodexWiring asks Codex what it sees in this cell, with the
// launch's own configuration and no model call. It answers the question the
// trust entry opens: whether the repository's `.codex/config.toml` is read at
// all, and whether the kapi it names is the build under test.
func probeEvalCodexWiring(ctx context.Context, p EvalPrepared) (*EvalCodexWiring, error) {
	wiring := &EvalCodexWiring{
		Trusted: evalTrustedPaths(p.Paths.Repo), Servers: []string{}, Args: []string{}, EnvVars: []string{},
	}
	probe := exec.CommandContext(ctx, p.Executable, "mcp", "-c", evalCodexForwardedEnv(p.Env), "list", "--json")
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
			return wiring, errors.New("the kapi server reaches Codex disabled")
		}
		wiring.Command, wiring.Args, wiring.EnvVars = server.Transport.Command, server.Transport.Args, server.Transport.EnvVars
	}
	if wiring.Command == "" {
		return wiring, errors.New("no kapi server reaches Codex in the fixture, so the project configuration went unread")
	}
	resolved, err := evalResolveOnPath(wiring.Command, p.Paths.Bin)
	if err != nil {
		return wiring, err
	}
	wiring.Resolved = resolved
	wiring.UnderTest = resolved == p.KapiBin
	for _, name := range []string{"KAPI_DATA_DIR", "KAPI_CONFIG_DIR", "KAPI_PLUGINS_DIR_ONLY", "XDG_DATA_HOME"} {
		if !slices.Contains(wiring.EnvVars, name) {
			return wiring, fmt.Errorf("the launch forwards no %s, so the cell's kapi roots would not reach the server", name)
		}
	}
	return wiring, nil
}
