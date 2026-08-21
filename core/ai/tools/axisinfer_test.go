package tools

import (
	"context"
	"errors"
	"testing"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What survives sanitisation becomes a coordinate, so this is where a model's
// looser output stops being trusted. Everything dropped here is dropped rather
// than repaired: guessing at what a proposal meant would put a point into the
// graph the corpus never argued for.
func TestSanitizeAxes(t *testing.T) {
	tests := []struct {
		name string
		in   []AxisProposal
		want []AxisProposal
	}{
		{
			name: "normalises case and whitespace, sorts values",
			in: []AxisProposal{
				{Axis: " Product_Line ", Values: []string{"Desktop", " cloud "}, Confidence: 0.8},
			},
			want: []AxisProposal{
				{Axis: "product_line", Values: []string{"cloud", "desktop"}, Confidence: 0.8},
			},
		},
		{
			name: "drops an axis with one value: that is a fact, not a dimension",
			in: []AxisProposal{
				{Axis: "brand", Values: []string{"acme"}, Confidence: 0.9},
				{Axis: "market", Values: []string{"emea", "japan"}, Confidence: 0.7},
			},
			want: []AxisProposal{
				{Axis: "market", Values: []string{"emea", "japan"}, Confidence: 0.7},
			},
		},
		{
			name: "drops a name that cannot be a coordinate",
			in: []AxisProposal{
				{Axis: "product line", Values: []string{"cloud", "desktop"}, Confidence: 0.8},
				{Axis: "Brand!", Values: []string{"acme", "northwind"}, Confidence: 0.8},
				{Axis: "9lives", Values: []string{"a", "b"}, Confidence: 0.8},
			},
			want: []AxisProposal{},
		},
		{
			name: "drops values that cannot be coordinates, keeping the rest",
			in: []AxisProposal{
				{Axis: "audience", Values: []string{"developer", "the buyer", "operator"}, Confidence: 0.6},
			},
			want: []AxisProposal{
				{Axis: "audience", Values: []string{"developer", "operator"}, Confidence: 0.6},
			},
		},
		{
			name: "a value list that falls below two after cleaning drops the axis",
			in: []AxisProposal{
				{Axis: "audience", Values: []string{"developer", "the buyer"}, Confidence: 0.6},
			},
			want: []AxisProposal{},
		},
		{
			name: "deduplicates axes and values",
			in: []AxisProposal{
				{Axis: "market", Values: []string{"emea", "EMEA", "japan"}, Confidence: 0.7},
				{Axis: "Market", Values: []string{"us", "ca"}, Confidence: 0.5},
			},
			want: []AxisProposal{
				{Axis: "market", Values: []string{"emea", "japan"}, Confidence: 0.7},
			},
		},
		{
			name: "clamps confidence into range",
			in: []AxisProposal{
				{Axis: "market", Values: []string{"emea", "japan"}, Confidence: 1.4},
				{Axis: "audience", Values: []string{"developer", "operator"}, Confidence: -0.2},
			},
			want: []AxisProposal{
				{Axis: "market", Values: []string{"emea", "japan"}, Confidence: 1},
				{Axis: "audience", Values: []string{"developer", "operator"}, Confidence: 0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeAxes(tt.in, 4)
			require.Len(t, got, len(tt.want))
			for i := range tt.want {
				assert.Equal(t, tt.want[i].Axis, got[i].Axis)
				assert.Equal(t, tt.want[i].Values, got[i].Values)
				assert.InDelta(t, tt.want[i].Confidence, got[i].Confidence, 0.001)
			}
		})
	}
}

// A corpus with real structure can still overwhelm a reviewer, so the cap is
// enforced on the way out rather than trusted from the prompt.
func TestSanitizeAxesCapsTheList(t *testing.T) {
	in := []AxisProposal{
		{Axis: "brand", Values: []string{"acme", "northwind"}, Confidence: 0.9},
		{Axis: "product", Values: []string{"editor", "analytics"}, Confidence: 0.8},
		{Axis: "market", Values: []string{"emea", "japan"}, Confidence: 0.7},
		{Axis: "audience", Values: []string{"developer", "buyer"}, Confidence: 0.6},
	}
	got := sanitizeAxes(in, 2)
	require.Len(t, got, 2)
	assert.Equal(t, "brand", got[0].Axis, "the cap keeps the order the model reported")
	assert.Equal(t, "product", got[1].Axis)
}

// An empty result is the correct answer for a corpus that varies along nothing,
// and callers must not read it as a failure.
func TestSanitizeAxesOnNothing(t *testing.T) {
	assert.Empty(t, sanitizeAxes(nil, 4))
	assert.Empty(t, sanitizeAxes([]AxisProposal{}, 4))
}

// stubAxisProvider returns a canned structured response, or an error, so the
// tool's own handling can be exercised without a model. The interface is
// embedded rather than reimplemented: this exercises one method, and a stub
// that has to grow every time the provider interface does teaches nothing.
type stubAxisProvider struct {
	aiprovider.LLMProvider
	content string
	err     error
}

func (s stubAxisProvider) ChatStructured(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &aiprovider.ChatResponse{Content: s.content}, nil
}

func TestInferAxesReadsTheModelsProposals(t *testing.T) {
	p := stubAxisProvider{content: `{"axes":[
		{"axis":"product_line","values":["cloud","desktop"],"confidence":0.8,"evidence":["Cloud plans","Desktop download"]},
		{"axis":"market","values":["emea","japan"],"confidence":0.6}
	]}`}

	axes, _, err := InferAxes(t.Context(), p, "some corpus", AxisOptions{})
	require.NoError(t, err)
	require.Len(t, axes, 2)
	assert.Equal(t, "product_line", axes[0].Axis)
	assert.Equal(t, []string{"cloud", "desktop"}, axes[0].Values)
	assert.NotEmpty(t, axes[0].Evidence)
	assert.Equal(t, "market", axes[1].Axis)
}

// A corpus that varies along nothing has no axes, and that is an answer rather
// than a failure — the common case for a first scan of one small site.
func TestInferAxesOnAUniformCorpus(t *testing.T) {
	p := stubAxisProvider{content: `{"axes":[]}`}

	axes, _, err := InferAxes(t.Context(), p, "some corpus", AxisOptions{})
	require.NoError(t, err)
	assert.Empty(t, axes)
}

func TestInferAxesRefusesWhatItCannotRead(t *testing.T) {
	t.Run("nil provider", func(t *testing.T) {
		_, _, err := InferAxes(t.Context(), nil, "corpus", AxisOptions{})
		require.Error(t, err)
	})

	t.Run("empty corpus", func(t *testing.T) {
		_, _, err := InferAxes(t.Context(), stubAxisProvider{content: `{"axes":[]}`}, "   ", AxisOptions{})
		require.ErrorIs(t, err, ErrEmptyCorpus)
	})

	t.Run("provider failure", func(t *testing.T) {
		_, _, err := InferAxes(t.Context(), stubAxisProvider{err: errors.New("upstream is down")}, "corpus", AxisOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "axis-discover")
	})

	t.Run("unreadable response", func(t *testing.T) {
		_, _, err := InferAxes(t.Context(), stubAxisProvider{content: "not json"}, "corpus", AxisOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode")
	})
}
