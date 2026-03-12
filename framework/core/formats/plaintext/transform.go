package plaintext

import "github.com/gokapi/gokapi/core/config"

// okapiPlaintextTransformer converts OkfPlaintextFilterConfig specs to
// PlaintextFormatConfig. The native plaintext format supports the same
// parameter set, so most parameters pass through unchanged. Only
// okapi-specific wrapper params are dropped.
type okapiPlaintextTransformer struct{}

func (okapiPlaintextTransformer) Transform(spec map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for key, val := range spec {
		switch key {
		// Okapi-only params — drop silently.
		// parametersClass selects the Java filter variant (paragraphs, spliced, regex).
		// The native format uses segmentByLine instead.
		case "parametersClass":
			continue
		default:
			result[key] = val
		}
	}
	return result, nil
}

func init() {
	config.DefaultTransforms.Register(
		config.OkapiFilterConfigKind("plaintext"), config.FormatConfigKind("plaintext"),
		okapiPlaintextTransformer{},
	)
}
