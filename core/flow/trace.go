package flow

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// TraceEventType identifies the kind of trace event.
type TraceEventType string

const (
	TraceEnter TraceEventType = "enter"
	TraceExit  TraceEventType = "exit"
)

// TraceEvent represents a single timestamped event during flow execution.
type TraceEvent struct {
	TS     int64          `json:"ts"`               // microseconds from flow start
	Type   TraceEventType `json:"type"`             // TraceEnter or TraceExit
	NodeID string         `json:"nodeId"`           // which node
	PartID string         `json:"partId,omitempty"` // which Part
	Meta   map[string]any `json:"meta,omitempty"`   // extra data
}

// PartSnapshot captures the state of a Part at a point in time.
type PartSnapshot struct {
	ID         string `json:"id"`
	Type       string `json:"type"`                 // "LayerStart", "LayerEnd", "Block", "Data", "Media", "GroupStart", "GroupEnd"
	Summary    string `json:"summary"`              // short description
	SourceText string `json:"sourceText,omitempty"` // source text for blocks
	TargetText string `json:"targetText,omitempty"` // target text for blocks
	// Detail carries the full part structure for a rich inspector — run
	// sequences (inline codes preserved), every target locale, and properties.
	// Populated for Block parts; nil for structural parts.
	Detail *PartDetail `json:"detail,omitempty"`
}

// PartDetail is the run-native, full view of a Block at a point in time, for the
// "drill into a part" inspector. Source/Targets are run sequences (not flattened
// strings) so inline placeholders and paired codes survive. Overlays and
// Annotations carry the block's stand-off state so an inspector can show what
// each tool attached (AD-002): segmentation spans, term/entity tags, QA
// findings, the redaction secret annotation. They are summarized eagerly at
// snapshot time — blocks mutate in place as they flow, so a snapshot must not
// alias live maps.
type PartDetail struct {
	Name         string                 `json:"name,omitempty"`
	Translatable bool                   `json:"translatable,omitempty"`
	Source       []model.Run            `json:"source,omitempty"`
	Targets      map[string][]model.Run `json:"targets,omitempty"`
	Properties   map[string]string      `json:"properties,omitempty"`
	HasSkeleton  bool                   `json:"hasSkeleton,omitempty"`
	Overlays     []OverlaySnapshot      `json:"overlays,omitempty"`
	Annotations  []AnnotationSnapshot   `json:"annotations,omitempty"`
}

// OverlaySnapshot summarizes one stand-off overlay (AD-002) at snapshot time:
// its type, the side it annotates, and each span projected to rune offsets in
// that side's flattened text (with the covered text, so an inspector can
// highlight without re-deriving run anchoring).
type OverlaySnapshot struct {
	Type  string         `json:"type"`            // "segmentation", "term", "entity", "qa", …
	Side  string         `json:"side"`            // "source" or the target variant key
	Layer string         `json:"layer,omitempty"` // segmentation layer; "" = primary
	Spans []SpanSnapshot `json:"spans,omitempty"`
}

// SpanSnapshot is one overlay span projected to the flattened text.
type SpanSnapshot struct {
	Start int    `json:"start"` // rune offset, half-open [Start, End)
	End   int    `json:"end"`
	Text  string `json:"text,omitempty"` // covered text (clipped to 80 runes)
	Note  string `json:"note,omitempty"` // compact payload summary (entity type, QA message, …)
}

// AnnotationSnapshot is one block-scoped annotation (key + compact summary).
type AnnotationSnapshot struct {
	Key     string `json:"key"`
	Summary string `json:"summary,omitempty"`
}

// PartSnapshotSet holds the initial snapshot and snapshots after each node.
type PartSnapshotSet struct {
	Initial   PartSnapshot            `json:"initial"`
	AfterNode map[string]PartSnapshot `json:"afterNode,omitempty"`
}

// FlowTrace is the top-level output of a traced flow execution.
type FlowTrace struct {
	Name        string                      `json:"name"`
	Description string                      `json:"description"`
	Nodes       []TraceNode                 `json:"nodes"`
	ChannelSize int                         `json:"channelSize"`
	Events      []TraceEvent                `json:"events"`
	Parts       map[string]*PartSnapshotSet `json:"parts"`
	InputFile   TraceFile                   `json:"inputFile"`
	OutputFile  TraceFile                   `json:"outputFile"`
	DurationUs  int64                       `json:"durationUs"`
}

// TraceNode describes a node in the flow graph.
type TraceNode struct {
	ID    string   `json:"id"`
	Type  NodeType `json:"type"` // NodeReader, NodeTool, or NodeWriter
	Name  string   `json:"name"`
	Label string   `json:"label"`
}

// TraceFile describes an input or output file.
type TraceFile struct {
	Name    string `json:"name"`
	Format  string `json:"format,omitempty"`
	Preview string `json:"preview"`
}

// TraceRecorder is a thread-safe event collector for flow tracing.
type TraceRecorder struct {
	mu        sync.Mutex
	start     time.Time
	events    []TraceEvent
	snapshots map[string]*PartSnapshotSet
}

// NewTraceRecorder creates a new TraceRecorder with the clock starting now.
func NewTraceRecorder() *TraceRecorder {
	return &TraceRecorder{
		start:     time.Now(),
		events:    make([]TraceEvent, 0, 256),
		snapshots: make(map[string]*PartSnapshotSet),
	}
}

// Record adds a timestamped event to the trace.
func (r *TraceRecorder) Record(eventType TraceEventType, nodeID string, partID string, meta map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, TraceEvent{
		TS:     time.Since(r.start).Microseconds(),
		Type:   eventType,
		NodeID: nodeID,
		PartID: partID,
		Meta:   meta,
	})
}

// SnapshotPart captures a snapshot of a Part. When phase is "initial", the
// snapshot is stored as the initial state. Otherwise, phase is treated as the
// nodeID and stored in AfterNode.
func (r *TraceRecorder) SnapshotPart(part *model.Part, nodeID string, phase string) {
	snap := snapshotFromPart(part)
	r.mu.Lock()
	defer r.mu.Unlock()
	id := part.Resource.ResourceID()
	if phase == "initial" {
		r.snapshots[id] = &PartSnapshotSet{
			Initial:   snap,
			AfterNode: make(map[string]PartSnapshot),
		}
	} else if ss, ok := r.snapshots[id]; ok {
		ss.AfterNode[nodeID] = snap
	}
}

// Events returns a copy of all recorded events.
func (r *TraceRecorder) Events() []TraceEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]TraceEvent, len(r.events))
	copy(out, r.events)
	return out
}

// DurationUs returns the elapsed time in microseconds since the recorder was created.
func (r *TraceRecorder) DurationUs() int64 {
	return time.Since(r.start).Microseconds()
}

// Snapshots returns a copy of all recorded part snapshots.
func (r *TraceRecorder) Snapshots() map[string]*PartSnapshotSet {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]*PartSnapshotSet, len(r.snapshots))
	for k, v := range r.snapshots {
		out[k] = v
	}
	return out
}

// snapshotFromPart creates a PartSnapshot from a Part.
func snapshotFromPart(part *model.Part) PartSnapshot {
	if part == nil || part.Resource == nil {
		return PartSnapshot{Type: "Unknown", Summary: "<nil>"}
	}

	snap := PartSnapshot{
		ID:   part.Resource.ResourceID(),
		Type: part.Type.String(),
	}

	switch part.Type {
	case model.PartBlock:
		block, ok := part.Resource.(*model.Block)
		if ok {
			srcText := block.SourceText()
			snap.SourceText = srcText
			// Get target text from the first locale found.
			for _, loc := range block.TargetLocales() {
				snap.TargetText = block.TargetText(loc)
				break
			}
			// Summary: first 40 chars of source text.
			if len(srcText) > 40 {
				snap.Summary = srcText[:40] + "..."
			} else if srcText != "" {
				snap.Summary = srcText
			} else {
				snap.Summary = "empty block"
			}
			// Full detail for the inspector: run sequences + every locale.
			detail := &PartDetail{
				Name:         block.Name,
				Translatable: block.Translatable,
				Source:       block.SourceRuns(),
				Properties:   block.Properties,
				HasSkeleton:  block.Skeleton != nil,
			}
			if len(block.Targets) > 0 {
				detail.Targets = make(map[string][]model.Run, len(block.Targets))
				for _, loc := range block.TargetLocales() {
					detail.Targets[string(loc)] = block.TargetRuns(loc)
				}
			}
			detail.Overlays = snapshotOverlays(block)
			detail.Annotations = snapshotAnnotations(block)
			snap.Detail = detail
		}
	case model.PartLayerStart:
		layer, ok := part.Resource.(*model.Layer)
		if ok {
			snap.Summary = "Layer: " + layer.Name
			if snap.Summary == "Layer: " {
				snap.Summary = "Layer: " + layer.ID
			}
		}
	case model.PartLayerEnd:
		snap.Summary = "end layer " + snap.ID
	case model.PartData:
		data, ok := part.Resource.(*model.Data)
		if ok {
			snap.Summary = "Data: " + data.Name
			if snap.Summary == "Data: " {
				snap.Summary = "Data: " + data.ID
			}
		}
	case model.PartMedia:
		media, ok := part.Resource.(*model.Media)
		if ok {
			snap.Summary = "Media: " + media.MimeType
			if snap.Summary == "Media: " {
				snap.Summary = "Media: " + media.ID
			}
		}
	case model.PartGroupStart:
		gs, ok := part.Resource.(*model.GroupStart)
		if ok {
			snap.Summary = "Group: " + gs.Name
			if snap.Summary == "Group: " {
				snap.Summary = "Group: " + gs.ID
			}
		}
	case model.PartGroupEnd:
		snap.Summary = "end group " + snap.ID
	default:
		snap.Summary = snap.Type + ": " + snap.ID
	}

	return snap
}

// snapshotOverlays summarizes the block's stand-off overlays eagerly: each
// span is projected to rune offsets in its side's flattened text and carries
// the covered text plus a compact payload note. Nothing in the result aliases
// the live block (blocks mutate in place as they flow).
func snapshotOverlays(b *model.Block) []OverlaySnapshot {
	if len(b.Overlays) == 0 {
		return nil
	}
	out := make([]OverlaySnapshot, 0, len(b.Overlays))
	for i := range b.Overlays {
		o := &b.Overlays[i]
		side := "source"
		runs := b.Source
		if o.Variant != nil {
			if key, err := o.Variant.MarshalText(); err == nil {
				side = string(key)
			}
			if t := b.Targets[*o.Variant]; t != nil {
				runs = t.Runs
			} else {
				runs = nil
			}
		}
		text := model.RunsText(runs)
		os := OverlaySnapshot{Type: string(o.Type), Side: side, Layer: o.Layer}
		for _, s := range o.Spans {
			start, end := s.Range.TextSpan(runs)
			os.Spans = append(os.Spans, SpanSnapshot{
				Start: start,
				End:   end,
				Text:  clipRunes(sliceRunes(text, start, end), 80),
				Note:  payloadNote(s.Value),
			})
		}
		out = append(out, os)
	}
	return out
}

// snapshotAnnotations summarizes the block-scoped annotations (key + note),
// sorted by key for stable output.
func snapshotAnnotations(b *model.Block) []AnnotationSnapshot {
	annos := b.AnnoMap()
	if len(annos) == 0 {
		return nil
	}
	keys := make([]string, 0, len(annos))
	for k := range annos {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]AnnotationSnapshot, 0, len(keys))
	for _, k := range keys {
		out = append(out, AnnotationSnapshot{Key: k, Summary: payloadNote(annos[k])})
	}
	return out
}

// payloadNote renders a compact, human-readable summary of a stand-off
// payload for the trace inspector: the well-known content payloads get a
// specific note, anything else falls back to its registered type name.
func payloadNote(p model.Payload) string {
	switch v := p.(type) {
	case nil:
		return ""
	case *model.EntityAnnotation:
		return string(v.Type)
	case *model.TermAnnotation:
		note := "term: " + v.SourceTerm
		if v.ConceptID != "" {
			note += " (" + v.ConceptID + ")"
		}
		return note
	case *check.FindingsAnnotation:
		if len(v.Findings) == 0 {
			return "no findings"
		}
		note := fmt.Sprintf("%d finding(s)", len(v.Findings))
		if msg := v.Findings[0].Message; msg != "" {
			note += ": " + clipRunes(msg, 80)
		}
		return note
	case fmt.Stringer:
		return clipRunes(v.String(), 80)
	default:
		return p.TypeName()
	}
}

// sliceRunes returns text[start:end) in rune offsets, clamped to bounds.
func sliceRunes(text string, start, end int) string {
	r := []rune(text)
	start = clampInt(start, 0, len(r))
	end = clampInt(end, start, len(r))
	return string(r[start:end])
}

// clipRunes truncates s to at most n runes, appending an ellipsis when clipped.
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func clampInt(v, lo, hi int) int { return min(max(v, lo), hi) }

// TracingTool wraps a tool.Tool and records enter/exit events for each Part.
type TracingTool struct {
	inner    tool.Tool
	nodeID   string
	recorder *TraceRecorder
}

// NewTracingTool creates a TracingTool that wraps inner and records events to recorder.
func NewTracingTool(inner tool.Tool, nodeID string, recorder *TraceRecorder) *TracingTool {
	return &TracingTool{inner: inner, nodeID: nodeID, recorder: recorder}
}

// Name returns the wrapped tool's name.
func (t *TracingTool) Name() string { return t.inner.Name() }

// Description returns the wrapped tool's description.
func (t *TracingTool) Description() string { return t.inner.Description() }

// Config returns the wrapped tool's configuration.
func (t *TracingTool) Config() tool.ToolConfig { return t.inner.Config() }

// SetConfig applies configuration to the wrapped tool.
func (t *TracingTool) SetConfig(c tool.ToolConfig) error { return t.inner.SetConfig(c) }

// Process intercepts Parts flowing through the inner tool, recording
// enter events on input and exit events (with snapshots) on output.
//
// Channel ownership: the executor creates channels and closes the output
// channel after Process returns. BaseTool.Process does NOT close its output
// channel — it simply returns when input is exhausted. Therefore:
//  1. We close tracedOut after inner.Process returns so the output interceptor terminates.
//  2. We do NOT close out — the executor handles that.
//
// The input interceptor must not block forever if the inner tool stops
// reading early (mid-stream error or context cancellation): its forwarding
// send selects on ctx.Done() and a stop channel that is closed once
// inner.Process returns. Both interceptor goroutines are joined before
// Process returns so neither leaks on the happy path or the cancel/error path.
func (t *TracingTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	return t.trace(ctx, in, out, func(innerIn <-chan *model.Part, innerOut chan<- *model.Part) error {
		return t.inner.Process(ctx, innerIn, innerOut)
	})
}

// SessionProcess forwards the session contract to the wrapped tool when it
// is a SessionTool, while still recording trace events — so persistent
// overlay caching survives the tracing wrapper.
func (t *TracingTool) SessionProcess(ctx context.Context, sess blockstore.Session, in <-chan *model.Part, out chan<- *model.Part) error {
	st, ok := t.inner.(tool.SessionTool)
	if !ok {
		return t.Process(ctx, in, out)
	}
	return t.trace(ctx, in, out, func(innerIn <-chan *model.Part, innerOut chan<- *model.Part) error {
		return st.SessionProcess(ctx, sess, innerIn, innerOut)
	})
}

// trace runs run() with trace-recording channels spliced around the inner
// tool's in/out.
func (t *TracingTool) trace(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part, run func(<-chan *model.Part, chan<- *model.Part) error) error {
	tracedIn := make(chan *model.Part, cap(in))
	tracedOut := make(chan *model.Part, cap(out))

	// stop is closed once the inner tool returns, signalling the input
	// interceptor to abandon any pending forward and exit.
	stop := make(chan struct{})

	var interceptors sync.WaitGroup
	interceptors.Add(2)

	// Input interceptor: record enter events, then forward to inner tool.
	go func() {
		defer interceptors.Done()
		defer close(tracedIn)
		for part := range in {
			id := part.Resource.ResourceID()
			t.recorder.Record(TraceEnter, t.nodeID, id, nil)
			select {
			case tracedIn <- part:
			case <-ctx.Done():
				return
			case <-stop:
				return
			}
		}
	}()

	// Output interceptor: record exit events and snapshots, then forward.
	go func() {
		defer interceptors.Done()
		for part := range tracedOut {
			id := part.Resource.ResourceID()
			t.recorder.SnapshotPart(part, t.nodeID, t.nodeID)
			t.recorder.Record(TraceExit, t.nodeID, id, nil)
			select {
			case out <- part:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Run the inner tool. BaseTool.Process returns when input is exhausted
	// but does not close its output channel.
	err := run(tracedIn, tracedOut)

	// Signal the input interceptor to stop forwarding (the inner tool is no
	// longer reading tracedIn) and close tracedOut so the output interceptor
	// goroutine terminates.
	close(stop)
	close(tracedOut)

	// Join both interceptors so neither goroutine outlives Process.
	interceptors.Wait()

	return err
}

// Verify TracingTool implements tool.Tool at compile time.
var _ tool.Tool = (*TracingTool)(nil)

// NewTraceRecorderWithStart creates a TraceRecorder using a shared start time.
// This allows multiple recorders to use the same time origin for batch tracing.
func NewTraceRecorderWithStart(start time.Time) *TraceRecorder {
	return &TraceRecorder{
		start:     start,
		events:    make([]TraceEvent, 0, 256),
		snapshots: make(map[string]*PartSnapshotSet),
	}
}

// BatchFlowTrace captures tracing data for a multi-file batch run.
type BatchFlowTrace struct {
	Name        string          `json:"name"`
	Concurrency int             `json:"concurrency"`
	FileTraces  []FileFlowTrace `json:"fileTraces"`
	DurationUs  int64           `json:"durationUs"`
}

// FileFlowTrace captures tracing data for a single file within a batch.
type FileFlowTrace struct {
	File       string       `json:"file"`
	Format     string       `json:"format"`
	StartUs    int64        `json:"startUs"`
	EndUs      int64        `json:"endUs"`
	Lane       int          `json:"lane"`
	Nodes      []TraceNode  `json:"nodes"`
	Events     []TraceEvent `json:"events"`
	DurationUs int64        `json:"durationUs"`
}
