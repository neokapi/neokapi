package host

import (
	"fmt"
	"os"
	"sync"
)

// formatProducer names what produces the parse of a file read as fmtName, for
// the parse cache key: "" for a built-in format, and for a plugin's format the
// plugin's name, its manifest version and a fingerprint of its manifest and
// binary (pluginhost.Plugin.Fingerprint). The plugin is the one the format's
// route names, the route its reader is launched through, so the key and the
// run agree.
//
// cacheable is false for a plugin whose version cannot be determined: such a
// file is parsed every time, never served from the cache, and kapi says so
// once per plugin.
func (a *App) formatProducer(fmtName string) (identity string, cacheable bool) {
	if a.PluginHost == nil || fmtName == "" {
		return "", true
	}
	names := []string{fmtName}
	if name, _, err := a.resolveFormatRef(fmtName); err == nil && name != fmtName {
		names = append(names, name)
	}
	for _, name := range names {
		route := a.PluginHost.FormatRoute(name)
		if route == nil || route.Plugin == nil {
			continue
		}
		p := route.Plugin
		if p.Version() == "" {
			a.noteUncachedPlugin(p.Name(), "it declares no version")
			return "", false
		}
		fp, err := p.Fingerprint()
		if err != nil {
			a.noteUncachedPlugin(p.Name(), err.Error())
			return "", false
		}
		return p.Name() + "@" + p.Version() + "#" + fp, true
	}
	return "", true
}

// parseCacheOffTold and uncachedPluginsTold keep the parse cache notices to
// one line each per process, whichever App (a converge worker included) meets
// the condition.
var (
	parseCacheOffTold   sync.Once
	uncachedPluginsTold sync.Map
)

// noteParseCacheOff records why the parse cache could not be opened, so a run
// summary can report it, and says it once on stderr: the run goes on parsing
// every file, and a person whose runs grew slow can see why.
func (a *App) noteParseCacheOff(err error) {
	a.parseCacheOff = err.Error()
	parseCacheOffTold.Do(func() {
		fmt.Fprintf(os.Stderr, "kapi: the parse cache is off for this run, so every file is parsed again: %v\n", err)
	})
}

// ParseCacheOff reports why the parse cache was off for the last command that
// wanted it, or "" when it was open.
func (a *App) ParseCacheOff() string { return a.parseCacheOff }

// parseProducer is formatProducer as the open parse cache answers it: once per
// format for the life of the cache, which is one command, so a development
// plugin's directory is walked once per run rather than once per file read.
// With no cache open it asks formatProducer.
func (a *App) parseProducer(fmtName string) (string, bool) {
	if a.docCache != nil && a.docCache.producer != nil {
		return a.docCache.producer(fmtName)
	}
	return a.formatProducer(fmtName)
}

// memoProducer answers each format once and reuses the answer.
func memoProducer(produce func(string) (string, bool)) func(string) (string, bool) {
	type answer struct {
		id string
		ok bool
	}
	var memo sync.Map
	return func(f string) (string, bool) {
		if v, ok := memo.Load(f); ok {
			a := v.(answer)
			return a.id, a.ok
		}
		id, ok := produce(f)
		memo.Store(f, answer{id, ok})
		return id, ok
	}
}

// noteUncachedPlugin says once per plugin that its files are parsed every time.
func (a *App) noteUncachedPlugin(name, why string) {
	if _, told := uncachedPluginsTold.LoadOrStore(name, true); told {
		return
	}
	fmt.Fprintf(os.Stderr, "kapi: the files the %s plugin reads are parsed every time rather than served from the parse cache: %s\n", name, why)
}
