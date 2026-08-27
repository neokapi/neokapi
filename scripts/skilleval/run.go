package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Driving one scenario: build a workspace, install the skill into it, run the
// agent headless, and read what it did out of the event stream.
//
// The transcript is the evidence, so it is kept rather than reduced to a
// boolean. A run that says "triggered: false" and nothing else cannot be argued
// with; one that also shows the tools the agent reached for instead can.

// Run is one pass of one scenario.
type Run struct {
	// Triggered says the agent loaded the kapi skill.
	Triggered bool `json:"triggered"`
	// Tools is every tool the agent called, in order, deduplicated on repeats
	// so a 40-call run stays readable.
	Tools []string `json:"tools"`
	// KapiCommands is every kapi invocation, verbatim. This is the row a reader
	// learns most from: it shows which verbs the skill actually leads to.
	KapiCommands []string `json:"kapiCommands,omitempty"`
	// MCPTools is every kapi MCP tool the agent called, in order of first use,
	// with the mcp__kapi__ prefix stripped. On an MCP scenario this is the
	// answer: which of the nineteen it reached for.
	MCPTools []string `json:"mcpTools,omitempty"`
	// FinalText is the agent's closing message.
	FinalText string `json:"finalText,omitempty"`
	// Messages counts assistant messages in the stream. It runs ahead of the
	// --max-turns cap, because one turn can emit several messages; it is a
	// measure of how much work the run took, not of the cap being honoured.
	Messages int `json:"messages"`
	// DurationMS is wall clock for the run.
	DurationMS int64 `json:"durationMs"`
	// Gate records the completion gate's result, when the scenario has one and
	// the run was in completion mode.
	Gate *GateResult `json:"gate,omitempty"`
	// Changed is the workspace diff: which files the agent created or edited.
	Changed []FileChange `json:"changed,omitempty"`
	// Err is set when the agent could not be run at all.
	Err string `json:"error,omitempty"`

	// Events is the session: every assistant message and every tool call with
	// the result it returned. Published to its own file rather than inline —
	// see splitSessions.
	Events []Event `json:"events,omitempty"`
	// EventsDropped counts events past the per-session cap, so a truncated
	// transcript says it is one.
	EventsDropped int `json:"eventsDropped,omitempty"`
	// eventBytes tracks what has been kept, against maxSessionBytes.
	eventBytes int
}

// GateResult is whether the scenario's own completion command passed.
type GateResult struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output,omitempty"`
}

// FileChange is one file the agent touched, with enough to render a diff.
type FileChange struct {
	Path string `json:"path"`
	// Kind is added, edited or removed.
	Kind string `json:"kind"`
	// Before and After hold text content when the file is text and small
	// enough to show. A binary file carries sizes instead, because a .docx
	// diff is meaningless as text and enormous as bytes.
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	// Binary marks a file whose content is not shown.
	Binary     bool `json:"binary,omitempty"`
	BytesAfter int  `json:"bytesAfter,omitempty"`
}

// harnessOwned are directories the harness puts in the workspace: the installed
// skill, and the throwaway config, data and cache roots the isolation contract
// requires. kapi writes into those on its first run, so without this every
// completion diff was two files of harness bookkeeping and nothing else — the
// agent's actual edits sat beside a change list that looked like it had done
// nothing but touch a plugins cache.
var harnessOwned = map[string]bool{
	".claude":      true,
	harnessDir:     true,
	"kapi-config":  true,
	"kapi-plugins": true,
	"xdg-data":     true,
	"xdg-cache":    true,
}

// uninteresting are directories an agent may create that say nothing about
// what it did to the content.
//
// The i18n scenarios install dependencies, and node_modules alone contributed
// 2015 of one scenario's 2037 recorded changes. The nine files that mattered
// were buried and the dataset ran to 12MB. What a reader wants from that run
// is the config the agent wrote and the catalogs it produced.
var uninteresting = map[string]bool{
	"node_modules": true,
	".git":         true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".cache":       true,
	"vendor":       true,
	"target":       true,
	".venv":        true,
	"__pycache__":  true,
}

// harnessDir holds what the harness writes into a workspace for a gate to
// read. Skipped by the snapshot, so it never shows up as the agent's doing.
const harnessDir = ".skilleval"

// answerFile is the agent's closing message, so a gate can check an answer
// rather than only an artefact.
const answerFile = harnessDir + "/answer.txt"

// maxShownBytes caps what a change carries into the dataset. Past this a diff
// stops being readable and the dataset stops being reviewable in a browser.
const maxShownBytes = 24 * 1024

// buildWorkspace materializes a scenario's fixture in dir and installs the
// skill where an agent will find it.
func buildWorkspace(dir string, sc *Scenario, repoRoot, kapiBin, arm string) error {
	for i := range sc.Fixture {
		f := &sc.Fixture[i]
		dst := filepath.Join(dir, f.As)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		var body []byte
		switch {
		case f.From != "":
			b, err := os.ReadFile(filepath.Join(repoRoot, f.From))
			if err != nil {
				return fmt.Errorf("fixture %s: %w", f.From, err)
			}
			body = b
		default:
			body = []byte(f.Body)
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			return err
		}
		f.Bytes = len(body)
	}

	if arm == armUnaided {
		// The control gets the workspace and nothing else: no skill, no MCP
		// server. Whatever it manages, it manages without kapi.
		return nil
	}
	if surfaceOf(*sc) == surfaceMCP {
		return writeMCPConfig(dir, kapiBin)
	}

	// The skill goes where a headless agent discovers it without a plugin
	// install, which is the same place the plugin would put it.
	skillDst := filepath.Join(dir, ".claude", "skills", "kapi")
	if err := os.MkdirAll(skillDst, 0o755); err != nil {
		return err
	}
	return copyTree(filepath.Join(repoRoot, "cli", "skills", "data", "kapi"), skillDst)
}

// mcpConfigName is the file handed to --strict-mcp-config, which is what keeps
// the run from inheriting whatever MCP servers the developer has configured.
const mcpConfigName = ".mcp.json"

// writeMCPConfig points the agent at this checkout's kapi as an MCP server,
// carrying the isolation environment with it: the server is a kapi process, so
// the same upward walk to the dogfood recipe applies to it.
func writeMCPConfig(dir, kapiBin string) error {
	env := map[string]string{}
	for _, kv := range isolationEnv(dir) {
		if i := strings.IndexByte(kv, '='); i > 0 {
			env[kv[:i]] = kv[i+1:]
		}
	}
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"kapi": map[string]any{
				"command": kapiBin,
				"args":    []string{"mcp"},
				"env":     env,
			},
		},
	}
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, mcpConfigName), body, 0o644)
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}

// snapshot records the workspace before the agent runs, so the changes it made
// can be recovered afterwards. Skips the installed skill, which is not the
// agent's doing.
func snapshot(dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return relErr
		}
		if info.IsDir() {
			// harnessOwned only at the top; uninteresting at any depth, since a
			// nested project installs its own dependencies.
			if harnessOwned[rel] || uninteresting[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == mcpConfigName {
			// Written by the harness, not the agent.
			return nil
		}
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil // unreadable is not fatal; it simply does not diff
		}
		out[rel] = b
		return nil
	})
	return out, err
}

// diffWorkspace compares two snapshots into something a dashboard can render.
func diffWorkspace(before, after map[string][]byte) []FileChange {
	var changes []FileChange
	for path, post := range after {
		pre, existed := before[path]
		if existed && string(pre) == string(post) {
			continue
		}
		c := FileChange{Path: path, Kind: "edited", BytesAfter: len(post)}
		if !existed {
			c.Kind = "added"
		}
		switch {
		case isBinary(post) || len(post) > maxShownBytes:
			c.Binary = true
		default:
			c.After = string(post)
			if existed && !isBinary(pre) && len(pre) <= maxShownBytes {
				c.Before = string(pre)
			}
		}
		changes = append(changes, c)
	}
	for path, pre := range before {
		if _, still := after[path]; still {
			continue
		}
		c := FileChange{Path: path, Kind: "removed"}
		if !isBinary(pre) && len(pre) <= maxShownBytes {
			c.Before = string(pre)
		} else {
			c.Binary = true
		}
		changes = append(changes, c)
	}
	sortChanges(changes)
	return changes
}

func isBinary(b []byte) bool {
	limit := min(len(b), 512)
	for i := range limit {
		if b[i] == 0 {
			return true
		}
	}
	return false
}

func sortChanges(c []FileChange) {
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j].Path < c[j-1].Path; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
}

// isolationEnv keeps an in-repo agent away from the developer's real kapi.
//
// The dogfood recipe at the repo root is found by an upward walk from any cwd
// inside the tree, so an unisolated run would bind to it and act on it. The
// workspace here is under os.TempDir() rather than the repo, but the contract
// is cheap to honour and the failure it prevents is silent.
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

// runScenario drives the agent once and reads the result out of the stream.
func runScenario(ctx context.Context, sc *Scenario, opts Options, arm string) Run {
	started := time.Now()
	r := Run{}

	dir, err := os.MkdirTemp("", "skilleval-"+sc.ID+"-")
	if err != nil {
		r.Err = scrubPaths(err.Error())
		return r
	}
	if opts.Keep {
		fmt.Fprintf(os.Stderr, "  workspace: %s\n", dir)
	} else {
		defer os.RemoveAll(dir)
	}

	if err := buildWorkspace(dir, sc, opts.RepoRoot, opts.KapiBin, arm); err != nil {
		r.Err = scrubPaths(err.Error())
		return r
	}
	before, err := snapshot(dir)
	if err != nil {
		r.Err = scrubPaths(err.Error())
		return r
	}

	turns := sc.Turns
	if opts.Mode == modeCompletion {
		// A scenario's Turns is sized for triggering, which shows up early.
		// Completing the job is a different budget: EVALS.md records one
		// scenario taking about 45 turns and another needing at least 25
		// before it reads as the right task at all. Two gates came back red on
		// the first completion sweep with both agents visibly mid-sentence,
		// which measured the budget rather than the skill.
		turns = max(turns, opts.CompletionTurnFloor)
	}
	if opts.Mode == modeTrigger && surfaceOf(*sc) != surfaceMCP {
		// On the skill surface, triggering shows up in the first turn or two, so
		// a hard cap keeps a sweep from paying for translation it will not
		// score.
		//
		// The MCP surface is not like that. The question there is WHICH of
		// nineteen tools the agent picked, and picking can come after reading a
		// file or two. Capping at four turns cut two of three passes off
		// mid-task and recorded them as wrong picks, which measured the budget
		// rather than the tool descriptions.
		turns = min(turns, opts.TriggerTurnCap)
	}

	args := []string{
		"-p", sc.Prompt,
		"--max-turns", strconv.Itoa(turns),
		"--permission-mode", "bypassPermissions",
		"--output-format", "stream-json",
		"--verbose",
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if surfaceOf(*sc) == surfaceMCP && arm != armUnaided {
		// --strict-mcp-config is what makes this an eval of kapi's server
		// rather than of whatever the developer happens to have configured.
		// The control gets neither.
		args = append(args, "--mcp-config", mcpConfigName, "--strict-mcp-config")
	}

	cmd := exec.CommandContext(ctx, opts.ClaudeBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), isolationEnv(dir)...)
	switch {
	case arm == armUnaided:
		// Every directory holding a kapi or toolbox binary is removed. A
		// developer's PATH has one, and leaving it would make this a control
		// that had kapi and merely lacked the skill.
		cmd.Env = append(cmd.Env, "PATH="+stripKapiFromPath(os.Getenv("PATH")))
	case opts.KapiBin != "":
		cmd.Env = append(cmd.Env, "PATH="+filepath.Dir(opts.KapiBin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.Err = scrubPaths(err.Error())
		return r
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		r.Err = scrubPaths(err.Error())
		return r
	}
	parseStream(stdout, &r)
	// A non-zero exit is ordinary here: hitting the turn cap is how a capped
	// positive ends. What the stream showed is the result either way.
	_ = cmd.Wait()

	r.DurationMS = time.Since(started).Milliseconds()

	// The agent's closing message, written where a gate can read it.
	//
	// A gate is a shell command run in the workspace, so it sees files and
	// nothing else. That works for a scenario whose deliverable is a file and
	// not at all for one whose deliverable is an answer: "what does slide 3
	// say" leaves no artefact, so it went ungated and verified nothing.
	if err := os.MkdirAll(filepath.Join(dir, harnessDir), 0o755); err == nil {
		_ = os.WriteFile(filepath.Join(dir, answerFile), []byte(r.FinalText), 0o644)
	}

	after, err := snapshot(dir)
	if err == nil {
		r.Changed = diffWorkspace(before, after)
	}

	if opts.Mode == modeCompletion && sc.CompletionGate != "" {
		// The gate always runs WITH kapi, in both arms. It is the measuring
		// instrument, not part of what is being measured: asking "is every
		// SalesPilot gone" needs a format-aware reader whether or not the
		// agent had one.
		r.Gate = runGate(ctx, dir, sc.CompletionGate, opts)
	}
	return r
}

// runGate executes the scenario's own definition of done.
//
// Through a shell, because half the useful gates are shell expressions: a sweep
// completed when `! kgrep -l utilize docs/` finds nothing, a target catalog
// exists when `test -f`. Restricting gates to a bare argv would push those into
// bespoke Go and make each one a thing to maintain rather than to read.
func runGate(ctx context.Context, dir, gate string, opts Options) *GateResult {
	if strings.TrimSpace(gate) == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", gate)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), isolationEnv(dir)...)
	if opts.KapiBin != "" {
		// The gate calls `kapi` and the toolbox names by bare word, so this
		// checkout's build has to win over anything installed on the machine.
		cmd.Env = append(cmd.Env,
			"PATH="+filepath.Dir(opts.KapiBin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	out, err := cmd.CombinedOutput()

	res := &GateResult{Command: gate, Output: scrubPaths(truncate(string(out), 4000))}
	if err != nil {
		res.ExitCode = 1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
		}
	}
	return res
}

// parseStream reads the agent's event stream and records what it did.
//
// The stream is one JSON object per line. Only three things matter: a tool_use
// naming the Skill tool (the activation signal EVALS.md names), every other
// tool_use in order, and the last assistant text.
func parseStream(r io.Reader, out *Run) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20)

	seenTool := map[string]bool{}
	seenMCP := map[string]bool{}
	// A call and its result arrive in different events, joined by the id the
	// stream puts on both. This holds the call's place in Events until the
	// result turns up.
	awaiting := map[string]int{}

	for sc.Scan() {
		var ev struct {
			Type    string `json:"type"`
			Message struct {
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
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}

		switch ev.Type {
		case "assistant":
			out.Messages++
			for _, c := range ev.Message.Content {
				switch c.Type {
				case "text":
					if t := strings.TrimSpace(c.Text); t != "" {
						out.FinalText = scrubPaths(truncate(t, 2000))
						out.record(Event{Kind: "text", Text: t})
					}
				case "tool_use":
					if !seenTool[c.Name] {
						seenTool[c.Name] = true
						out.Tools = append(out.Tools, c.Name)
					}
					if tool, ok := strings.CutPrefix(c.Name, "mcp__kapi__"); ok {
						out.Triggered = true
						if !seenMCP[tool] {
							seenMCP[tool] = true
							out.MCPTools = append(out.MCPTools, tool)
						}
					}
					if c.Name == "Skill" {
						var in struct {
							Skill string `json:"skill"`
						}
						if json.Unmarshal(c.Input, &in) == nil && in.Skill == "kapi" {
							out.Triggered = true
						}
					}
					if c.Name == "Bash" {
						var in struct {
							Command string `json:"command"`
						}
						if json.Unmarshal(c.Input, &in) == nil && mentionsKapi(in.Command) {
							out.Triggered = true
							out.KapiCommands = append(out.KapiCommands, scrubPaths(truncate(in.Command, 300)))
						}
					}
					before := len(out.Events)
					out.record(Event{Kind: "tool", Name: c.Name, Input: string(c.Input)})
					if c.ID != "" && len(out.Events) > before {
						awaiting[c.ID] = before
					}
				}
			}
		case "user":
			// The half the summary never had: what the tool returned, which is
			// what the agent read before deciding what to do next.
			for _, c := range ev.Message.Content {
				if c.Type != "tool_result" {
					continue
				}
				idx, ok := awaiting[c.ToolUseID]
				if !ok {
					continue
				}
				delete(awaiting, c.ToolUseID)
				out.recordResult(idx, resultText(c.Content), c.IsError)
			}
		}
	}
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
			// An image, or anything else without text. Naming the kind is
			// more use than dropping the block silently.
			parts = append(parts, "["+b.Type+"]")
		}
	}
	return strings.Join(parts, "\n")
}

// mentionsKapi reports whether a shell command drives kapi or one of the
// toolbox names it installs.
//
// Reaching for `kgrep` counts as activation even when the Skill tool was never
// called: the skill's own content is what taught the agent that kgrep exists,
// and scoring only the Skill tool would miss an agent that read SKILL.md once
// and then worked from it.
func mentionsKapi(cmd string) bool {
	// Command position only. Scanning every token counted `grep kapi README.md`
	// as the agent driving kapi, which is a false positive on the side that
	// matters most: a negative scenario searching a file for the word would
	// have scored as a false trigger, and a false trigger is the finding this
	// suite treats as most serious.
	//
	// A command starts the string, or follows a separator. Environment
	// assignments come before the command word, so they are stepped over.
	for _, segment := range splitOnSeparators(cmd) {
		if isKapiBinary(commandWord(segment)) {
			return true
		}
	}
	return false
}

// commandWord returns the word a segment invokes, stepping over any NAME=value
// prefixes. Empty when the segment holds nothing but assignments.
func commandWord(segment string) string {
	for tok := range strings.FieldsSeq(segment) {
		if isEnvAssignment(tok) {
			continue
		}
		return tok
	}
	return ""
}

// splitOnSeparators breaks a shell line into the segments that can each start a
// command: pipes, sequencing, boolean chains, and command substitution.
func splitOnSeparators(cmd string) []string {
	return strings.FieldsFunc(cmd, func(r rune) bool {
		switch r {
		case '|', ';', '&', '(', ')', '\n':
			return true
		}
		return false
	})
}

// isEnvAssignment reports whether a token is a NAME=value prefix rather than
// the command itself.
func isEnvAssignment(tok string) bool {
	name, _, found := strings.Cut(tok, "=")
	if !found || name == "" {
		return false
	}
	for _, r := range name {
		if r != '_' && !(r >= 'A' && r <= 'Z') && !(r >= 'a' && r <= 'z') &&
			!(r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// isKapiBinary reports whether a command word is kapi or one of the toolbox
// names it installs.
//
// The toolbox counts: reaching for kgrep is activation even when the Skill tool
// was never called, because the skill's own content is what taught the agent
// that kgrep exists.
func isKapiBinary(tok string) bool {
	tok = strings.TrimLeft(tok, "\"'")
	if i := strings.LastIndex(tok, "/"); i >= 0 {
		tok = tok[i+1:]
	}
	switch tok {
	case "kapi", "kgrep", "ksed", "kcat", "kdiff", "kconv":
		return true
	}
	return false
}

// scrubPaths removes absolute paths from anything that reaches the committed
// dataset.
//
// The dataset is checked in, and scripts/check-abs-paths.sh holds every tracked
// file to zero of them: an absolute home path resolves on exactly one laptop.
//
// An agent's own commands are how they get in. It works in a throwaway
// directory under os.TempDir() and reaches kapi by whatever path it found, so a
// transcript can carry both the developer's home and a temp root that will not
// exist tomorrow. Neither tells a reader anything the relative path does not.
func scrubPaths(msg string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		msg = strings.ReplaceAll(msg, home, "~")
	}
	return absPathPattern.ReplaceAllString(msg, "<path>")
}

// absPathPattern matches the absolute paths that survive the home-directory
// substitution: a scenario's temp workspace, a system root, another checkout.
var absPathPattern = regexp.MustCompile(`/(?:Users|home|private|var|tmp|opt)/[^\s"']*`)

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…truncated…"
}
