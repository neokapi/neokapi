// Command authoringlab writes the same document at two coordinates, with and
// without the governance bound there, across several models — and publishes the
// prose rather than a score.
//
// This is the read-it-yourself half of the authoring evals. `voice-guide-steering`
// answers whether a guide moves a number, over six 120-word briefs on one model.
// It cannot show what a coordinate does to a document, because 120 words has no
// structure to change and one model cannot say whether the effect survives a
// change of model.
//
// So this one scores nothing. The output is sixteen Markdown documents and a
// page that puts them side by side: same product, same feature, same brand
// voice, one axis different. If a coordinate does nothing, the two governed
// documents read alike and a reader sees that as directly as they would see the
// opposite. A number would be easier to quote and worth less, because nobody
// has yet said what a good user guide is, and inventing that rubric here would
// be measuring the rubric.
//
//	make authoring-lab                  # the whole matrix (spends)
//	make authoring-lab MODELS=claude-sonnet-5
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

	coreprofile "github.com/neokapi/neokapi/core/profile"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"gopkg.in/yaml.v3"
)

// DefaultOut is where the dataset lands, relative to the repo root.
const DefaultOut = "web/src/pages/authoring-lab/_authoringlab.json"

// DocDir is where the prose lands. Markdown, committed: sixteen documents of
// about 600 words is some 60KB, and the point of the exercise is that a person
// can read the diff.
const DocDir = "web/static/authoring-lab"

// Doc is one cell of the matrix.
type Doc struct {
	Model string `json:"model"`
	// Resolved is the model that actually answered, from the CLI's own usage
	// report. Asked and answered are separate fields because an alias that
	// quietly resolves elsewhere would turn a comparison of two models into a
	// comparison of one with itself.
	Resolved string `json:"resolved,omitempty"`
	Audience string `json:"audience"`
	// Bare is the document written from the task alone.
	Bare string `json:"bare"`
	// Governed is the same task with the voice profile bound at this point.
	Governed string `json:"governed"`
	// Files are where the two landed, relative to DocDir, for a reader who
	// would rather open them than scroll.
	BareFile     string `json:"bareFile"`
	GovernedFile string `json:"governedFile"`
	Error        string `json:"error,omitempty"`
}

// Report is the dataset the page reads.
type Report struct {
	Generated string `json:"generated"`
	// Brief and the guides are published because a reader cannot judge the
	// documents without seeing what produced them.
	Brief  string            `json:"brief"`
	Guides map[string]string `json:"guides"`
	Tasks  map[string]string `json:"tasks"`
	Labels map[string]string `json:"labels"`
	Runner string            `json:"runner"`
	Docs   []Doc             `json:"docs"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "authoringlab:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		out      = flag.String("out", "", "dataset path (default: "+DefaultOut+" under the repo root)")
		provider = flag.String("provider", "claude-code", "AI provider")
		models   = flag.String("models", "", "comma-separated model ids (default: the catalogued set)")
		only     = flag.String("only", "", "one audience: end-user or developer")
		date     = flag.String("date", "", "date to stamp (default: today)")
		workers  = flag.Int("concurrency", 4, "documents in flight")
	)
	flag.Parse()

	ctx := context.Background()
	root, err := repoRoot(ctx)
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

	// The guide each point's profile renders to, produced exactly as production
	// produces it, so what the page shows is what the model was given.
	guides := map[string]string{}
	for _, p := range pts {
		var prof coreprofile.VoiceProfile
		if err := yaml.Unmarshal([]byte(p.Voice), &prof); err != nil {
			return fmt.Errorf("%s profile: %w", p.Audience, err)
		}
		guide := strings.TrimSpace(coreprofile.RenderVoiceGuideCompact(&prof))
		if guide == "" {
			return fmt.Errorf("%s profile rendered an empty guide, so its governed arm would be "+
				"identical to its bare one", p.Audience)
		}
		guides[p.Audience] = guide
	}

	rep := &Report{
		Generated: stamp,
		Brief:     productBrief,
		Guides:    guides,
		Tasks:     map[string]string{},
		Labels:    map[string]string{},
		Runner: fmt.Sprintf("%s, greedy sampling, one pass per cell. Documents are the output; "+
			"nothing here is scored.", *provider),
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

	fmt.Fprintf(os.Stderr, "authoring-lab: %d document(s) across %d model(s) and %d coordinate(s)\n",
		len(cells)*2, len(wanted), len(pts))

	docs := make([]Doc, len(cells))
	sem := make(chan struct{}, max(1, *workers))
	var wg sync.WaitGroup
	for i, c := range cells {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			docs[i] = write(ctx, *provider, c.model, c.point, guides[c.point.Audience])
			status := "ok"
			if docs[i].Error != "" {
				status = "FAILED: " + docs[i].Error
			}
			fmt.Fprintf(os.Stderr, "  %-18s %-10s %s\n", c.model, c.point.Audience, status)
		})
	}
	wg.Wait()

	if err := writeDocs(root, docs); err != nil {
		return err
	}
	rep.Docs = docs

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

// write produces one cell: the same task twice, once with the point's
// governance and once without.
func write(ctx context.Context, providerID, model string, p Point, guide string) Doc {
	d := Doc{Model: model, Audience: p.Audience}
	llm, err := aiprovider.NewProvider(aiprovider.ProviderID(providerID), aiprovider.Config{
		APIKey: os.Getenv("ANTHROPIC_API_KEY"),
		Model:  model,
		// Greedy: the difference being shown is what the context did, and
		// sampling noise across sixteen generations would be read as it.
		Temperature: new(float64),
	})
	if err != nil {
		d.Error = err.Error()
		return d
	}
	defer llm.Close()

	bare, err := ask(ctx, llm, "", p)
	if err != nil {
		d.Error = "bare: " + err.Error()
		return d
	}
	governed, err := ask(ctx, llm, guide, p)
	if err != nil {
		d.Error = "governed: " + err.Error()
		return d
	}
	d.Bare, d.Governed = bare, governed
	d.Resolved = resolvedModel(llm, model)
	return d
}

// ask writes one document. The guide, when there is one, goes in the system
// turn, which is where kapi puts it in production.
func ask(ctx context.Context, llm aiprovider.LLMProvider, guide string, p Point) (string, error) {
	var sys strings.Builder
	sys.WriteString("You are writing product documentation. Use only the facts in the brief. ")
	sys.WriteString("Output Markdown and nothing else: no preamble, no commentary on your own work.")
	if guide != "" {
		sys.WriteString("\n\n")
		sys.WriteString(guide)
	}
	user := p.Task + "\n\n---\n\n" + productBrief

	resp, err := llm.Chat(ctx, []aiprovider.Message{
		aiprovider.TextMessage(aiprovider.RoleSystem, sys.String()),
		aiprovider.TextMessage(aiprovider.RoleUser, user),
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(resp.Content)
	if text == "" {
		return "", errors.New("the model returned nothing")
	}
	return text, nil
}

// resolvedModel reports what actually answered, when the provider says.
func resolvedModel(llm aiprovider.LLMProvider, asked string) string {
	type resolver interface{ LastModel() string }
	if r, ok := llm.(resolver); ok {
		if m := r.LastModel(); m != "" {
			return m
		}
	}
	return asked
}

// writeDocs puts the prose on disk, one file per arm, so it can be opened and
// diffed outside a browser.
func writeDocs(root string, docs []Doc) error {
	dir := filepath.Join(root, DocDir)
	// Only the cells this run produced. Clearing the whole tree would let
	// `-models one -only one-audience` delete fifteen documents it cannot
	// reproduce, which is what it did the first time.
	for i := range docs {
		if docs[i].Error != "" {
			continue
		}
		cell := filepath.Join(dir, docs[i].Model, docs[i].Audience)
		if err := os.RemoveAll(cell); err != nil {
			return err
		}
	}
	for i := range docs {
		d := &docs[i]
		if d.Error != "" {
			continue
		}
		for _, f := range []struct {
			arm, body string
			out       *string
		}{
			{"bare", d.Bare, &d.BareFile},
			{"governed", d.Governed, &d.GovernedFile},
		} {
			rel := filepath.Join(d.Model, d.Audience, f.arm+".md")
			path := filepath.Join(dir, rel)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			header := fmt.Sprintf("<!-- %s · %s · %s arm · generated by `make authoring-lab` -->\n\n",
				d.Model, d.Audience, f.arm)
			if err := os.WriteFile(path, []byte(header+f.body+"\n"), 0o644); err != nil {
				return err
			}
			*f.out = filepath.ToSlash(rel)
		}
	}
	return nil
}

func repoRoot(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", errors.New("not in a git checkout")
	}
	return strings.TrimSpace(string(out)), nil
}
