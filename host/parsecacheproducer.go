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
		fp := p.Fingerprint()
		if p.Version() == "" || fp == "" {
			a.noteUncachedPlugin(p.Name())
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

// noteUncachedPlugin says once per plugin that its files are parsed every time.
func (a *App) noteUncachedPlugin(name string) {
	if _, told := uncachedPluginsTold.LoadOrStore(name, true); told {
		return
	}
	fmt.Fprintf(os.Stderr, "kapi: the %s plugin declares no version, so the files it reads are parsed every time rather than served from the parse cache\n", name)
}
