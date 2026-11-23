package project

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/dataroot"
)

// A person brings a checkout's state from somewhere kapi does not read it into
// the checkout's state directory with `kapi state import` (state an earlier
// kapi left inside the checkout) or `kapi state relink` (the state directory of
// the checkout's earlier location). Both show a StateTransfer first and then
// take exactly what it shows: the same files, with the same content.

// StateFile is one file a transfer takes: where it is now, where it goes in
// the checkout's state directory, and its size and content digest when shown.
type StateFile struct {
	// Source is the file's path relative to the transfer's From directory,
	// with forward slashes.
	Source string `json:"source"`
	// Target is the file's path relative to the checkout's state directory,
	// with forward slashes.
	Target string `json:"target"`
	Size   int64  `json:"size"`
	Digest string `json:"-"`
}

// StateTransfer is what a transfer will take: the directory it takes from, the
// files, and the servers the sync state among them was issued by.
type StateTransfer struct {
	From    string      `json:"from"`
	Files   []StateFile `json:"files"`
	Servers []string    `json:"servers,omitempty"`
	// Retire is the state directory a relink takes from, removed once its
	// files are in place; empty for an import.
	Retire string `json:"-"`
}

// importSources maps where an earlier kapi kept state inside a checkout
// (relative to `.kapi/`) to where it goes in the checkout's state directory.
var importSources = []struct{ from, to string }{
	{VaultDirName, VaultDirName},
	{filepath.Join(WorkDirName, VaultDirName), VaultDirName},
	{filepath.Join(WorkDirName, CacheDirName, RedactionDirName), filepath.Join(VaultDirName, VaultBatchesDirName)},
	{filepath.Join(CacheDirName, RedactionDirName), filepath.Join(VaultDirName, VaultBatchesDirName)},
	{SyncDirName, SyncDirName},
	{filepath.Join(WorkDirName, CacheDirName, "refs.json"), filepath.Join(SyncDirName, "refs.json")},
	{filepath.Join(WorkDirName, CacheDirName, "sync-cache.json"), filepath.Join(SyncDirName, "sync-cache.json")},
	{LocalFiltersFilename, LocalFiltersFilename},
}

// PlanImport lists the state an earlier kapi left in the layout's checkout
// that `kapi state import` would take. A link or an irregular file among it is
// refused.
func PlanImport(l Layout) (StateTransfer, error) {
	out := StateTransfer{From: l.StateDir}
	root, err := os.OpenRoot(l.StateDir)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	defer root.Close()
	for _, m := range importSources {
		files, err := listFiles(root, m.from)
		if err != nil {
			return out, err
		}
		for _, f := range files {
			rel, _ := filepath.Rel(m.from, f.path)
			target := m.to
			if rel != "." {
				target = filepath.Join(m.to, rel)
			}
			out.Files = append(out.Files, StateFile{Source: filepath.ToSlash(f.path), Target: filepath.ToSlash(target), Size: f.size, Digest: f.digest})
		}
	}
	out.Servers = syncServers(root, out.Files)
	return out, nil
}

// PlanRelink lists the state of the checkout kapi recorded at from that
// `kapi state relink` would move into the layout's checkout: the vault, the
// sync state and the saved filters. The work directory is rebuilt instead. The record must
// name exactly one other state directory at that location.
func PlanRelink(l Layout, from string) (StateTransfer, error) {
	path := dataroot.NormalizePath(from)
	own := l.LocalStateDir()
	var found []dataroot.Checkout
	for _, c := range dataroot.Checkouts() {
		if c.Record.Path == path && filepath.Clean(c.Dir) != filepath.Clean(own) {
			found = append(found, c)
		}
	}
	switch len(found) {
	case 0:
		return StateTransfer{}, fmt.Errorf("project: kapi recorded no state for a checkout at %s", path)
	case 1:
	default:
		return StateTransfer{}, fmt.Errorf("project: kapi recorded the state of %d checkouts at %s; remove the ones you do not want from %s and run again", len(found), path, dataroot.CheckoutsDir())
	}
	src := found[0].Dir
	out := StateTransfer{From: src, Retire: src}
	root, err := os.OpenRoot(src)
	if err != nil {
		return out, err
	}
	defer root.Close()
	files, err := listFiles(root, ".")
	if err != nil {
		return out, err
	}
	for _, f := range files {
		// The work directory is rebuilt where the checkout is now; what
		// cannot be rebuilt is what comes across.
		if f.path == dataroot.CheckoutRecordFilename || strings.HasPrefix(filepath.ToSlash(f.path), WorkDirName+"/") {
			continue
		}
		p := filepath.ToSlash(f.path)
		out.Files = append(out.Files, StateFile{Source: p, Target: p, Size: f.size, Digest: f.digest})
	}
	out.Servers = syncServers(root, out.Files)
	return out, nil
}

// ApplyTransfer takes the files a transfer showed into the layout's state
// directory. The transfer is planned again first, and anything that differs
// from what was shown (a file added, removed or changed) refuses the whole
// transfer. A file whose target already holds different content is refused
// too: nothing kapi keeps is overwritten. removeSource removes each source
// file once every file is in place; a relink always removes the state
// directory it took from.
func ApplyTransfer(l Layout, shown StateTransfer, removeSource bool) error {
	var now StateTransfer
	var err error
	if shown.Retire != "" {
		rec, rerr := dataroot.ReadRecord(shown.Retire)
		if rerr != nil {
			return rerr
		}
		now, err = PlanRelink(l, rec.Path)
	} else {
		now, err = PlanImport(l)
	}
	if err != nil {
		return err
	}
	if !sameTransfer(now, shown) {
		return errors.New("project: the state changed after it was shown, so nothing was taken; run the command again to see it as it is now")
	}
	if len(now.Files) == 0 {
		return nil
	}
	if err := EnsureLayout(l); err != nil {
		return err
	}
	src, err := os.OpenRoot(now.From)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenRoot(l.LocalStateDir())
	if err != nil {
		return err
	}
	defer dst.Close()
	var conflicts []string
	for _, f := range now.Files {
		if existing, err := dst.ReadFile(filepath.FromSlash(f.Target)); err == nil {
			if digestOf(existing) != f.Digest {
				conflicts = append(conflicts, f.Target)
			}
		}
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("project: this checkout already keeps %s, with other content; move it aside and run again", strings.Join(conflicts, ", "))
	}
	for _, f := range now.Files {
		data, err := src.ReadFile(filepath.FromSlash(f.Source))
		if err != nil {
			return err
		}
		if digestOf(data) != f.Digest {
			return fmt.Errorf("project: %s changed while it was taken; run the command again", f.Source)
		}
		if err := writeStateFileIn(dst, filepath.FromSlash(f.Target), data); err != nil {
			return err
		}
	}
	switch {
	case now.Retire != "":
		// The state directory a relink took from sits in the data root's
		// checkouts directory; it is removed through a root on that directory.
		checkouts, err := os.OpenRoot(filepath.Dir(now.Retire))
		if err != nil {
			return err
		}
		defer checkouts.Close()
		return checkouts.RemoveAll(filepath.Base(now.Retire))
	case removeSource:
		for _, f := range now.Files {
			if err := src.Remove(filepath.FromSlash(f.Source)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func sameTransfer(a, b StateTransfer) bool {
	if a.From != b.From || len(a.Files) != len(b.Files) {
		return false
	}
	for i := range a.Files {
		if a.Files[i] != b.Files[i] {
			return false
		}
	}
	return true
}

// listedFile is one regular file under a transfer's source.
type listedFile struct {
	path   string
	size   int64
	digest string
}

// listFiles lists the regular files at or under rel in root, refusing a link or
// an irregular file. A missing rel lists nothing.
func listFiles(root *os.Root, rel string) ([]listedFile, error) {
	info, err := root.Lstat(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []listedFile
	add := func(p string) error {
		data, err := root.ReadFile(p)
		if err != nil {
			return err
		}
		out = append(out, listedFile{path: p, size: int64(len(data)), digest: digestOf(data)})
		return nil
	}
	switch {
	case info.Mode().IsRegular():
		return out, add(rel)
	case info.IsDir():
		err := fs.WalkDir(root.FS(), filepath.ToSlash(rel), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			switch {
			case d.IsDir():
				return nil
			case d.Type().IsRegular():
				return add(filepath.FromSlash(p))
			default:
				return fmt.Errorf("project: %s is a link or not a regular file, so kapi does not take it", p)
			}
		})
		sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
		return out, err
	default:
		return nil, fmt.Errorf("project: %s is a link or not a regular file, so kapi does not take it", rel)
	}
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// syncServers reads the server each sync file among files names, for the
// person deciding whether to take it.
func syncServers(root *os.Root, files []StateFile) []string {
	var out []string
	for _, f := range files {
		if !strings.HasPrefix(f.Target, SyncDirName+"/") || !strings.HasSuffix(f.Target, ".json") {
			continue
		}
		data, err := root.ReadFile(filepath.FromSlash(f.Source))
		if err != nil {
			continue
		}
		var doc struct {
			ServerURL string `json:"server_url"`
		}
		if json.Unmarshal(data, &doc) != nil || doc.ServerURL == "" {
			continue
		}
		if s := displaySafe(doc.ServerURL); !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// writeStateFileIn writes data whole to rel beneath root, creating its parent
// directories owner-only, through a temporary file and a rename.
func writeStateFileIn(root *os.Root, rel string, data []byte) error {
	if parent := filepath.Dir(rel); parent != "." {
		if err := root.MkdirAll(parent, 0o700); err != nil {
			return err
		}
	}
	tmp := rel + ".taking"
	if err := root.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := root.Rename(tmp, rel); err != nil {
		_ = root.Remove(tmp)
		return err
	}
	return nil
}

// displaySafe drops control and invisible format characters from s, which came
// from a file kapi did not write, and caps its length.
func displaySafe(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || r == 0x200e || r == 0x200f || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			continue
		}
		if n == 200 {
			b.WriteString("...")
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}
