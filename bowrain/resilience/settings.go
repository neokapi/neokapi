package resilience

import "time"

// Kind classifies a dependency. It selects the default thresholds and the
// user-facing wording, and it is a bounded Prometheus label — never derive it
// from user input.
type Kind string

const (
	// KindAI is an LLM provider (Bedrock, Anthropic, OpenAI, Azure, Gemini, Ollama).
	KindAI Kind = "ai"
	// KindMT is a machine-translation provider.
	KindMT Kind = "mt"
	// KindConnector is a content-source integration (WordPress, Figma, HubSpot, PostHog).
	KindConnector Kind = "connector"
	// KindForge is a source-control host REST API (GitHub, GitLab).
	KindForge Kind = "forge"
	// KindIdentity is the identity provider (Cognito, Keycloak) — OIDC discovery,
	// JWKS and token exchange.
	KindIdentity Kind = "identity"
	// KindMail is the outbound mail relay (Resend, SMTP).
	KindMail Kind = "mail"
	// KindBilling is the payment processor (Stripe).
	KindBilling Kind = "billing"
	// KindInternal is a first-party service reached over the network.
	KindInternal Kind = "internal"
)

// Settings are the resolved thresholds for one breaker.
type Settings struct {
	// Enabled is the kill switch. When false the breaker never rejects: Do
	// becomes a pass-through that still records metrics. Flipping it off in
	// ctrl is the escape hatch if a breaker ever misbehaves in production.
	Enabled bool
	// FailureRatio is the share of failed calls, within Window, that opens the
	// circuit — but only once MinimumRequests calls have been observed.
	FailureRatio float64
	// MinimumRequests is the volume floor. Below it the circuit never opens, so
	// a single unlucky call on an idle dependency cannot trip anything.
	MinimumRequests uint32
	// Window is the rolling period over which failures are counted.
	Window time.Duration
	// Cooldown is how long the circuit stays open before admitting probes.
	Cooldown time.Duration
	// HalfOpenProbes is how many concurrent calls are admitted while half-open.
	// They must all succeed to close the circuit; the first failure re-opens it.
	HalfOpenProbes uint32
}

// DefaultsFor returns the built-in thresholds for a dependency class. They
// differ only where the cost of being wrong differs:
//
//   - AI is the most failure-prone and the most deferrable, so it trips
//     readily and cools down for half a minute.
//   - Identity gets the shortest cooldown: sign-in has to recover promptly,
//     and it needs a higher failure ratio because bad credentials are common
//     traffic (and are classified as caller faults anyway).
//   - Billing is the most conservative — declining to attempt a payment is
//     worse than a slow one — so it needs a clear majority of failures.
//   - Mail is the least urgent: a long cooldown is fine because mail is queued
//     and retried anyway.
func DefaultsFor(kind Kind) Settings {
	base := Settings{
		Enabled:         true,
		FailureRatio:    0.5,
		MinimumRequests: 10,
		Window:          60 * time.Second,
		Cooldown:        30 * time.Second,
		HalfOpenProbes:  2,
	}
	switch kind {
	case KindIdentity:
		base.FailureRatio = 0.7
		base.MinimumRequests = 20
		base.Cooldown = 15 * time.Second
	case KindBilling:
		base.FailureRatio = 0.8
		base.MinimumRequests = 20
		base.Cooldown = 20 * time.Second
	case KindMail:
		base.MinimumRequests = 5
		base.Cooldown = 60 * time.Second
	case KindForge, KindConnector:
		base.FailureRatio = 0.6
	}
	return base
}

// Overrides are the operator-tunable thresholds from platform config. Every
// field is optional: a nil field keeps the per-kind default, so an instance
// that has never been tuned behaves exactly as the code intends.
type Overrides struct {
	Enabled         *bool
	FailureRatio    *float64
	MinimumRequests *int
	WindowSeconds   *int
	CooldownSeconds *int
	HalfOpenProbes  *int
}

// Apply layers overrides onto s, ignoring values outside their sane range
// rather than trusting them. A ratio of 0 would open the circuit on the first
// success and a cooldown of 0 would spin, so both are rejected here — the
// breaker must not be configurable into a state that harms a healthy
// dependency.
func (s Settings) Apply(o Overrides) Settings {
	if o.Enabled != nil {
		s.Enabled = *o.Enabled
	}
	if o.FailureRatio != nil && *o.FailureRatio > 0 && *o.FailureRatio <= 1 {
		s.FailureRatio = *o.FailureRatio
	}
	if o.MinimumRequests != nil && *o.MinimumRequests > 0 {
		s.MinimumRequests = uint32(*o.MinimumRequests)
	}
	if o.WindowSeconds != nil && *o.WindowSeconds > 0 {
		s.Window = time.Duration(*o.WindowSeconds) * time.Second
	}
	if o.CooldownSeconds != nil && *o.CooldownSeconds > 0 {
		s.Cooldown = time.Duration(*o.CooldownSeconds) * time.Second
	}
	if o.HalfOpenProbes != nil && *o.HalfOpenProbes > 0 {
		s.HalfOpenProbes = uint32(*o.HalfOpenProbes)
	}
	return s
}
