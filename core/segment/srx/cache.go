package srx

import (
	"sync"

	"golang.org/x/sync/singleflight"
)

// rulesCache memoises compiled rule lists per locale. SRX rule compilation
// (one regexp2 program per rule) is comparatively expensive, so an engine reuse
// across many blocks in the same locale should compile each locale's rules at
// most once.
//
// Compilation runs outside any lock: a sync.Map holds the finished results and
// a singleflight.Group keyed by locale collapses concurrent first-time requests
// for the same locale into a single build() call. Distinct locales therefore
// compile in parallel, and a slow compile never blocks lookups for locales that
// are already cached. Failed compilations are not cached.
type rulesCache struct {
	m     sync.Map // locale string -> []compiledRule
	group singleflight.Group
}

func (c *rulesCache) get(locale string, build func() ([]compiledRule, error)) ([]compiledRule, error) {
	if v, ok := c.m.Load(locale); ok {
		return v.([]compiledRule), nil
	}
	v, err, _ := c.group.Do(locale, func() (any, error) {
		// Re-check: another goroutine may have finished while we waited to enter.
		if v, ok := c.m.Load(locale); ok {
			return v, nil
		}
		rules, err := build()
		if err != nil {
			return nil, err
		}
		c.m.Store(locale, rules)
		return rules, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]compiledRule), nil
}
