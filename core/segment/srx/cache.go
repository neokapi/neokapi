package srx

import "sync"

// rulesCache memoises compiled rule lists per locale. SRX rule compilation
// (one regexp2 program per rule) is comparatively expensive, so an engine reuse
// across many blocks in the same locale should compile each locale's rules at
// most once. Failed compilations are not cached.
type rulesCache struct {
	mu sync.Mutex
	m  map[string][]compiledRule
}

func (c *rulesCache) get(locale string, build func() ([]compiledRule, error)) ([]compiledRule, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = make(map[string][]compiledRule)
	}
	if rules, ok := c.m[locale]; ok {
		return rules, nil
	}
	rules, err := build()
	if err != nil {
		return nil, err
	}
	c.m[locale] = rules
	return rules, nil
}
