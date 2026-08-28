// Command authoringlab has an agent read a real repository and document it,
// once with the governance bound at a coordinate and once without, across
// several models — and publishes the prose rather than a score.
//
// This is the read-it-yourself half of the authoring evals. `voice-guide-steering`
// answers whether a guide moves a number, over six 120-word briefs on one model.
// It cannot show what a coordinate does to a document: 120 words has no
// structure to change, one model cannot say whether an effect survives a change
// of model, and a brief hands over every fact in the order it is needed, which
// measures expansion rather than documentation.
//
// So the subject is ripgrep, pinned, and the agent reads it. Each run records
// which files it opened and what it searched for, because that is most of the
// difference between a document grounded in the source and one assembled from
// what the model already believed about ripgrep.
//
// It scores nothing. Nobody has written down what a good user guide is, and a
// rubric invented here would be measuring the rubric. The output is the
// documents, the context that produced them, and the reading each one did.
//
//	./scripts/fetch-lab-repo.sh          # once: clone the subject
//	make authoring-lab                   # the whole matrix (spends)
//	make authoring-lab AUTHORINGLAB_ARGS="-models claude-sonnet-5"
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultOut is where the dataset lands, relative to the repo root.
const DefaultOut = "web/src/pages/authoring-lab/_authoringlab.json"

// DocDir is where the prose lands. Markdown, committed: sixteen documents of
// about 600 words is some 60KB, and the point of the exercise is that a person
// can read the diff.
const DocDir = "web/static/authoring-lab"

// SessionDir is where the sessions land, beside the documents. Same tree, so
// one publish target carries both.
const SessionDir = "web/static/authoring-lab/transcripts"

// Doc is one cell of the matrix: one task, at one coordinate, on one model,
// written twice.
type Doc struct {
	Model    string `json:"model"`
	Audience string `json:"audience"`
	// Bare is the run given the repository and the task.
	Bare AgentRun `json:"bare"`
	// Governed is the same, plus the guide the profile renders at this point.
	Governed AgentRun `json:"governed"`
	// BareAgain is a second sample of the bare arm, run only when the panel is.
	// It is what a null pair is made of: two documents that differ in nothing
	// the panel is being asked about.
	BareAgain AgentRun `json:"bareAgain,omitzero"`
	// Files are where the two documents landed, relative to DocDir, for a
	// reader who would rather open them than scroll.
	BareFile     string `json:"bareFile"`
	GovernedFile string `json:"governedFile"`
}

// Report is the dataset the page reads.
type Report struct {
	Generated string `json:"generated"`
	// Repo names the tree the agent read, at the tag it was pinned to.
	Repo string `json:"repo"`
	// Guides, Tasks and Labels are published because a reader cannot judge a
	// document without seeing what the model was given. The guide especially:
	// it is the whole independent variable.
	Guides map[string]string `json:"guides"`
	Tasks  map[string]string `json:"tasks"`
	Labels map[string]string `json:"labels"`
	// Profile is the voice profile as YAML, so a reader can see the rules the
	// guide was rendered from rather than only the rendering.
	Profile string `json:"profile"`
	Runner  string `json:"runner"`
	Docs    []Doc  `json:"docs"`
	// Panel is the judged pass, when one was run. Lenses says what each judge
	// was asked and what it catches; Verdicts is every judgement including the
	// controls; Summary is what the controls support saying.
	Lenses   []Lens        `json:"lenses,omitempty"`
	Verdicts []Verdict     `json:"verdicts,omitempty"`
	Panel    *JudgeSummary `json:"panel,omitempty"`
	// JudgeModel records who judged, because four Claude models are independent
	// runs rather than independent minds and a reader should know which.
	JudgeModel string `json:"judgeModel,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "authoringlab:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		out        = flag.String("out", "", "dataset path (default: "+DefaultOut+" under the repo root)")
		models     = flag.String("models", "", "comma-separated model ids (default: the catalogued set)")
		only       = flag.String("only", "", "one audience: end-user or developer")
		date       = flag.String("date", "", "date to stamp (default: today)")
		workers    = flag.Int("concurrency", 2, "documents in flight")
		judge      = flag.Bool("judge", false, "run the judge panel over the documents this run produced")
		judgeOnly  = flag.Bool("judge-existing", false, "judge the documents already in the dataset, generating only the extra bare samples the null pairs need")
		judgeModel = flag.String("judge-model", "claude-opus-5", "model the panel runs on")
		nulls      = flag.Int("null-samples", 1, "extra bare samples per cell, for the null pairs the panel is checked with")
	)
	flag.Parse()

	ctx := context.Background()
	root, err := repoRoot(ctx)
	if err != nil {
		return err
	}
	subject, err := repoDir(root)
	if err != nil {
		return err
	}
	claudeBin, err := findClaude()
	if err != nil {
		return err
	}
	target := *out
	if target == "" {
		target = filepath.Join(root, DefaultOut)
	}
	stamp := *date
	if stamp == "" {
		stamp = time.Now().UTC().Format("2006-01-02")
	}

	wanted := labModels
	if strings.TrimSpace(*models) != "" {
		wanted = nil
		for m := range strings.SplitSeq(*models, ",") {
			if m = strings.TrimSpace(m); m != "" {
				wanted = append(wanted, m)
			}
		}
	}
	pts := points
	if *only != "" {
		pts = nil
		for _, p := range points {
			if p.Audience == *only {
				pts = append(pts, p)
			}
		}
		if len(pts) == 0 {
			return fmt.Errorf("no audience %q: have end-user, developer", *only)
		}
	}

	base, err := loadProfile()
	if err != nil {
		return err
	}
	guides := map[string]string{}
	for _, p := range pts {
		g, err := guideFor(base, p)
		if err != nil {
			return err
		}
		guides[p.Audience] = g
	}

	rep := &Report{
		Generated: stamp,
		Repo:      LabRepo,
		Guides:    guides,
		Tasks:     map[string]string{},
		Labels:    map[string]string{},
		Profile:   string(ripgrepProfileYAML),
		Runner: fmt.Sprintf("claude -p in the cloned tree, %d turns maximum per document, "+
			"tools on, isolation contract applied. Documents are the output; nothing is scored.",
			maxAgentTurns),
	}
	for _, p := range pts {
		rep.Tasks[p.Audience] = p.Task
		rep.Labels[p.Audience] = p.Label
	}

	type cell struct {
		model string
		point Point
	}
	var cells []cell
	for _, m := range wanted {
		for _, p := range pts {
			cells = append(cells, cell{m, p})
		}
	}

	// Judging what is already published, rather than regenerating it.
	//
	// The documents cost far more than the judging, and a panel run should not
	// re-measure the thing it is judging: a re-run produces different documents,
	// so the verdicts would not be about the ones on the page. Only the extra
	// bare samples the null pairs need are generated.
	if *judgeOnly {
		prior, err := readReport(target)
		if err != nil {
			return err
		}
		docs := prior.Docs
		fmt.Fprintf(os.Stderr, "authoring-lab: judging %d cell(s) already in %s\n", len(docs), target)

		missing := 0
		for i := range docs {
			if docs[i].Bare.Text == "" || docs[i].BareAgain.Text != "" {
				continue
			}
			missing++
		}
		if missing > 0 {
			fmt.Fprintf(os.Stderr, "authoring-lab: %d null sample(s) to generate\n", missing)
			sem := make(chan struct{}, max(1, *workers))
			var wg sync.WaitGroup
			for i := range docs {
				if docs[i].Bare.Text == "" || docs[i].BareAgain.Text != "" {
					continue
				}
				wg.Go(func() {
					sem <- struct{}{}
					defer func() { <-sem }()
					docs[i].BareAgain = runAgent(ctx, AgentOpts{
						ClaudeBin: claudeBin, RepoDir: subject,
						Model: docs[i].Model, Prompt: prior.Tasks[docs[i].Audience],
					})
					fmt.Fprintf(os.Stderr, "  null  %-18s %-10s %s\n",
						docs[i].Model, docs[i].Audience, armStatus(docs[i].BareAgain))
				})
			}
			wg.Wait()
		}

		verdicts, err := runPanel(ctx, docs, prior.Tasks, *judgeModel)
		if err != nil {
			return err
		}
		sortVerdicts(verdicts)
		summary := summarise(verdicts)
		prior.Lenses, prior.Verdicts, prior.Panel = lenses, verdicts, &summary
		prior.JudgeModel = *judgeModel
		prior.Docs = docs
		reportPanel(summary)

		body, err := json.MarshalIndent(prior, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, append(body, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "authoring-lab: wrote %s\n", target)
		return nil
	}

	fmt.Fprintf(os.Stderr, "authoring-lab: %d document(s) across %d model(s) and %d coordinate(s), reading %s\n",
		len(cells)*2, len(wanted), len(pts), LabRepo)

	docs := make([]Doc, len(cells))
	sem := make(chan struct{}, max(1, *workers))
	var wg sync.WaitGroup
	for i, c := range cells {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			d := Doc{Model: c.model, Audience: c.point.Audience}
			base := AgentOpts{ClaudeBin: claudeBin, RepoDir: subject, Model: c.model, Prompt: c.point.Task}
			d.Bare = runAgent(ctx, base)

			governed := base
			governed.SystemPrompt = guides[c.point.Audience]
			d.Governed = runAgent(ctx, governed)

			// A second bare sample, so the panel can be shown a pair where the
			// right answer is a coin flip. Without it, a judge with a standing
			// preference is indistinguishable from a real effect, and nothing
			// else in the run would reveal the difference.
			if *judge && *nulls > 0 {
				d.BareAgain = runAgent(ctx, base)
			}

			docs[i] = d
			fmt.Fprintf(os.Stderr, "  %-18s %-10s bare:%s governed:%s\n",
				c.model, c.point.Audience, armStatus(d.Bare), armStatus(d.Governed))
		})
	}
	wg.Wait()

	if err := writeDocs(root, docs); err != nil {
		return err
	}
	// Sessions out of the dataset and into their own files: the page imports the
	// dataset, and a sixty-turn session per cell would be paid for by every
	// reader of the summary.
	if err := writeSessions(root, docs); err != nil {
		return err
	}
	rep.Docs = docs

	if *judge {
		verdicts, err := runPanel(ctx, docs, rep.Tasks, *judgeModel)
		if err != nil {
			return err
		}
		sortVerdicts(verdicts)
		summary := summarise(verdicts)
		rep.Lenses, rep.Verdicts, rep.Panel = lenses, verdicts, &summary
		rep.JudgeModel = *judgeModel
		reportPanel(summary)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, append(body, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "authoring-lab: wrote %s and %s/\n", target, DocDir)
	return nil
}

// readReport loads the dataset a previous run published.
func readReport(target string) (*Report, error) {
	body, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("no dataset at %s to judge: run the lab first", target)
	}
	var rep Report
	if err := json.Unmarshal(body, &rep); err != nil {
		return nil, fmt.Errorf("%s: %w", target, err)
	}
	if len(rep.Docs) == 0 {
		return nil, fmt.Errorf("%s holds no documents", target)
	}
	return &rep, nil
}

// armStatus is one arm's one-line result: what it read, and what it cost.
func armStatus(a AgentRun) string {
	if a.Err != "" {
		return "FAILED(" + a.Err + ")"
	}
	return fmt.Sprintf("%d files/%dk ctx/%ds", len(a.FilesRead), a.InputTokens/1000, a.DurationMS/1000)
}

// writeDocs puts the prose on disk, one file per arm, so it can be opened and
// diffed outside a browser.
func writeDocs(root string, docs []Doc) error {
	dir := filepath.Join(root, DocDir)
	// Only the cells this run produced. Clearing the whole tree would let
	// `-models one -only one-audience` delete the documents it cannot
	// reproduce, which is what it did the first time.
	for i := range docs {
		if docs[i].Model == "" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, docs[i].Model, docs[i].Audience)); err != nil {
			return err
		}
	}
	for i := range docs {
		d := &docs[i]
		for _, f := range []struct {
			arm string
			run AgentRun
			out *string
		}{
			{"bare", d.Bare, &d.BareFile},
			{"governed", d.Governed, &d.GovernedFile},
		} {
			if f.run.Err != "" || f.run.Text == "" {
				continue
			}
			rel := filepath.Join(d.Model, d.Audience, f.arm+".md")
			path := filepath.Join(dir, rel)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			header := fmt.Sprintf("<!-- %s · %s · %s arm · read %d file(s) of %s · generated by `make authoring-lab` -->\n\n",
				d.Model, d.Audience, f.arm, len(f.run.FilesRead), LabRepo)
			if err := os.WriteFile(path, []byte(header+f.run.Text+"\n"), 0o644); err != nil {
				return err
			}
			*f.out = filepath.ToSlash(rel)
		}
	}
	return nil
}

// LabSession is one arm's recorded session, as published.
type LabSession struct {
	Model    string  `json:"model"`
	Audience string  `json:"audience"`
	Arm      string  `json:"arm"`
	Prompt   string  `json:"prompt"`
	Events   []Event `json:"events"`
}

// writeSessions publishes each arm's session and points the dataset at it.
//
// A file count says the agent read six files. Only the session says which six,
// what it made of them, and where it went wrong — which is the question a
// reader has when a document surprises them.
func writeSessions(root string, docs []Doc) error {
	dir := filepath.Join(root, SessionDir)
	// Only the cells this run produced, for the same reason writeDocs clears
	// only those: a partial run must not delete what it cannot reproduce.
	for i := range docs {
		if docs[i].Model == "" {
			continue
		}
		for _, arm := range []string{"bare", "governed"} {
			_ = os.Remove(filepath.Join(dir, sessionName(docs[i].Model, docs[i].Audience, arm)))
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for i := range docs {
		d := &docs[i]
		for _, f := range []struct {
			arm string
			run *AgentRun
		}{{"bare", &d.Bare}, {"governed", &d.Governed}} {
			if len(f.run.Events) == 0 {
				continue
			}
			name := sessionName(d.Model, d.Audience, f.arm)
			body, err := json.MarshalIndent(LabSession{
				Model: d.Model, Audience: d.Audience, Arm: f.arm,
				Prompt: taskFor(d.Audience), Events: f.run.Events,
			}, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, name), append(body, '\n'), 0o644); err != nil {
				return err
			}
			f.run.Transcript = name
			// Out of the dataset now that it is on disk.
			f.run.Events = nil
		}
	}
	return nil
}

// sessionName is stable across runs, so a re-run overwrites rather than
// accumulating, and safe as a path and a URL.
func sessionName(model, audience, arm string) string {
	return fmt.Sprintf("%s--%s--%s.json", model, audience, arm)
}

// taskFor is the prompt a coordinate's runs opened on.
func taskFor(audience string) string {
	for _, p := range points {
		if p.Audience == audience {
			return p.Task
		}
	}
	return ""
}

func repoRoot(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", errors.New("not in a git checkout")
	}
	return strings.TrimSpace(string(out)), nil
}
