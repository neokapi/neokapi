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
)

func preparePairedAgent(ctx context.Context, launch PairedLaunch) (PairedPrepared, error) {
	p := PairedPrepared{Launch: launch, Args: []string{}, Env: []string{}, Blockers: []string{}, IsolationNotes: []string{}}
	if launch.Agent.Host != "codex" && launch.Agent.Host != "claude" {
		return p, errors.New("host must be codex or claude")
	}
	if launch.Condition != "baseline" && launch.Condition != "skill-cli" && launch.Condition != "mcp" {
		return p, errors.New("condition must be baseline, skill-cli or mcp")
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
	if err := pairedToolPath(launch); err != nil {
		return p, err
	}
	p.Env = pairedEnvironment(launch)
	if launch.Condition == "skill-cli" {
		destination := filepath.Join(launch.Workspace, ".claude", "skills", "kapi")
		if launch.Agent.Host == "codex" {
			destination = filepath.Join(launch.Workspace, ".agents", "skills", "kapi")
		}
		if err := copyTree(filepath.Join(launch.RepoRoot, "cli", "skills", "data", "kapi"), destination); err != nil {
			return p, fmt.Errorf("install shipped skill: %w", err)
		}
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
	p.IsolationNotes = append(p.IsolationNotes,
		"Fresh personal configuration; only the assigned kapi integration is discoverable. Ordinary shell and file tools remain available.",
		"Interface isolation plus transcript audit is accidental-contamination control, not a hostile-code boundary. Absolute binary paths or indirect shell execution remain possible; detected wrong-route attempts are invalid.",
		"Subscription quota is not inferred from token counts; the runner must pause after its configured live-attempt batch.")
	return p, nil
}

func pairedToolPath(launch PairedLaunch) error {
	// A private PATH retains ordinary local editing tools, without inheriting
	// arbitrary developer executables, aliases, shell startup files or kapi.
	names := []string{"bash", "sh", "zsh", "cat", "cp", "mv", "rm", "mkdir", "ls", "pwd", "find", "sed", "awk", "grep", "head", "tail", "wc", "sort", "uniq", "cut", "tr", "diff", "git", "rg", "python3", "jq", "file", "which", "env", "printf", "touch", "date", "node", "unzip", "zip", "tar", "perl", "ruby", "xargs", "tee", "basename", "dirname"}
	for _, name := range names {
		source, err := exec.LookPath(name)
		if err != nil {
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
	if launch.Condition == "skill-cli" {
		if launch.KapiBin == "" {
			return errors.New("skill-cli requires kapi binary")
		}
		destination := filepath.Join(launch.StateDir, "bin", "kapi")
		if _, err := os.Lstat(destination); errors.Is(err, os.ErrNotExist) {
			wrapper := "#!/bin/sh\nexec " + pairedShellQuote(launch.KapiBin) + " -p " + pairedShellQuote(filepath.Join(launch.Workspace, "kapi.yaml")) + " \"$@\"\n"
			return os.WriteFile(destination, []byte(wrapper), 0o700)
		}
	}
	return nil
}

func pairedEnvironment(launch PairedLaunch) []string {
	env := []string{"PATH=" + filepath.Join(launch.StateDir, "bin"), "HOME=" + filepath.Join(launch.StateDir, "home"), "SHELL=/bin/sh", "LANG=en_US.UTF-8", "TERM=dumb", "NO_COLOR=1"}
	env = append(env, isolationEnv(launch.Workspace)...)
	// These are agent runtime settings, not API provider configuration. Provider
	// keys, endpoint overrides and alternate billing routes are not inherited.
	env = append(env, "CLAUDE_CONFIG_DIR="+filepath.Join(launch.StateDir, "claude"), "CODEX_HOME="+filepath.Join(launch.StateDir, "codex"), "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "ENABLE_CLAUDEAI_MCP_SERVERS=false", "CLAUDE_CODE_DISABLE_AUTO_MEMORY=1", "DISABLE_AUTOUPDATER=1")
	return env
}

func preparePairedClaude(ctx context.Context, p *PairedPrepared) error {
	token, err := pairedClaudeSubscriptionToken(ctx)
	if err != nil {
		p.Blockers = append(p.Blockers, err.Error())
	} else {
		p.Env = append(p.Env, "CLAUDE_CODE_OAUTH_TOKEN="+token)
		p.AuthMode = "claude.ai subscription"
	}
	settings := map[string]any{
		"autoMemoryEnabled": false,
		"permissions":       map[string]any{"defaultMode": "acceptEdits", "blockReadsOutsideWorkingDirectories": true, "allow": []string{"Bash", "Read", "Edit", "Write", "Glob", "Grep", "mcp__kapi__*"}},
		"sandbox":           map[string]any{"enabled": true, "autoAllowBashIfSandboxed": true, "allowUnsandboxedCommands": false, "filesystem": map[string]any{"denyRead": []string{p.Launch.StateDir, p.Launch.RepoRoot}, "allowRead": []string{p.Launch.Workspace, filepath.Join(p.Launch.StateDir, "bin"), p.Launch.KapiBin}}, "network": map[string]any{"allowedDomains": []string{}, "strictAllowlist": true}},
	}
	settingsPath := filepath.Join(p.Launch.StateDir, "claude-settings.json")
	if err := pairedWriteJSON(settingsPath, settings); err != nil {
		return err
	}
	mcp := map[string]any{"mcpServers": map[string]any{}}
	if p.Launch.Condition == "mcp" {
		if p.Launch.KapiBin == "" {
			return errors.New("mcp requires kapi binary")
		}
		mcp["mcpServers"] = map[string]any{"kapi": map[string]any{"command": p.Launch.KapiBin, "args": []string{"-p", filepath.Join(p.Launch.Workspace, "kapi.yaml"), "mcp"}, "env": pairedKapiEnv(p.Launch.Workspace)}}
	}
	mcpPath := filepath.Join(p.Launch.StateDir, "claude-mcp.json")
	if err := pairedWriteJSON(mcpPath, mcp); err != nil {
		return err
	}
	p.Args = []string{"--print", "--output-format", "stream-json", "--verbose", "--model", p.Launch.Agent.Model, "--effort", p.Launch.Agent.Effort, "--setting-sources", "", "--settings", settingsPath, "--strict-mcp-config", "--mcp-config", mcpPath, "--no-session-persistence", "--permission-mode", "acceptEdits", "--tools", "Bash,Read,Edit,Write,Glob,Grep,Skill,ToolSearch"}
	if p.Launch.Condition != "skill-cli" {
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
	config := "forced_login_method = \"chatgpt\"\napproval_policy = \"never\"\nsandbox_mode = \"workspace-write\"\nweb_search = \"disabled\"\nallow_login_shell = false\nmodel_reasoning_effort = " + strconv.Quote(p.Launch.Agent.Effort) + "\n[features]\napps = false\nplugins = false\nhooks = false\nmulti_agent = false\nbrowser_use = false\ncomputer_use = false\nimage_generation = false\nshell_snapshot = false\n[shell_environment_policy]\ninherit = \"none\"\n[shell_environment_policy.set]\n"
	for _, pair := range p.Env {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			config += strconv.Quote(key) + " = " + strconv.Quote(value) + "\n"
		}
	}
	if p.Launch.Condition == "mcp" {
		if p.Launch.KapiBin == "" {
			return errors.New("mcp requires kapi binary")
		}
		config += "[mcp_servers.kapi]\ncommand = " + strconv.Quote(p.Launch.KapiBin) + "\nargs = [\"-p\", " + strconv.Quote(filepath.Join(p.Launch.Workspace, "kapi.yaml")) + ", \"mcp\"]\n[mcp_servers.kapi.env]\n"
		for key, value := range pairedKapiEnv(p.Launch.Workspace) {
			config += strconv.Quote(key) + " = " + strconv.Quote(value) + "\n"
		}
	}
	if err := os.WriteFile(filepath.Join(p.Launch.StateDir, "codex", "config.toml"), []byte(config), 0o600); err != nil {
		return err
	}
	p.Args = []string{"exec", "--strict-config", "--ignore-rules", "--json", "--skip-git-repo-check", "--model", p.Launch.Agent.Model, "--cd", p.Launch.Workspace, "-"}
	p.IsolationNotes = append(p.IsolationNotes, "Codex workspace-write confines writes. Installed CLI deny-read profile probe did not exclude its canary; read containment is not claimed. Rollout turn metadata verifies selected model identity.")
	return nil
}

func pairedClaudeSubscriptionToken(ctx context.Context) (string, error) {
	if token := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); token != "" {
		return token, nil
	}
	if runtime.GOOS != "darwin" {
		return "", errors.New("Claude subscription OAuth unavailable: export CLAUDE_CODE_OAUTH_TOKEN through a private credential launcher")
	}
	command := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", "Claude Code-credentials", "-w")
	data, err := command.Output()
	if err != nil {
		return "", errors.New("existing Claude subscription credential could not be read")
	}
	defer clear(data)
	credential := map[string]any{}
	if json.Unmarshal(data, &credential) != nil {
		return "", errors.New("existing Claude credential has an unsupported shape")
	}
	oauth := pairedObject(credential, "claudeAiOauth")
	token := pairedString(oauth, "accessToken")
	if token == "" {
		return "", errors.New("Claude subscription credential does not contain an OAuth access token")
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
