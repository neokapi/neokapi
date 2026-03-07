package config

import "fmt"

// RegisterFormatDecoder registers a SpecDecoder for a native format kind.
func RegisterFormatDecoder(reg *Registry, kind Kind, applyFn func(spec map[string]any) error) {
	reg.Register(kind, SpecDecoderFunc(func(spec map[string]any) (any, error) {
		if err := applyFn(spec); err != nil {
			return nil, fmt.Errorf("decode %s: %w", kind, err)
		}
		return spec, nil
	}))
}
