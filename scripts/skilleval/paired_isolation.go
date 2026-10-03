package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func preparePairedAgent(ctx context.Context, launch PairedLaunch) (PairedPrepared, error) {
	p := PairedPrepared{Launch: launch, Args: []string{}, Env: []string{}, Blockers: []string{}, IsolationNotes: []string{}}
	if launch.Agent.Host != "codex" && launch.Agent.Host != "claude" {
		return p, errors.New("host must be codex or claude")
	}
	arm, err := pairedArmFor(launch.Condition)
	if err != nil {
		return p, err
	}
	if launch.Agent.Model == "" || launch.Agent.Effort == "" {
		return p, errors.New("explicit model and effort required")
	}
	if !filepath.IsAbs(launch.Workspace) || !filepath.IsAbs(launch.StateDir) {
		return p, errors.New("absolute workspace and state paths required")
	}
	if rel, err := filepath.Rel(launch.Workspace, launch.StateDir); err != nil || rel == "." || !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return p, errors.New("state directory must be outside workspace")
	}
	for _, dir := range []string{launch.StateDir, filepath.Join(launch.StateDir, "home"), filepath.Join(launch.StateDir, "bin"), filepath.Join(launch.StateDir, "codex"), filepath.Join(launch.StateDir, "claude")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return p, err
		}
	}
	tmp, err := makePairedCellTmp()
	if err != nil {
		return p, err
	}
	launch.TmpDir = tmp
	p.Launch.TmpDir = tmp
	// The fixture's context reaches a gate through the store, so it is read in
	// before the agent starts and every arm with a project works from the
	// same context. A cell with no project holds no recipe to read it from.
	if !arm.noProject() {
		if err := readPairedContext(ctx, launch.Workspace, launch.KapiBin); err != nil {
			return p, err
		}
	}
	if len(arm.Executables) > 0 && launch.KapiBin == "" {
		p.Blockers = append(p.Blockers, "bin/kapi from this tree is missing: run make build")
	}
	preparePairedMCPReadiness(ctx, &p)
	executable, err := exec.LookPath(launch.Agent.Host)
	if err != nil {
		p.Blockers = append(p.Blockers, "agent executable unavailable: "+launch.Agent.Host)
		return p, nil
	}
	p.Executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return p, err
	}
	version, err := exec.CommandContext(ctx, p.Executable, "--version").Output()
	if err != nil {
		p.Blockers = append(p.Blockers, "agent version probe failed")
		return p, nil
	}
	p.Version = strings.TrimSpace(string(version))
	if launch.KapiBin != "" && (len(arm.Executables) > 0 || arm.MCP) {
		cell, err := linkPairedKapi(launch)
		if err != nil {
			return p, fmt.Errorf("link kapi into the cell: %w", err)
		}
		launch.CellKapi = cell
		p.Launch.CellKapi = cell
	}
	if err := pairedToolPath(launch); err != nil {
		return p, err
	}
	p.Env = pairedEnvironment(launch)
	if err := installPairedSkill(launch, arm); err != nil {
		return p, fmt.Errorf("install the %s skill: %w", arm.Skill, err)
	}
	switch launch.Agent.Host {
	case "claude":
		err = preparePairedClaude(ctx, &p)
	case "codex":
		err = preparePairedCodex(ctx, &p)
	}
	if err != nil {
		return p, err
	}
	// Last, so the commit holds the project as the agent finds it and
	// `git status` starts clean.
	if err := initPairedGit(ctx, launch); err != nil {
		return p, err
	}
	toolNote := "Fresh personal configuration; only the assigned kapi integration is discoverable. Ordinary shell and file tools remain available."
	if launch.NoTools {
		toolNote = "Fixed-input review prohibits tools. Claude disables its tool set; both host transcripts reject observed tool use. This is a protocol control, not a hostile-code boundary."
	}
	p.IsolationNotes = append(p.IsolationNotes,
		toolNote,
		"Interface isolation plus transcript audit is accidental-contamination control, not a hostile-code boundary. Absolute binary paths or indirect shell execution remain possible; detected wrong-route attempts are invalid.",
		"Subscription quota is not inferred from token counts; the runner must pause after its configured live-attempt batch.")
	return p, nil
}

func pairedToolPath(launch PairedLaunch) error {
	// A private PATH retains ordinary local editing tools, without inheriting
	// arbitrary developer executables, aliases, shell startup files or kapi.
	names := []string{"bash", "sh", "zsh", "cat", "cp", "mv", "rm", "mkdir", "ls", "pwd", "find", "sed", "awk", "grep", "head", "tail", "wc", "sort", "uniq", "cut", "tr", "diff", "git", "rg", "python3", "jq", "file", "which", "env", "printf", "touch", "date", "node", "unzip", "zip", "tar", "perl", "ruby", "xargs", "tee", "basename", "dirname"}
	for _, name := range names {
		source := pairedSystemTool(name)
		if source == "" {
			continue
		}
		destination := filepath.Join(launch.StateDir, "bin", name)
		if _, err := os.Lstat(destination); err == nil {
			continue
		}
		if err := os.Symlink(source, destination); err != nil {
			return err
		}
	}
	arm, err := pairedArmFor(launch.Condition)
	if err != nil {
		return err
	}
	for _, name := range arm.Executables {
		if launch.KapiBin == "" {
			return fmt.Errorf("%s requires the kapi binary built from this tree", launch.Condition)
		}
		destination := filepath.Join(launch.StateDir, "bin", name)
		if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
			continue
		}
		if !arm.Project && name == pairedFilesAlias {
			// The alias is the binary under another name: argv[0] selects the
			// project-free root, which turns discovery off itself.
			if err := os.Symlink(launch.agentKapi(), destination); err != nil {
				return err
			}
			continue
		}
		if !arm.Project {
			// kapi itself with discovery off: no upward walk, and no
			// KAPI_PROJECT to bind a recipe.
			wrapper := "#!/bin/sh\nexport KAPI_NO_PROJECT=1\nunset KAPI_PROJECT\nexec " + pairedShellQuote(launch.agentKapi()) + " \"$@\"\n"
			if err := os.WriteFile(destination, []byte(wrapper), 0o700); err != nil {
				return err
			}
			continue
		}
		// Some commands have no recipe flag, such as ksed, which finds the
		// project its files belong to. Bind the fixture through the
		// environment without changing arguments. A nonempty KAPI_PROJECT
		// resolves before any upward walk.
		wrapper := "#!/bin/sh\nexport KAPI_NO_PROJECT=''\nexport KAPI_PROJECT=" +
			pairedShellQuote(filepath.Join(launch.Workspace, "kapi.yaml")) +
			"\nexec " + pairedShellQuote(launch.agentKapi()) + " \"$@\"\n"
		if err := os.WriteFile(destination, []byte(wrapper), 0o700); err != nil {
			return err
		}
	}
	return nil
}

func pairedEnvironment(launch PairedLaunch) []string {
	env := []string{"PATH=" + filepath.Join(launch.StateDir, "bin"), "HOME=" + filepath.Join(launch.StateDir, "home"), "SHELL=/bin/sh", "LANG=en_US.UTF-8", "TERM=dumb", "NO_COLOR=1"}
	if launch.TmpDir != "" {
		// Each cell has a temporary directory of its own. Claude Code's
		// sandbox otherwise gives every session /tmp/claude, or the
		// developer's /tmp/claude-<uid>, and a file one session left there
		// is readable by the next.
		env = append(env, "TMPDIR="+launch.TmpDir, "CLAUDE_CODE_TMPDIR="+launch.TmpDir)
	}
	env = append(env, isolationEnv(launch.Workspace)...)
	// kapi records and polices what the agent's shell runs as an agent's, on
	// both hosts alike, rather than relying on each host's marker variable
	// reaching the shell (host.ResolveCommandActor).
	env = append(env, "KAPI_ACTOR=agent")
	// These are agent runtime settings, not API provider configuration. Provider
	// keys, endpoint overrides and alternate billing routes are not inherited.
	env = append(env, "CLAUDE_CONFIG_DIR="+filepath.Join(launch.StateDir, "claude"), "CODEX_HOME="+filepath.Join(launch.StateDir, "codex"), "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "ENABLE_CLAUDEAI_MCP_SERVERS=false", "CLAUDE_CODE_DISABLE_AUTO_MEMORY=1", "DISABLE_AUTOUPDATER=1")
	// Claude Code bundles skills of its own. Off in every arm, so the arms
	// differ only in kapi's skill; Codex's system skills are the same four in
	// every arm.
	env = append(env, "CLAUDE_CODE_DISABLE_BUNDLED_SKILLS=1")
	return env
}

// pairedClaudeHostSkills are the skills Claude Code shows that
// CLAUDE_CODE_DISABLE_BUNDLED_SKILLS leaves on: its doctor and a skill of a
// plugin built into it. The surface probe fails a cell that shows any other,
// so a Claude Code release that adds one is caught before a session runs.
var pairedClaudeHostSkills = []string{"doctor", "plugin-authoring"}

// pairedCellTmpLimit is the longest per-user temporary directory Claude Code
// accepts under CLAUDE_CODE_TMPDIR before it falls back to the shared
// /tmp/claude-<uid> (it keeps sockets there).
const pairedCellTmpLimit = 44

// makePairedCellTmp creates a cell's temporary directory. It sits directly in
// /tmp because Claude Code's limit allows no deeper path; the runner moves it
// into the cell's state directory when the session ends.
func makePairedCellTmp() (string, error) {
	dir, err := os.MkdirTemp("/tmp", "kpe-")
	if err != nil {
		return "", err
	}
	perUser := filepath.Join(pairedResolve(dir), "claude-"+strconv.Itoa(os.Getuid()))
	if len(perUser) > pairedCellTmpLimit {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("the cell's temporary directory %s is too long for Claude Code (%d bytes, limit %d)", perUser, len(perUser), pairedCellTmpLimit)
	}
	return dir, nil
}

// pairedSharedTemp are the temporary directories Claude Code shares between
// sessions when nothing gives it its own: /tmp/claude, which its sandbox
// leaves writable, and the per-user /tmp/claude-<uid>.
func pairedSharedTemp() []string {
	uid := "claude-" + strconv.Itoa(os.Getuid())
	return []string{"/tmp/claude", "/private/tmp/claude", "/tmp/" + uid, "/private/tmp/" + uid}
}

// settlePairedCellTmp moves a finished cell's temporary directory into its
// state directory, so what the agent wrote there stays with the attempt and
// /tmp keeps nothing of it. It returns where the files now are.
func settlePairedCellTmp(launch PairedLaunch) string {
	if launch.TmpDir == "" {
		return ""
	}
	destination := filepath.Join(launch.StateDir, "tmp")
	if err := os.Rename(launch.TmpDir, destination); err != nil {
		return launch.TmpDir
	}
	return destination
}

// discardPairedCellTmp removes the temporary directory of a cell that will
// not run.
func discardPairedCellTmp(p PairedPrepared) {
	if dir := p.Launch.TmpDir; dir != "" && strings.HasPrefix(filepath.Base(dir), "kpe-") {
		_ = os.RemoveAll(dir)
	}
}

func preparePairedClaude(ctx context.Context, p *PairedPrepared) error {
	token, err := pairedClaudeSubscriptionToken(ctx, p.Launch.Timeout)
	if err != nil {
		p.Blockers = append(p.Blockers, err.Error())
	} else {
		p.Env = append(p.Env, "CLAUDE_CODE_OAUTH_TOKEN="+token)
		p.AuthMode = "claude.ai subscription"
	}
	arm, err := pairedArmFor(p.Launch.Condition)
	if err != nil {
		return err
	}
	allow := []string{"Bash", "Read", "Edit", "Write", "Glob", "Grep"}
	if arm.Skill != "" {
		allow = append(allow, "Skill("+arm.Skill+")")
	}
	if arm.MCP {
		allow = append(allow, "mcp__kapi__*")
	}
	filesystem := map[string]any{
		"denyRead":  pairedClaudeDenyRead(p.Launch),
		"allowRead": []string{p.Launch.Workspace, filepath.Join(p.Launch.StateDir, "bin"), filepath.Join(p.Launch.StateDir, "kapi")},
	}
	if p.Launch.TmpDir != "" {
		filesystem["allowRead"] = append(filesystem["allowRead"].([]string), p.Launch.TmpDir)
		filesystem["allowWrite"] = []string{p.Launch.TmpDir}
	}
	settings := map[string]any{
		"autoMemoryEnabled": false,
		"permissions":       map[string]any{"defaultMode": "acceptEdits", "blockReadsOutsideWorkingDirectories": true, "allow": allow},
		"sandbox":           map[string]any{"enabled": true, "autoAllowBashIfSandboxed": true, "allowUnsandboxedCommands": false, "filesystem": filesystem, "network": map[string]any{"allowedDomains": []string{}, "strictAllowlist": true}},
	}
	settingsPath := filepath.Join(p.Launch.StateDir, "claude-settings.json")
	if err := pairedWriteJSON(settingsPath, settings); err != nil {
		return err
	}
	// Claude Code shows a few skills of its own that its bundled-skill switch
	// leaves on. Claude reads skill overrides from the project's settings
	// only, so the workspace carries them; git ignores the folder.
	project := filepath.Join(p.Launch.Workspace, ".claude")
	if err := os.MkdirAll(project, 0o700); err != nil {
		return err
	}
	overrides := map[string]string{}
	for _, skill := range pairedClaudeHostSkills {
		overrides[skill] = "off"
	}
	if err := pairedWriteJSON(filepath.Join(project, "settings.json"), map[string]any{"skillOverrides": overrides}); err != nil {
		return err
	}
	mcp := map[string]any{"mcpServers": map[string]any{}}
	if arm.MCP {
		if p.Launch.KapiBin == "" {
			return errors.New("mcp requires kapi binary")
		}
		mcp["mcpServers"] = map[string]any{"kapi": map[string]any{"command": p.Launch.agentKapi(), "args": []string{"-p", filepath.Join(p.Launch.Workspace, "kapi.yaml"), "mcp"}, "env": pairedKapiEnv(p.Launch.Workspace)}}
	}
	mcpPath := filepath.Join(p.Launch.StateDir, "claude-mcp.json")
	if err := pairedWriteJSON(mcpPath, mcp); err != nil {
		return err
	}
	// Only the project source is read: it is where a skill installed in the
	// workspace is discovered, and a workspace holds nothing else Claude reads.
	// The user and local sources stay excluded in every arm, and the strict,
	// explicit MCP configuration replaces every other server.
	p.Args = []string{"--print", "--output-format", "stream-json", "--verbose", "--model", p.Launch.Agent.Model, "--effort", p.Launch.Agent.Effort, "--setting-sources", "project", "--settings", settingsPath, "--strict-mcp-config", "--mcp-config", mcpPath, "--no-session-persistence", "--permission-mode", "acceptEdits", "--tools", "Bash,Read,Edit,Write,Glob,Grep,Skill,ToolSearch"}
	if p.Launch.NoTools {
		for i := range p.Args {
			if p.Args[i] == "--tools" {
				p.Args[i+1] = ""
			}
		}
	}
	if arm.Skill == "" {
		p.Args = append(p.Args, "--disable-slash-commands")
	}
	if p.Launch.MaxTurns > 0 {
		p.Args = append(p.Args, "--max-turns", strconv.Itoa(p.Launch.MaxTurns))
	}
	p.IsolationNotes = append(p.IsolationNotes, "Claude sandbox settings prohibit unsandboxed command fallback. Native host application of these settings requires live smoke audit; offline preparation does not assert runtime confinement.")
	return nil
}

func preparePairedCodex(ctx context.Context, p *PairedPrepared) error {
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
		destination := filepath.Join(p.Launch.StateDir, "codex", "auth.json")
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
	var config strings.Builder
	// Writes go to the workspace and the cell's own TMPDIR. /tmp is shared by
	// every session, so a file one left there could reach another.
	config.WriteString("forced_login_method = \"chatgpt\"\napproval_policy = \"never\"\nsandbox_mode = \"workspace-write\"\nweb_search = \"disabled\"\nallow_login_shell = false\nmodel_reasoning_effort = " + strconv.Quote(p.Launch.Agent.Effort) + "\n[sandbox_workspace_write]\nexclude_slash_tmp = true\nexclude_tmpdir_env_var = false\n[features]\napps = false\nplugins = false\nhooks = false\nmulti_agent = false\nbrowser_use = false\ncomputer_use = false\nimage_generation = false\nshell_snapshot = false\n[shell_environment_policy]\ninherit = \"none\"\n[shell_environment_policy.set]\n")
	for _, pair := range p.Env {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			config.WriteString(strconv.Quote(key) + " = " + strconv.Quote(value) + "\n")
		}
	}
	if p.Launch.Condition == "mcp" {
		if p.Launch.KapiBin == "" {
			return errors.New("mcp requires kapi binary")
		}
		// This isolated server operates on the authorized fixture. Preapprove
		// its tools explicitly: approval_policy=never cannot resolve a prompt.
		config.WriteString("[mcp_servers.kapi]\ndefault_tools_approval_mode = \"approve\"\ncommand = " + strconv.Quote(p.Launch.agentKapi()) + "\nargs = [\"-p\", " + strconv.Quote(filepath.Join(p.Launch.Workspace, "kapi.yaml")) + ", \"mcp\"]\n[mcp_servers.kapi.env]\n")
		for key, value := range pairedKapiEnv(p.Launch.Workspace) {
			config.WriteString(strconv.Quote(key) + " = " + strconv.Quote(value) + "\n")
		}
	}
	if err := os.WriteFile(filepath.Join(p.Launch.StateDir, "codex", "config.toml"), []byte(config.String()), 0o600); err != nil {
		return err
	}
	p.Args = []string{"exec", "--strict-config", "--ignore-rules", "--json", "--skip-git-repo-check", "--model", p.Launch.Agent.Model, "--cd", p.Launch.Workspace, "-"}
	p.IsolationNotes = append(p.IsolationNotes, "Codex workspace-write confines writes. Installed CLI deny-read profile probe did not exclude its canary; read containment is not claimed. Rollout turn metadata verifies selected model identity.")
	return nil
}

// pairedTokenMargin is how long a keychain token must stay valid beyond a
// session's limit. The cell receives the token as a fixed value it cannot
// refresh, so a token that expires during a session fails it.
const pairedTokenMargin = 15 * time.Minute

func pairedClaudeSubscriptionToken(ctx context.Context, session time.Duration) (string, error) {
	if token := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); token != "" {
		return token, nil
	}
	if runtime.GOOS != "darwin" {
		return "", errors.New("claude subscription OAuth unavailable: export CLAUDE_CODE_OAUTH_TOKEN through a private credential launcher")
	}
	command := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", "Claude Code-credentials", "-w")
	data, err := command.Output()
	if err != nil {
		return "", errors.New("existing Claude subscription credential could not be read")
	}
	defer clear(data)
	return pairedClaudeCredentialToken(data, time.Now(), session+pairedTokenMargin)
}

// pairedClaudeCredentialToken returns the keychain's access token when it
// stays valid for at least need.
func pairedClaudeCredentialToken(data []byte, now time.Time, need time.Duration) (string, error) {
	credential := map[string]any{}
	if json.Unmarshal(data, &credential) != nil {
		return "", errors.New("existing Claude credential has an unsupported shape")
	}
	oauth := pairedObject(credential, "claudeAiOauth")
	// Keychain expiry is Unix milliseconds. An opaque externally supplied token
	// has no local expiry evidence and is handled by the caller unchanged.
	if expiry, known := oauth["expiresAt"].(float64); known {
		left := time.UnixMilli(int64(expiry)).Sub(now)
		if left <= 0 {
			return "", errors.New("existing Claude subscription OAuth has expired; refresh the Claude login before launching a stage")
		}
		if left < need {
			return "", fmt.Errorf("the Claude subscription OAuth token expires in %s, and a session needs %s; "+
				"export a long-lived CLAUDE_CODE_OAUTH_TOKEN from claude setup-token, or refresh the login, before launching",
				left.Round(time.Minute), need.Round(time.Minute))
		}
	}
	token := pairedString(oauth, "accessToken")
	if token == "" {
		return "", errors.New("claude subscription credential does not contain an OAuth access token")
	}
	return token, nil
}
func pairedKapiEnv(workspace string) map[string]string {
	result := map[string]string{}
	for _, pair := range isolationEnv(workspace) {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}
func pairedWriteJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o600)
}

func pairedShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
