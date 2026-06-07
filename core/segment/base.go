package segment

import (
	"context"
	"sync"
)

// BaseBreaker computes base sentence-break rune offsets over already-flattened
// text. It exists so the SRX engine can reproduce Okapi's `useIcu4jBreakRules`
// behaviour: ICU/UAX-29 supplies the candidate sentence breaks and the SRX
// rules are layered on top as exceptions — a no-break rule suppresses a base
// break, a break rule adds one, and earlier SRX rules override the base breaks
// (first decision at a position wins).
//
// Only the ICU-backed uax29 engine registers a base breaker, and only in cgo
// builds. On nocgo/wasm builds none is registered: [HasBaseBreaker] reports
// false and the SRX engine falls back to pure-rule breaking from its own break
// rules. This keeps the pure-Go srx package free of any cgo/ICU dependency —
// the base breaker is resolved at runtime through this registry, never imported.
type BaseBreaker interface {
	// BaseBreaks returns the interior sentence-break offsets (rune indices into
	// text, excluding 0 and len(text)) for the given resolved BCP-47 locale.
	BaseBreaks(ctx context.Context, text []rune, locale string) ([]int, error)
}

var (
	bbMu        sync.RWMutex
	baseBreaker BaseBreaker
)

// RegisterBaseBreaker installs the process-wide base breaker. The last
// registration wins; the ICU uax29 package registers one from its init.
func RegisterBaseBreaker(b BaseBreaker) {
	bbMu.Lock()
	baseBreaker = b
	bbMu.Unlock()
}

// HasBaseBreaker reports whether a base breaker is available (i.e. ICU is
// linked). The SRX engine uses this to choose hybrid vs pure-rule mode and to
// select its default ruleset.
func HasBaseBreaker() bool {
	bbMu.RLock()
	defer bbMu.RUnlock()
	return baseBreaker != nil
}

// BaseBreaks runs the registered base breaker. The bool is false when none is
// registered (nocgo), in which case the caller falls back to pure-rule breaking.
func BaseBreaks(ctx context.Context, text []rune, locale string) ([]int, bool, error) {
	bbMu.RLock()
	b := baseBreaker
	bbMu.RUnlock()
	if b == nil {
		return nil, false, nil
	}
	breaks, err := b.BaseBreaks(ctx, text, locale)
	return breaks, true, err
}
