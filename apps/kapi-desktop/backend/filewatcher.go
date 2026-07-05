package backend

import (
	"os"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/project"
)

// fileWatcher polls a project directory for file changes and emits
// Wails events when the file tree changes. Uses the same polling
// approach as the plugin cache watcher.
type fileWatcher struct {
	app    *App
	tabID  string
	dir    string
	stop   chan struct{}
	stopFn func()
	ticker *time.Ticker
	// snapshot of relative paths → modtime for change detection
	snapshot map[string]time.Time
}

func newFileWatcher(app *App, tabID, dir string) *fileWatcher {
	fw := &fileWatcher{
		app:      app,
		tabID:    tabID,
		dir:      dir,
		stop:     make(chan struct{}),
		snapshot: make(map[string]time.Time),
	}
	fw.stopFn = sync.OnceFunc(func() {
		close(fw.stop)
		if fw.ticker != nil {
			fw.ticker.Stop()
		}
	})
	return fw
}

func (fw *fileWatcher) Start() {
	// Take initial snapshot.
	fw.snapshot = fw.scan()
	fw.ticker = time.NewTicker(2 * time.Second)
	go fw.loop()
}

func (fw *fileWatcher) Stop() {
	fw.stopFn()
}

func (fw *fileWatcher) loop() {
	for {
		select {
		case <-fw.stop:
			return
		case <-fw.ticker.C:
			current := fw.scan()
			if fw.changed(current) {
				fw.snapshot = current
				fw.emit()
			}
		}
	}
}

func (fw *fileWatcher) scan() map[string]time.Time {
	snap := make(map[string]time.Time)
	_ = project.WalkProjectDir(fw.dir, func(rel string, info os.FileInfo) error {
		snap[rel] = info.ModTime()
		return nil
	})
	return snap
}

func (fw *fileWatcher) changed(current map[string]time.Time) bool {
	if len(current) != len(fw.snapshot) {
		return true
	}
	for k, v := range current {
		if prev, ok := fw.snapshot[k]; !ok || !prev.Equal(v) {
			return true
		}
	}
	return false
}

func (fw *fileWatcher) emit() {
	fw.app.emitEvent("project-files-changed", fw.tabID)
}
