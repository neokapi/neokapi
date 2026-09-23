package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Capturing an agent's first saved version.
//
// Measure 1 counts failing findings twice: on the first version of each file an
// agent saved, and on the version it finished with. The difference between the
// two is what the agent did after writing, which is where a check it ran, or a
// context it read late, shows up.
//
// The watcher polls the cell's working tree while the session runs and keeps
// the first content it sees for each file that differs from the baseline. It
// polls rather than hooking the host's tools so that both hosts are captured
// the same way, whichever tool wrote the file. A file written twice within one
// polling interval is captured at its second version; the interval is short
// enough that an agent, which reads and reasons between writes, rarely does so.

// evalWatchInterval is how often the watcher looks at the tree.
const evalWatchInterval = 250 * time.Millisecond

// evalWatchMaxFile is the largest file the watcher keeps. The fixture's pages
// are a few kilobytes; anything larger is not prose an agent wrote for a task.
const evalWatchMaxFile = 1 << 20

// evalWatchSkipped are the directories that hold wiring and state rather than
// content, so a change in them is never a version of the agent's work.
var evalWatchSkipped = []string{".git", ".kapi", ".claude", ".agents", ".codex", ".cursor", ".vscode", "node_modules"}

// evalWatcher holds a tree's baseline and the first changed version of each
// file.
type evalWatcher struct {
	repo     string
	baseline map[string][]byte
	mu       sync.Mutex
	// first maps a path to its first changed content; a nil value is a file
	// the agent removed before writing anything else to it.
	first map[string][]byte
}

// newEvalWatcher reads the tree's baseline.
func newEvalWatcher(repo string) (*evalWatcher, error) {
	baseline, err := evalReadTree(repo)
	if err != nil {
		return nil, err
	}
	return &evalWatcher{repo: repo, baseline: baseline, first: map[string][]byte{}}, nil
}

// run polls until ctx ends, then takes one last look so a write made just
// before the session ended is not missed.
func (w *evalWatcher) run(ctx context.Context) {
	ticker := time.NewTicker(evalWatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.poll()
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

// poll records the first changed version of every file not yet captured. A
// tree that cannot be read at this moment, because an agent is halfway through
// moving a file, is read again at the next tick.
func (w *evalWatcher) poll() {
	current, err := evalReadTree(w.repo)
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for name, data := range current {
		if _, captured := w.first[name]; captured {
			continue
		}
		if before, ok := w.baseline[name]; ok && bytes.Equal(before, data) {
			continue
		}
		w.first[name] = data
	}
	for name := range w.baseline {
		if _, captured := w.first[name]; captured {
			continue
		}
		if _, ok := current[name]; !ok {
			w.first[name] = nil
		}
	}
}

// versions returns the baseline, the first version and the current version of
// every file the session changed, in a fixed order of names.
func (w *evalWatcher) versions() (names []string, baseline, first, final map[string][]byte, err error) {
	w.poll()
	current, err := evalReadTree(w.repo)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	first, final = map[string][]byte{}, map[string][]byte{}
	for name, data := range w.first {
		names = append(names, name)
		first[name] = data
		final[name] = current[name]
	}
	slices.Sort(names)
	return names, w.baseline, first, final, nil
}

// evalReadTree reads every content file under repo, keyed by slash path.
func evalReadTree(repo string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(repo, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			if path != repo && slices.Contains(evalWatchSkipped, entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() > evalWatchMaxFile {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		relative, err := filepath.Rel(repo, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = data
		return nil
	})
	return files, err
}
