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
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/rootpath"
)

// kapi uses as its own only state that kapi on this machine wrote. When kapi
// creates a project's state directory it writes a mark into it (MarkFilename):
// a MAC, under a key that exists only in this user's configuration on this
// machine, over the identity of the project root and of the state directory
// and a random id minted with the mark. Before any state is read the mark is
// verified (CheckStateOwned).

// State without a valid mark is not used: what kapi rebuilds anyway (the work
// directory) is set aside and rebuilt, and what it cannot rebuild (the vault,
// the sync state, the saved filters) is refused until a person vouches for it
// (`kapi trust allow --state`, or KAPI_TRUST_STATE=1 in kapi's environment).

// What a valid mark says: kapi on this machine, as this user, created this
// state directory in this checkout. It binds the directory, not each byte in
// it: a file another process on this machine writes into a marked directory
// later is outside what it covers, and so is a process that can read this
// user's key. The threat it answers is state that arrived from elsewhere: a
// commit, an archive, a copy or a synchronised folder.

// MarkFilename is the file in the state directory that holds the mark.
const MarkFilename = "state.mark"

// stateKeyFile is the per-user, per-machine key, under the configuration
// directory: never in a project, never in a data directory a project names.
const (
	stateKeyDirName = "state"
	stateKeyFile    = "machine.key"
)

// markVersion is the version of the mark's MAC input.
const markVersion = 1

// RegenerableState reports, for each entry of StateEntries, whether kapi
// rebuilds it when it is gone: the work directory holds the project store, the
// caches, the drafts and the locks, all derived from the working tree and the
// workspace. The vault (withheld originals), the sync state (a claim token, the
// confirmed hashes) and the saved filters (a person's own settings) cannot be
// rebuilt.
var RegenerableState = map[string]bool{
	WorkDirName:          true,
	SyncDirName:          false,
	VaultDirName:         false,
	LocalFiltersFilename: false,
}

// SetAsideDirName is the state directory's subdirectory that holds state kapi
// set aside because this machine did not write it.
const SetAsideDirName = "set-aside"

// stateMark is the mark's file content. Host, Path and Created are what the
// state directory looked like when the mark was written, shown to a person who
// is asked to vouch for state; they are not part of the MAC.
type stateMark struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	MAC     string `json:"mac"`
	Host    string `json:"host,omitempty"`
	Path    string `json:"path,omitempty"`
	Created string `json:"created,omitempty"`
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
}

func (e *UnverifiedStateError) Error() string {
	from := ""
	if e.Origin != "" {
		from = " It appears to come from " + e.Origin + "."
	}
	return fmt.Sprintf("project: %s was not written by kapi on this machine, so kapi does not use it.%s "+
		"If you vouch for it, run `kapi trust allow --state -p %s`; otherwise delete it and run again",
		strings.Join(e.Entries, ", "), from, e.Root)
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

// checkStateMark verifies the state directory's mark, and when it does not
// verify, sets aside what kapi rebuilds and refuses what it cannot. A state
// directory with no state in it yet gets a fresh mark. A state directory kapi
// keeps outside the project (the desktop's drafts, in its data directory) is
// not marked: no project can name it.
func checkStateMark(l Layout) error {
	if l.Root == "" || !rootpath.IsBeneath(l.Root, l.StateDir) {
		return nil
	}
	if _, err := rootpath.Stat(l.Root, l.StateDir); err != nil {
		// No state directory yet: whatever appears there from now on, this
		// process wrote (stateSeen).
		stateSeen.LoadOrStore(l.Root, time.Now())
		return nil
	}
	present := presentState(l)
	key, err := stateKey()
	if err != nil {
		return err
	}
	mark, merr := readMark(l)
	if merr == nil && markValid(key, l, mark) {
		stateSeen.LoadOrStore(l.Root, time.Now())
		return nil
	}
	if os.Getenv(TrustStateEnv) == "1" {
		// The person running this process vouches for the state.
		return writeMark(key, l)
	}
	if len(present) == 0 || (errors.Is(merr, fs.ErrNotExist) && writtenSinceSeen(l, present)) {
		return writeMark(key, l)
	}
	var refused []string
	rel := stateRel(l)
	for _, name := range present {
		if regenerable(l, name) {
			to, err := setAside(l, name)
			if err != nil {
				return err
			}
			StateNotice(fmt.Sprintf("kapi: %s/%s was not written by kapi on this machine; it is set aside in %s and rebuilt", rel, name, to))
			continue
		}
		refused = append(refused, rel+"/"+name)
	}
	if len(refused) > 0 {
		return &UnverifiedStateError{Root: l.Root, Entries: refused, Origin: markOrigin(mark)}
	}
	return writeMark(key, l)
}

// stateSeen holds, per project root, when this process first found its state
// directory absent or carrying a valid mark. State that appears unmarked after
// that, and was written after it, is this process's own (a draft or a cache a
// route made before the store opened), and gets the mark instead of being set
// aside.
var stateSeen sync.Map // project root -> time.Time

// writtenSinceSeen reports whether this process saw the state directory empty
// or marked, and every present entry was modified since.
func writtenSinceSeen(l Layout, present []string) bool {
	seen, ok := stateSeen.Load(l.Root)
	if !ok {
		return false
	}
	for _, name := range present {
		info, err := rootpath.Stat(l.Root, filepath.Join(l.StateDir, name))
		if err != nil || info.ModTime().Before(seen.(time.Time)) {
			return false
		}
	}
	return true
}

// legacyWorkState are files earlier layouts kept under the work directory that
// cannot be rebuilt: the vault and the working store with its staged
// decisions, which core/projectdb folds forward. A work directory holding one
// is refused until a person vouches for it, and stays where it is.
var legacyWorkState = []string{VaultDirName, "state.db"}

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
	}
	return true
}

// ensureStateMark writes a mark into a state directory that has no valid one.
// Its callers have just created the directory, or verified the state in it.
func ensureStateMark(l Layout) error {
	if l.Root == "" || !rootpath.IsBeneath(l.Root, l.StateDir) {
		return nil
	}
	key, err := stateKey()
	if err != nil {
		return err
	}
	if mark, err := readMark(l); err == nil && markValid(key, l, mark) {
		return nil
	}
	return writeMark(key, l)
}

// AdoptState writes a fresh mark over the state directory, so the state in it
// is used as this machine's own. It is what a person runs to vouch for state
// (`kapi trust allow --state`); nothing calls it on a person's behalf.
func AdoptState(l Layout) error {
	key, err := stateKey()
	if err != nil {
		return err
	}
	return writeMark(key, l)
}

// StateOrigin describes the state in the layout's state directory for a person
// asked to vouch for it: the entries present, whether the mark verifies, and
// where the state appears to come from.
type StateOrigin struct {
	Entries  []string
	Verified bool
	Origin   string
}

// DescribeState reports what DescribeState's caller shows before adopting.
func DescribeState(l Layout) (StateOrigin, error) {
	if _, err := rootpath.Stat(l.Root, l.StateDir); err != nil {
		return StateOrigin{}, nil
	}
	out := StateOrigin{}
	for _, name := range presentState(l) {
		out.Entries = append(out.Entries, stateRel(l)+"/"+name)
	}
	key, err := stateKey()
	if err != nil {
		return out, err
	}
	mark, err := readMark(l)
	out.Verified = err == nil && markValid(key, l, mark)
	out.Origin = markOrigin(mark)
	return out, nil
}

// presentState lists the entries of StateEntries the state directory holds,
// asking the volume for each by the name kapi uses, so an entry spelled
// another way that this volume resolves to it (WORK on a case-insensitive
// volume) counts as that entry.
func presentState(l Layout) []string {
	var out []string
	for _, name := range StateEntries {
		if _, err := rootpath.Stat(l.Root, filepath.Join(l.StateDir, name)); err == nil {
			out = append(out, name)
		}
	}
	return out
}

// markOrigin is where an unverified mark says its state was made, for the
// refusal: "no mark" when there is none.
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
	return strings.Join(parts, ", ")
}

// markMAC is the MAC of the mark's id over the identity of the project root and
// of the state directory. The identities make a mark copied from another of
// this person's checkouts, or a state directory moved into another checkout,
// fail; the path is left out, so moving or renaming a checkout on its volume
// keeps its state.
func markMAC(key []byte, l Layout, id string) (string, error) {
	rootID, err := dirIdentity(l.Root)
	if err != nil {
		return "", err
	}
	stateID, err := dirIdentity(l.StateDir)
	if err != nil {
		return "", err
	}
	h := hmac.New(sha256.New, key)
	fmt.Fprintf(h, "kapi-state-mark/v%d\x00%s\x00%s\x00%s", markVersion, rootID, stateID, id)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func markValid(key []byte, l Layout, m *stateMark) bool {
	if m == nil || m.Version != markVersion || m.ID == "" || m.MAC == "" {
		return false
	}
	want, err := markMAC(key, l, m.ID)
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(want), []byte(m.MAC))
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
	return &m, nil
}

// writeMark mints a fresh id and writes the mark into the state directory
// through an os.Root on it, replacing any mark there.
func writeMark(key []byte, l Layout) error {
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return err
	}
	id := hex.EncodeToString(idBytes)
	mac, err := markMAC(key, l, id)
	if err != nil {
		return err
	}
	host, _ := os.Hostname()
	data, err := json.MarshalIndent(stateMark{
		Version: markVersion, ID: id, MAC: mac,
		Host: host, Path: l.Root, Created: time.Now().UTC().Format(time.RFC3339),
	}, "", "  ")
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(l.StateDir)
	if err != nil {
		return err
	}
	defer root.Close()
	tmp := MarkFilename + ".tmp"
	if err := root.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := root.Rename(tmp, MarkFilename); err != nil {
		_ = root.Remove(tmp)
		return err
	}
	return nil
}

// setAside moves a state entry into SetAsideDirName under a name that says why,
// and returns the new location relative to the project root.
func setAside(l Layout, name string) (string, error) {
	root, err := os.OpenRoot(l.StateDir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err := root.MkdirAll(SetAsideDirName, 0o700); err != nil {
		return "", err
	}
	to := filepath.Join(SetAsideDirName, fmt.Sprintf("%s.not-written-on-this-machine.%s", name, time.Now().UTC().Format("20060102T150405.000000000Z")))
	if err := root.Rename(name, to); err != nil {
		return "", fmt.Errorf("project: set aside %s: %w", name, err)
	}
	return stateRel(l) + "/" + filepath.ToSlash(to), nil
}

// stateKeys caches the key per path for the life of the process.
var stateKeys sync.Map // key path -> []byte

// stateKey returns this user's per-machine state key, creating it on first
// use: 32 random bytes in a file only this user can read, in a directory only
// this user can open.
func stateKey() ([]byte, error) {
	dir := stateKeyDir()
	path := filepath.Join(dir, stateKeyFile)
	if k, ok := stateKeys.Load(path); ok {
		return k.([]byte), nil
	}
	key, err := os.ReadFile(path) //pathguard: this user's own key file under the configuration directory, which no project names
	if errors.Is(err, fs.ErrNotExist) {
		key, err = createStateKey(dir, path)
	}
	if err != nil {
		return nil, fmt.Errorf("project: read the state key %s: %w", path, err)
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
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //pathguard: this user's own key file under the configuration directory
	if errors.Is(err, fs.ErrExist) {
		// Another process created it first: use that one.
		return os.ReadFile(path) //pathguard: this user's own key file under the configuration directory
	}
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(key); err != nil {
		_ = f.Close()
		return nil, err
	}
	return key, f.Close()
}

// stateKeyDir is the directory of the state key: under KAPI_CONFIG_DIR when it
// is set, as every isolated invocation sets it; in a test binary with none,
// one of the process's own under the temporary directory; otherwise under the
// user's configuration directory.
func stateKeyDir() string {
	if dir := os.Getenv("KAPI_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, stateKeyDirName)
	}
	if flag.Lookup("test.v") != nil {
		return filepath.Join(os.TempDir(), fmt.Sprintf("kapi-test-statekey-%d", os.Getpid()))
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "kapi", stateKeyDirName)
}
