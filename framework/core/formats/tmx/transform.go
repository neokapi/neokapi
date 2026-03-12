package tmx

import "github.com/gokapi/gokapi/core/config"

// okapiTMXTransformer converts OkfTmxFilterConfig specs to TmxFormatConfig.
// The TMX format has no configurable parameters, so all okapi-specific
// wrapper params are dropped silently.
type okapiTMXTransformer struct{}

func (okapiTMXTransformer) Transform(spec map[string]any) (map[string]any, error) {
	// TMX has no native config params — drop everything from the okapi spec.
	return map[string]any{}, nil
}

func init() {
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("tmx"), config.FormatConfigKind("tmx"),
		okapiTMXTransformer{},
	)
}
