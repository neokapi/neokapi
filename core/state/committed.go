package state

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The committed record is what a project keeps and a reviewer reads: one line
// per unit, sharded by the document the unit belongs to.
//
// JSON Lines rather than one JSON document, because the artifact is appended to
// and diffed far more often than it is read whole. A single indented array meant
// that recording one decision rewrote every byte of the file, so the diff for a
// one-word change was the whole project and two branches touching unrelated
// documents conflicted on sight. A line per unit makes a decision a one-line
// diff.
//
// Sharded by document scope for the same reason at a larger grain: editing the
// documentation should not rewrite the shard holding the interface strings.
//
// Lines are sorted within a shard so a file's bytes depend only on its contents.

// CommittedExt is the extension of a shard file.
const CommittedExt = ".jsonl"

// shardFileName maps a document scope to a file name that is safe on every
// platform and stable across runs. Scope keys are opaque, so the name is derived
// rather than trusted: a scope is not a path and must never be able to escape
// the state directory.
func shardFileName(scope string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, scope)
	if clean == "" {
		clean = "unscoped"
	}
	return clean + CommittedExt
}

// WriteCommitted serializes units into per-scope shards under dir, replacing
// shards it owns and removing ones that no longer have any units.
func WriteCommitted(dir string, units []UnitState) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("state: state dir: %w", err)
	}

	byShard := map[string][]UnitState{}
	for _, u := range units {
		name := shardFileName(shardOf(u))
		byShard[name] = append(byShard[name], u)
	}

	for name, group := range byShard {
		sortUnits(group)
		if err := writeShard(filepath.Join(dir, name), group); err != nil {
			return err
		}
	}
	return pruneShards(dir, byShard)
}

func writeShard(path string, units []UnitState) error {
	var buf strings.Builder
	for _, u := range units {
		line, err := json.Marshal(u)
		if err != nil {
			return fmt.Errorf("state: marshal unit %s: %w", u.Unit, err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	// Temp + rename, so an interrupted write cannot leave a half-written shard
	// that would fail to parse on the next read.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(buf.String()), 0o644); err != nil {
		return fmt.Errorf("state: write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("state: rename %s: %w", path, err)
	}
	return nil
}

// pruneShards removes shards whose last unit is gone, so a deleted document does
// not leave a stale file behind claiming units that no longer exist.
func pruneShards(dir string, keep map[string][]UnitState) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("state: read state dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), CommittedExt) {
			continue
		}
		if _, ok := keep[e.Name()]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("state: prune %s: %w", e.Name(), err)
		}
	}
	return nil
}

// ReadCommitted loads every shard under dir. A missing directory is an empty
// project, not an error; a malformed line is an error, because state is
// authoritative and must never be silently discarded.
func ReadCommitted(dir string) ([]UnitState, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("state: read state dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), CommittedExt) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var out []UnitState
	for _, name := range names {
		units, err := readShard(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		out = append(out, units...)
	}
	return out, nil
}

func readShard(path string) ([]UnitState, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("state: open %s: %w", path, err)
	}
	defer f.Close()

	var out []UnitState
	sc := bufio.NewScanner(f)
	// Units carry review notes, so a line can exceed the default 64KB budget.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var u UnitState
		if err := json.Unmarshal([]byte(text), &u); err != nil {
			return nil, fmt.Errorf("state: %s line %d: %w", filepath.Base(path), line, err)
		}
		out = append(out, u)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("state: read %s: %w", path, err)
	}
	return out, nil
}
