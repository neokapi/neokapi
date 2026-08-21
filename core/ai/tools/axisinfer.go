package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/ai/prompt"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// AxisProposal is one dimension a corpus varies along, and the values it takes.
//
// The axis name is the corpus' own vocabulary, not a name from a fixed list:
// one project has product lines, another has regional markets, another has
// audiences. That openness is the point — SyncContextEntry.coordinates is a
// map<string,string> precisely so a project's context space can be its own.
type AxisProposal struct {
	Axis       string   `json:"axis"`
	Values     []string `json:"values"`
	Evidence   []string `json:"evidence,omitempty"`
	Confidence float64  `json:"confidence"`
}

// AxisOptions configures an axis-discovery run.
type AxisOptions struct {
	// Known are axes the project already declares. The model is told about them
	// so it proposes values ON them rather than re-proposing the dimension under
	// a synonym.
	Known []string
	// Domain is an optional subject-domain hint ("medical", "developer tools").
	Domain string
	// MaxAxes caps how many axes to report. Defaults to 4.
	MaxAxes int
	// MaxCorpusChars bounds how much corpus text is sent. Defaults to 24000.
	MaxCorpusChars int
}

func (o AxisOptions) withDefaults() AxisOptions {
	if o.MaxAxes <= 0 {
		o.MaxAxes = 4
	}
	if o.MaxCorpusChars <= 0 {
		o.MaxCorpusChars = 24000
	}
	return o
}

// axisToken is the shape an axis name and an axis value must take to be usable
// as a coordinate: lower_snake_case, which is what a recipe writes and what the
// context hash sorts. A model asked for that shape mostly obliges; this is what
// happens when it does not.
var axisToken = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// InferAxes reads a corpus for the dimensions its content varies along.
//
// It returns an empty slice, not an error, when the corpus varies along
// nothing: a project whose content is uniform genuinely has no axes, and that
// is the common case for a first scan of one small site. Callers must not read
// an empty result as a failure.
func InferAxes(ctx context.Context, provider aiprovider.LLMProvider, corpus string, opts AxisOptions) ([]AxisProposal, aiprovider.TokenUsage, error) {
	var usage aiprovider.TokenUsage
	if provider == nil {
		return nil, usage, errors.New("axis-discover: nil provider")
	}
	corpus = strings.TrimSpace(corpus)
	if corpus == "" {
		return nil, usage, ErrEmptyCorpus
	}
	opts = opts.withDefaults()
	corpus = truncateRunes(corpus, opts.MaxCorpusChars)

	turns := prompt.AxisDiscover{
		Known:   opts.Known,
		Domain:  opts.Domain,
		MaxAxes: opts.MaxAxes,
	}.Turns(corpus)
	ctx = prompt.WithID(ctx, prompt.IDAxisDiscover)

	resp, err := provider.ChatStructured(ctx, aiprovider.MessagesFromTurns(turns), axisDiscoverLLMSchema())
	if err != nil {
		return nil, usage, fmt.Errorf("axis-discover: %w", err)
	}
	usage = resp.Usage

	var out struct {
		Axes []AxisProposal `json:"axes"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &out); err != nil {
		return nil, usage, fmt.Errorf("axis-discover: decode structured response: %w", err)
	}
	return sanitizeAxes(out.Axes, opts.MaxAxes), usage, nil
}

// sanitizeAxes drops what cannot be a coordinate and normalises what can.
//
// A proposal is dropped rather than repaired when its name is not a usable axis
// token: guessing at what the model meant would put a coordinate into the graph
// that the corpus never argued for. A single-valued axis is dropped too — one
// value is a fact about the project, not a dimension it varies along, and the
// prompt says so.
func sanitizeAxes(in []AxisProposal, maxAxes int) []AxisProposal {
	seen := map[string]bool{}
	out := make([]AxisProposal, 0, len(in))
	for _, a := range in {
		axis := strings.ToLower(strings.TrimSpace(a.Axis))
		if !axisToken.MatchString(axis) || seen[axis] {
			continue
		}

		values := make([]string, 0, len(a.Values))
		valueSeen := map[string]bool{}
		for _, v := range a.Values {
			v = strings.ToLower(strings.TrimSpace(v))
			if !axisToken.MatchString(v) || valueSeen[v] {
				continue
			}
			valueSeen[v] = true
			values = append(values, v)
		}
		if len(values) < 2 {
			continue
		}
		sort.Strings(values)

		conf := a.Confidence
		if conf < 0 {
			conf = 0
		}
		if conf > 1 {
			conf = 1
		}

		seen[axis] = true
		out = append(out, AxisProposal{
			Axis:       axis,
			Values:     values,
			Evidence:   a.Evidence,
			Confidence: conf,
		})
		if len(out) == maxAxes {
			break
		}
	}
	return out
}

// axisDiscoverLLMSchema is the structured-output contract for axis discovery.
func axisDiscoverLLMSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name: "axis_discovery",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"axes": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"axis": map[string]any{
								"type":        "string",
								"description": "lower_snake_case name of the dimension, in the corpus' own vocabulary",
							},
							"values": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"description": "lower_snake_case values seen in the corpus; at least two",
							},
							"evidence": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"description": "short quotes or references supporting the axis",
							},
							"confidence": map[string]any{
								"type":    "number",
								"minimum": 0,
								"maximum": 1,
							},
						},
						"required": []string{"axis", "values", "confidence"},
					},
				},
			},
			"required": []string{"axes"},
		},
	}
}
