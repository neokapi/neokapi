package vtt

import "github.com/gokapi/gokapi/core/config"

// okapiVTTTransformer converts okf_vtt filter config specs to native VTT config.
// The native VTT format has no configurable parameters, so all Okapi-specific
// parameters (maxCharsPerLine, maxLinesPerCaption, mergeCaptions, discardCues,
// overwriteCues, keepTimecodes, splitWords) are dropped silently.
type okapiVTTTransformer struct{}

func (okapiVTTTransformer) Transform(spec map[string]any) (map[string]any, error) {
	// The native VTT format has no parameters — drop everything.
	return make(map[string]any), nil
}

func init() {
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("vtt"), config.FormatConfigKind("vtt"),
		okapiVTTTransformer{},
	)
}
