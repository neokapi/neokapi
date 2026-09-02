// Package flowdef carries the product's flow-definition concerns on top of
// the framework's flow graph model (core/flow): the built-in flow catalog
// (BuiltInFlows) and the filesystem store for user flow definitions
// (FlowStore). The engine — graph model, executor, file runner — stays in
// core/flow; this package is what the kapi CLI, Kapi Desktop, and the bowrain
// platform share when they list, resolve, or persist flows.
package flowdef

import (
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
)

// BuiltInFlows returns the default set of built-in flow definitions. The graphs
// are tool nodes only; a flow's I/O ends are bindings resolved at run time, not
// nodes (AD-026).
func BuiltInFlows() []flow.FlowDefinition {
	return []flow.FlowDefinition{
		{
			ID:          "translate",
			Name:        "Translate",
			Description: "Translate with guardrails: content memory reuse, then AI translate, then deterministic checks",
			Source:      registry.SourceBuiltIn,
			// The porcelain `kapi translate` runs this flow, so the pitch
			// command carries the three pillars: recycle leverages a bound
			// content memory (a no-op without one), translate produces, and the
			// deterministic check pass verifies placeholders/tags before the
			// result is written. The raw tool stays at `kapi exec translate`.
			//
			// skipMatched is what makes the chain mean "recycle, then translate
			// the REMAINDER". The translate tool's own default is to translate
			// every translatable block it is handed, which is right for `kapi
			// exec translate` — you pointed it at the content — and wrong here:
			// without it the AI step overwrites every unit recycle had just
			// filled from approved wording, so a project with a full content
			// memory paid for a model call per unit and shipped the model's
			// answer instead of its own.
			Nodes: []flow.FlowNode{
				{ID: "recycle", Type: flow.NodeTool, Name: "recycle", Label: "Memory Reuse", Position: flow.NodePosition{X: 0, Y: 100}},
				{ID: "translate", Type: flow.NodeTool, Name: "translate", Label: "Translate", Position: flow.NodePosition{X: 250, Y: 100},
					Config: map[string]any{"skipMatched": true}},
				{ID: "qa", Type: flow.NodeTool, Name: "qa", Label: "Quality Check", Position: flow.NodePosition{X: 500, Y: 100}},
			},
			Edges: []flow.FlowEdge{
				{ID: "e-recycle-translate", Source: "recycle", Target: "translate"},
				{ID: "e-translate-qa", Source: "translate", Target: "qa"},
			},
		},
		{
			ID:          "translate-qa",
			Name:        "Translate + Quality Check",
			Description: "Translate content then run an LLM-judged quality check",
			Source:      registry.SourceBuiltIn,
			Nodes: []flow.FlowNode{
				{ID: "translate", Type: flow.NodeTool, Name: "translate", Label: "Translate", Position: flow.NodePosition{X: 0, Y: 100}},
				// Pin a provider so the check step runs the LLM judge rather
				// than the deterministic default. --provider overrides it.
				{ID: "qa", Type: flow.NodeTool, Name: "qa", Label: "Quality Check", Position: flow.NodePosition{X: 250, Y: 100},
					Config: map[string]any{"provider": "anthropic"}},
			},
			Edges: []flow.FlowEdge{
				{ID: "e-translate-qa", Source: "translate", Target: "qa"},
			},
		},
		{
			ID:          "pseudo-translate",
			Name:        "Pseudo Translate",
			Description: "Generate pseudo-translations for testing",
			Source:      registry.SourceBuiltIn,
			Nodes: []flow.FlowNode{
				{ID: "pseudo-translate", Type: flow.NodeTool, Name: "pseudo-translate", Label: "Pseudo Translate", Position: flow.NodePosition{X: 0, Y: 100}},
			},
		},
		{
			ID:          "qa",
			Name:        "Quality Check",
			Description: "Run rule-based quality checks on translations",
			Source:      registry.SourceBuiltIn,
			Nodes: []flow.FlowNode{
				{ID: "qa", Type: flow.NodeTool, Name: "qa", Label: "Quality Check", Position: flow.NodePosition{X: 0, Y: 100}},
			},
		},
		{
			ID:          "recycle",
			Name:        "Recycle",
			Description: "Pre-fill translations from content memory",
			Source:      registry.SourceBuiltIn,
			Nodes: []flow.FlowNode{
				{ID: "recycle", Type: flow.NodeTool, Name: "recycle", Label: "Recycle", Position: flow.NodePosition{X: 0, Y: 100}},
			},
		},
		{
			ID:          "audio-to-subtitles",
			Name:        "Audio to Subtitles",
			Description: "Transcribe an audio file (ASR) and AI-translate the cues. Pair with a subtitle output (.vtt/.srt) to produce a translated subtitle track",
			Source:      registry.SourceBuiltIn,
			Nodes: []flow.FlowNode{
				{ID: "translate", Type: flow.NodeTool, Name: "translate", Label: "Translate", Position: flow.NodePosition{X: 0, Y: 100}},
			},
		},
		{
			ID:          "video-to-subtitles",
			Name:        "Video to Subtitles",
			Description: "Demux a video and AI-translate the spoken (timing-anchored) cues. Pair with a subtitle output (.vtt/.srt) to produce a translated subtitle track",
			Source:      registry.SourceBuiltIn,
			Nodes: []flow.FlowNode{
				{ID: "translate", Type: flow.NodeTool, Name: "translate", Label: "Translate", Position: flow.NodePosition{X: 0, Y: 100}},
			},
		},
		{
			ID:          "image-ocr-translate",
			Name:        "Image OCR Translate",
			Description: "AI-translate the text OCR'd from an image. Round-trips translated alt-text to the image's sidecar (the whole image is also replaceable per locale)",
			Source:      registry.SourceBuiltIn,
			Nodes: []flow.FlowNode{
				{ID: "translate", Type: flow.NodeTool, Name: "translate", Label: "Translate", Position: flow.NodePosition{X: 0, Y: 100}},
			},
		},
		{
			ID:          "secure-translate",
			Name:        "Secure Translate",
			Description: "Redact sensitive content, AI-translate, then restore the originals locally",
			Source:      registry.SourceBuiltIn,
			// Redact precedes the remote-egress step (translate) — the order the
			// placement pass enforces; unredact restores after translation.
			Nodes: []flow.FlowNode{
				{ID: "redact", Type: flow.NodeTool, Name: "redact", Label: "Redact", Position: flow.NodePosition{X: 0, Y: 100}},
				{ID: "translate", Type: flow.NodeTool, Name: "translate", Label: "Translate", Position: flow.NodePosition{X: 250, Y: 100}},
				{ID: "unredact", Type: flow.NodeTool, Name: "unredact", Label: "Unredact", Position: flow.NodePosition{X: 500, Y: 100}},
			},
			Edges: []flow.FlowEdge{
				{ID: "e-redact-translate", Source: "redact", Target: "translate"},
				{ID: "e-translate-unredact", Source: "translate", Target: "unredact"},
			},
		},
		{
			ID:          "redact-pii",
			Name:        "Redact PII",
			Description: "Detect named entities (NER) and redact people, organizations, locations, and dates before processing",
			Source:      registry.SourceBuiltIn,
			// Ordinary ordered steps (AD-006): the NER annotator runs first and its
			// entity overlay drives redact, which produces the source rewrite. The
			// NER step egresses source during detection — the documented AD-020
			// trade-off the placement pass permits because redact consumes the
			// entity port that step produces.
			Nodes: []flow.FlowNode{
				{ID: "entity-extract", Type: flow.NodeTool, Name: "entity-extract", Label: "Detect Entities (NER)", Position: flow.NodePosition{X: 0, Y: 100}},
				{ID: "redact", Type: flow.NodeTool, Name: "redact", Label: "Redact", Position: flow.NodePosition{X: 250, Y: 100},
					Config: map[string]any{
						"detectors":   []string{"entities"},
						"entityTypes": []string{"person", "org", "location", "date"},
					}},
			},
			Edges: []flow.FlowEdge{
				{ID: "e-extract-redact", Source: "entity-extract", Target: "redact"},
			},
		},
	}
}
