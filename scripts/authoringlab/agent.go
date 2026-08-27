package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Driving an agent that reads the repository before it writes.
//
// The first version of this lab handed a model a 600-word brief and asked for a
// document. That measures expansion, not documentation: every fact was already
// in front of it, in the order it needed them. A person writing docs for a tool
// reads the tool.
//
// So the subject is a real source tree, pinned (scripts/fetch-lab-repo.sh), and
// the agent is given tools and left to read. What it looked at is recorded
// alongside what it wrote, because "which files did it open" is most of the
// difference between a document grounded in the code and one assembled from
// what the model already believed about ripgrep.

// maxAgentTurns bounds one document.
//
// Reading a 212-file tree and writing 800 words takes tens of turns. The cap is
// against a run that has lost the thread, not against the work: on the skill
// eval, gates that failed at eight turns passed at forty, which measured the
// budget rather than the agent.
const maxAgentTurns = 60

// AgentRun is what one document cost and what produced it.
type AgentRun struct {
	// Text is the document.
	Text string `json:"text"`
	// FilesRead is every path the agent opened, in order of first read. The
	// evidence that it read the repository rather than recalling it.
	//
	// Counted from the Read tool AND from read-like shell commands, because
	// models do not agree on how to open a file: opus-5 reads a source tree
	// with `ls` and `cat` and never calls Read at all. Counting only the tool
	// recorded zero files for a run that spent 1.7M tokens of context on the
	// repository, which is a fact about the parser published as a fact about
	// the model.
	FilesRead []string `json:"filesRead,omitempty"`
	// Searches is every grep or glob pattern it ran, which shows what it went
	// looking for before it found anything.
	Searches []string `json:"searches,omitempty"`
	// ToolCalls counts every tool by name. Unlike FilesRead this needs no
	// interpretation, so it is the number to check when FilesRead looks wrong.
	ToolCalls map[string]int `json:"toolCalls,omitempty"`
	// Messages, Turns and DurationMS say what the reading cost.
	Messages   int   `json:"messages"`
	DurationMS int64 `json:"durationMs"`
	// InputTokens is context consumed, including what was served from cache:
	// the number this redesign exists to move. OutputTokens and CostUSD are
	// what it cost to consume it.
	InputTokens  int     `json:"inputTokens,omitempty"`
	OutputTokens int     `json:"outputTokens,omitempty"`
	CostUSD      float64 `json:"costUsd,omitempty"`
	Model        string  `json:"model,omitempty"`
	// Sandboxed says whether the OS confined this run. Recorded rather than
	// assumed: the harness runs unconfined where the platform has no seatbelt,
	// and a reader should be able to tell which they are looking at.
	Sandboxed bool   `json:"sandboxed"`
	Err       string `json:"error,omitempty"`
}

// runAgent gives the agent the repository and a task, and reads back what it
// wrote and what it read.
func runAgent(ctx context.Context, opts AgentOpts) AgentRun {
	started := time.Now()
	r := AgentRun{}

	// A throwaway home per run: the isolation contract, so an in-repo agent
	// cannot reach the developer's kapi config, plugins or caches.
	home, err := os.MkdirTemp("", "authoringlab-")
	if err != nil {
		r.Err = err.Error()
		return r
	}
	defer os.RemoveAll(home)

	out := filepath.Join(home, "DOCUMENT.md")
	prompt := opts.Prompt + "\n\nWrite the finished document to " + out +
		" and nothing else to disk. Read whatever you need from the source tree first."

	args := []string{
		"-p", prompt,
		"--max-turns", strconv.Itoa(maxAgentTurns),
		"--permission-mode", "bypassPermissions",
		"--output-format", "stream-json",
		"--verbose",
		"--model", opts.Model,
	}
	if opts.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.SystemPrompt)
	}

	// Confined by Claude Code's own sandbox: bypassPermissions keeps a headless
	// run from hanging on a prompt, and the sandbox decides what the process
	// can actually reach. The two are layered, not traded off. See sandbox.go.
	sandbox, confined := sandboxArgs(home)
	args = append(args, sandbox...)
	r.Sandboxed = confined

	cmd := exec.CommandContext(ctx, opts.ClaudeBin, args...)
	cmd.Dir = opts.RepoDir
	cmd.Env = append(agentEnv(), isolationEnv(home)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.Err = err.Error()
		return r
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		r.Err = err.Error()
		return r
	}
	parseAgentStream(stdout, &r, opts.RepoDir)
	_ = cmd.Wait()
	r.DurationMS = time.Since(started).Milliseconds()

	body, err := os.ReadFile(out)
	if err != nil {
		// A run that hit the turn cap mid-task leaves no file. Say so rather
		// than publishing an empty document as a result.
		if r.Err == "" {
			r.Err = "the agent wrote no document (turn cap, or it answered in chat instead)"
		}
		return r
	}
	r.Text = strings.TrimSpace(string(body))
	return r
}

// AgentOpts is one run's inputs.
type AgentOpts struct {
	ClaudeBin string
	RepoDir   string
	Model     string
	Prompt    string
	// SystemPrompt is the governance, appended to the agent's own system
	// prompt exactly as `kapi voice guide` output would be pasted into one.
	// Empty in the bare arm.
	SystemPrompt string
}

// parseAgentStream reads the tool calls to learn what the agent looked at.
func parseAgentStream(rd io.Reader, out *AgentRun, repoDir string) {
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20)

	seenFile := map[string]bool{}
	for sc.Scan() {
		var ev struct {
			Type    string `json:"type"`
			Message struct {
				Model   string `json:"model"`
				Content []struct {
					Type  string          `json:"type"`
					Name  string          `json:"name"`
					Input json.RawMessage `json:"input"`
				} `json:"content"`
			} `json:"message"`
			// The run's totals arrive on the final `result` event. A per-message
			// `usage.input_tokens` counts only what was not served from cache, so
			// summing those reported ten tokens for a run that read five files.
			Usage struct {
				InputTokens         int `json:"input_tokens"`
				CacheReadTokens     int `json:"cache_read_input_tokens"`
				CacheCreationTokens int `json:"cache_creation_input_tokens"`
				OutputTokens        int `json:"output_tokens"`
			} `json:"usage"`
			TotalCostUSD float64 `json:"total_cost_usd"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.Type == "result" {
			out.InputTokens = ev.Usage.InputTokens + ev.Usage.CacheReadTokens + ev.Usage.CacheCreationTokens
			out.OutputTokens = ev.Usage.OutputTokens
			out.CostUSD = ev.TotalCostUSD
			continue
		}
		if ev.Type != "assistant" {
			continue
		}
		out.Messages++
		if ev.Message.Model != "" {
			out.Model = ev.Message.Model
		}

		for _, c := range ev.Message.Content {
			if c.Type != "tool_use" {
				continue
			}
			if out.ToolCalls == nil {
				out.ToolCalls = map[string]int{}
			}
			out.ToolCalls[c.Name]++

			addFile := func(p string) {
				p = relToRepo(p, repoDir)
				// "." is the tree itself, which a Read on the directory
				// produces. A directory is not a file read.
				if p == "" || p == "." || seenFile[p] {
					return
				}
				seenFile[p] = true
				out.FilesRead = append(out.FilesRead, p)
			}

			switch c.Name {
			case "Read":
				var in struct {
					FilePath string `json:"file_path"`
				}
				if json.Unmarshal(c.Input, &in) == nil && in.FilePath != "" {
					addFile(in.FilePath)
				}
			case "Grep", "Glob":
				var in struct {
					Pattern string `json:"pattern"`
				}
				if json.Unmarshal(c.Input, &in) == nil && in.Pattern != "" {
					out.Searches = append(out.Searches, in.Pattern)
				}
			case "Bash":
				var in struct {
					Command string `json:"command"`
				}
				if json.Unmarshal(c.Input, &in) != nil {
					break
				}
				for _, f := range filesFromShell(in.Command, repoDir) {
					addFile(f)
				}
			}
		}
	}
}

// readCommands are the shell commands that open a file to look at it.
//
// Deliberately not `ls`, which lists a directory without reading anything, and
// not `rg`/`grep`, whose subject is a pattern rather than a file — those are
// searches and are counted as such.
var readCommands = map[string]bool{
	"cat": true, "head": true, "tail": true, "less": true, "more": true,
	"bat": true, "sed": true, "awk": true, "wc": true, "kcat": true,
}

// filesFromShell picks the files a shell command opened.
//
// A heuristic over a command line, and it says so: a token is taken as a file
// when it follows a reading command, is not a flag, and names something that
// exists in the tree. Testing the filesystem is what keeps `head -20` from
// being recorded as a file, and it is why this cannot invent a path the agent
// never touched. It can still miss one — a path built by a variable, a file
// behind a pipe — so ToolCalls is recorded beside it as the number that needs
// no interpretation.
func filesFromShell(command, repoDir string) []string {
	var found []string
	for _, segment := range strings.FieldsFunc(command, func(r rune) bool {
		return r == '|' || r == ';' || r == '\n' || r == '&'
	}) {
		fields := strings.Fields(segment)
		reading := false
		for _, f := range fields {
			base := path.Base(f)
			if readCommands[base] {
				reading = true
				continue
			}
			if !reading || strings.HasPrefix(f, "-") {
				continue
			}
			candidate := strings.Trim(f, `"'`)
			if candidate == "" || strings.ContainsAny(candidate, "*?$") {
				continue
			}
			full := candidate
			if !filepath.IsAbs(full) {
				full = filepath.Join(repoDir, candidate)
			}
			if st, err := os.Stat(full); err == nil && !st.IsDir() {
				found = append(found, full)
			}
		}
	}
	return found
}

// relToRepo names a path the way a reader of the repository would.
func relToRepo(p, repoDir string) string {
	if rel, err := filepath.Rel(repoDir, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.Base(p)
}

// agentEnv is the environment the agent gets: an allow-list, not the
// developer's.
//
// The agent runs with bypassPermissions over a THIRD-PARTY source tree that
// scripts/fetch-lab-repo.sh cloned from a URL an operator can override. That is
// a different exposure from the skill eval's, whose workspaces hold fixtures
// this repository authors. Prose in someone else's README is untrusted input to
// a model with a shell, and `os.Environ()` handed that shell every credential
// on the machine — ANTHROPIC_API_KEY, GITHUB_TOKEN, AWS_*, whatever else is
// exported — none of which the lab needs.
//
// What it does need: PATH to find its tools, HOME because the CLI authenticates
// from ~/.claude, and enough locale and terminal state to run. Everything else
// is dropped.
//
// This narrows what a compromised or merely obedient run can exfiltrate. It is
// not a sandbox: the agent still has a shell and a network. See issue #2243.
func agentEnv() []string {
	const allowed = "PATH HOME LANG LC_ALL LC_CTYPE TERM TMPDIR SHELL USER LOGNAME"
	var out []string
	for name := range strings.SplitSeq(allowed, " ") {
		if v, ok := os.LookupEnv(name); ok {
			out = append(out, name+"="+v)
		}
	}
	return out
}

// isolationEnv is the in-repo isolation contract: an agent driven here must not
// reach the developer's kapi config, plugins or caches. See CLAUDE.md.
func isolationEnv(home string) []string {
	return []string{
		"KAPI_NO_PROJECT=1",
		"KAPI_CONFIG_DIR=" + filepath.Join(home, "kapi-config"),
		"XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"KAPI_PLUGINS_DIR_ONLY=1",
		"KAPI_PLUGINS_DIR=" + filepath.Join(home, "kapi-plugins"),
	}
}

// findClaude locates the CLI that drives the agent.
func findClaude() (string, error) {
	if p := os.Getenv("CLAUDE_BIN"); p != "" {
		return p, nil
	}
	p, err := exec.LookPath("claude")
	if err != nil {
		return "", errors.New("no `claude` on PATH: the lab drives the CLI (set CLAUDE_BIN to override)")
	}
	return p, nil
}
