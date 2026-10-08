package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ComparePaths names the directories one cell owns. The repository sits under
// the cell root and kapi's roots beside it.
type ComparePaths = EvalPaths

func comparePaths(root, project string) ComparePaths {
	paths := evalPaths(root)
	paths.Repo = filepath.Join(root, project)
	return paths
}

// compareEnv is the environment every process in a cell runs under: the agent
// host, the kapi it starts over MCP, and the kapi the agent runs from a shell.
// It is the evaluation's isolation contract (fresh HOME, private PATH, the
// cell's own kapi roots), with git identities that name the sample project.
func compareEnv(paths ComparePaths, project string) []string {
	env := []string{}
	for _, pair := range evalEnv(paths) {
		key, _, _ := strings.Cut(pair, "=")
		if strings.HasPrefix(key, "GIT_AUTHOR_") || strings.HasPrefix(key, "GIT_COMMITTER_") {
			continue
		}
		env = append(env, pair)
	}
	name := strings.ToUpper(project[:1]) + project[1:]
	return append(env,
		"GIT_AUTHOR_NAME="+name+" docs", "GIT_AUTHOR_EMAIL=docs@"+project+".invalid",
		"GIT_COMMITTER_NAME="+name+" docs", "GIT_COMMITTER_EMAIL=docs@"+project+".invalid",
	)
}

// CompareWiring records how a cell was set up, for the attempt's record.
type CompareWiring struct {
	Arm      string   `json:"arm"`
	Project  string   `json:"project"`
	Files    []string `json:"files"`
	Steps    []string `json:"steps"`
	Baseline string   `json:"baseline"`
	// ContextAnswer is what kapi says about the first held rule's file, in the
	// kapi arm, read once after the rules are held.
	ContextAnswer string `json:"context_answer,omitempty"`
	// Probe is the gate self-test: each house rule's probe text, and whether
	// `kapi check` failed it. Only in the kapi arm and the grader cell.
	Probe []CompareProbe `json:"probe,omitempty"`
}

// CompareProbe is one gate self-test.
type CompareProbe struct {
	File    string   `json:"file"`
	Text    string   `json:"text"`
	Rules   []string `json:"rules"`
	Failing int      `json:"failing"`
}

func compareRunIn(ctx context.Context, dir string, env []string, actor, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Env = env
	if actor != "" {
		command.Env = append(slices.Clone(env), "KAPI_ACTOR="+actor)
	}
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%s %s: %w: %s", filepath.Base(name), strings.Join(args, " "), err, strings.TrimSpace(stderr.String()+" "+stdout.String()))
	}
	return stdout.String(), nil
}

// compareToolPath fills the cell's private bin directory: the ordinary
// editing tools in every arm, and kapi's names only in the kapi arm. An arm
// without kapi has no kapi to find. The binary is copied into the cell (a hard
// link where the volume allows), so the sandbox can deny every read of the
// checkout and of the developer's home.
func compareToolPath(paths ComparePaths, arm, kapiBin string) error {
	if compareKapiArm(arm) {
		if kapiBin == "" {
			return errors.New("the kapi arm needs this checkout's kapi binary; run `make build` first")
		}
		cellKapi, err := compareLinkKapi(paths, kapiBin)
		if err != nil {
			return fmt.Errorf("copy kapi into the cell: %w", err)
		}
		for _, name := range evalKapiNames {
			destination := filepath.Join(paths.Bin, name)
			if _, err := os.Lstat(destination); err == nil {
				continue
			}
			if err := os.Symlink(cellKapi, destination); err != nil {
				return err
			}
		}
	}
	for _, name := range evalTools {
		source := pairedSystemTool(name)
		if source == "" {
			if found, err := exec.LookPath(name); err == nil {
				source = found
			}
		}
		if source == "" {
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

// compareCellKapi is where a cell keeps its own copy of the binary under test.
func compareCellKapi(paths ComparePaths) string {
	return filepath.Join(paths.Root, "kapi-bin", "kapi")
}

// compareLinkKapi puts the binary under test into the cell.
func compareLinkKapi(paths ComparePaths, kapiBin string) (string, error) {
	destination := compareCellKapi(paths)
	if _, err := os.Stat(destination); err == nil {
		return destination, nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", err
	}
	if err := os.Link(kapiBin, destination); err == nil {
		return destination, nil
	}
	data, err := os.ReadFile(kapiBin)
	if err != nil {
		return "", err
	}
	return destination, os.WriteFile(destination, data, 0o700)
}

// prepareCompareCell builds one cell: the project's content under git, then
// the arm's wiring, committed. The commit it ends on is the baseline the
// attempt's work is read against. The grader cell is a kapi-arm cell that no
// agent runs in.
func prepareCompareCell(ctx context.Context, root string, project CompareProject, arm, kapiBin string) (ComparePaths, CompareWiring, error) {
	paths := comparePaths(root, project.Name)
	wiring := CompareWiring{Arm: arm, Project: project.Name, Steps: []string{}}
	if err := os.RemoveAll(root); err != nil {
		return paths, wiring, err
	}
	for _, dir := range []string{paths.Root, paths.Repo, paths.Data, paths.Config, paths.Cache, paths.Plugins, paths.Home, paths.Bin, paths.State, filepath.Join(paths.State, "claude"), filepath.Join(paths.State, "codex")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return paths, wiring, err
		}
	}
	if err := compareToolPath(paths, arm, kapiBin); err != nil {
		return paths, wiring, err
	}
	for name, data := range project.Files {
		target := filepath.Join(paths.Repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return paths, wiring, err
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			return paths, wiring, err
		}
	}
	env := compareEnv(paths, project.Name)
	git := func(args ...string) error {
		_, err := compareRunIn(ctx, paths.Repo, env, "", "git", args...)
		return err
	}
	commit := func(message string) error {
		if err := git("add", "-A"); err != nil {
			return err
		}
		return git("commit", "--quiet", "--allow-empty", "-m", message)
	}
	if err := git("init", "--initial-branch=main", "--quiet"); err != nil {
		return paths, wiring, err
	}
	if err := commit("The docs as they stand"); err != nil {
		return paths, wiring, err
	}
	switch arm {
	case compareArmKapi, compareArmKapiFiles:
		if err := compareWireKapi(ctx, paths, env, project, arm == compareArmKapiFiles, &wiring); err != nil {
			return paths, wiring, err
		}
		if err := commit("Wire the project for kapi"); err != nil {
			return paths, wiring, err
		}
	case compareArmRules:
		for _, name := range []string{"CLAUDE.md", "AGENTS.md"} {
			if err := os.WriteFile(filepath.Join(paths.Repo, name), project.Rules, 0o600); err != nil {
				return paths, wiring, err
			}
		}
		wiring.Steps = append(wiring.Steps, "CLAUDE.md and AGENTS.md: the project's writing guide, the same rules kapi holds in the kapi arm")
		if err := commit("Add the writing guide"); err != nil {
			return paths, wiring, err
		}
	case compareArmBare:
		wiring.Steps = append(wiring.Steps, "content only")
	default:
		return paths, wiring, fmt.Errorf("unknown arm %q", arm)
	}
	head, err := compareRunIn(ctx, paths.Repo, env, "", "git", "rev-parse", "HEAD")
	if err != nil {
		return paths, wiring, err
	}
	wiring.Baseline = strings.TrimSpace(head)
	wiring.Files, err = evalWiringFiles(paths.Repo)
	return paths, wiring, err
}

// compareWireKapi brings a cell to the state a kapi project is in once a
// person has set it up: `kapi init --agents all`, the project's recipe, its
// voice read into the store with `kapi store import`, and each held rule
// recorded and kept. The voice file is then removed: a kapi project keeps its
// context in the store, not in the checkout.
func compareWireKapi(ctx context.Context, paths ComparePaths, env []string, project CompareProject, rulesFiles bool, wiring *CompareWiring) error {
	kapi := filepath.Join(paths.Bin, "kapi")
	run := func(args ...string) (string, error) {
		return compareRunIn(ctx, paths.Repo, env, evalActorPerson, kapi, args...)
	}
	initArgs := []string{"init", "--agents", "all"}
	if !rulesFiles {
		initArgs = append(initArgs, "--no-rules-files")
	}
	if _, err := run(initArgs...); err != nil {
		return err
	}
	wiring.Steps = append(wiring.Steps, "kapi "+strings.Join(initArgs, " "))
	if err := os.WriteFile(filepath.Join(paths.Repo, "kapi.yaml"), project.Recipe, 0o600); err != nil {
		return err
	}
	wiring.Steps = append(wiring.Steps, "kapi.yaml: the project's recipe (collections and profiles)")
	voice := filepath.Join(paths.Repo, ".kapi", "voice.yaml")
	if err := os.WriteFile(voice, project.Voice, 0o600); err != nil {
		return err
	}
	if _, err := run(append(slices.Clone(compareVerbImport), "--json")...); err != nil {
		return err
	}
	if err := os.Remove(voice); err != nil {
		return err
	}
	wiring.Steps = append(wiring.Steps, "kapi "+strings.Join(compareVerbImport, " ")+": the voice and its word rules into the store; the voice file removed")
	for _, held := range project.Held {
		args := append(slices.Clone(compareVerbRecord), held.Summary, "--term", held.Term)
		for _, avoided := range held.InsteadOf {
			args = append(args, "--instead-of", avoided)
		}
		args = append(args, "--seen-in", held.SeenIn, "--quote", held.Quote, "--json")
		out, err := run(args...)
		if err != nil {
			return err
		}
		var recorded struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(out), &recorded); err != nil {
			return fmt.Errorf("read the record of %q: %w: %s", held.Term, err, strings.TrimSpace(out))
		}
		if recorded.ID == "" {
			return fmt.Errorf("the record of %q carries no id: %s", held.Term, strings.TrimSpace(out))
		}
		keep := append(slices.Clone(compareVerbKeep), recorded.ID)
		if held.WidenTo != "" {
			keep = append(keep, "--widen-to", held.WidenTo)
		}
		if _, err := run(append(keep, "--json")...); err != nil {
			return err
		}
		wiring.Steps = append(wiring.Steps, fmt.Sprintf("held: %s, not %s, recorded at %s and kept by a person", held.Term, strings.Join(held.InsteadOf, ", "), held.SeenIn))
	}
	if rulesFiles {
		// kapi refreshes the rules files when a rule lands; the sync is the
		// manual trigger, run once so the files reflect everything held.
		if _, err := run("context", "sync", "--files-only"); err != nil {
			return err
		}
		written, err := compareRulesFiles(paths.Repo)
		if err != nil {
			return err
		}
		if len(written) == 0 {
			return errors.New("kapi wrote no rules files")
		}
		wiring.Steps = append(wiring.Steps, "kapi context sync --files-only: rules files "+strings.Join(written, ", "))
	}
	if len(project.Held) != 0 {
		answer, err := compareRunIn(ctx, paths.Repo, env, "", kapi, "context", project.Held[0].SeenIn)
		if err != nil {
			return err
		}
		if !strings.Contains(answer, project.Held[0].Term) {
			return fmt.Errorf("kapi context %s does not carry the held rule %q", project.Held[0].SeenIn, project.Held[0].Term)
		}
		wiring.ContextAnswer = answer
	}
	return nil
}

// compareGateProbe is the grader's self-test: one sentence breaking every
// house rule, appended to a file in each part of the project, must fail
// `kapi check` there. A rule the gate does not see would make the kapi-check
// column meaningless, so preflight refuses a project whose probe passes.
func compareGateProbe(ctx context.Context, paths ComparePaths, project CompareProject, baseline string) ([]CompareProbe, error) {
	env := compareEnv(paths, project.Name)
	probes := []CompareProbe{}
	for _, held := range project.Held {
		text := compareProbeText(project)
		target := filepath.Join(paths.Repo, filepath.FromSlash(held.SeenIn))
		original, err := os.ReadFile(target)
		if err != nil {
			return probes, err
		}
		if err := os.WriteFile(target, append(slices.Clone(original), []byte("\n"+text+"\n")...), 0o600); err != nil {
			return probes, err
		}
		out, _ := compareRunIn(ctx, paths.Repo, env, "", filepath.Join(paths.Bin, "kapi"), "check", "--diff-against", baseline, "--json")
		if err := os.WriteFile(target, original, 0o600); err != nil {
			return probes, err
		}
		check := evalParseCheck([]byte(out))
		probe := CompareProbe{File: held.SeenIn, Text: text, Rules: []string{}}
		for _, finding := range check.failing() {
			probe.Rules = pairedUnique(probe.Rules, finding.Rule+" "+finding.Term)
			probe.Failing++
		}
		probes = append(probes, probe)
		if check.Error != "" {
			return probes, fmt.Errorf("probe check in %s: %s", held.SeenIn, check.Error)
		}
		if probe.Failing == 0 {
			return probes, fmt.Errorf("the gate passed a sentence that breaks every house rule in %s", held.SeenIn)
		}
	}
	return probes, nil
}

// compareProbeText writes one paragraph that breaks every forbid-pattern
// house rule of a project, built from the forbidden forms the rules name.
func compareProbeText(project CompareProject) string {
	switch project.Name {
	case "teamboard":
		return "Log in to your Workspace with your e-mail. We guarantee you never lose a card when you leave Boardly!"
	case "ledgerly":
		return "Autopay pays your client instantly with no fees, and every invoice is tax-compliant, unlike Invoicr! Each pay-out is free."
	case "harbor":
		return "Harbor Cloud is secure by default and gives you zero-downtime deploys, unlike Dockyard. Simply whitelist your IP!"
	}
	return ""
}

// ComparePrepared is one attempt's launch description, made with no model call.
type ComparePrepared struct {
	Attempt    CompareAttempt   `json:"attempt"`
	Paths      ComparePaths     `json:"paths"`
	Wiring     CompareWiring    `json:"wiring"`
	Server     *EvalServer      `json:"server,omitempty"`
	Codex      *EvalCodexWiring `json:"codex,omitempty"`
	Isolation  EvalIsolation    `json:"isolation"`
	Executable string           `json:"executable"`
	Args       []string         `json:"args"`
	Version    string           `json:"version"`
	AuthMode   string           `json:"auth_mode"`
	Blockers   []string         `json:"blockers"`
	Env        []string         `json:"-"`
	Prompt     string           `json:"-"`
	Timeout    time.Duration    `json:"timeout"`
	MaxTurns   int              `json:"max_turns"`
	// TmpDir is the cell's own temporary directory, short because Claude
	// Code keeps sockets under it, and removed when the attempt ends.
	TmpDir string `json:"tmp_dir"`
	// Sandbox says how the host confines the agent's commands.
	Sandbox string `json:"sandbox"`
}

// prepareCompareAttempt prepares one attempt's cell and host, with no model call.
func prepareCompareAttempt(ctx context.Context, cellsDir, kapiBin string, m CompareManifest, attempt CompareAttempt) (ComparePrepared, error) {
	p := ComparePrepared{Attempt: attempt, Args: []string{}, Blockers: []string{}, Timeout: m.attemptTimeout(), MaxTurns: m.MaxTurns}
	task, err := findCompareTask(attempt.Task)
	if err != nil {
		return p, err
	}
	p.Prompt = task.Prompt
	project, err := loadCompareProject(attempt.Project)
	if err != nil {
		return p, err
	}
	p.Paths, p.Wiring, err = prepareCompareCell(ctx, filepath.Join(cellsDir, attempt.Phase, attempt.ID), project, attempt.Arm, kapiBin)
	if err != nil {
		return p, err
	}
	p.Env = compareEnv(p.Paths, project.Name)
	if p.TmpDir, err = makePairedCellTmp(); err != nil {
		return p, err
	}
	p.Env = append(p.Env, "TMPDIR="+p.TmpDir, "CLAUDE_CODE_TMPDIR="+p.TmpDir)
	if compareKapiArm(attempt.Arm) {
		server, serverErr := probeEvalServer(ctx, p.Paths, compareCellKapi(p.Paths))
		p.Server = server
		switch {
		case serverErr != nil:
			p.Blockers = append(p.Blockers, "wired MCP server: "+serverErr.Error())
		case !server.UnderTest:
			p.Blockers = append(p.Blockers, fmt.Sprintf("the wired command %q resolves to %s, not this checkout's build", server.Command, server.Resolved))
		case len(server.Tools) == 0:
			p.Blockers = append(p.Blockers, "the wired MCP server offers no tools")
		}
	}
	p.Isolation, err = evalIsolation(p.Paths, p.Env)
	if err != nil {
		return p, err
	}
	if len(p.Isolation.Ancestors) != 0 {
		p.Blockers = append(p.Blockers, "discoverable project or assistant files above the cell: "+strings.Join(p.Isolation.Ancestors, ", "))
	}
	executable, err := exec.LookPath(attempt.Host.Host)
	if err != nil {
		p.Blockers = append(p.Blockers, "agent executable unavailable: "+attempt.Host.Host)
		return p, nil
	}
	if p.Executable, err = filepath.EvalSymlinks(executable); err != nil {
		return p, err
	}
	version, err := exec.CommandContext(ctx, p.Executable, "--version").Output()
	if err != nil {
		p.Blockers = append(p.Blockers, "agent version probe failed")
		return p, nil
	}
	p.Version = strings.TrimSpace(string(version))
	switch attempt.Host.Host {
	case "claude":
		err = prepareCompareClaude(ctx, &p)
	case "codex":
		err = prepareCompareCodex(ctx, &p, kapiBin)
	}
	return p, err
}

// prepareCompareClaude reads the project's own settings source, which is
// where `kapi init` puts the MCP entry and the skill and where CLAUDE.md
// lives, and nothing of the developer's. Every arm gets the same tools.
func prepareCompareClaude(ctx context.Context, p *ComparePrepared) error {
	token, err := pairedClaudeSubscriptionToken(ctx, p.Timeout)
	if err != nil {
		p.Blockers = append(p.Blockers, err.Error())
	} else {
		p.Env = append(p.Env, "CLAUDE_CODE_OAUTH_TOKEN="+token)
		p.AuthMode = "claude.ai subscription"
	}
	deny, err := compareDenyRead(p.Paths.Root)
	if err != nil {
		p.Blockers = append(p.Blockers, err.Error())
	}
	settings := map[string]any{
		"autoMemoryEnabled":          false,
		"enableAllProjectMcpServers": true,
		"permissions": map[string]any{
			"defaultMode": "acceptEdits",
			"allow":       []string{"Bash", "Read", "Edit", "Write", "Glob", "Grep", "Skill", "mcp__kapi__*"},
		},
		// Every Bash command runs in Claude's sandbox: it writes only inside
		// the cell and its temporary directory, reads nothing of the checkout,
		// the developer's home or the shared temporary directories, and
		// reaches no network host (the tasks need none; the model API and the
		// kapi MCP server run outside the sandbox).
		"sandbox": map[string]any{
			"enabled":                  true,
			"autoAllowBashIfSandboxed": true,
			"allowUnsandboxedCommands": false,
			"filesystem": map[string]any{
				"allowWrite": []string{p.Paths.Root, p.TmpDir},
				"denyRead":   deny,
			},
			"network": map[string]any{"allowedDomains": []string{}, "strictAllowlist": true},
		},
	}
	settingsPath := filepath.Join(p.Paths.State, "claude-settings.json")
	if err := pairedWriteJSON(settingsPath, settings); err != nil {
		return err
	}
	p.Args = []string{
		"--print", "--output-format", "stream-json", "--verbose",
		"--model", p.Attempt.Host.Model, "--effort", p.Attempt.Host.Effort,
		"--setting-sources", "project", "--settings", settingsPath,
		"--no-session-persistence", "--permission-mode", "acceptEdits",
		"--tools", "Bash,Read,Edit,Write,Glob,Grep,Skill,ToolSearch",
		"--max-turns", strconv.Itoa(p.MaxTurns),
	}
	p.Env = append(p.Env, "CLAUDE_CODE_DISABLE_BUNDLED_SKILLS=1")
	p.Sandbox = "claude sandbox: writes limited to the cell and its TMPDIR, no network, unsandboxed fallback refused"
	return nil
}

// compareDenyRead are the places a cell's commands may not read: the
// developer's home (which holds this checkout and its evidence) and the
// temporary directories Claude Code shares between sessions. A cell inside
// one of them could not run its own commands, so it is refused.
func compareDenyRead(root string) ([]string, error) {
	deny := pairedSharedTemp()
	if home, err := os.UserHomeDir(); err == nil {
		deny = append([]string{home}, deny...)
	}
	resolved := pairedResolve(root)
	for _, dir := range deny {
		for _, candidate := range []string{root, resolved} {
			if candidate == dir || strings.HasPrefix(candidate, dir+string(filepath.Separator)) {
				return deny, fmt.Errorf("the cell %s lies under %s, which the sandbox denies; choose a cells directory outside it (for example /tmp/kapi-compare-cells)", root, dir)
			}
		}
	}
	return deny, nil
}

// prepareCompareCodex marks the cell's repository as trusted in the cell's
// own CODEX_HOME (the prompt a person accepts on first open), which is what
// makes Codex read the `.codex/config.toml` that `kapi init` wrote. AGENTS.md
// needs no trust. The kapi arm forwards the cell's kapi roots to the server.
func prepareCompareCodex(ctx context.Context, p *ComparePrepared, kapiBin string) error {
	ep := EvalPrepared{
		Session:    EvalSession{Host: p.Attempt.Host},
		Paths:      p.Paths,
		Wiring:     EvalWiring{Harness: []string{}},
		Executable: p.Executable,
		KapiBin:    compareCellKapi(p.Paths),
		Env:        p.Env,
		Blockers:   []string{},
	}
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
		p.Blockers = append(p.Blockers, "Codex file-backed subscription login unavailable")
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
	if err := os.WriteFile(filepath.Join(p.Paths.State, "codex", "config.toml"), []byte(compareCodexConfig(ep, p.TmpDir)), 0o600); err != nil {
		return err
	}
	p.Sandbox = "codex workspace-write: writable roots are the cell and its TMPDIR, no network, /tmp excluded"
	p.Args = []string{"exec", "--strict-config", "--ignore-rules", "--json", "--skip-git-repo-check"}
	if compareKapiArm(p.Attempt.Arm) {
		p.Args = append(p.Args, "-c", evalCodexForwardedEnv(p.Env))
		codex, err := probeEvalCodexWiring(ctx, ep)
		p.Codex = codex
		switch {
		case err != nil:
			p.Blockers = append(p.Blockers, "Codex MCP wiring: "+err.Error())
		case !codex.UnderTest:
			p.Blockers = append(p.Blockers, fmt.Sprintf("Codex starts %q from %s, not this checkout's build", codex.Command, codex.Resolved))
		}
	}
	p.Args = append(p.Args, "--model", p.Attempt.Host.Model, "--cd", p.Paths.Repo, "-")
	return nil
}

// compareCodexConfig is the cell's own Codex configuration: the evaluation's
// configuration with the sandbox in workspace-write mode, writable only in the
// cell and its temporary directory, with no network.
func compareCodexConfig(ep EvalPrepared, tmp string) string {
	config := evalCodexConfig(ep)
	config = strings.Replace(config, "sandbox_mode = \"danger-full-access\"\n", "sandbox_mode = \"workspace-write\"\n", 1)
	roots := []string{strconv.Quote(ep.Paths.Root)}
	if tmp != "" {
		roots = append(roots, strconv.Quote(tmp))
	}
	head, rest, _ := strings.Cut(config, "[features]\n")
	return head + "[sandbox_workspace_write]\nwritable_roots = [" + strings.Join(roots, ", ") + "]\nnetwork_access = false\nexclude_slash_tmp = true\nexclude_tmpdir_env_var = false\n[features]\n" + rest
}

// compareRulesFiles lists the AGENTS.md and CLAUDE.md files under a
// repository, outside the wiring folders.
func compareRulesFiles(repo string) ([]string, error) {
	found := []string{}
	err := filepath.WalkDir(repo, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(repo, name)
		if err != nil {
			return err
		}
		if entry.IsDir() && rel != "." && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		if !entry.IsDir() && (entry.Name() == "AGENTS.md" || entry.Name() == "CLAUDE.md") {
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	slices.Sort(found)
	return found, err
}
