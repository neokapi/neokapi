package host

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/yamledit"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms/ktb"
)

// Writing a project's context back out as files.
//
// A snapshot is the store rendered in the layout `kapi context import` reads: the terms bundle,
// one content-memory bundle, the voice profiles at the paths governance
// resolves them from, and the decision record. A clean clone holding a snapshot
// and no store reads it back through the same importers the seeding pass uses,
// and governs its content by the same fingerprint as the project that wrote it.
//
// Three properties hold, and the tests in contextsnapshot_test.go hold them:
//
//   - DETERMINISTIC. Every file is written through the serializer its format
//     already has — ktb.Marshal, kmb.Marshal, core/yamledit, core/state — each
//     of which sorts what it writes and stamps no clock.
//   - BYTE-STABLE. A file whose bytes have not moved is not written, so a
//     snapshot with nothing to say leaves no mtime behind and puts nothing in
//     a diff.
//   - ROUND-TRIPPING. Importing a snapshot into an empty store and snapshotting
//     again produces the same bytes.
//
// The redaction vault is not in it. The vault holds the originals of withheld
// content and lives under `.kapi/work/`, which a snapshot neither reads nor
// writes: a snapshot is a thing people put in git and hand to each other, and
// the vault is the one part of a project that must never travel.

// snapshotVoiceHeader is written above a voice profile a snapshot creates, so
// a reader who opens the file knows what wrote it. A profile already on disk
// keeps its own header, along with the rest of its comments.
const snapshotVoiceHeader = "# Written by `kapi context snapshot` from this project's store.\n" +
	"# Edit the project's context and snapshot again rather than editing this file.\n"

// ContextSnapshotRequest names where to write.
type ContextSnapshotRequest struct {
	// Out is the directory to write the layout into. It is required: `.kapi/`
	// is a checkout's cache, so a snapshot goes where its caller says.
	Out string
}

// ContextSnapshot reports what a snapshot wrote.
type ContextSnapshot struct {
	// Dir is the layout that was written.
	Dir string `json:"dir"`
	// Concepts, Entries, VoiceProfiles and Decisions count what the store
	// held.
	Concepts      int `json:"concepts"`
	Entries       int `json:"entries"`
	VoiceProfiles int `json:"voiceProfiles"`
	Decisions     int `json:"decisions"`
	// Written lists the files whose bytes moved, relative to Dir, in path
	// order. Empty when the snapshot already matched what was there.
	Written []string `json:"written,omitempty"`
}

// FormatText renders the snapshot for a reader.
func (r ContextSnapshot) FormatText(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "Wrote the project's context to %s: %s, %s, %s, %s.\n",
		r.Dir,
		pluralUnit(r.Concepts, "concept", "concepts"),
		pluralUnit(r.Entries, "content-memory entry", "content-memory entries"),
		pluralUnit(r.VoiceProfiles, "voice profile", "voice profiles"),
		pluralUnit(r.Decisions, "recorded decision", "recorded decisions"),
	); err != nil {
		return err
	}
	if len(r.Written) == 0 {
		_, err := fmt.Fprintln(w, "Every file already held these bytes.")
		return err
	}
	for _, f := range r.Written {
		if _, err := fmt.Fprintf(w, "  %s\n", f); err != nil {
			return err
		}
	}
	return nil
}

// SnapshotProjectContext writes the context in force for the project into a
// `.kapi/` layout.
//
// Everything in it comes out of the store, the decision record included: the
// snapshot is what the project knows, written as files, rather than a merge of
// the store with whatever the checkout was already carrying.
//
// A directory that already holds hand-authored content-memory bundles keeps
// them: a snapshot writes the store's one bundle beside them rather than
// deleting files it did not write. The decision record is whole-directory,
// because core/state owns that directory and prunes the shards a deleted
// document left behind.
func (a *App) SnapshotProjectContext(ctx context.Context, projectPath string, req ContextSnapshotRequest) (ContextSnapshot, error) {
	var res ContextSnapshot

	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return res, err
	}
	if req.Out == "" {
		return res, errors.New("context snapshot: name the directory to write (--out)")
	}
	out, err := filepath.Abs(req.Out)
	if err != nil {
		return res, fmt.Errorf("resolve %s: %w", req.Out, err)
	}
	res.Dir = reportedPath(layout.Root, out)

	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return res, err
	}

	writer := &snapshotWriter{dir: out, stands: relSlash(layout.Root, out)}
	if err := a.snapshotTerms(ctx, db, writer, &res); err != nil {
		return res, err
	}
	if err := a.snapshotMemory(ctx, db, writer, &res); err != nil {
		return res, err
	}
	if err := a.snapshotVoice(ctx, db, writer, &res); err != nil {
		return res, err
	}
	if err := a.snapshotRecord(ctx, db, writer, &res); err != nil {
		return res, err
	}
	sort.Strings(writer.written)
	res.Written = writer.written
	return res, nil
}

// snapshotWriter writes files under one directory and remembers which ones
// moved.
type snapshotWriter struct {
	dir string
	// stands is the project-relative directory this snapshot stands in for,
	// `.kapi` for the project's own layout and whatever `--out` named inside
	// the project otherwise. A binding under it is written at the path it has
	// there, so a project keeping its context files in one directory snapshots
	// back over them.
	stands  string
	written []string
}

// write puts data at rel under the snapshot directory, leaving the file alone
// when it already holds those bytes.
func (s *snapshotWriter) write(rel string, data []byte) error {
	path := filepath.Join(s.dir, filepath.FromSlash(rel))
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(rel), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", rel, err)
	}
	s.written = append(s.written, rel)
	return nil
}

// existing returns the bytes already at rel, or nil when nothing is there.
func (s *snapshotWriter) existing(rel string) []byte {
	data, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	return data
}

// snapshotTerms writes the terms store as the layout's terms bundle, through
// the exporter `kapi terms export --format bundle` uses.
func (a *App) snapshotTerms(ctx context.Context, db *projectdb.DB, w *snapshotWriter, res *ContextSnapshot) error {
	tb := db.Terms()
	if tb == nil {
		return nil
	}
	var buf bytes.Buffer
	if err := ExportKTB(ctx, tb, &buf); err != nil {
		return err
	}
	n, err := tb.Count(ctx)
	if err != nil {
		return fmt.Errorf("count concepts: %w", err)
	}
	res.Concepts = n
	return w.write(ktb.ConventionalName, buf.Bytes())
}

// snapshotMemory writes the content memory as the layout's one bundle, through
// the exporter `kapi memory export --format bundle` uses.
func (a *App) snapshotMemory(ctx context.Context, db *projectdb.DB, w *snapshotWriter, res *ContextSnapshot) error {
	tm := db.Memory()
	if tm == nil {
		return nil
	}
	var buf bytes.Buffer
	if err := ExportKMB(ctx, tm, &buf); err != nil {
		return err
	}
	n, err := tm.Count(ctx)
	if err != nil {
		return fmt.Errorf("count content-memory entries: %w", err)
	}
	res.Entries = n
	return w.write(project.MemoryDirName+"/"+kmb.ConventionalName, buf.Bytes())
}

// snapshotVoice writes each voice profile the store holds at the path it is
// authored, so a clean clone resolves the same voice at the same point.
func (a *App) snapshotVoice(ctx context.Context, db *projectdb.DB, w *snapshotWriter, res *ContextSnapshot) error {
	profiles, err := storedVoiceProfiles(ctx, db)
	if err != nil {
		return err
	}
	for _, bp := range profiles {
		rel := voiceSnapshotRel(bp.binding, w.stands)
		data, err := renderSnapshotProfile(w.existing(rel), bp.profile)
		if err != nil {
			return fmt.Errorf("render voice profile %s: %w", bp.profile.ID, err)
		}
		if err := w.write(rel, data); err != nil {
			return err
		}
		res.VoiceProfiles++
	}
	return nil
}

// voiceSnapshotRel turns a project-relative binding into a path under the
// snapshot directory. stands is the directory the snapshot stands in for, and a
// binding under it, or under `.kapi/`, keeps the path it has there. Anything
// else (a recipe pointing at a profile elsewhere in the tree) is filed under
// the profile directory, so a snapshot never writes above the directory it was
// given.
func voiceSnapshotRel(binding, stands string) string {
	binding = filepath.ToSlash(binding)
	for _, dir := range []string{stands, project.StateDirName} {
		if dir == "" || dir == "." {
			continue
		}
		prefix := dir + "/"
		if len(binding) > len(prefix) && binding[:len(prefix)] == prefix {
			return binding[len(prefix):]
		}
	}
	return project.ProfilesDirName + "/" + binding
}

// renderSnapshotProfile serializes a voice profile for the snapshot. A document
// already there supplies its comments and key order, which is what keeps a
// snapshot over an authored profile a reviewable diff rather than a re-issue;
// a fresh file gets the generated header instead.
func renderSnapshotProfile(original []byte, prof *coreprofile.VoiceProfile) ([]byte, error) {
	data, err := yamledit.Marshal(original, AuthoredVoiceProfile(prof))
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(original)) > 0 {
		return data, nil
	}
	return append([]byte(snapshotVoiceHeader), data...), nil
}

// snapshotRecord writes this checkout's view of the decision ledger into the
// snapshot, through the writer core/state owns.
//
// That writer owns the whole directory: it replaces the shards it writes and
// prunes the ones no unit is left in, which is what keeps a snapshot from
// claiming units a deleted document took with it. It rewrites every shard
// rather than comparing bytes, so the shards that actually moved are worked out
// here and reported.
func (a *App) snapshotRecord(ctx context.Context, db *projectdb.DB, w *snapshotWriter, res *ContextSnapshot) error {
	st := db.Work()
	if st == nil {
		return nil
	}
	units, err := st.All(ctx)
	if err != nil {
		return err
	}
	res.Decisions = len(units)
	dir := filepath.Join(w.dir, project.UnitStateDirName)
	if len(units) == 0 {
		// Nothing to write, and nothing to prune from a directory that is not
		// there; a project with no record leaves the snapshot without one.
		if _, serr := os.Stat(dir); serr != nil {
			return nil
		}
	}
	before, err := recordShardBytes(dir)
	if err != nil {
		return err
	}
	if err := state.WriteCommitted(dir, units); err != nil {
		return err
	}
	after, err := recordShardBytes(dir)
	if err != nil {
		return err
	}
	for name, data := range after {
		if !bytes.Equal(before[name], data) {
			w.written = append(w.written, project.UnitStateDirName+"/"+name)
		}
	}
	for name := range before {
		if _, kept := after[name]; !kept {
			w.written = append(w.written, project.UnitStateDirName+"/"+name)
		}
	}
	return nil
}

// recordShardBytes reads every decision-record shard in dir. A directory that
// is not there holds no shards.
func recordShardBytes(dir string) (map[string][]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]byte{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", project.UnitStateDirName, err)
	}
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != state.CommittedExt {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), rerr)
		}
		out[e.Name()] = data
	}
	return out, nil
}
