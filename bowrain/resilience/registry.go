package resilience

import (
	"cmp"
	"slices"
	"sync"
	"time"
)

// Registry owns the breaker per dependency. Two call sites naming the same
// dependency get the same breaker — that is the whole point: a provider outage
// observed by the worker must also short-circuit the server's synchronous path.
//
// A Registry is safe for concurrent use and cheap to read; breakers are created
// on first use and never removed.
type Registry struct {
	mu        sync.RWMutex
	breakers  map[string]*Breaker
	overrides Overrides
}

// NewRegistry returns an empty registry with built-in thresholds. Tests should
// use this; production code uses [Default] so state is shared process-wide.
func NewRegistry() *Registry {
	return &Registry{breakers: map[string]*Breaker{}}
}

var (
	defaultOnce     sync.Once
	defaultRegistry *Registry
)

// Default is the process-wide registry. The server, the worker and every
// package they call share it, so the ctrl Health page can enumerate one
// authoritative set of states.
func Default() *Registry {
	defaultOnce.Do(func() { defaultRegistry = NewRegistry() })
	return defaultRegistry
}

// Get returns the breaker for a dependency, creating it on first use.
//
// name is a stable, low-cardinality identifier — "ai:bedrock",
// "connector:wordpress", "identity:oidc". It becomes a Prometheus label and a
// row on the Health page, so it must never carry a workspace id, a URL, or
// anything else unbounded.
func (r *Registry) Get(name string, kind Kind) *Breaker {
	r.mu.RLock()
	b, ok := r.breakers[name]
	r.mu.RUnlock()
	if ok {
		return b
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// Re-check: another goroutine may have created it between the two locks.
	if b, ok := r.breakers[name]; ok {
		return b
	}
	b = newBreaker(name, kind, DefaultsFor(kind).Apply(r.overrides))
	r.breakers[name] = b
	return b
}

// Configure applies operator overrides to every existing breaker and to any
// created later. Called at startup and again whenever platform config changes,
// so a threshold edited in ctrl takes effect without a redeploy.
//
// Reconfiguring resets each breaker to closed. That is deliberate: the safe
// response to "an operator just changed the rules" is to stop rejecting and
// re-observe, never to keep enforcing a decision made under the old rules.
func (r *Registry) Configure(o Overrides) {
	r.mu.Lock()
	r.overrides = o
	existing := make([]*Breaker, 0, len(r.breakers))
	for _, b := range r.breakers {
		existing = append(existing, b)
	}
	r.mu.Unlock()

	for _, b := range existing {
		b.reconfigure(DefaultsFor(b.kind).Apply(o))
	}
}

// Status is one breaker's externally visible state, as rendered on the ctrl
// Health page and returned by the admin health endpoint.
type Status struct {
	// Dependency is the breaker name, e.g. "ai:bedrock".
	Dependency string `json:"dependency"`
	// Kind is the dependency class.
	Kind string `json:"kind"`
	// State is "closed", "half_open" or "open".
	State string `json:"state"`
	// Healthy is true unless the circuit is open — the field the UI colours on.
	Healthy bool `json:"healthy"`
	// Enabled reports whether the breaker may reject at all; false means the
	// operator has switched breakers off.
	Enabled bool `json:"enabled"`
	// RetryAfterSeconds is populated only while open: the estimated seconds
	// until the next recovery probe.
	RetryAfterSeconds int `json:"retry_after_seconds,omitempty"`
}

// States returns every known breaker's status, sorted by name so the Health
// page does not reshuffle between polls. Dependencies that have never been
// called do not appear — there is nothing truthful to say about them yet.
func (r *Registry) States() []Status {
	r.mu.RLock()
	breakers := make([]*Breaker, 0, len(r.breakers))
	for _, b := range r.breakers {
		breakers = append(breakers, b)
	}
	r.mu.RUnlock()

	out := make([]Status, 0, len(breakers))
	for _, b := range breakers {
		st := b.State()
		s := Status{
			Dependency: b.Name(),
			Kind:       string(b.Kind()),
			State:      string(st),
			Healthy:    st != StateOpen,
			Enabled:    b.Settings().Enabled,
		}
		if st == StateOpen {
			s.RetryAfterSeconds = int((b.retryAfter() + time.Second - 1) / time.Second)
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Status) int { return cmp.Compare(a.Dependency, b.Dependency) })
	return out
}

// Open reports whether any breaker is currently open, and which. Used by the
// readiness check to describe the platform as degraded rather than ready.
func (r *Registry) Open() []string {
	var open []string
	for _, s := range r.States() {
		if s.State == string(StateOpen) {
			open = append(open, s.Dependency)
		}
	}
	slices.Sort(open)
	return open
}

// Get is [Registry.Get] on the process-wide registry.
func Get(name string, kind Kind) *Breaker { return Default().Get(name, kind) }
