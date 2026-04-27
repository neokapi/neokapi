package project

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRegisterExtension_RoundTrip(t *testing.T) {
	ResetExtensionsForTest()
	defer ResetExtensionsForTest()

	type ServerSpec struct {
		URL string `yaml:"url"`
	}

	RegisterExtension(Extension{
		Name:  "server",
		Scope: ScopeProject,
		Group: "platform",
		Decoder: ExtensionDecoderFunc(func(n yaml.Node) error {
			var s ServerSpec
			if err := n.Decode(&s); err != nil {
				return err
			}
			if s.URL == "" {
				return errors.New("url is required")
			}
			return nil
		}),
	})

	p := &KapiProject{Version: CurrentVersion}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
server:
  url: https://example.com/team/proj
`), p))

	require.NoError(t, p.Validate())
	require.NotNil(t, p.Extras["server"])
}

func TestValidate_ExtensionDecoderError(t *testing.T) {
	ResetExtensionsForTest()
	defer ResetExtensionsForTest()

	RegisterExtension(Extension{
		Name:  "server",
		Scope: ScopeProject,
		Group: "platform",
		Decoder: ExtensionDecoderFunc(func(n yaml.Node) error {
			var s struct {
				URL string `yaml:"url"`
			}
			if err := n.Decode(&s); err != nil {
				return err
			}
			if s.URL == "" {
				return errors.New("url is required")
			}
			return nil
		}),
	})

	p := &KapiProject{}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
server: {}
`), p))

	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server: url is required")
}

func TestValidate_UnknownKeyRoundTrips(t *testing.T) {
	ResetExtensionsForTest()
	defer ResetExtensionsForTest()

	p := &KapiProject{}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
future_thing:
  some: value
`), p))

	// Unknown key with no registered decoder is preserved, not rejected.
	require.NoError(t, p.Validate())
	require.NotNil(t, p.Extras["future_thing"])
}

func TestValidate_RequiresMissingGroup(t *testing.T) {
	ResetExtensionsForTest()
	defer ResetExtensionsForTest()

	p := &KapiProject{
		Version:  CurrentVersion,
		Requires: []string{"bowrain"},
	}
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `requires extension group "bowrain"`)
	assert.Contains(t, err.Error(), `not built with`)
}

func TestValidate_RequiresPresentGroup(t *testing.T) {
	ResetExtensionsForTest()
	defer ResetExtensionsForTest()

	RegisterExtension(Extension{
		Name:  "anything",
		Scope: ScopeProject,
		Group: "bowrain",
	})

	p := &KapiProject{
		Version:  CurrentVersion,
		Requires: []string{"bowrain"},
	}
	require.NoError(t, p.Validate())
}

func TestRegisterExtensionGroup_StampsGroup(t *testing.T) {
	ResetExtensionsForTest()
	defer ResetExtensionsForTest()

	RegisterExtensionGroup("myext", []Extension{
		{Name: "alpha", Scope: ScopeProject},
		{Name: "beta", Scope: ScopeItem},
	})

	assert.True(t, HasExtensionGroup("myext"))
	a, ok := extensionFor(ScopeProject, "alpha")
	require.True(t, ok)
	assert.Equal(t, "myext", a.Group)
	b, ok := extensionFor(ScopeItem, "beta")
	require.True(t, ok)
	assert.Equal(t, "myext", b.Group)
}

func TestRegisterExtension_DuplicatePanics(t *testing.T) {
	ResetExtensionsForTest()
	defer ResetExtensionsForTest()

	RegisterExtension(Extension{Name: "x", Scope: ScopeProject})
	assert.Panics(t, func() {
		RegisterExtension(Extension{Name: "x", Scope: ScopeProject})
	})
}

func TestRegisterExtension_DifferentScopesNoConflict(t *testing.T) {
	ResetExtensionsForTest()
	defer ResetExtensionsForTest()

	RegisterExtension(Extension{Name: "collection", Scope: ScopeProject})
	RegisterExtension(Extension{Name: "collection", Scope: ScopeItem})
	RegisterExtension(Extension{Name: "collection", Scope: ScopeDefaults})

	_, ok := extensionFor(ScopeProject, "collection")
	assert.True(t, ok)
	_, ok = extensionFor(ScopeItem, "collection")
	assert.True(t, ok)
	_, ok = extensionFor(ScopeDefaults, "collection")
	assert.True(t, ok)
}

func TestValidate_ItemScopeDecoder(t *testing.T) {
	ResetExtensionsForTest()
	defer ResetExtensionsForTest()

	RegisterExtension(Extension{
		Name:  "max_size",
		Scope: ScopeItem,
		Decoder: ExtensionDecoderFunc(func(n yaml.Node) error {
			var s string
			return n.Decode(&s)
		}),
	})

	p := &KapiProject{}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
content:
  - name: ui
    items:
      - path: src/foo.json
        max_size: "10MB"
`), p))

	require.NoError(t, p.Validate())

	// Now make it bad — sequence where string expected.
	bad := &KapiProject{}
	require.NoError(t, yaml.Unmarshal([]byte(`
version: v1
content:
  - name: ui
    items:
      - path: src/foo.json
        max_size: [a, b, c]
`), bad))

	err := bad.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content[0].items[0].max_size:")
}
