package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformRegistry_Basic(t *testing.T) {
	reg := NewTransformRegistry()
	from := OkapiFilterConfigKind("html")
	to := FormatConfigKind("html")
	reg.Register(from, to, TransformerFunc(func(spec map[string]any) (map[string]any, error) {
		result := make(map[string]any)
		for k, v := range spec {
			switch k {
			case "quoteMode", "quoteModeDefined":
				continue
			}
			result[k] = v
		}
		return result, nil
	}))

	spec := map[string]any{
		"parser":           map[string]any{"preserveWhitespace": true},
		"quoteMode":        3,
		"quoteModeDefined": true,
	}
	result, err := reg.Transform(from, to, spec)
	require.NoError(t, err)
	assert.NotNil(t, result["parser"])
	assert.Nil(t, result["quoteMode"])
	assert.Nil(t, result["quoteModeDefined"])
}

func TestTransformRegistry_NotFound(t *testing.T) {
	reg := NewTransformRegistry()
	from := OkapiFilterConfigKind("html")
	to := FormatConfigKind("html")

	_, err := reg.Transform(from, to, map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no transforms registered from")

	reg.Register(from, to, TransformerFunc(func(spec map[string]any) (map[string]any, error) {
		return spec, nil
	}))

	_, err = reg.Transform(from, FormatConfigKind("json"), map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no transform registered from")
}

func TestTransformRegistry_Has(t *testing.T) {
	reg := NewTransformRegistry()
	from := OkapiFilterConfigKind("html")
	to := FormatConfigKind("html")
	assert.False(t, reg.Has(from, to))

	reg.Register(from, to, TransformerFunc(func(spec map[string]any) (map[string]any, error) {
		return spec, nil
	}))
	assert.True(t, reg.Has(from, to))
	assert.False(t, reg.Has(from, FormatConfigKind("json")))
}

func TestTransformRegistry_TransformError(t *testing.T) {
	reg := NewTransformRegistry()
	from := OkapiFilterConfigKind("html")
	to := FormatConfigKind("html")
	reg.Register(from, to, TransformerFunc(func(spec map[string]any) (map[string]any, error) {
		return nil, fmt.Errorf("transform failed")
	}))

	_, err := reg.Transform(from, to, map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transform failed")
}

func TestRegistry_Basic(t *testing.T) {
	reg := NewRegistry()
	htmlKind := FormatConfigKind("html")
	reg.Register(htmlKind, SpecDecoderFunc(func(spec map[string]any) (any, error) {
		return spec, nil
	}))

	assert.True(t, reg.Has(htmlKind))
	assert.False(t, reg.Has(FormatConfigKind("json")))

	env := &Envelope{
		APIVersion: "v1",
		Kind:       htmlKind,
		Spec:       map[string]any{"parser": map[string]any{"preserveWhitespace": true}},
	}
	result, err := reg.Decode(env)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRegistry_NotFound(t *testing.T) {
	reg := NewRegistry()
	env := &Envelope{APIVersion: "v1", Kind: FormatConfigKind("html"), Spec: map[string]any{}}
	_, err := reg.Decode(env)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no decoder registered")
}
