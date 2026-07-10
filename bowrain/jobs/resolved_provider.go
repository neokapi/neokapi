package jobs

import (
	"context"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"golang.org/x/time/rate"
)

// ProviderSource identifies where a resolved translation provider's key came
// from. It is the hybrid-AI (Epic 004) billing gate: only the platform-held key
// is metered in credits; a workspace-supplied BYO key burns no credits (usage is
// still recorded to ai_usage as an abuse cap by the quota store).
//
// This type and ResolvedProvider are the shared handshake between the provider
// resolve path (agent A) and the billing deduction (agent B): resolveProvider
// classifies the Source, and the billing hook deducts only when
// Source == ProviderSourcePlatform.
type ProviderSource string

const (
	// ProviderSourcePlatform is the shared platform-held key (metered in credits).
	ProviderSourcePlatform ProviderSource = "platform"
	// ProviderSourceBYO is a per-workspace bring-your-own key (never metered).
	ProviderSourceBYO ProviderSource = "byo"
)

// ResolvedProvider is the outcome of resolving a job's translation provider: the
// LLM to translate with, its rate limiter, and the source that decides billing.
type ResolvedProvider struct {
	LLM     aiprovider.LLMProvider
	Limiter *rate.Limiter
	Source  ProviderSource
}

// ProviderConfigResolver resolves a saved per-workspace provider config by id,
// scoped to the durable workspace id, returning the decrypted key plus provider
// settings. *store.ProviderConfigStore satisfies it; a fake is used in unit
// tests. Replaces the machine-global keychain CredStore for resolving worker
// translation providers. Scoping is by durable workspace_id only — never by the
// mutable, reusable workspace slug (a slug alone must never authorize secret
// retrieval).
type ProviderConfigResolver interface {
	Resolve(ctx context.Context, workspaceID, id string) (bstore.ProviderConfig, error)
}
