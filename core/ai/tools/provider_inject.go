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
// TakeInjected before schema.ApplyConfig runs.
const ProviderKey = "llm_provider"

// ObserverKey is where a host puts a decorator that every provider an AI tool
// calls passes through, whoever built it.
//
// The grant under ProviderKey covers the provider the host supplies. A config
// that names its own provider or carries its own key builds one inside the
// factory instead, where a host that must see every model call has nothing to
// hold. The observer is that hold: it wraps whatever provider the tool ends up
// with, so a host counting calls counts the ones it did not supply the key for.
const ObserverKey = "llm_observer"

// ProviderObserver decorates the provider an AI tool calls. It returns the
// handle the tool uses, which is normally a wrapper around the one it was
// given.
type ProviderObserver func(aiprovider.LLMProvider) aiprovider.LLMProvider

// Injected is what a host put in an AI tool's config about the provider the
// tool will call: a built provider to use, a decorator to wrap whatever
// provider the tool ends up with, or both. Either may be absent.
type Injected struct {
	Provider aiprovider.LLMProvider
	Observe  ProviderObserver
}

// TakeInjected removes the host's provider and observer from config and returns
// them.
//
// It must run before schema.ApplyConfig: the map is JSON-encoded there, and a
// provider holding an HTTP client, or a function value, encodes to an error
// rather than to a value.
func TakeInjected(config map[string]any) Injected {
	var in Injected
	if p, ok := config[ProviderKey].(aiprovider.LLMProvider); ok {
		in.Provider = p
	}
	if o, ok := config[ObserverKey].(ProviderObserver); ok {
		in.Observe = o
	}
	dropInjected(config)
	return in
}

// dropInjected discards what a host injected, for the backends that call no
// LLM: an MT engine, the deterministic check, the local NER model. They read
// the same config map, and a live handle left in it would fail the JSON round
// trip their own config takes.
func dropInjected(config map[string]any) {
	delete(config, ProviderKey)
	delete(config, ObserverKey)
}

// providerFor returns the provider an AI tool calls.
//
// A config that names its own provider keeps it: the grant fills a gap rather
// than overriding a choice, so a step deliberately pointed at a local model
// still reaches it on a host that holds a cloud credential. With no provider
// named, the host's grant is used, and with neither the tool falls back to the
// registry default the way it always has. Whichever it is, the host's observer
// wraps it, because a host that meters a run must see every call in it.
func providerFor(in Injected, name string, cfg aiprovider.Config) (aiprovider.LLMProvider, error) {
	p, err := resolveProvider(in.Provider, name, cfg)
	if err != nil {
		return nil, err
	}
	if in.Observe == nil || p == nil {
		return p, nil
	}
	return in.Observe(p), nil
}

// resolveProvider picks between the host's grant and the config's own choice.
func resolveProvider(injected aiprovider.LLMProvider, name string, cfg aiprovider.Config) (aiprovider.LLMProvider, error) {
	if injected != nil && name == "" {
		return injected, nil
	}
	return ProviderFromConfig(name, cfg)
}
