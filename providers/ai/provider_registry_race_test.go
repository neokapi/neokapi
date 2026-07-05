package aiprovider

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProviderRegistry_ConcurrentRegisterAndRead is a -race regression test:
// RegisterProvider explicitly supports post-init() plugin registration, so it
// must be safe to call concurrently with the registry read paths. Before the
// registry was guarded by globalProvidersMu this failed under `go test -race`.
func TestProviderRegistry_ConcurrentRegisterAndRead(t *testing.T) {
	t.Parallel()

	const writers = 8
	const readsPerReader = 200

	var wg sync.WaitGroup

	// Writers: register unique plugin-style providers (no ModelPrefixes, so
	// model inference for other tests is unaffected).
	for w := range writers {
		wg.Go(func() {
			id := ProviderID(fmt.Sprintf("race-test-%d", w))
			RegisterProviderWithAliases(
				ProviderInfo{Name: id, Label: "Race Test", Local: true},
				func(_ Config) LLMProvider { return &MockProvider{ProviderName: string(id)} },
				ProviderID(fmt.Sprintf("race-test-alias-%d", w)),
			)
		})
	}

	// Readers: exercise every registry read path while writers append.
	for range writers {
		wg.Go(func() {
			for range readsPerReader {
				for _, info := range Providers() {
					_ = info.Name
				}
				_ = ProviderNames()
				_, _ = ProviderInfoFor(Anthropic)
				_ = IsLocalProvider(Ollama)
				_, _ = ProviderForModel("claude-sonnet-4")

				// assert (not require) inside goroutines: FailNow must only be
				// called from the test goroutine.
				p, err := NewProvider(Demo, Config{})
				if assert.NoError(t, err) {
					assert.NoError(t, p.Close())
				}

				// Unknown provider exercises the error path (which lists names).
				_, err = NewProvider("race-test-unknown", Config{})
				assert.Error(t, err)
			}
		})
	}

	wg.Wait()

	// All concurrent registrations landed and are resolvable by name and alias.
	names := ProviderNames()
	for w := range writers {
		id := ProviderID(fmt.Sprintf("race-test-%d", w))
		assert.Contains(t, names, string(id))

		p, err := NewProvider(id, Config{})
		require.NoError(t, err)
		assert.Equal(t, id, p.Name())

		alias := ProviderID(fmt.Sprintf("race-test-alias-%d", w))
		_, err = NewProvider(alias, Config{})
		require.NoError(t, err)

		assert.True(t, IsLocalProvider(id))
	}
}
