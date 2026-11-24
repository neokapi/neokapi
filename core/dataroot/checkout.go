package dataroot

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/partjoin"
)

// A checkout's own state (its project store, caches, drafts, locks, sync
// state, redaction vault and saved filters) lives under the data root, never
// in the checkout. Each checkout has one directory, keyed by the checkout's
// resolved location and the identity of its root directory, which kapi reads
// from the operating system:
//
//	<data root>/checkouts/<key>/            the checkout's state
//	<data root>/checkouts/<key>/checkout.json   where it was, and its identity
//
// Nothing a checkout contains feeds the key. A recipe can copy another
// project's id or name, but it cannot give its directory another directory's
// identity, so a repository cloned where another project lived does not reach
// that project's state, and two checkouts of one project keep separate state.

// A checkout renamed or moved on its volume keeps its identity. When no state
// exists for its new location and a record has its identity and a location
// that no longer exists, the state follows it. Any other move starts with no
// state; a person brings the state across with `kapi state relink`.

// CheckoutsDirName is the directory under the data root that holds one
// directory per checkout.
const CheckoutsDirName = "checkouts"

// CheckoutRecordFilename is the record in each checkout's directory.
const CheckoutRecordFilename = "checkout.json"

// CheckoutRecord is what kapi records about a checkout beside its state: where
// it was and the identity its root directory had. Told lists the one-time
// notices the person was given for it. Project names the project whose state
// it holds; a record written before kapi recorded it has none.
type CheckoutRecord struct {
	Path     string           `json:"path"`
	Identity string           `json:"identity"`
	Created  string           `json:"created"`
	Told     map[string]bool  `json:"told,omitempty"`
	Project  *CheckoutProject `json:"project,omitempty"`
}

// CheckoutProject is the project a checkout's state belongs to, as its recipe
// stated it when kapi last wrote the state: the recipe's id, and its name for
// a person to read. Either may be empty.
type CheckoutProject struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// SetRecordProject records the project the state in dir belongs to, when the
// record says otherwise.
func SetRecordProject(dir string, p CheckoutProject) error {
	return updateRecord(dir, func(rec *CheckoutRecord) bool {
		if rec.Project != nil && *rec.Project == p {
			return false
		}
		rec.Project = &p
		return true
	})
}

// Checkout is one checkout's state directory and its record.
type Checkout struct {
	Dir    string
	Record CheckoutRecord
}

// CheckoutsDir is the directory that holds every checkout's state.
func CheckoutsDir() string { return filepath.Join(Dir(), CheckoutsDirName) }

// checkoutKey keys a checkout by its resolved location and its root's
// identity.
func checkoutKey(path, id string) string {
	sum := sha256.Sum256([]byte(partjoin.NUL("kapi-checkout/v1", path, id)))
	return hex.EncodeToString(sum[:16])
}

// followed caches, per resolved location and identity, the directory a
// checkout's state was found in, so the rename search runs once per process.
var followed sync.Map // followKey -> dir

// followKey keys followed: a checkout's resolved location and identity.
type followKey struct{ path, id string }

// CheckoutDir returns the directory that holds the state of the checkout at
// root, whether or not it exists yet. When it does not exist and a record
// carries this root's identity and a location that no longer exists, the
// checkout was renamed or moved on its volume: that state directory is
// renamed to this one, its record updated, and this one returned. A root that
// does not exist has no identity, and its directory holds nothing.
func CheckoutDir(root string) string {
	path := NormalizePath(root)
	id, err := identity(path)
	if err != nil {
		return filepath.Join(CheckoutsDir(), checkoutKey(path, ""))
	}
	cacheKey := followKey{path: path, id: id}
	if dir, ok := followed.Load(cacheKey); ok {
		return dir.(string)
	}
	dir := filepath.Join(CheckoutsDir(), checkoutKey(path, id))
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		if same := recordedUnderAnotherSpelling(path, id); same != "" {
			// The same directory, typed another way on a volume that does not
			// tell spellings apart: one checkout, one state directory.
			dir = same
		} else if followable(id) {
			followRename(dir, path, id)
		}
	}
	followed.Store(cacheKey, dir)
	return dir
}

// recordedUnderAnotherSpelling returns the state directory recorded for this
// very directory under another spelling of its location (Docs and docs on a
// volume that folds case), and "" when there is none. The recorded location
// must still name the same directory now.
func recordedUnderAnotherSpelling(path, id string) string {
	here, err := os.Stat(path)
	if err != nil {
		return ""
	}
	for _, c := range Checkouts() {
		if c.Record.Identity != id || c.Record.Path == path {
			continue
		}
		there, err := os.Stat(c.Record.Path)
		if err == nil && os.SameFile(here, there) {
			return c.Dir
		}
	}
	return ""
}

// followRename moves the state of a checkout that had this identity at a
// location that no longer exists to dir.
func followRename(dir, path, id string) {
	for _, c := range Checkouts() {
		if c.Record.Identity != id || c.Record.Path == path {
			continue
		}
		if _, err := os.Stat(c.Record.Path); !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err := os.Rename(c.Dir, dir); err != nil {
			return
		}
		rec := c.Record
		rec.Path = path
		_ = writeRecord(dir, rec)
		return
	}
}

// EnsureCheckoutDir creates the state directory of the checkout at root when
// it does not exist, with its record, in one step: the directory is staged
// beside it and renamed into place. It returns the directory, and the state
// directories of earlier checkouts recorded at the same location with another
// identity when it created this one: a directory now at the location is not
// the one whose state they hold, and they are kept for a person to relink.
func EnsureCheckoutDir(root string) (dir string, earlier []Checkout, err error) {
	path := NormalizePath(root)
	id, err := identity(path)
	if err != nil {
		return "", nil, err
	}
	dir = CheckoutDir(root)
	if _, err := os.Stat(dir); err == nil {
		// A state directory with no record (one a CI cache restored into the
		// directory `kapi state path` named) is this checkout's by its key
		// alone, and gets its record now.
		if _, rerr := ReadRecord(dir); rerr != nil {
			if werr := writeRecord(dir, CheckoutRecord{Path: path, Identity: id, Created: time.Now().UTC().Format(time.RFC3339)}); werr != nil {
				return "", nil, werr
			}
			return dir, recordedAt(path, id), nil
		}
		return dir, nil, nil
	}
	parent := CheckoutsDir()
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", nil, err
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return "", nil, err
	}
	staged := filepath.Join(parent, ".creating-"+hex.EncodeToString(suffix))
	if err := os.Mkdir(staged, 0o700); err != nil {
		return "", nil, err
	}
	rec := CheckoutRecord{Path: path, Identity: id, Created: time.Now().UTC().Format(time.RFC3339)}
	if err := writeRecord(staged, rec); err != nil {
		_ = os.RemoveAll(staged)
		return "", nil, err
	}
	earlier = recordedAt(path, id)
	if err := os.Rename(staged, dir); err != nil {
		_ = os.RemoveAll(staged)
		if _, serr := os.Stat(dir); serr == nil {
			return dir, nil, nil
		}
		return "", nil, err
	}
	return dir, earlier, nil
}

// recordedAt lists the state directories recorded for other directories at
// path: those with another identity.
func recordedAt(path, id string) []Checkout {
	var out []Checkout
	for _, c := range Checkouts() {
		if c.Record.Path == path && c.Record.Identity != id {
			out = append(out, c)
		}
	}
	return out
}

// Checkouts lists every checkout's state directory with its record, ordered by
// location.
func Checkouts() []Checkout {
	entries, err := os.ReadDir(CheckoutsDir())
	if err != nil {
		return nil
	}
	var out []Checkout
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] == '.' {
			continue
		}
		dir := filepath.Join(CheckoutsDir(), e.Name())
		rec, err := ReadRecord(dir)
		if err != nil {
			continue
		}
		out = append(out, Checkout{Dir: dir, Record: rec})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Record.Path < out[j].Record.Path })
	return out
}

// ReadRecord reads the record in a checkout's state directory.
func ReadRecord(dir string) (CheckoutRecord, error) {
	data, err := os.ReadFile(filepath.Join(dir, CheckoutRecordFilename))
	if err != nil {
		return CheckoutRecord{}, err
	}
	var rec CheckoutRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return CheckoutRecord{}, err
	}
	return rec, nil
}

// MarkTold records that the one-time notice name was given for the checkout
// whose state is in dir, and reports whether it had been given before.
func MarkTold(dir, name string) (bool, error) {
	var before bool
	err := updateRecord(dir, func(rec *CheckoutRecord) bool {
		if rec.Told[name] {
			before = true
			return false
		}
		if rec.Told == nil {
			rec.Told = map[string]bool{}
		}
		rec.Told[name] = true
		return true
	})
	return before, err
}

// CheckoutExists reports whether the state directory of the checkout at root
// exists.
func CheckoutExists(root string) bool {
	_, err := os.Stat(CheckoutDir(root))
	return err == nil
}

// RecordedElsewhereAt reports whether kapi recorded the state of another
// directory at root's location: one with another identity than root has now.
func RecordedElsewhereAt(root string) bool {
	path := NormalizePath(root)
	id, err := identity(path)
	if err != nil {
		return false
	}
	for _, c := range Checkouts() {
		if c.Record.Path == path && c.Record.Identity != id {
			return true
		}
	}
	return false
}

// CheckoutRootOf returns the root of the checkout whose state directory is
// dir, from the record kapi keeps beside it, and false when dir is not a
// checkout's state directory.
func CheckoutRootOf(dir string) (string, bool) {
	if NormalizePath(filepath.Dir(dir)) != NormalizePath(CheckoutsDir()) {
		return "", false
	}
	rec, err := ReadRecord(dir)
	if err != nil || rec.Path == "" {
		return "", false
	}
	return rec.Path, true
}

// writeRecord writes a checkout's record whole, by rename.
//
// The record is written to a temporary file of its own and renamed over the
// old one, so a reader sees one whole record or the other, and two writers
// never share a temporary file.
func writeRecord(dir string, rec CheckoutRecord) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+CheckoutRecordFilename+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(append(data, '\n'))
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, CheckoutRecordFilename)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// updateRecord reads the record in dir, lets change edit it, and writes it
// back when change reports an edit.
//
// It takes no lock. Two kapi processes on one checkout (a CLI run and Kapi
// Desktop) can each rewrite the record at once, and then the later rename
// keeps one writer's edit and drops the other's. Each record is still whole,
// since every write goes through a temporary file of its own and a rename.
// What can be lost is safe to lose: a lost project reads as a record that
// cannot say which project its state belongs to, so a relink leaves the
// vault and the sync state in place, and a lost notice is shown once more.
// The record's other fields are the same in every writer.
func updateRecord(dir string, change func(*CheckoutRecord) bool) error {
	rec, err := ReadRecord(dir)
	if err != nil {
		return err
	}
	if !change(&rec) {
		return nil
	}
	return writeRecord(dir, rec)
}
