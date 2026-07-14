package platformconfig

import "encoding/json"

// NewInMemoryService builds a non-persistent Service whose cache is pre-seeded
// with the given resolved key/values (each marshaled to JSON with encoding/json).
// It has no database, so writes are rejected and Refresh is a no-op. It is
// intended for tests and for callers that want a fixed, in-process configuration
// rather than a live, ctrl-managed one. Keys are the Key* constants; unset keys
// fall back to the supplied Defaults exactly as with the DB-backed service.
func NewInMemoryService(defaults Defaults, seed map[string]any) *Service {
	svc := NewService(nil, defaults)
	cache := make(map[string]json.RawMessage, len(seed))
	for k, v := range seed {
		if b, err := json.Marshal(v); err == nil {
			cache[k] = b
		}
	}
	svc.mu.Lock()
	svc.cache = cache
	svc.mu.Unlock()
	return svc
}
