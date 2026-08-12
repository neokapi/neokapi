// Package refcache holds a project's freshness refs on disk: for each stream,
// the position this project has consumed and the governance identities its last
// contact with the server established.
//
// The file is DESTINATION-KEYED and DISPOSABLE. Everything in it is true of one
// server and one project and of no other, so a cache belonging to a different
// destination is discarded rather than reconciled; and everything in it is
// re-derivable, so deleting it costs exactly one negotiation round trip and can
// never cost a wrong answer. That is why it is not migrated, versioned or
// repaired: a file this side cannot read is a file this side re-fetches.
//
// It sits in the framework, beside the ref it holds, because the transport is
// not its only reader: the retrieval surfaces report that a project's
// governance has moved, and they run in a binary that links no platform code.
// A reader that cannot name a destination uses LoadObserved and ObservedRef;
// everything that WRITES goes through Load, which is destination-keyed.
package refcache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref"
)

// Filename is the file written under <state-dir>/work/cache/.
const Filename = "refs.json"

// Cache is one project's refs, keyed to the destination they describe.
type Cache struct {
	// ServerURL and ProjectID name the destination every ref below belongs to.
	ServerURL string `json:"server_url,omitempty"`
	ProjectID string `json:"project_id,omitempty"`

	// ObservedAt is when the destination was last reached. Reporting only: no
	// decision is taken from a clock, because a clock cannot say whether the
	// thing it timed has moved since.
	ObservedAt time.Time `json:"observed_at,omitzero"`

	// Streams holds one ref per stream name.
	Streams map[string]ref.Ref `json:"streams,omitempty"`
}

// PathFor returns the cache's on-disk path for a project layout. Bowrain owns
// this path; the framework layout has no notion of a sync destination.
func PathFor(layout coreproj.Layout) string {
	return filepath.Join(layout.CacheDir(), Filename)
}

// Load reads the cache for a destination, returning an empty one when the file
// is missing, unreadable, or belongs to a different server or project.
//
// Discarding on a destination mismatch is the whole discipline: a position is a
// place in one server's change feed and a governance identity is one
// workspace's state, so carrying either across a re-point would answer a
// question about the new destination with a fact about the old one.
func Load(layout coreproj.Layout, serverURL, projectID string) *Cache {
	empty := &Cache{ServerURL: serverURL, ProjectID: projectID, Streams: map[string]ref.Ref{}}

	data, err := os.ReadFile(PathFor(layout))
	if err != nil {
		return empty
	}
	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		return empty
	}
	if cache.ServerURL != serverURL || cache.ProjectID != projectID {
		return empty
	}
	if cache.Streams == nil {
		cache.Streams = map[string]ref.Ref{}
	}
	return &cache
}

// Ref returns the ref recorded for a stream, or the zero ref when the stream
// has never been synchronized. The zero ref makes no claim, so a caller
// comparing against it learns "unknown" rather than a difference it must
// resolve.
func (c *Cache) Ref(stream string) ref.Ref {
	if c == nil {
		return ref.Ref{}
	}
	return c.Streams[normalizeStream(stream)]
}

// LoadObserved reads the cache for REPORTING, whatever destination wrote it.
//
// It is the reader's half of Load, for a caller that wants to say what the
// project last observed but does not know which server or project the recipe
// binds — the retrieval surfaces, which read the graph and must be able to say
// it moved, without taking on the transport's configuration to do it.
//
// Nothing decides from what this returns: a report that names the wrong
// destination is a report to correct, while a WRITE against the wrong
// destination is a position in one server's change feed applied to another.
// Load keeps the destination check for exactly that reason, and this does not
// inherit it.
func LoadObserved(layout coreproj.Layout) *Cache {
	empty := &Cache{Streams: map[string]ref.Ref{}}

	data, err := os.ReadFile(PathFor(layout))
	if err != nil {
		return empty
	}
	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		return empty
	}
	if cache.Streams == nil {
		cache.Streams = map[string]ref.Ref{}
	}
	return &cache
}

// ObservedRef returns the ref recorded for a stream, and — for a caller that
// cannot name one — the ref of the only stream recorded when the cache holds
// exactly one.
//
// The fallback is what makes the cache readable by a surface that has not
// loaded the recipe: a project on a named stream would otherwise be reported
// against the default stream's absent ref, which reads as "never observed"
// forever. Two or more streams recorded leaves it unanswered rather than
// guessed, because there is no way to tell which one the caller sits on.
func (c *Cache) ObservedRef(stream string) ref.Ref {
	if c == nil {
		return ref.Ref{}
	}
	if r, ok := c.Streams[normalizeStream(stream)]; ok {
		return r
	}
	if stream == "" && len(c.Streams) == 1 {
		for _, r := range c.Streams {
			return r
		}
	}
	return ref.Ref{}
}

// Observe records the governance identities a contact with the server
// established for a stream — published by the server, or put in force by a
// write this project confirmed.
//
// The observed position is deliberately not taken. A position is what THIS
// project has consumed, and the server's is a different number entirely; taking
// it would mark content as read that no reader has seen. Consume moves the
// position and nothing else does.
//
// An empty identity is skipped rather than stored. A server that cannot answer
// for a component says nothing about it, and recording that silence as "no
// governance" would turn a degraded answer into a divergence report.
func (c *Cache) Observe(stream string, observed ref.Ref) {
	c.update(stream, func(current ref.Ref) ref.Ref {
		for _, component := range ref.Components() {
			if value := observed.Identity(component); value != "" {
				current = current.WithIdentity(component, value)
			}
		}
		return current
	})
}

// Record replaces every governance identity for a stream with the one the
// server answered, empty values included.
//
// The difference from Observe is what the answer means. Observe merges a
// possibly-silent answer — a response that omits a component said nothing about
// it. Record takes a DEFINITE one: the server was asked for its ref and gave it,
// so an empty component there is the fact that the component holds nothing, and
// keeping an older value beside it would be keeping a claim the server has just
// contradicted.
func (c *Cache) Record(stream string, answered ref.Ref) {
	c.update(stream, func(current ref.Ref) ref.Ref {
		for _, component := range ref.Components() {
			current = current.WithIdentity(component, answered.Identity(component))
		}
		return current
	})
}

// SetIdentity records one governance identity for a stream, including an empty
// one — which is how a component that genuinely holds nothing (a workspace with
// no terminology, a project with no committed decisions) is recorded as such.
func (c *Cache) SetIdentity(stream string, component ref.Component, value string) {
	c.update(stream, func(current ref.Ref) ref.Ref {
		return current.WithIdentity(component, value)
	})
}

// Consume advances a stream's position to a cursor the project has read
// through. It is monotonic (ref.Ref.Advance): an answer carrying no position
// leaves the recorded one standing rather than rewinding the stream to its
// beginning.
func (c *Cache) Consume(stream string, cursor int64) {
	c.update(stream, func(current ref.Ref) ref.Ref {
		return current.Advance(cursor)
	})
}

// Reset drops a stream's position, so the next transfer starts from the
// beginning of the change feed. It is what a forced re-pull asks for, and the
// one sanctioned way past the monotonic guard.
func (c *Cache) Reset(stream string) {
	c.update(stream, func(current ref.Ref) ref.Ref {
		current.Content = 0
		return current
	})
}

// Touch stamps the moment the destination was reached.
func (c *Cache) Touch(at time.Time) {
	if c != nil {
		c.ObservedAt = at.UTC()
	}
}

// Save writes the cache under the layout's cache directory, creating it if
// missing.
func (c *Cache) Save(layout coreproj.Layout) error {
	if c == nil {
		return nil
	}
	if err := os.MkdirAll(layout.CacheDir(), 0o755); err != nil {
		return fmt.Errorf("refcache: create cache dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("refcache: marshal refs: %w", err)
	}
	return os.WriteFile(PathFor(layout), data, 0o644)
}

func (c *Cache) update(stream string, apply func(ref.Ref) ref.Ref) {
	if c == nil {
		return
	}
	if c.Streams == nil {
		c.Streams = map[string]ref.Ref{}
	}
	name := normalizeStream(stream)
	c.Streams[name] = apply(c.Streams[name])
}

// normalizeStream resolves the unnamed stream to the protocol's default, so a
// caller that never chose one and a caller that chose "main" share a ref rather
// than accumulating two that describe the same place.
func normalizeStream(stream string) string {
	if stream == "" {
		return ref.DefaultStream
	}
	return stream
}
