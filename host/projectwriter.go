package host

import (
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// The project store's writers, as the interfaces the rest of host takes.
//
// Every write to a project's terms, content memory and voice profiles goes
// through the projector (App.Projector), which records it in the workspace's
// log before applying it. These helpers hand its stores to code written
// against the subsystem interfaces, and answer a nil interface for a build
// whose store lacks the subsystem: a nil pointer inside an interface is not a
// nil interface, and every reader of one would call through it.

// termsWriter is the project's terms store, written through the projector.
func termsWriter(p *projector.Projector) terms.Store {
	if t := p.Terms(); t != nil {
		return t
	}
	return nil
}

// memoryWriter is the project's content memory, written through the projector.
func memoryWriter(p *projector.Projector) memory.Store {
	if m := p.Memory(); m != nil {
		return m
	}
	return nil
}

// memoryWriterOf answers a facade as the interface, nil for a nil facade.
func memoryWriterOf(m *projector.Memory) memory.Store {
	if m != nil {
		return m
	}
	return nil
}

// voiceWriter is the project's voice-profile store, written through the
// projector.
func voiceWriter(p *projector.Projector) coreprofile.Store {
	if v := p.Voice(); v != nil {
		return v
	}
	return nil
}
