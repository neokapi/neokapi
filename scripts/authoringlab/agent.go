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
	"regexp"
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
	// KapiCommands is every kapi invocation, in order. The pulled arm's whole
	// result: it was given the skill and a project that binds a voice, and this
	// says whether it went and asked. Empty here is a finding, not a gap in the
	// recording — an arm that never ran the command wrote its document from the
	// repository alone, whatever the workspace offered it.
	KapiCommands []string `json:"kapiCommands,omitempty"`
	// ToolCalls counts every tool by name. Unlike FilesRead this needs no
	// interpretation, so it is the number to check when FilesRead looks wrong.
	ToolCalls map[string]int `json:"toolCalls,omitempty"`
	// Events is the session: every message the agent wrote and every tool call
	// with the result it returned. A file count says the agent read six files;
	// only the session says which six and what it made of them. Published to
	// its own file rather than inline, because the page imports the dataset.
	Events []Event `json:"events,omitempty"`
	// Transcript names the published file, when there is one.
	Transcript string `json:"transcript,omitempty"`
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

	// A pristine tree per run, and for the pulled arm a project and a skill on
	// top of it. See pull.go for why this is not the shared checkout.
	tree, err := prepareWorkspace(opts.Root, home, opts.Arm)
	if err != nil {
		r.Err = err.Error()
		return r
	}

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
		// The workspace's settings and skills, and not the developer's.
		//
		// A run inherits HOME, because the CLI authenticates from ~/.claude, and
		// with it every plugin and skill installed on this machine. A probe found
		// 76 of them in a lab run: Go style guides, a design-guidelines skill, a
		// code reviewer. The lab publishes what four models wrote given one
		// context, and none of that is part of it — nobody re-running this would
		// have the same set, and a writing-related skill steers the very thing
		// being compared.
		//
		// `project` keeps the workspace's own .claude, which is where the pulled
		// arm's skill is installed and how that arm differs from the others.
		"--setting-sources", "project",
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
	cmd.Dir = tree
	if opts.Arm.pull {
		bin, err := kapiOnlyBin(home, opts.KapiBin)
		if err != nil {
			r.Err = err.Error()
			return r
		}
		cmd.Env = withKapiOnPath(append(agentEnv(), pullEnv(home, tree)...), bin)
	} else {
		cmd.Env = append(agentEnv(), isolationEnv(home)...)
	}
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
	parseAgentStream(stdout, &r, tree)
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
	// Root is this repository, which is where the pristine subject archive and
	// the shipped skill are found. The tree the agent works in is extracted per
	// run and is not this.
	Root   string
	Model  string
	Prompt string
	// SystemPrompt is the governance, appended to the agent's own system prompt
	// exactly as `kapi voice guide` output would be pasted into one. Set in the
	// pushed arm and in neither of the others: the pulled arm has to go and get
	// the same text itself.
	SystemPrompt string
	// Arm says what the workspace carries. See pull.go.
	Arm armSetup
	// KapiBin is put on PATH for the pulled arm, so the command its skill tells
	// it to run is one it can actually run.
	KapiBin string
}

// parseAgentStream reads the tool calls to learn what the agent looked at.
func parseAgentStream(rd io.Reader, out *AgentRun, repoDir string) {
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20)

	seenFile := map[string]bool{}
	awaiting := map[string]int{}
	for sc.Scan() {
		var ev struct {
			Type    string `json:"type"`
			Message struct {
				Model   string `json:"model"`
				Content []struct {
					Type      string          `json:"type"`
					Text      string          `json:"text"`
					Name      string          `json:"name"`
					ID        string          `json:"id"`
					Input     json.RawMessage `json:"input"`
					ToolUseID string          `json:"tool_use_id"`
					Content   json.RawMessage `json:"content"`
					IsError   bool            `json:"is_error"`
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
		if ev.Type == "user" {
			// Where tool results arrive, joined to their call by the id the
			// stream carries on both.
			for _, c := range ev.Message.Content {
				if c.Type != "tool_result" {
					continue
				}
				if idx, ok := awaiting[c.ToolUseID]; ok {
					delete(awaiting, c.ToolUseID)
					out.Events[idx].Output = scrubPaths(resultText(c.Content))
					out.Events[idx].Failed = c.IsError
				}
			}
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
			if c.Type == "text" {
				if t := strings.TrimSpace(c.Text); t != "" {
					out.Events = append(out.Events, Event{Kind: "text", Text: scrubPaths(t)})
				}
				continue
			}
			if c.Type != "tool_use" {
				continue
			}
			out.Events = append(out.Events, Event{
				Kind: "tool", Name: c.Name, Input: scrubPaths(string(c.Input)),
			})
			if c.ID != "" {
				awaiting[c.ID] = len(out.Events) - 1
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
			case "Skill":
				// Loading the skill is reaching for kapi even when no command
				// follows, and counting only shell invocations would report an
				// agent that read the guidance and chose not to act as one that
				// never looked — the same under-measurement that recorded zero
				// files for an agent reading with `cat`.
				var in struct {
					Skill string `json:"skill"`
				}
				if json.Unmarshal(c.Input, &in) == nil && strings.HasSuffix(in.Skill, "kapi") {
					out.KapiCommands = append(out.KapiCommands, "Skill("+in.Skill+")")
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
				out.KapiCommands = append(out.KapiCommands, kapiCalls(in.Command)...)
			}
		}
	}
}

// Event is one step of a session, in the order it happened. Same shape the
// skill eval publishes, so one reader renders both.
type Event struct {
	Kind   string `json:"kind"`
	Name   string `json:"name,omitempty"`
	Text   string `json:"text,omitempty"`
	Input  string `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
	Failed bool   `json:"failed,omitempty"`
}

// resultText renders a tool result, which the stream sends either as a string
// or as the content blocks a tool returned.
func resultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return string(raw)
	}
	var parts []string
	for _, b := range blocks {
		switch {
		case b.Text != "":
			parts = append(parts, b.Text)
		case b.Type != "":
			parts = append(parts, "["+b.Type+"]")
		}
	}
	return strings.Join(parts, "\n")
}

// scrubPaths removes what is true of one machine only.
//
// Mandatory rather than tidy: a transcript carries the temp workspace, the
// developer's home and whatever the agent printed of both, and these files are
// published. The character class excludes a backslash so a path inside an
// escaped quote does not lose the escape and corrupt the surrounding JSON.
func scrubPaths(msg string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		msg = strings.ReplaceAll(msg, home, "~")
	}
	return absPathPattern.ReplaceAllString(msg, "<path>")
}

var absPathPattern = regexp.MustCompile(`/(?:Users|home|private|var|tmp|opt)/[^\s"'\\]*`)

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

// kapiCalls picks the kapi invocations out of a shell command.
//
// Split on the same separators filesFromShell uses, and a segment counts when
// its first word is the binary — so `cd crates && kapi voice guide` is found and
// `rg kapi` is not. Recorded verbatim, because which subcommand the agent
// reached for is the interesting part: `kapi voice guide` is the assistant
// taking the skill's advice, and `kapi context <path>` is it asking what applies
// where the file will sit.
func kapiCalls(command string) []string {
	var found []string
	for _, segment := range strings.FieldsFunc(command, func(r rune) bool {
		return r == '|' || r == ';' || r == '\n' || r == '&'
	}) {
		fields := strings.Fields(segment)
		if len(fields) == 0 || path.Base(fields[0]) != "kapi" {
			continue
		}
		// `&` separates segments AND appears inside `2>&1`, so splitting leaves a
		// dangling redirect on the end of the command. The command is shown to a
		// reader, and `kapi voice guide 2>` is not a command anyone ran.
		for len(fields) > 0 && danglingRedirect.MatchString(fields[len(fields)-1]) {
			fields = fields[:len(fields)-1]
		}
		if len(fields) == 0 {
			continue
		}
		found = append(found, strings.Join(fields, " "))
	}
	return found
}

var danglingRedirect = regexp.MustCompile(`^\d?[<>]+$`)

// withKapiOnPath puts the CLI's directory first, replacing the PATH the
// allow-list carried rather than appending a second one, which the child would
// ignore.
func withKapiOnPath(env []string, kapiBin string) []string {
	if kapiBin == "" {
		return env
	}
	prefix := filepath.Dir(kapiBin)
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		if rest, ok := strings.CutPrefix(kv, "PATH="); ok {
			out = append(out, "PATH="+prefix+string(os.PathListSeparator)+rest)
			replaced = true
			continue
		}
		out = append(out, kv)
	}
	if !replaced {
		out = append(out, "PATH="+prefix)
	}
	return out
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
