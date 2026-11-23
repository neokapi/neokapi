package format

import (
	"sort"
	"sync"
)

// SubfilterTrace is a SubfilterResolver that records every format it resolves
// a reader for, at any depth, so a cache can key a parse by every reader that
// produced part of it: the format the file was opened in, and each format its
// embedded content was handed to.
//
// A reader must obtain the reader for embedded content through the resolver
// it is given (SetSubfilterResolver) for that reader's plugin to be in the
// key. A plugin's reader is reachable only through the format registry, and
// no format package imports the registry (TestFormatPackagesReachOtherReadersOnlyThroughTheResolver),
// so a reader cannot reach one any other way.
type SubfilterTrace struct {
	inner SubfilterResolver

	mu      sync.Mutex
	formats map[string]bool
}

// TraceSubfilters wraps inner and hands the trace to reader as its resolver
// when reader resolves embedded content. The trace it returns records what
// the reader resolves; for a reader that resolves nothing it stays empty.
func TraceSubfilters(reader any, inner SubfilterResolver) *SubfilterTrace {
	t := &SubfilterTrace{inner: inner, formats: map[string]bool{}}
	if sa, ok := reader.(SubfilterAware); ok && inner != nil {
		sa.SetSubfilterResolver(t)
	}
	return t
}

// ResolveReader records formatName, resolves it through the wrapped resolver
// and hands the trace on to the reader it returns, so nested embedded content
// is recorded too.
func (t *SubfilterTrace) ResolveReader(formatName string) (DataFormatReader, error) {
	t.mu.Lock()
	t.formats[formatName] = true
	t.mu.Unlock()
	r, err := t.inner.ResolveReader(formatName)
	if err != nil {
		return nil, err
	}
	if sa, ok := r.(SubfilterAware); ok {
		sa.SetSubfilterResolver(t)
	}
	return r, nil
}

// ResolveWriter resolves through the wrapped resolver. A writer produces no
// part of a parse, so nothing is recorded.
func (t *SubfilterTrace) ResolveWriter(formatName string) (DataFormatWriter, error) {
	return t.inner.ResolveWriter(formatName)
}

// Formats lists the formats resolved so far, sorted.
func (t *SubfilterTrace) Formats() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.formats))
	for f := range t.formats {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}
