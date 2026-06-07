//go:build cgo

package uax29

import (
	"context"

	"github.com/neokapi/neokapi/core/segment"
)

// Register the ICU sentence breaker as the process-wide base breaker, so the
// pure-Go SRX engine can run Okapi's `useIcu4jBreakRules` hybrid (ICU base +
// SRX exceptions) without importing this cgo package. nocgo builds compile the
// stub instead and register nothing, so the SRX engine falls back to pure-rule
// breaking.
func init() {
	segment.RegisterBaseBreaker(icuBaseBreaker{})
}

type icuBaseBreaker struct{}

// BaseBreaks runs ICU's sentence BreakIterator over the flattened text and
// returns the interior break offsets (rune indices). The SRX hybrid applies its
// own whitespace-backup adjustment and rule exceptions on top.
func (icuBaseBreaker) BaseBreaks(ctx context.Context, text []rune, locale string) ([]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(text) == 0 {
		return nil, nil
	}
	return (&icuEngine{}).boundaries(text, bcp47ToICU(locale))
}
