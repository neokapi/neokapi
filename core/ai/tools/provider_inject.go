package tools

import (
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// ProviderKey is where a host puts an already-built LLM provider in an AI
// tool's config map, the way core/memory.ConfigKey carries a content memory.
//
// A host that owns its credentials builds the provider once and hands the same
// handle to every AI tool a run constructs. The platform server does exactly
// that: the workspace's provider configuration lives in Postgres and the
// hosted key is held by the process, so a stored flow definition must never
// need to carry a provider id or a secret for its steps to reach a model.
//
// A provider is a live handle, so it cannot survive the JSON round trip the
// rest of a tool's config takes. Every AI factory lifts it out with
// TakeProvider before schema.ApplyConfig runs.
const ProviderKey = "llm_provider"

// TakeProvider removes the host's injected provider from config and returns
// it, or nil when the host injected none.
//
// It must run before schema.ApplyConfig: the map is JSON-encoded there, and a
// provider holding an HTTP client encodes to an error rather than to a value.
func TakeProvider(config map[string]any) aiprovider.LLMProvider {
	p, ok := config[ProviderKey].(aiprovider.LLMProvider)
	if !ok {
		dropProvider(config)
		return nil
	}
	delete(config, ProviderKey)
	return p
}

// dropProvider discards an injected provider, for the backends that call no
// LLM: an MT engine, the deterministic check, the local NER model. They read
// the same config map, and a live handle left in it would fail the JSON round
// trip their own config takes.
func dropProvider(config map[string]any) {
	delete(config, ProviderKey)
}

// providerFor returns the provider an AI tool calls.
//
// A config that names its own provider keeps it: the grant fills a gap rather
// than overriding a choice, so a step deliberately pointed at a local model
// still reaches it on a host that holds a cloud credential. With no provider
// named, the host's grant is used, and with neither the tool falls back to the
// registry default the way it always has.
func providerFor(injected aiprovider.LLMProvider, name string, cfg aiprovider.Config) (aiprovider.LLMProvider, error) {
	if injected != nil && name == "" {
		return injected, nil
	}
	return ProviderFromConfig(name, cfg)
}
