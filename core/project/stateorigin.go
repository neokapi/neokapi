package project

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/rootpath"
	"github.com/neokapi/neokapi/core/storage/filelock"
)

// kapi uses as its own only state that kapi on this machine wrote. The state
// directory carries a mark (MarkFilename) holding MACs under a key that exists
// only in this user's configuration on this machine. One MAC covers the
// directory: the identity of the project root and of the state directory, and
// a random id minted with the mark. One more covers each state entry kapi
// wrote (StateEntries), recorded when kapi creates or writes it.

// The work directory is bound by its identity, the identity of the project
// store in it, and the identity of any file an earlier layout kept there. The
// vault, the sync state and the saved filters are bound by their content,
// which kapi writes whole through WriteStateFile, updating the MAC in the same
// step.

// Before any state is read the MACs are verified (CheckStateOwned), and the
// MACs are the only thing that decides: no file's time, recorded id, path or
// origin vouches for state. An entry whose MAC does not verify is not used:
// the work directory is set aside and rebuilt, and the rest is refused until
// a person vouches for it (`kapi trust allow --state`). KAPI_TRUST_STATE=1 in
// kapi's environment skips the refusal for that one process and writes
// nothing.

// What a verified mark answers: the state did not arrive with a clone, a copy,
// a restored backup or an archive unpacked into a new place, and no entry was
// swapped under this checkout (an archive unpacked over it, a sync client
// replacing a file).

// What it does not answer: a process or tool on this machine that rewrites the
// work directory's contents in place (the project store, its write-ahead log,
// the caches or the drafts), keeping their identity; and a process that can
// read this user's key. The project store also records the mark's id
// (core/projectdb), so a store from another checkout written over this one in
// place is set aside too.

// MarkFilename is the file in the state directory that holds the mark.
const MarkFilename = "state.mark"

// stateKeyFile is the per-user, per-machine key, under the configuration
// directory: never in a project, never in a data directory a project names.
const (
	stateKeyDirName = "state"
	stateKeyFile    = "machine.key"
)

// markVersion is the version of the mark's MAC input.
const markVersion = 2

// RegenerableState reports, for each entry of StateEntries, whether kapi
// rebuilds it when it is gone: the work directory holds the project store, the
// caches, the drafts and the locks, all derived from the working tree and the
// workspace. The vault (withheld originals), the sync state (a claim token, the
// confirmed hashes) and the saved filters (a person's own settings) cannot be
// rebuilt, and are bound by their content.
var RegenerableState = map[string]bool{
	WorkDirName:          true,
	SyncDirName:          false,
	VaultDirName:         false,
	LocalFiltersFilename: false,
}

// SetAsideDirName is the state directory's subdirectory that holds state kapi
// set aside because this machine did not write it. It keeps the latest copy of
// each entry and nothing older.
const SetAsideDirName = "set-aside"

// setAsideSuffix names a set-aside copy: <entry><suffix><time>.
const setAsideSuffix = ".not-written-on-this-machine."

// stateMark is the mark's file content. Host, Path and Created are what the
// state directory looked like when the mark was written, and From is the
// origin an earlier mark this one replaced showed; all four are shown to a
// person asked to vouch for state, and none is part of a MAC.
type stateMark struct {
	Version int                    `json:"version"`
	ID      string                 `json:"id"`
	MAC     string                 `json:"mac"`
	Entries map[string]entryRecord `json:"entries,omitempty"`
	Host    string                 `json:"host,omitempty"`
	Path    string                 `json:"path,omitempty"`
	Created string                 `json:"created,omitempty"`
	From    string                 `json:"from,omitempty"`
}

// entryRecord holds the MACs an entry verifies against: one, or two for the
// length of a write (the content before it and after it), so an interrupted
// write leaves the entry verifying either way.
type entryRecord struct {
	MACs []string `json:"macs"`
}

// StateNotice receives the one line kapi prints when it sets state aside. The
// host points it at its own error stream; the default writes to standard
// error.
var StateNotice = func(msg string) { fmt.Fprintln(os.Stderr, msg) }

// UnverifiedStateError is the refusal of state kapi cannot rebuild and this
// machine did not write. Entries names it, relative to the project root.
type UnverifiedStateError struct {
	Root    string
	Entries []string
	Origin  string
	// NewKey is set when this machine's key was created by this process, so
	// state made before cannot verify.
	NewKey bool
}

func (e *UnverifiedStateError) Error() string {
	from := ""
	if e.Origin != "" {
		from = fmt.Sprintf(" It appears to come from %q.", e.Origin)
	}
	return fmt.Sprintf("project: %s was not written by kapi on this machine, so kapi does not use it.%s%s "+
		"If you vouch for it, run `kapi trust allow --state -p %s`; otherwise delete it and run again",
		strings.Join(e.Entries, ", "), from, newKeyNote(e.NewKey), e.Root)
}

// newKeyNote is the sentence that says why state made earlier does not verify
// when the key is new.
func newKeyNote(created bool) string {
	if !created {
		return ""
	}
	return " This machine's key was created just now, so state made earlier is not recognised."
}

// CheckStateOwned is the check every route runs before it reads a project's
// state: whether git tracks it (CheckStateTracked, the early refusal), then
// whether this machine wrote it (the mark).
func CheckStateOwned(ctx context.Context, l Layout) error {
	if err := CheckStateTracked(ctx, l); err != nil {
		return err
	}
	return checkStateMark(l)
}

// checkStateMark verifies the mark of the state directory and of each entry in
// it. An entry that does not verify is set aside when kapi rebuilds it and
// refused when it cannot. A state directory whose own MAC does not verify gets
// a fresh mark with no entry in it: the directory's MAC alone vouches for no
// state, so each entry already there stays unverified, and only what kapi
// writes from then on is recorded. With KAPI_TRUST_STATE=1 the person running
// this process vouches for this run only: nothing is refused and nothing
// written. A state directory kapi keeps outside the project (the desktop's
// drafts, in its data directory) is not marked: no project can name it.
func checkStateMark(l Layout) error {
	if l.Root == "" || !rootpath.IsBeneath(l.Root, l.StateDir) {
		return nil
	}
	if _, err := rootpath.Stat(l.Root, l.StateDir); err != nil {
		return nil // no state directory yet
	}
	key, err := keyFor(l)
	if err != nil {
		return err
	}
	if os.Getenv(TrustStateEnv) == "1" {
		return nil
	}
	unlock, err := lockMark(l)
	if err != nil {
		return err
	}
	mark, merr := readMark(l)
	origin := markOrigin(mark)
	if merr != nil || !dirValid(key, l, mark) {
		fresh, err := newMark(l)
		if err != nil {
			unlock()
			return err
		}
		fresh.From = origin
		if err := writeMark(key, l, fresh); err != nil {
			unlock()
			return err
		}
		mark = fresh
	} else {
		origin = mark.From
	}
	var refused, rebuild []string
	for _, name := range presentState(l) {
		if entryValid(key, l, mark, name) {
			continue
		}
		if regenerable(l, name) {
			rebuild = append(rebuild, name)
			continue
		}
		refused = append(refused, stateRel(l)+"/"+name)
	}
	unlock()
	for _, name := range rebuild {
		if err := setAsideEntry(l, name); err != nil {
			return err
		}
	}
	if len(refused) > 0 {
		return &UnverifiedStateError{Root: l.Root, Entries: refused, Origin: origin, NewKey: keyIsNew()}
	}
	return nil
}

// setAsideEntry moves an entry into SetAsideDirName and says so in one line.
func setAsideEntry(l Layout, name string) error {
	to, err := setAside(l, name)
	if err != nil {
		return err
	}
	StateNotice(fmt.Sprintf("kapi: %s/%s was not written by kapi on this machine; it is set aside in %s and rebuilt.%s",
		stateRel(l), name, to, newKeyNote(keyIsNew())))
	return nil
}

// legacyWorkState are files earlier layouts kept that core/projectdb folds
// into the vault, the sync state or the store: they cannot be rebuilt. A work
// directory holding one is refused until a person vouches for it, and stays
// where it is. Their identity is part of the work directory's MAC, so one that
// appears in a checkout kapi made does not verify.
var legacyWorkState = []string{
	VaultDirName,
	"state.db",
	filepath.Join(CacheDirName, RedactionDirName),
	filepath.Join(CacheDirName, "refs.json"),
	filepath.Join(CacheDirName, "sync-cache.json"),
}

// legacyStateFiles are files the flat layout kept beside the work directory
// that core/projectdb folds into the vault. They are bound with the work
// directory, as legacyWorkState is.
var legacyStateFiles = []string{filepath.Join(CacheDirName, RedactionDirName)}

// regenerable reports whether a present state entry is rebuilt when set aside.
func regenerable(l Layout, name string) bool {
	if !RegenerableState[name] {
		return false
	}
	if name == WorkDirName {
		for _, legacy := range legacyWorkState {
			if _, err := rootpath.Stat(l.Root, filepath.Join(l.WorkDir(), legacy)); err == nil {
				return false
			}
		}
		for _, legacy := range legacyStateFiles {
			if _, err := rootpath.Stat(l.Root, filepath.Join(l.StateDir, legacy)); err == nil {
				return false
			}
		}
	}
	return true
}

// SetAsideWork sets the work directory aside, so it is rebuilt: what
// core/projectdb calls when the store in it records another checkout's mark.
func SetAsideWork(l Layout) error {
	if _, err := rootpath.Stat(l.Root, l.WorkDir()); err != nil {
		return nil
	}
	return setAsideEntry(l, WorkDirName)
}

// StateMarkID is the id of the state directory's mark when its MAC verifies,
// and "" when it does not or there is none.
func StateMarkID(l Layout) string {
	if l.Root == "" || !rootpath.IsBeneath(l.Root, l.StateDir) {
		return ""
	}
	key, err := keyFor(l)
	if err != nil {
		return ""
	}
	mark, err := readMark(l)
	if err != nil || !dirValid(key, l, mark) {
		return ""
	}
	return mark.ID
}

// RecordStateEntry records the state entry name as kapi left it, so it
// verifies as this machine's own. kapi calls it after it creates an entry, or
// creates or replaces the project store; a write of a bound file goes through
// WriteStateFile instead. Without a verified mark on the state directory
// (KAPI_TRUST_STATE=1 over state from elsewhere) nothing is recorded.
func RecordStateEntry(l Layout, name string) error {
	if l.Root == "" || !rootpath.IsBeneath(l.Root, l.StateDir) || !slices.Contains(StateEntries, name) {
		return nil
	}
	key, err := keyFor(l)
	if err != nil {
		return err
	}
	unlock, err := lockMark(l)
	if err != nil {
		return err
	}
	defer unlock()
	mark, err := readMark(l)
	if err != nil || !dirValid(key, l, mark) {
		return nil
	}
	if mark.Entries == nil {
		mark.Entries = map[string]entryRecord{}
	}
	if _, err := rootpath.Stat(l.Root, filepath.Join(l.StateDir, name)); err != nil {
		if _, ok := mark.Entries[name]; !ok {
			return nil
		}
		delete(mark.Entries, name)
		return writeMark(key, l, mark)
	}
	binding, err := entryBinding(l, name, nil)
	if err != nil {
		return err
	}
	mac := entryMAC(key, l, mark, name, binding)
	if rec := mark.Entries[name]; len(rec.MACs) == 1 && rec.MACs[0] == mac {
		return nil
	}
	mark.Entries[name] = entryRecord{MACs: []string{mac}}
	return writeMark(key, l, mark)
}

// WriteStateFile writes data whole to path. A path in a state entry bound by
// content (the vault, the sync state, the saved filters) of a state directory
// with a verified mark is written in step with the entry's MAC. The MAC of the
// content after the write is added, the file replaced, and the MAC of the
// content before removed, so an interrupted write leaves the entry verifying.
// The entry must verify before the write; an entry that does not is refused
// rather than recorded. Any other path is written whole, by rename.
func WriteStateFile(path string, data []byte, perm fs.FileMode) error {
	l, name, rel, ok := stateEntryOf(path)
	if !ok {
		return writeWhole(filepath.Dir(path), filepath.Base(path), data, perm)
	}
	key, err := keyFor(l)
	if err != nil {
		return err
	}
	unlock, err := lockMark(l)
	if err != nil {
		return err
	}
	defer unlock()
	mark, merr := readMark(l)
	if merr != nil || !dirValid(key, l, mark) {
		// Only KAPI_TRUST_STATE=1 reaches a write here: the person vouches for
		// this run, and nothing is recorded.
		return writeWhole(l.StateDir, filepath.Join(name, rel), data, perm)
	}
	var before []string
	if _, err := rootpath.Stat(l.Root, filepath.Join(l.StateDir, name)); err == nil {
		if !entryValid(key, l, mark, name) {
			return &UnverifiedStateError{Root: l.Root, Entries: []string{stateRel(l) + "/" + name}, Origin: mark.From, NewKey: keyIsNew()}
		}
		before = mark.Entries[name].MACs
	}
	after, err := entryBinding(l, name, map[string][]byte{filepath.ToSlash(filepath.Join(name, rel)): data})
	if err != nil {
		return err
	}
	afterMAC := entryMAC(key, l, mark, name, after)
	if mark.Entries == nil {
		mark.Entries = map[string]entryRecord{}
	}
	mark.Entries[name] = entryRecord{MACs: append(slices.Clone(before), afterMAC)}
	if err := writeMark(key, l, mark); err != nil {
		return err
	}
	if err := writeWhole(l.StateDir, filepath.Join(name, rel), data, perm); err != nil {
		return err
	}
	mark.Entries[name] = entryRecord{MACs: []string{afterMAC}}
	return writeMark(key, l, mark)
}

// stateEntryOf finds the state entry bound by content that path is in: the
// nearest state directory above it with a mark, beside a recipe.
func stateEntryOf(path string) (l Layout, name, rel string, ok bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Layout{}, "", "", false
	}
	for dir := filepath.Dir(abs); ; dir = filepath.Dir(dir) {
		if filepath.Base(dir) == StateDirName {
			root := filepath.Dir(dir)
			if _, err := rootpath.Stat(root, filepath.Join(dir, MarkFilename)); err == nil {
				r, err := filepath.Rel(dir, abs)
				if err != nil {
					return Layout{}, "", "", false
				}
				first, rest, _ := strings.Cut(filepath.ToSlash(r), "/")
				if RegenerableState[first] || !slices.Contains(StateEntries, first) {
					return Layout{}, "", "", false
				}
				return Layout{Root: root, StateDir: dir}, first, filepath.FromSlash(rest), true
			}
		}
		if parent := filepath.Dir(dir); parent == dir {
			return Layout{}, "", "", false
		}
	}
}

// writeWhole writes data to rel beneath dir through an os.Root on dir: to a
// temporary file at the top of dir, then by rename, creating the parent
// directories owner-only. A temporary file an interrupted write leaves is
// outside every state entry, so it changes none of their content.
func writeWhole(dir, rel string, data []byte, perm fs.FileMode) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if parent := filepath.Dir(rel); parent != "." {
		if err := root.MkdirAll(parent, 0o700); err != nil {
			return err
		}
	}
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	tmp := ".write-" + hex.EncodeToString(suffix) + ".tmp"
	if err := root.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := root.Rename(tmp, rel); err != nil {
		_ = root.Remove(tmp)
		return err
	}
	return nil
}

// AdoptState records the state exactly as DescribeState showed it as this
// machine's own: what a person runs to vouch for state (`kapi trust allow
// --state`, the desktop's dialog); nothing calls it on a person's behalf. State
// that changed since it was shown is refused, and nothing is recorded.
func AdoptState(l Layout, shown StateOrigin) error {
	key, err := keyFor(l)
	if err != nil {
		return err
	}
	unlock, err := lockMark(l)
	if err != nil {
		return err
	}
	defer unlock()
	now, err := stateBindings(l)
	if err != nil {
		return err
	}
	if shown.bindings == nil || !mapsEqual(now, shown.bindings) {
		return errors.New("project: the state changed after it was shown, so nothing was recorded; run the command again to see it as it is now")
	}
	mark, err := readMark(l)
	if err != nil || !dirValid(key, l, mark) {
		fresh, ferr := newMark(l)
		if ferr != nil {
			return ferr
		}
		// The id an earlier mark carried stays, so a project store that
		// records it is kept along with the state it was vouched for in.
		if mark != nil && isHexID(mark.ID) {
			fresh.ID = mark.ID
		}
		mark = fresh
	}
	mark.From = ""
	mark.Entries = map[string]entryRecord{}
	for name, binding := range now {
		mark.Entries[name] = entryRecord{MACs: []string{entryMAC(key, l, mark, name, binding)}}
	}
	return writeMark(key, l, mark)
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

func isHexID(s string) bool {
	if len(s) != 32 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// StateOrigin describes the state in the layout's state directory for a person
// asked to vouch for it: the entries present, whether all of them verify, and
// where the state appears to come from. AdoptState records exactly what it
// describes.
type StateOrigin struct {
	Entries  []string
	Verified bool
	Origin   string
	bindings map[string]string
}

// DescribeState reports what a person is shown before vouching for state.
func DescribeState(l Layout) (StateOrigin, error) {
	if _, err := rootpath.Stat(l.Root, l.StateDir); err != nil {
		return StateOrigin{bindings: map[string]string{}}, nil
	}
	out := StateOrigin{}
	key, err := keyFor(l)
	if err != nil {
		return out, err
	}
	out.bindings, err = stateBindings(l)
	if err != nil {
		return out, err
	}
	mark, err := readMark(l)
	valid := err == nil && dirValid(key, l, mark)
	out.Verified = valid
	for _, name := range presentState(l) {
		out.Entries = append(out.Entries, stateRel(l)+"/"+name)
		if !valid || !entryValid(key, l, mark, name) {
			out.Verified = false
		}
	}
	out.Origin = markOrigin(mark)
	if valid && mark.From != "" {
		out.Origin = mark.From
	}
	return out, nil
}

// stateBindings is the binding of each present entry.
func stateBindings(l Layout) (map[string]string, error) {
	out := map[string]string{}
	for _, name := range presentState(l) {
		b, err := entryBinding(l, name, nil)
		if err != nil {
			return nil, err
		}
		out[name] = b
	}
	return out, nil
}

// presentState lists the entries of StateEntries the state directory holds,
// asking the volume for each by the name kapi uses, so an entry spelled
// another way that this volume resolves to it (WORK on a case-insensitive
// volume) counts as that entry. A file the flat layout kept counts as the work
// directory, which it is bound with.
func presentState(l Layout) []string {
	var out []string
	for _, name := range StateEntries {
		if _, err := rootpath.Stat(l.Root, filepath.Join(l.StateDir, name)); err == nil {
			out = append(out, name)
			continue
		}
		if name == WorkDirName {
			for _, legacy := range legacyStateFiles {
				if _, err := rootpath.Stat(l.Root, filepath.Join(l.StateDir, legacy)); err == nil {
					out = append(out, name)
					break
				}
			}
		}
	}
	return out
}

// markOrigin is where a mark says its state was made, for a person.
func markOrigin(m *stateMark) string {
	if m == nil {
		return ""
	}
	parts := []string{}
	if m.Host != "" {
		parts = append(parts, "host "+m.Host)
	}
	if m.Path != "" {
		parts = append(parts, m.Path)
	}
	if m.Created != "" {
		parts = append(parts, "created "+m.Created)
	}
	return displaySafe(strings.Join(parts, ", "))
}

// dirMAC is the MAC of the mark's id over the identity of the project root and
// of the state directory. The identities make a mark copied from another of
// this person's checkouts, or a state directory moved into another checkout,
// fail; the path is left out, so moving or renaming a checkout on its volume
// keeps its state.
func dirMAC(key []byte, l Layout, id string) (string, error) {
	rootID, stateID, err := dirIdentities(l)
	if err != nil {
		return "", err
	}
	h := hmac.New(sha256.New, key)
	fmt.Fprintf(h, "kapi-state-mark/v%d\x00%s\x00%s\x00%s", markVersion, rootID, stateID, id)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func dirIdentities(l Layout) (string, string, error) {
	rootID, err := dirIdentity(l.Root)
	if err != nil {
		return "", "", err
	}
	stateID, err := dirIdentity(l.StateDir)
	if err != nil {
		return "", "", err
	}
	return rootID, stateID, nil
}

// entryMAC is the MAC of one entry's binding, tied to the directory's
// identities and the mark's id.
func entryMAC(key []byte, l Layout, m *stateMark, name, binding string) string {
	rootID, stateID, _ := dirIdentities(l)
	h := hmac.New(sha256.New, key)
	fmt.Fprintf(h, "kapi-state-entry/v%d\x00%s\x00%s\x00%s\x00%s\x00%s", markVersion, rootID, stateID, m.ID, name, binding)
	return hex.EncodeToString(h.Sum(nil))
}

func dirValid(key []byte, l Layout, m *stateMark) bool {
	if m == nil || m.Version != markVersion || m.ID == "" || m.MAC == "" {
		return false
	}
	want, err := dirMAC(key, l, m.ID)
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(want), []byte(m.MAC))
}

// entryValid reports whether a present entry verifies under a mark whose
// directory MAC verifies.
func entryValid(key []byte, l Layout, m *stateMark, name string) bool {
	rec, ok := m.Entries[name]
	if !ok || len(rec.MACs) == 0 {
		return false
	}
	binding, err := entryBinding(l, name, nil)
	if err != nil {
		return false
	}
	want := []byte(entryMAC(key, l, m, name, binding))
	for _, mac := range rec.MACs {
		if hmac.Equal(want, []byte(mac)) {
			return true
		}
	}
	return false
}

// entryBinding is what an entry's MAC covers. The work directory is bound by
// identity: its own, the project store's and that of each legacy file in it.
// Every other entry is bound by its content: the path and SHA-256 of each file
// in it, with subst standing in for the files a write is about to replace
// (keyed by slash path from the state directory). A link or an irregular file
// in an entry fails the binding.
func entryBinding(l Layout, name string, subst map[string][]byte) (string, error) {
	root, err := os.OpenRoot(l.StateDir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if RegenerableState[name] {
		var b strings.Builder
		for _, rel := range append(append([]string{name, filepath.Join(name, StoreFileName)}, legacyPaths(name)...), legacyStateFiles...) {
			id, err := entryIdentity(root, l.StateDir, rel)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "%s=%s;", filepath.ToSlash(rel), id)
		}
		return "identity:" + b.String(), nil
	}
	files := map[string][]byte{}
	info, err := root.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist) && len(subst) > 0:
		// An entry a write is about to create holds what it writes.
	case err != nil:
		return "", err
	case info.Mode().IsRegular():
		files[name] = nil
	case info.IsDir():
		err := fs.WalkDir(root.FS(), name, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			switch {
			case d.IsDir():
				return nil
			case d.Type().IsRegular():
				files[p] = nil
				return nil
			default:
				return fmt.Errorf("project: %s/%s is not a regular file", stateRel(l), p)
			}
		})
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("project: %s/%s is not a regular file or directory", stateRel(l), name)
	}
	for p := range subst {
		files[p] = nil
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		data, ok := subst[p]
		if !ok {
			data, err = root.ReadFile(p)
			if err != nil {
				return "", err
			}
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(h, "%s\x00%x\n", p, sum)
	}
	return "content:" + hex.EncodeToString(h.Sum(nil)), nil
}

func legacyPaths(name string) []string {
	out := make([]string, 0, len(legacyWorkState))
	for _, p := range legacyWorkState {
		out = append(out, filepath.Join(name, p))
	}
	return out
}

// entryIdentity is the identity of rel beneath the state directory, or
// "absent". A link fails.
func entryIdentity(root *os.Root, stateDir, rel string) (string, error) {
	info, err := root.Lstat(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return "absent", nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return "", fmt.Errorf("project: %s is a link", rel)
	}
	return dirIdentity(filepath.Join(stateDir, rel))
}

func readMark(l Layout) (*stateMark, error) {
	data, err := rootpath.ReadFileWithin(l.Root, filepath.Join(l.StateDir, MarkFilename))
	if err != nil {
		return nil, err
	}
	var m stateMark
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	// The display fields come from a mark that may have arrived from anywhere:
	// they are made safe to show here, where they are read, so no caller can
	// forget. Whoever shows them also quotes them.
	m.Host, m.Path, m.Created, m.From = displaySafe(m.Host), displaySafe(m.Path), displaySafe(m.Created), displaySafe(m.From)
	return &m, nil
}

// maxDisplayField caps a display field of a mark, in runes.
const maxDisplayField = 200

// displaySafe drops control and invisible format characters from s and caps
// its length.
func displaySafe(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError {
			continue
		}
		if n == maxDisplayField {
			b.WriteString("...")
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// createStateDir creates the layout's state directory with its mark in one
// step: the mark is written into a staged directory beside it, which is then
// renamed into place, so no state directory kapi creates is ever there without
// its mark. A state directory that appeared meanwhile (another process) is
// left as it is, and the staged one removed.
func createStateDir(l Layout) error {
	key, err := keyFor(l)
	if err != nil {
		return err
	}
	parent, err := os.OpenRoot(filepath.Dir(l.StateDir))
	if err != nil {
		return err
	}
	defer parent.Close()
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	staged := filepath.Base(l.StateDir) + ".creating-" + hex.EncodeToString(suffix)
	if err := parent.Mkdir(staged, 0o755); err != nil {
		return err
	}
	stagedLayout := l
	stagedLayout.StateDir = filepath.Join(filepath.Dir(l.StateDir), staged)
	// The directory's MAC binds the state directory's identity, which the
	// rename keeps.
	m, err := newMark(stagedLayout)
	if err == nil {
		m.Path = l.Root
		err = writeMark(key, stagedLayout, m)
	}
	if err != nil {
		_ = parent.RemoveAll(staged)
		return err
	}
	if err := parent.Rename(staged, filepath.Base(l.StateDir)); err != nil {
		_ = parent.RemoveAll(staged)
		if _, serr := parent.Stat(filepath.Base(l.StateDir)); serr == nil {
			return nil
		}
		return err
	}
	return nil
}

// newMark mints a mark with a fresh id and no entries.
func newMark(l Layout) (*stateMark, error) {
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	return &stateMark{
		Version: markVersion, ID: hex.EncodeToString(idBytes),
		Host: host, Path: l.Root, Created: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// writeMark writes m into the state directory with its directory MAC, through
// an os.Root on it, replacing any mark there.
func writeMark(key []byte, l Layout, m *stateMark) error {
	mac, err := dirMAC(key, l, m.ID)
	if err != nil {
		return err
	}
	m.Version, m.MAC = markVersion, mac
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeWhole(l.StateDir, MarkFilename, append(data, '\n'), 0o600)
}

// setAside moves a state entry into SetAsideDirName under a name that says why,
// removing any earlier copy of the same entry, and returns the new location
// relative to the project root.
func setAside(l Layout, name string) (string, error) {
	root, err := os.OpenRoot(l.StateDir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err := root.MkdirAll(SetAsideDirName, 0o700); err != nil {
		return "", err
	}
	if earlier, err := fs.ReadDir(root.FS(), SetAsideDirName); err == nil {
		for _, e := range earlier {
			if strings.HasPrefix(e.Name(), name+setAsideSuffix) {
				if err := root.RemoveAll(filepath.Join(SetAsideDirName, e.Name())); err != nil {
					return "", fmt.Errorf("project: remove an earlier copy of %s: %w", name, err)
				}
			}
		}
	}
	to := filepath.Join(SetAsideDirName, name+setAsideSuffix+time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := root.Rename(name, to); err != nil {
		return "", fmt.Errorf("project: set aside %s: %w", name, err)
	}
	return stateRel(l) + "/" + filepath.ToSlash(to), nil
}

// markLocks orders the mark's updates within this process, per state
// directory; lockMark adds a lock file beside the key for other processes.
var markLocks sync.Map // state directory -> *sync.Mutex

// lockMark takes the lock that orders updates to one state directory's mark.
func lockMark(l Layout) (func(), error) {
	v, _ := markLocks.LoadOrStore(filepath.Clean(l.StateDir), &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	dir, err := keyDirOnDisk()
	if err != nil || dir == "" {
		return mu.Unlock, err
	}
	sum := sha256.Sum256([]byte(filepath.Clean(l.StateDir)))
	lock, err := filelock.Open(filepath.Join(dir, "mark-"+hex.EncodeToString(sum[:8])+".lock"))
	if err != nil {
		mu.Unlock()
		return nil, err
	}
	if err := lock.LockBrief(); err != nil {
		_ = lock.Close()
		mu.Unlock()
		return nil, err
	}
	return func() {
		lock.Unlock()
		_ = lock.Close()
		mu.Unlock()
	}, nil
}

// stateKeys caches the key per path for the life of the process.
var stateKeys sync.Map // key path -> []byte

// createdKeys holds the key paths this process created: a refusal under one
// says the key is new, so state made before cannot verify.
var createdKeys sync.Map // key path -> struct{}

// keyIsNew reports whether this process created the key it uses now.
func keyIsNew() bool {
	if usesTestKey() {
		return false
	}
	dir, err := stateKeyDir()
	if err != nil {
		return false
	}
	_, ok := createdKeys.Load(filepath.Join(dir, stateKeyFile))
	return ok
}

// usesTestKey reports whether this process keeps its key in memory: a test
// binary with no KAPI_CONFIG_DIR, so it neither reads the developer's key nor
// leaves one behind.
func usesTestKey() bool {
	return os.Getenv("KAPI_CONFIG_DIR") == "" && flag.Lookup("test.v") != nil
}

// keyDirOnDisk is the key's directory, or "" for a key kept in memory.
func keyDirOnDisk() (string, error) {
	if usesTestKey() {
		return "", nil
	}
	return stateKeyDir()
}

// keyFor returns the key for state in l, refusing a configuration directory
// inside the project, where the project's own files could hold the key.
func keyFor(l Layout) ([]byte, error) {
	if !usesTestKey() && l.Root != "" {
		dir, err := stateKeyDir()
		if err != nil {
			return nil, err
		}
		if within(resolved(dir), resolved(l.Root)) {
			return nil, fmt.Errorf("project: the configuration directory %s is inside the project %s, and kapi keeps its state key outside every project; set KAPI_CONFIG_DIR to a directory outside it", filepath.Dir(dir), l.Root)
		}
	}
	return stateKey()
}

// resolved is p with its links resolved as far as it exists.
func resolved(p string) string {
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(resolved(parent), filepath.Base(p))
}

func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// stateKey returns this user's per-machine state key, creating it on first
// use: 32 random bytes in a regular file only this user can read and write
// (mode 0600, owned by the user), in a directory only this user can write. A
// key file or directory that is anything else is refused, with the fix.
func stateKey() ([]byte, error) {
	if usesTestKey() {
		return testStateKey()
	}
	dir, err := stateKeyDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, stateKeyFile)
	if k, ok := stateKeys.Load(path); ok {
		return k.([]byte), nil
	}
	var key []byte
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) { //pathguard: this user's own key file under the configuration directory, which no project names
		key, err = createStateKey(dir, path)
		if err != nil {
			return nil, fmt.Errorf("project: create the state key %s: %w", path, err)
		}
	}
	dinfo, err := os.Lstat(dir) //pathguard: this user's own key directory under the configuration directory
	if err != nil {
		return nil, fmt.Errorf("project: the state key directory %s: %w", dir, err)
	}
	info, err := os.Lstat(path) //pathguard: this user's own key file under the configuration directory
	if err != nil {
		return nil, fmt.Errorf("project: the state key %s: %w", path, err)
	}
	if err := keyPrivate(dir, path, dinfo, info); err != nil {
		return nil, err
	}
	if key == nil {
		key, err = os.ReadFile(path) //pathguard: this user's own key file under the configuration directory, which no project names
		if err != nil {
			return nil, fmt.Errorf("project: read the state key %s: %w", path, err)
		}
	}
	if len(key) < 32 {
		return nil, fmt.Errorf("project: the state key %s is damaged; remove it and run again, then vouch for each checkout's state", path)
	}
	stateKeys.Store(path, key)
	return key, nil
}

func createStateKey(dir, path string) ([]byte, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil { //pathguard: this user's configuration directory, which no project names
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil { //pathguard: the key's own directory under the configuration directory
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	// The key is written whole to a temporary file and linked into place, so
	// no reader sees a partly written key, and a key another process linked
	// first is the one every process uses.
	f, err := os.CreateTemp(dir, ".machine.key-*") //pathguard: this user's own key directory under the configuration directory
	if err != nil {
		return nil, err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }() //pathguard: the temporary key file this function created
	if _, err := f.Write(key); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	err = os.Link(tmp, path) //pathguard: this user's own key file under the configuration directory
	if errors.Is(err, errors.ErrUnsupported) {
		// A platform without links (the browser) runs one kapi at a time.
		err = os.Rename(tmp, path) //pathguard: this user's own key file under the configuration directory
	}
	if errors.Is(err, fs.ErrExist) {
		return os.ReadFile(path) //pathguard: this user's own key file under the configuration directory
	} else if err != nil {
		return nil, err
	}
	createdKeys.Store(path, struct{}{})
	return key, nil
}

// testStateKey is a test binary's process-lifetime key.
var testStateKey = sync.OnceValues(func() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
})

// stateKeyDir is the directory the key lives in: under KAPI_CONFIG_DIR when it
// is set, as every isolated invocation sets it, and otherwise under the user's
// configuration directory. It is absolute, so no working directory, and so no
// project, can decide where the key is.
func stateKeyDir() (string, error) {
	dir := os.Getenv("KAPI_CONFIG_DIR")
	if dir != "" {
		dir = filepath.Join(dir, stateKeyDirName)
	} else if configDir, err := os.UserConfigDir(); err == nil {
		dir = filepath.Join(configDir, "kapi", stateKeyDirName)
	}
	if !filepath.IsAbs(dir) {
		return "", errors.New("project: no configuration directory to keep the state key in; set KAPI_CONFIG_DIR to an absolute path")
	}
	return dir, nil
}
