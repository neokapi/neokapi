package host

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
	"gopkg.in/yaml.v3"
)

// `kapi exec voice-infer` had no output at all: the tool drafted a profile,
// stored it on itself, and exposed it through Draft(), which nothing in the
// tree called. Not the CLI, not the platform, not the desktop app. Its
// --profile-name flag was accepted and did nothing. See issue #2225.
//
// Two things had to be true for a draft to come out, and neither was.
//
// The run needed somewhere to put it. Every other exec tool reports through a
// collector, and voice-infer declares no output port, so it got none.
//
// And the draft had to be of the corpus rather than of a file. RunToolOnFiles
// builds a tool per file and a collector once per run, so a per-file tool
// inferring from its own stream gives one partial profile per document.
// `kapi exec voice-infer docs/*.md` should answer with the voice of docs/, not
// with a stack of guesses. The collector is the piece that sees every file, so
// it accumulates the text and infers once.

// voiceInferCollector gathers a run's source text and drafts one profile from
// it.
type voiceInferCollector struct {
	// newTool builds the tool the inference runs through. It is the same
	// factory the pipeline uses, so the draft is made with the provider,
	// model and credentials the command resolved, without this file having to
	// resolve any of them again.
	newTool func() (tool.Tool, error)

	mu     sync.Mutex
	corpus strings.Builder
	blocks int
}

// NewVoiceInferCollectorFor returns a collector factory for voice-infer, or nil
// for any other tool.
func NewVoiceInferCollectorFor(toolName string, newTool func() (tool.Tool, error)) func() flow.Collector {
	if toolName != "voice-infer" {
		return nil
	}
	return func() flow.Collector { return &voiceInferCollector{newTool: newTool} }
}

// maxInferCorpusBytes bounds what one run accumulates.
//
// The inference truncates to its own rune budget anyway, and a run over a large
// directory should not hold the whole tree in memory to hand a fraction of it
// to a model. Four bytes per rune is the worst case for UTF-8, so this is at
// least as many runes as the tool will use.
const maxInferCorpusBytes = 4 * aitools.DefaultMaxCorpusChars

// Collect appends one document's source text.
func (c *voiceInferCollector) Collect(_ context.Context, _ *flow.Item, parts []*model.Part) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range parts {
		if p == nil || p.Type != model.PartBlock || c.corpus.Len() >= maxInferCorpusBytes {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok || b == nil {
			continue
		}
		text := strings.TrimSpace(b.SourceText())
		if text == "" {
			continue
		}
		c.blocks++
		if c.corpus.Len() > 0 {
			c.corpus.WriteString("\n")
		}
		c.corpus.WriteString(text)
	}
	return nil
}

// Result drafts the profile and renders it as the YAML a reader can save,
// validate and import.
//
// YAML rather than a report envelope: the useful next thing to do with a draft
// is `> voice.yaml` and then `kapi voice validate`, and a profile wrapped in a
// findings shape would have to be unwrapped first.
func (c *voiceInferCollector) Result() (flow.CollectorResult, error) {
	c.mu.Lock()
	corpus, blocks := c.corpus.String(), c.blocks
	c.mu.Unlock()

	if strings.TrimSpace(corpus) == "" {
		return flow.CollectorResult{}, errors.New("voice-infer: no source text in the input")
	}
	t, err := c.newTool()
	if err != nil {
		return flow.CollectorResult{}, err
	}
	inferrer, ok := t.(*aitools.VoiceInferTool)
	if !ok {
		return flow.CollectorResult{}, errors.New("voice-infer: the registry returned a different tool")
	}
	draft, _, err := inferrer.InferFrom(context.Background(), corpus)
	if err != nil {
		return flow.CollectorResult{}, err
	}
	return flow.CollectorResult{Name: "profile", Data: voiceDraft{Profile: draft, Blocks: blocks}}, nil
}

// voiceDraft is the drafted profile, rendered as a profile.
type voiceDraft struct {
	Profile *coreprofile.VoiceProfile `json:"profile"`
	// Blocks is how much text the draft was made from, so a reader can tell a
	// profile inferred from a corpus apart from one inferred from a paragraph.
	Blocks int `json:"blocks"`
}

// FormatTable writes the draft as YAML, which is what a profile is.
func (d voiceDraft) FormatTable(w io.Writer) {
	if d.Profile == nil {
		return
	}
	out, err := yaml.Marshal(d.Profile)
	if err != nil {
		return
	}
	_, _ = w.Write(out)
}
