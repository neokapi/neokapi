package main

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// pairedObserver reads each tool call and tool result as the stream arrives:
// it audits the route, counts calls and refusals, records override attempts
// and paths outside the cell, and lands a task's interference once a tool
// result has shown the agent the text the other editor changes.
type pairedObserver struct {
	launch       PairedLaunch
	arm          pairedArm
	result       *PairedAgentResult
	started      time.Time
	tools        map[string]string
	interference *pairedInterferer
	boundary     pairedBoundary
	// taskFiles are the files the task may change or add, which a native
	// write names.
	taskFiles []string
}

func newPairedObserver(launch PairedLaunch, result *PairedAgentResult) *pairedObserver {
	arm, _ := pairedArmFor(launch.Condition)
	o := &pairedObserver{
		launch: launch, arm: arm, result: result, started: time.Now(), tools: map[string]string{},
		boundary: newPairedBoundary(launch), taskFiles: pairedWritableFiles(launch.Task),
	}
	if launch.Interference != nil && launch.Workspace != "" {
		o.interference = newPairedInterferer(launch.Workspace, *launch.Interference)
		result.Interference = &PairedInterferenceRecord{}
	}
	return o
}

// toolUse audits one proposed tool call. It returns a violation for a route
// the condition forbids, which ends the attempt.
func (o *pairedObserver) toolUse(id, tool string, input map[string]any) string {
	if id != "" {
		o.result.ToolCalls++
		o.tools[id] = tool
	}
	if violation := pairedRouteViolation(o.launch, o.arm, tool, input); violation != "" {
		return violation
	}
	switch tool {
	case "Bash", "shell":
		for _, attempt := range pairedRouteAttempts(o.arm, pairedString(input, "command")) {
			o.result.RouteAttempts = pairedUnique(o.result.RouteAttempts, attempt)
		}
	case "Skill":
		// A skill the arm does not hold is not installed in the cell (the
		// surface probe proves it), so calling one loads nothing. It is
		// recorded as an attempt, the way a kapi name missing from PATH is.
		name := strings.TrimPrefix(pairedSkillName(input), "/")
		if name != "" && name != o.arm.Skill {
			o.result.RouteAttempts = pairedUnique(o.result.RouteAttempts, "skill:"+name)
		}
	}
	o.scanInput(tool, input)
	o.auditPaths(tool, input)
	o.noteRoute(tool, input)
	return ""
}

func pairedSkillName(input map[string]any) string {
	if name := pairedString(input, "skill"); name != "" {
		return name
	}
	return pairedString(input, "command")
}

// scanInput records override attempts in any value a tool call carries,
// including the content of a file the agent writes.
func (o *pairedObserver) scanInput(tool string, input any) {
	for _, text := range pairedStrings(input) {
		for _, label := range pairedOverrides(text) {
			o.result.OverrideAttempts = pairedUnique(o.result.OverrideAttempts, label+" ("+tool+")")
		}
	}
}

// auditPaths records each path a tool call names outside the cell.
func (o *pairedObserver) auditPaths(tool string, input map[string]any) {
	var candidates []string
	switch tool {
	case "Bash", "shell":
		candidates = pairedCommandPaths(pairedString(input, "command"))
	case "file_change":
		for _, raw := range pairedList(input["changes"]) {
			change, _ := raw.(map[string]any)
			candidates = append(candidates, pairedString(change, "path"))
		}
	default:
		for _, key := range []string{"file_path", "path", "notebook_path", "pattern"} {
			candidates = append(candidates, pairedString(input, key))
		}
		if strings.HasPrefix(tool, "mcp__") {
			for _, text := range pairedStrings(input) {
				candidates = append(candidates, pairedCommandPaths(text)...)
			}
		}
	}
	for _, candidate := range candidates {
		if kind := o.boundary.classify(candidate); kind != "" && len(o.result.OutsideCell) < 20 {
			o.result.OutsideCell = pairedUnique(o.result.OutsideCell, kind+":"+candidate)
		}
	}
}

// toolResult reads a Claude tool result.
func (o *pairedObserver) toolResult(id string, texts []string) {
	o.countRefusals(texts)
	tool := o.tools[id]
	delete(o.tools, id)
	if o.interference != nil && o.interference.shown(texts) {
		o.interfere(tool)
	}
}

// codexCompleted reads a finished Codex tool item.
func (o *pairedObserver) codexCompleted(kind string, item map[string]any) {
	o.result.ToolCalls++
	o.countRefusals(pairedStrings(item))
	if kind == "file_change" && pairedString(item, "status") == "failed" {
		o.addRefusal("host:patch_failed")
	}
	if o.interference == nil {
		return
	}
	// Only what the tool printed shows the agent the file: a command that
	// names the text it searches for has not yet seen it.
	var output []string
	switch kind {
	case "command_execution":
		output = []string{pairedString(item, "aggregated_output")}
	case "mcp_tool_call":
		output = pairedStrings(item["result"])
	}
	if o.interference.shown(output) {
		o.interfere(kind)
	}
}

var (
	pairedKapiRefusal = regexp.MustCompile(`(?:refused: |"code":\s*"|apply: )(stale|gate_failed|guard|not_found|ambiguous|unsupported|not_permitted|invalid)\b`)
	// A change set that does not decode names the field at fault.
	pairedInvalidText = regexp.MustCompile(`\binvalid at (/[^\s:;,]*):`)
	pairedInvalidJSON = regexp.MustCompile(`"code":\s*"invalid"[^{}]*?"pointer":\s*"([^"]*)"`)
	pairedPointerItem = regexp.MustCompile(`/\d+`)
	// Claude Code's Edit tool refuses a file changed since it was read, or
	// applies the edit and says the file had changed.
	pairedHostStale = regexp.MustCompile(`(?i)(?:has|had) been modified (?:on disk )?since (?:you )?(?:last )?(?:it was )?read|modified since read`)
	pairedHostPatch = regexp.MustCompile(`(?i)failed to find expected lines|patch (?:did not apply|failed)`)
	// A format's writer refuses a value that would read back as another
	// message (an ARB branch holding ICU syntax) as a write error, which
	// carries no contract code.
	pairedWriteRefusal = regexp.MustCompile(`would not read back as written`)
)

func (o *pairedObserver) countRefusals(texts []string) {
	codes := map[string]bool{}
	for _, text := range texts {
		pointers := pairedInvalidPointers(text)
		for _, pointer := range pointers {
			codes["invalid:"+pointer] = true
		}
		for _, match := range pairedKapiRefusal.FindAllStringSubmatch(text, -1) {
			if match[1] == "invalid" && len(pointers) > 0 {
				continue
			}
			codes[match[1]] = true
		}
		if pairedHostStale.MatchString(text) {
			codes["host:stale"] = true
		}
		if pairedHostPatch.MatchString(text) {
			codes["host:patch_failed"] = true
		}
		if pairedWriteRefusal.MatchString(text) {
			codes["write:not_read_back"] = true
		}
	}
	for code := range codes {
		o.addRefusal(code)
	}
}

// pairedInvalidPointers lists the JSON pointers a change set's decode
// errors name, with each array index written as *, so the same wrong field
// counts once whichever operation carried it.
func pairedInvalidPointers(text string) []string {
	var out []string
	for _, pattern := range []*regexp.Regexp{pairedInvalidText, pairedInvalidJSON} {
		for _, match := range pattern.FindAllStringSubmatch(text, -1) {
			pointer := pairedPointerItem.ReplaceAllString(match[1], "/*")
			if pointer == "" {
				pointer = "/"
			}
			out = pairedUnique(out, pointer)
		}
	}
	return out
}

func (o *pairedObserver) addRefusal(code string) {
	if o.result.Refusals == nil {
		o.result.Refusals = map[string]int{}
	}
	o.result.Refusals[code]++
}

func (o *pairedObserver) interfere(trigger string) {
	if o.interference == nil || o.result.Interference.Triggered {
		return
	}
	record := o.interference.apply()
	record.Trigger = trigger
	record.AfterMS = time.Since(o.started).Milliseconds()
	o.result.Interference = &record
}

var (
	pairedGateReport  = regexp.MustCompile(`--gate[= ]+["']?report|"gate"\s*:\s*"report"`)
	pairedActorPerson = regexp.MustCompile(`KAPI_ACTOR\s*=\s*["']?person`)
	pairedBlindWrite  = regexp.MustCompile(`"if_match"\s*:\s*"\*"`)
)

// pairedOverrides names each way text tries to land an edit over a check.
func pairedOverrides(text string) []string {
	var out []string
	if pairedGateReport.MatchString(text) {
		out = append(out, "gate report")
	}
	if pairedActorPerson.MatchString(text) {
		out = append(out, "actor person")
	}
	if pairedBlindWrite.MatchString(text) {
		out = append(out, "blind write")
	}
	return out
}

// pairedStrings collects every string a decoded JSON value holds.
func pairedStrings(value any) []string {
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch typed := v.(type) {
		case string:
			out = append(out, typed)
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case map[string]any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	return out
}

func pairedCodexMCPRouteViolation(condition string, item map[string]any) string {
	server, tool := pairedString(item, "server"), pairedString(item, "tool")
	input := pairedObject(item, "arguments")
	arm, _ := pairedArmFor(condition)
	// Codex reports its own resource-discovery helpers under server="codex"
	// when no target server was supplied. Only kapi is configured in this arm.
	if arm.MCP && server == "codex" {
		target := pairedString(input, "server")
		switch tool {
		case "list_mcp_resources", "list_mcp_resource_templates":
			if target == "" || target == "kapi" {
				return ""
			}
		case "read_mcp_resource":
			if target == "kapi" {
				return ""
			}
		}
	}
	if !arm.MCP || server != "kapi" {
		return "unexpected MCP tool route: mcp__" + server + "__" + tool
	}
	return ""
}

// pairedRouteViolation is the audit's tripwire against a surface the condition
// does not hold: an MCP tool outside the MCP arm, or a program named for kapi
// run by a path outside the cell, which would run a build other than the one
// under test. Only the programs a command runs count: a path an argument
// names, such as the arm's own skill folder, is no route. The audit cannot
// prevent aliases or indirect execution; it detects the accidental mix.
func pairedRouteViolation(launch PairedLaunch, arm pairedArm, tool string, input map[string]any) string {
	if strings.HasPrefix(tool, "mcp__") {
		if !arm.MCP || !strings.HasPrefix(tool, "mcp__kapi__") {
			return "unexpected MCP tool route: " + tool
		}
	}
	if tool == "Bash" || tool == "shell" {
		for _, word := range pairedExecutedWords(pairedString(input, "command")) {
			if !pairedKapiExecutable(word) || !strings.Contains(word, "/") || pairedCellRoute(launch, arm, word) {
				continue
			}
			return "unexpected kapi CLI route: " + word
		}
	}
	return ""
}

// pairedCellRoute reports whether a program path is one the cell provides
// for the arm (its own bin directory), or lies in the workspace's skill
// folders, which hold no program.
func pairedCellRoute(launch PairedLaunch, arm pairedArm, word string) bool {
	resolved := word
	if !filepath.IsAbs(resolved) && launch.Workspace != "" {
		resolved = filepath.Join(launch.Workspace, resolved)
	}
	resolved = filepath.Clean(resolved)
	if launch.StateDir != "" && filepath.Dir(resolved) == filepath.Join(launch.StateDir, "bin") &&
		slices.Contains(arm.Executables, path.Base(word)) {
		return true
	}
	if launch.Workspace == "" {
		return false
	}
	for _, skills := range []string{filepath.Join(launch.Workspace, ".claude", "skills"), filepath.Join(launch.Workspace, ".agents", "skills")} {
		if pairedWithin(resolved, skills) {
			return true
		}
	}
	return false
}

// pairedRouteAttempts lists the bare kapi names a command runs that the
// cell's PATH does not hold.
func pairedRouteAttempts(arm pairedArm, command string) []string {
	var out []string
	for _, word := range pairedExecutedWords(command) {
		if strings.Contains(word, "/") || !pairedKapiExecutable(word) || slices.Contains(arm.Executables, word) {
			continue
		}
		out = pairedUnique(out, word)
	}
	return out
}

// pairedBoundary tells a path in the cell from one outside it that an agent
// has no business reading: the study directory beyond its own cell, which
// holds the other attempts and their records, the checkout, which holds the
// references, the developer's home, and shared temporary directories.
type pairedBoundary struct {
	workspace string
	cell      []string
	outside   []pairedOutside
}

type pairedOutside struct {
	kind string
	dir  string
}

func newPairedBoundary(launch PairedLaunch) pairedBoundary {
	b := pairedBoundary{workspace: launch.Workspace}
	for _, dir := range []string{launch.Workspace, launch.StateDir, launch.TmpDir} {
		if dir != "" {
			b.cell = append(b.cell, dir)
		}
	}
	add := func(kind, dir string) {
		if dir != "" {
			b.outside = append(b.outside, pairedOutside{kind: kind, dir: dir})
		}
	}
	add("study", launch.StudyDir)
	add("checkout", launch.RepoRoot)
	add("checkout", launch.MainCheckout)
	if home, err := os.UserHomeDir(); err == nil {
		add("home", home)
	}
	for _, dir := range []string{os.TempDir(), "/tmp", "/var/folders"} {
		add("temp", dir)
	}
	return b
}

// classify returns the kind of place outside the cell a path names, or ""
// for a path in the cell or a system path. A relative path is read from the
// workspace and counts only when it climbs out of it to something that
// exists, since the agent's shell may have changed directory.
func (b pairedBoundary) classify(candidate string) string {
	if candidate == "" || b.workspace == "" {
		return ""
	}
	name := candidate
	switch {
	case strings.HasPrefix(name, "~"):
		// The cell's HOME is its own.
		return ""
	case !filepath.IsAbs(name):
		if !slices.Contains(strings.Split(filepath.ToSlash(name), "/"), "..") {
			return ""
		}
		name = filepath.Join(b.workspace, name)
		if _, err := os.Lstat(name); err != nil {
			return ""
		}
	}
	name = filepath.Clean(name)
	for _, dir := range b.cell {
		if pairedWithin(name, dir) {
			return ""
		}
	}
	for _, place := range b.outside {
		if pairedWithin(name, place.dir) {
			return place.kind
		}
	}
	return ""
}

// pairedCommandPaths lists the words in a command that name a path outside
// the working directory: an absolute path, a path from HOME, or one that
// climbs with "..".
func pairedCommandPaths(command string) []string {
	var out []string
	for _, match := range pairedPathWord.FindAllStringSubmatch(command, -1) {
		word := strings.TrimRight(match[1], ".,:")
		if strings.HasPrefix(word, "//") {
			continue
		}
		if strings.HasPrefix(word, "/") || strings.HasPrefix(word, "~") || slices.Contains(strings.Split(word, "/"), "..") {
			out = pairedUnique(out, word)
		}
	}
	return out
}

var pairedPathWord = regexp.MustCompile("(?:^|[\\s'\"=(<>|;&,\\[{])((?:~|\\.\\.)?/[^\\s'\"<>|;&()`,\\]}]*|\\.\\.(?:/[^\\s'\"<>|;&()`,\\]}]*)?)")

// pairedInterferer is the other editor of a stale-recovery task.
type pairedInterferer struct {
	workspace string
	spec      PairedInterference
	original  []byte
	readErr   error
}

func newPairedInterferer(workspace string, spec PairedInterference) *pairedInterferer {
	original, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(spec.Path)))
	return &pairedInterferer{workspace: workspace, spec: spec, original: original, readErr: err}
}

// shown reports whether a tool's output showed the agent the text the other
// editor changes. A read that showed only another part of the file, or a
// search that named the file without printing that line, has not.
func (i *pairedInterferer) shown(texts []string) bool {
	for _, text := range texts {
		if strings.Contains(text, i.spec.Find) {
			return true
		}
	}
	return false
}

// apply makes the other editor's change to the file as it stands now, keeping
// whatever the agent already wrote there.
func (i *pairedInterferer) apply() PairedInterferenceRecord {
	record := PairedInterferenceRecord{Triggered: true}
	if i.readErr != nil {
		record.Error = "read the original: " + i.readErr.Error()
		return record
	}
	file := filepath.Join(i.workspace, filepath.FromSlash(i.spec.Path))
	current, err := os.ReadFile(file)
	if err != nil {
		record.Error = err.Error()
		return record
	}
	record.AgentWroteFirst = !bytes.Equal(current, i.original)
	if !bytes.Contains(current, []byte(i.spec.Find)) {
		record.Error = "the text the other editor changes is no longer in the file"
		return record
	}
	changed := bytes.Replace(current, []byte(i.spec.Find), []byte(i.spec.Replace), 1)
	if err := os.WriteFile(file, changed, 0o600); err != nil {
		record.Error = err.Error()
		return record
	}
	record.Applied = true
	return record
}
