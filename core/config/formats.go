package config

import "fmt"

// RegisterFormatDecoder registers a SpecDecoder for a native format's apiVersion.
// The decoder applies the spec map to the format's config using the provided
// applyFn. This is a convenience wrapper for the common pattern of decoding
// envelope specs into format configs.
func RegisterFormatDecoder(reg *Registry, apiVersion string, applyFn func(spec map[string]any) error) {
	reg.Register(apiVersion, SpecDecoderFunc(func(spec map[string]any) (any, error) {
		if err := applyFn(spec); err != nil {
			return nil, fmt.Errorf("decode %s: %w", apiVersion, err)
		}
		return spec, nil
	}))
}
