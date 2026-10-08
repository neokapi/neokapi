package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Judging voice fit.
//
// Terminology, competitor names and banned claims are machine-scored; a judge
// scores the writing around them. Each judge reads the project's voice, the
// task and what the attempt wrote, never the arm or the host, and answers five
// yes/no questions. Two judges from different model families score every
// attempt. The judged score is published only when they agree at Cohen's
// kappa >= 0.6, the bar the context eval sets.
//
// The questions ask about what can be pointed at in the text, each with an
// example that passes and one that fails, and a judge quotes the words behind
// every "no". Impression questions ("does it match the register") left two
// judges near chance agreement, so none is asked. A verdict records the rubric
// it answered, and one under another rubric is asked again.

// compareRubricVersion names the rubric below. Change it with the questions.
const compareRubricVersion = "v4-observable"

// compareCriteria are the rubric's questions, in the order the report prints
// them, each with an example that passes and one that fails.
var compareCriteria = []struct{ ID, Question, Yes, No string }{
	{"address", "Is the reader addressed directly, as \"you\" or with imperative instructions, everywhere the text speaks to them? Answer no if it refers to the reader in the third person (\"users\", \"customers\", \"the user\", \"teams can\") anywhere it could have said \"you\".",
		"\"Select Move, then pick the destination.\" / \"You keep the data in your previous tool.\"",
		"\"Users can move a board by selecting Move.\" / \"Customers will find their data intact.\""},
	{"lead", "Does the first sentence of prose say what the reader needs: the answer to their question, the action to take, or the change? Skip titles, headings, dates, greetings and sign-offs: they are not the opening. In a release note or any list of changes, answer yes when each item opens with what changed. Answer no only when the first sentence of prose is thanks, a restatement of the question, a preview of what follows (\"Here's what you need to know\"), or background.",
		"\"Hi Sam,\" then \"The import copies your boards, lists and cards into a new Space.\" / A release note whose items read \"Offline mode: edit boards without a connection.\"",
		"\"Hi Sam,\" then \"Thanks for reaching out about moving your team.\" / \"Here's what the import does and what to watch for.\""},
	{"plain", "Is it free of promotional and emotional wording: superlatives, intensifiers and sales language such as \"powerful\", \"seamless\", \"effortless\", \"blazing\", \"easy\", \"quick and painless\", \"we're excited\", \"great news\", \"rest assured\", \"don't worry\"?",
		"\"Cold starts are about 40% faster.\"",
		"\"Cold starts are now blazing fast, so you can rest assured your apps feel snappy.\""},
	{"faithful", "Does every factual statement come from the task's facts or from the files as they were before the change (shown below), without promises, numbers, features or reassurances found in neither? Facts carried over from the file before the change are faithful.",
		"Task: payouts arrive in 2 business days. Text: \"Your payout reaches your bank 2 business days after your customer pays.\" / A rename update that keeps the file's existing sentence \"Apps run in three regions.\"",
		"Task: payouts arrive in 2 business days. Text: \"Most payouts arrive the same day, and you'll never wait more than 2 days.\""},
	{"short", "Are the sentences short: almost every sentence 25 words or fewer, and no paragraph longer than four sentences?",
		"Three sentences of 9, 14 and 12 words.",
		"A 40-word sentence joining three clauses with \"and\" and \"which\"."},
}

// compareKappaBar is the agreement a judged score needs to be reported as validated.
const compareKappaBar = 0.6

// CompareVerdict is one judge's answer for one attempt.
type CompareVerdict struct {
	Judge      PairedAgentSpec `json:"judge"`
	Rubric     string          `json:"rubric"`
	Answers    map[string]bool `json:"answers"`
	Reason     string          `json:"reason,omitempty"`
	Error      string          `json:"error,omitempty"`
	DurationMS int64           `json:"duration_ms"`
}

func (v CompareVerdict) complete() bool {
	if v.Error != "" || v.Rubric != compareRubricVersion {
		return false
	}
	for _, criterion := range compareCriteria {
		if _, ok := v.Answers[criterion.ID]; !ok {
			return false
		}
	}
	return true
}

// score is the share of criteria a verdict answered yes.
func (v CompareVerdict) score() float64 {
	yes := 0
	for _, criterion := range compareCriteria {
		if v.Answers[criterion.ID] {
			yes++
		}
	}
	return float64(yes) / float64(len(compareCriteria))
}

// compareVoiceSection is the "Voice" section of a project's writing guide:
// the description a judge scores against.
func compareVoiceSection(rules string) string {
	_, after, ok := strings.Cut(rules, "## Voice\n")
	if !ok {
		return rules
	}
	section, _, _ := strings.Cut(after, "\n## ")
	return strings.TrimSpace(section)
}

func compareJudgePrompt(project CompareProject, task CompareTask, text string) string {
	var b strings.Builder
	b.WriteString("You review writing for a software company against its house voice. Read the voice, the task the writer was given, and what the writer produced, then answer five yes/no questions about what is in the text.\n\n")
	b.WriteString("## The voice\n\n" + compareVoiceSection(string(project.Rules)) + "\n\n")
	b.WriteString("## The task\n\n" + task.Prompt + "\n\n")
	b.WriteString("## What the writer produced\n\nFor a new file, the whole file. For a changed file, only the lines the writer added or changed (for a JSON file, the keys and values, which are interface strings). Lines that sit between them are not shown, and headings, list markers and code are not sentences.\n\n")
	b.WriteString(text + "\n\n")
	if before := compareFilesBefore(project, text); before != "" {
		b.WriteString("## The changed files as they were before the change\n\n" + before + "\n\n")
	}
	b.WriteString("## Questions\n\nWhich product names and terms the writer used is checked separately: do not judge which names or terms appear. Answer each question about the text as written, not about how good it is overall. Answer yes unless you can quote the words that make the answer no.\n\n")
	for _, criterion := range compareCriteria {
		fmt.Fprintf(&b, "- %s: %s\n  Yes, for example: %s\n  No, for example: %s\n", criterion.ID, criterion.Question, criterion.Yes, criterion.No)
	}
	b.WriteString("\nDo not use any tools. Reply with one JSON object and nothing else, of the form {\"address\": true, \"lead\": false, \"plain\": true, \"faithful\": true, \"short\": true, \"reason\": \"for each no, the quoted words\"}.\n")
	return b.String()
}

// compareChangedHeader finds the files a grade's text marks as changed.
var compareChangedHeader = regexp.MustCompile(`(?m)^### (\S+) \(changed\)$`)

// compareFilesBefore renders each file the text changed as the project held
// it, so a judge can tell a fact carried over from one the writer added.
func compareFilesBefore(project CompareProject, text string) string {
	var b strings.Builder
	for _, match := range compareChangedHeader.FindAllStringSubmatch(text, -1) {
		if original, ok := project.Files[match[1]]; ok {
			fmt.Fprintf(&b, "### %s (before)\n\n%s\n\n", match[1], strings.TrimSpace(string(original)))
		}
	}
	return strings.TrimSpace(b.String())
}

// compareParseVerdict reads the JSON object a judge replied with.
func compareParseVerdict(text string) (map[string]bool, string, error) {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return nil, "", errors.New("no JSON object in the reply")
	}
	raw := map[string]any{}
	if err := json.Unmarshal([]byte(text[start:end+1]), &raw); err != nil {
		return nil, "", err
	}
	answers := map[string]bool{}
	for _, criterion := range compareCriteria {
		value, ok := raw[criterion.ID].(bool)
		if !ok {
			return nil, "", fmt.Errorf("no yes/no answer for %s", criterion.ID)
		}
		answers[criterion.ID] = value
	}
	reason, _ := raw["reason"].(string)
	return answers, reason, nil
}

// judgeCompare asks every judge about every graded attempt that wrote
// something, skipping verdicts already saved.
func judgeCompare(ctx context.Context, opts CompareOptions, m CompareManifest) error {
	type job struct {
		dir   string
		judge PairedAgentSpec
		grade CompareGrade
	}
	jobs := []job{}
	for _, dir := range comparePhaseDirs(opts) {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			attemptDir := filepath.Join(dir, entry.Name())
			var grade CompareGrade
			if !entry.IsDir() || readPairedJSON(filepath.Join(attemptDir, "grade.json"), &grade) != nil || strings.TrimSpace(grade.Text) == "" {
				continue
			}
			for _, judge := range m.Judges {
				var saved CompareVerdict
				if readPairedJSON(filepath.Join(attemptDir, "judge-"+judge.Host+".json"), &saved) == nil && saved.complete() {
					continue
				}
				jobs = append(jobs, job{dir: attemptDir, judge: judge, grade: grade})
			}
		}
	}
	fmt.Fprintf(os.Stderr, "judge: %d verdicts to ask for\n", len(jobs))
	queue := make(chan job, len(jobs))
	for _, j := range jobs {
		queue <- j
	}
	close(queue)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for range max(1, opts.Concurrency) {
		wg.Go(func() {
			for j := range queue {
				if ctx.Err() != nil {
					return
				}
				task, err := findCompareTask(j.grade.Attempt.Task)
				verdict := CompareVerdict{Judge: j.judge, Rubric: compareRubricVersion}
				if err == nil {
					var project CompareProject
					project, err = loadCompareProject(task.Project)
					if err == nil {
						verdict = runCompareJudge(ctx, opts, j.judge, compareJudgePrompt(project, task, j.grade.Text))
					}
				}
				if err != nil {
					verdict.Error = err.Error()
				}
				_ = writeCompareJSON(filepath.Join(j.dir, "judge-"+j.judge.Host+".json"), verdict)
				mu.Lock()
				done++
				state := "ok"
				if verdict.Error != "" {
					state = verdict.Error
				}
				fmt.Fprintf(os.Stderr, "[%d/%d] %s by %s: %s\n", done, len(jobs), filepath.Base(j.dir), j.judge.Host, state)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return ctx.Err()
}

// runCompareJudge asks one judge, in a throwaway home with no tools, no MCP
// server and no project, and reads its verdict.
func runCompareJudge(ctx context.Context, opts CompareOptions, judge PairedAgentSpec, prompt string) CompareVerdict {
	verdict := CompareVerdict{Judge: judge, Rubric: compareRubricVersion}
	started := time.Now()
	defer func() { verdict.DurationMS = time.Since(started).Milliseconds() }()
	home, err := os.MkdirTemp("", "kapi-compare-judge-")
	if err != nil {
		verdict.Error = err.Error()
		return verdict
	}
	defer os.RemoveAll(home)
	work := filepath.Join(home, "work")
	for _, dir := range []string{work, filepath.Join(home, "claude"), filepath.Join(home, "codex")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			verdict.Error = err.Error()
			return verdict
		}
	}
	executable, err := exec.LookPath(judge.Host)
	if err != nil {
		verdict.Error = err.Error()
		return verdict
	}
	env := []string{"PATH=/usr/bin:/bin", "HOME=" + home, "LANG=en_US.UTF-8", "TERM=dumb", "NO_COLOR=1",
		"CLAUDE_CONFIG_DIR=" + filepath.Join(home, "claude"), "CODEX_HOME=" + filepath.Join(home, "codex"),
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "ENABLE_CLAUDEAI_MCP_SERVERS=false", "CLAUDE_CODE_DISABLE_AUTO_MEMORY=1",
		"DISABLE_AUTOUPDATER=1", "CLAUDE_CODE_DISABLE_BUNDLED_SKILLS=1"}
	var args []string
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	switch judge.Host {
	case "claude":
		token, err := pairedClaudeSubscriptionToken(ctx, 10*time.Minute)
		if err != nil {
			verdict.Error = err.Error()
			return verdict
		}
		env = append(env, "CLAUDE_CODE_OAUTH_TOKEN="+token)
		mcp := filepath.Join(home, "mcp.json")
		if err := os.WriteFile(mcp, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
			verdict.Error = err.Error()
			return verdict
		}
		args = []string{"--print", "--output-format", "json", "--model", judge.Model, "--effort", judge.Effort,
			"--setting-sources", "project", "--strict-mcp-config", "--mcp-config", mcp, "--tools", "",
			"--no-session-persistence", "--disable-slash-commands", "--max-turns", "2"}
	case "codex":
		original, err := os.UserHomeDir()
		if err != nil {
			verdict.Error = err.Error()
			return verdict
		}
		source := os.Getenv("CODEX_HOME")
		if source == "" {
			source = filepath.Join(original, ".codex")
		}
		if err := os.Symlink(filepath.Join(source, "auth.json"), filepath.Join(home, "codex", "auth.json")); err != nil {
			verdict.Error = err.Error()
			return verdict
		}
		config := "forced_login_method = \"chatgpt\"\napproval_policy = \"never\"\nsandbox_mode = \"read-only\"\nweb_search = \"disabled\"\nmodel_reasoning_effort = \"" + judge.Effort + "\"\n[features]\napps = false\nplugins = false\nhooks = false\nmulti_agent = false\n"
		if err := os.WriteFile(filepath.Join(home, "codex", "config.toml"), []byte(config), 0o600); err != nil {
			verdict.Error = err.Error()
			return verdict
		}
		args = []string{"exec", "--json", "--skip-git-repo-check", "--ephemeral", "--model", judge.Model, "--cd", work, "-"}
	default:
		verdict.Error = "unsupported judge host " + judge.Host
		return verdict
	}
	stdout, stderr, runErr := compareRunCommand(ctx, work, env, prompt, executable, args...)
	text := ""
	switch judge.Host {
	case "claude":
		var reply struct {
			Result  string `json:"result"`
			IsError bool   `json:"is_error"`
		}
		if err := json.Unmarshal([]byte(stdout), &reply); err == nil {
			text = reply.Result
			if reply.IsError {
				runErr = errors.Join(runErr, errors.New("judge reported an error: "+reply.Result))
			}
		}
	case "codex":
		scanner := bufio.NewScanner(strings.NewReader(stdout))
		scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
		for scanner.Scan() {
			event := map[string]any{}
			if json.Unmarshal(scanner.Bytes(), &event) != nil || pairedString(event, "type") != "item.completed" {
				continue
			}
			item := pairedObject(event, "item")
			if pairedString(item, "type") == "agent_message" {
				text = pairedString(item, "text")
			}
		}
	}
	answers, reason, parseErr := compareParseVerdict(text)
	if parseErr != nil {
		tail := strings.TrimSpace(stderr)
		if len(tail) > 400 {
			tail = tail[len(tail)-400:]
		}
		verdict.Error = strings.TrimSpace(fmt.Sprintf("%v %v %s", parseErr, runErr, pairedRedactText(tail, env)))
		return verdict
	}
	verdict.Answers, verdict.Reason = answers, reason
	return verdict
}

// compareKappa is Cohen's kappa for two raters' yes/no answers on the same
// items. It is NaN when the raters' answers leave no room for chance (both
// constant and equal).
func compareKappa(a, b []bool) float64 {
	n := len(a)
	if n == 0 || n != len(b) {
		return math.NaN()
	}
	agree, aYes, bYes := 0, 0, 0
	for i := range a {
		if a[i] == b[i] {
			agree++
		}
		if a[i] {
			aYes++
		}
		if b[i] {
			bYes++
		}
	}
	po := float64(agree) / float64(n)
	pa, pb := float64(aYes)/float64(n), float64(bYes)/float64(n)
	pe := pa*pb + (1-pa)*(1-pb)
	if pe == 1 {
		return math.NaN()
	}
	return (po - pe) / (1 - pe)
}

// compareJudgePairs collects the two judges' paired answers over every
// attempt both scored, overall and per criterion.
func compareJudgePairs(verdicts map[string]map[string]CompareVerdict, first, second string) (all [2][]bool, by map[string][2][]bool) {
	by = map[string][2][]bool{}
	ids := []string{}
	for id := range verdicts {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		a, okA := verdicts[id][first]
		b, okB := verdicts[id][second]
		if !okA || !okB || !a.complete() || !b.complete() {
			continue
		}
		for _, criterion := range compareCriteria {
			all[0] = append(all[0], a.Answers[criterion.ID])
			all[1] = append(all[1], b.Answers[criterion.ID])
			pair := by[criterion.ID]
			pair[0] = append(pair[0], a.Answers[criterion.ID])
			pair[1] = append(pair[1], b.Answers[criterion.ID])
			by[criterion.ID] = pair
		}
	}
	return all, by
}
