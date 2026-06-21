package pluginhost

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/neokapi/neokapi/core/segment"
)

// daemonSegmenter implements segment.Segmenter by routing one block's
// code-masked text to a Mode-C daemon's BridgeService.Segment RPC and projecting
// the returned boundaries back to run-anchored spans. It is the single generic
// bridge for every plugin-declared segmenter — no per-plugin code: the engine
// name, plugin route, and config params are all data captured at registration.
type daemonSegmenter struct {
	pool   *DaemonPool
	plugin *Plugin
	engine string // manifest capabilities.segmenters[].name → SegmentRequest.engine
	mask   segment.MaskOptions
	lang   string
	params map[string]string
}

// newDaemonSegmenter builds the bridge segmenter. params is the unified config
// map for the segmentation tool; the engine-relevant entries (e.g. model,
// threshold) are forwarded as strings and the daemon reads the ones it knows.
func newDaemonSegmenter(pool *DaemonPool, plugin *Plugin, engine string, base segment.BaseConfig, params map[string]any) *daemonSegmenter {
	sp := make(map[string]string, len(params))
	for k, v := range params {
		sp[k] = fmt.Sprint(v)
	}
	return &daemonSegmenter{
		pool:   pool,
		plugin: plugin,
		engine: engine,
		mask:   base.Mask,
		lang:   base.Language,
		params: sp,
	}
}

// Layer reports that plugin segmenters produce primary sentence segmentation.
// (A plugin that needs a different layer can be extended later via the manifest.)
func (d *daemonSegmenter) Layer() string { return segment.LayerSentence }

// Segment flattens the runs under the mask, asks the daemon for interior
// boundaries, and projects them to run-anchored spans — mirroring the in-process
// engines, which also operate over the flattened text and call Flattened.Spans.
func (d *daemonSegmenter) Segment(ctx context.Context, runs []model.Run, loc model.LocaleID) ([]model.Span, error) {
	fl := segment.Flatten(runs, d.mask)
	text := fl.Text()
	if text == "" {
		return nil, nil
	}
	locale := d.lang
	if locale == "" {
		locale = string(loc)
	}

	client, err := d.pool.Acquire(ctx, d.plugin)
	if err != nil {
		return nil, fmt.Errorf("acquire daemon for plugin %q: %w", d.plugin.Name(), err)
	}
	resp, err := pb.NewBridgeServiceClient(client.Conn).Segment(ctx, &pb.SegmentRequest{
		Engine: d.engine,
		Text:   text,
		Locale: locale,
		Params: d.params,
	})
	if err != nil {
		return nil, fmt.Errorf("segment (plugin %q): %w", d.plugin.Name(), err)
	}
	if resp.GetError() != "" {
		return nil, fmt.Errorf("segment (plugin %q): %s", d.plugin.Name(), resp.GetError())
	}

	boundaries := make([]int, 0, len(resp.GetBoundaries()))
	for _, b := range resp.GetBoundaries() {
		boundaries = append(boundaries, int(b))
	}
	return fl.Spans(boundaries), nil
}

var _ segment.Segmenter = (*daemonSegmenter)(nil)
