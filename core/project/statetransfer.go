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
	// Project names the project a relink's state was recorded for.
	Project string `json:"project,omitempty"`
	// Unverified reports a relink whose record cannot say the state is this
	// project's: a record kapi wrote before it recorded the project, or a
	// recipe with no id.
	Unverified bool `json:"unverified,omitempty"`
	// Withheld are the vault and sync state files an unverified relink leaves
	// where they are, unless the person states that the state is this
	// project's (RelinkOptions.SameProject).
	Withheld []StateFile `json:"withheld,omitempty"`
	// WithheldWhy says why each group of Withheld stays, for the person.
	WithheldWhy []string `json:"withheld_why,omitempty"`
	// SyncProject is the server project the sync state among the files is
	// bound to, when its sync cache names one.
	SyncProject *SyncBinding `json:"sync_project,omitempty"`
	// RemovesSource reports a relink that removes the state directory it takes
	// from, once every file is in place and read back.
	RemovesSource bool `json:"removes_source,omitempty"`
	// Retire is the state directory a relink takes from; empty for an import.
	Retire string `json:"-"`
	// sameProject is the RelinkOptions the plan was made with, so applying it
	// plans it again the same way.
	sameProject bool
}

// RelinkOptions are what a person states when asking for a relink.
type RelinkOptions struct {
	// SameProject states that the earlier checkout's state belongs to this
	// project when its record cannot say so, so the vault and the sync state
	// come across too. It also brings the sync state of a checkout whose
	// recipe names no server project to compare it with; it never brings
	// sync state bound to another server project than the recipe names.
	SameProject bool
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
	for _, f := range out.Files {
		if f.Target == SyncDirName+"/sync-cache.json" {
			if b, ok := syncBindingAt(root, filepath.FromSlash(f.Source)); ok {
				out.SyncProject = &b
			}
		}
	}
	return out, nil
}

// PlanRelink lists the state of the checkout kapi recorded at from that
// `kapi state relink` would move into the layout's checkout: the vault, the
// sync state and the saved filters. The work directory is rebuilt instead.
// The record must name exactly one other state directory at that location.
//
// The state must be this project's. A record of another project is refused,
// naming both. A record that cannot say (written before kapi recorded the
// project, or a recipe with no id) is unverified: the plan leaves the vault
// and the sync state where they are, and keeps the state directory, unless
// opts.SameProject states that the state is this project's. The vault holds
// the original values of this checkout's withheld text and the sync state
// holds the sign-in for a server project, and neither belongs in another
// project's checkout.
func PlanRelink(l Layout, from string, opts RelinkOptions) (StateTransfer, error) {
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
	out := StateTransfer{From: src, Retire: src, sameProject: opts.SameProject, Project: describeProject(found[0].Record.Project)}
	current, _ := checkoutProjectOf(l)
	verified, err := relinkVerdict(found[0].Record.Project, current)
	if err != nil {
		return StateTransfer{}, err
	}
	out.Unverified = !verified
	root, err := os.OpenRoot(src)
	if err != nil {
		return out, err
	}
	defer root.Close()
	files, err := listFiles(root, ".")
	if err != nil {
		return out, err
	}
	keepSecrets := out.Unverified && !opts.SameProject
	if keepSecrets {
		out.WithheldWhy = append(out.WithheldWhy, "kapi's record cannot say this state belongs to this project, so the vault and the sync state stay; pass --same-project if it does")
	}
	keepSync, why := syncWithheld(l, root, opts)
	if keepSync && !keepSecrets {
		out.WithheldWhy = append(out.WithheldWhy, why)
	}
	if b, ok := recordedSyncBinding(root); ok {
		out.SyncProject = &b
	}
	for _, f := range files {
		// The work directory is rebuilt where the checkout is now; what
		// cannot be rebuilt is what comes across. The record, and a temporary
		// file a stopped write of it left, stay with the directory.
		if f.path == dataroot.CheckoutRecordFilename || dataroot.IsRecordTemp(f.path) || strings.HasPrefix(filepath.ToSlash(f.path), WorkDirName+"/") {
			continue
		}
		p := filepath.ToSlash(f.path)
		file := StateFile{Source: p, Target: p, Size: f.size, Digest: f.digest}
		isVault, isSync := strings.HasPrefix(p, VaultDirName+"/"), strings.HasPrefix(p, SyncDirName+"/")
		if (keepSecrets && (isVault || isSync)) || (keepSync && isSync) {
			out.Withheld = append(out.Withheld, file)
			continue
		}
		out.Files = append(out.Files, file)
	}
	out.Servers = syncServers(root, append(slices.Clone(out.Files), out.Withheld...))
	out.RemovesSource = len(out.Withheld) == 0
	return out, nil
}

// ApplyTransfer takes the files a transfer showed into the layout's state
// directory. The transfer is planned again first, and anything that differs
// from what was shown (a file added, removed or changed) refuses the whole
// transfer. A file whose target already holds different content is refused
// too: nothing kapi keeps is overwritten. Every file is read back from where
// it was written before any source is removed. removeSource removes each
// source file of an import; a relink removes the state directory it took from
// when its plan says so (RemovesSource).
func ApplyTransfer(l Layout, shown StateTransfer, removeSource bool) error {
	var now StateTransfer
	var err error
	if shown.Retire != "" {
		rec, rerr := dataroot.ReadRecord(shown.Retire)
		if rerr != nil {
			return rerr
		}
		now, err = PlanRelink(l, rec.Path, RelinkOptions{SameProject: shown.sameProject})
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
		paths := make([]string, len(conflicts))
		for i, c := range conflicts {
			paths[i] = filepath.Join(l.LocalStateDir(), filepath.FromSlash(c))
		}
		return fmt.Errorf("project: this checkout already keeps %s, with other content, so nothing was taken; move it aside and run the command again", strings.Join(paths, ", "))
	}
	// The files this transfer writes where nothing was before. A transfer that
	// stops part way removes them again, so the checkout's state is as it was
	// and running the command again starts over.
	var wrote []string
	undo := func(what string) error {
		var left []string
		for _, rel := range wrote {
			if err := dst.Remove(filepath.FromSlash(rel)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				left = append(left, filepath.Join(l.LocalStateDir(), filepath.FromSlash(rel)))
			}
		}
		if len(left) > 0 {
			return fmt.Errorf("project: %s, so nothing was taken and %s is kept; delete %s and run the command again", what, now.From, strings.Join(left, ", "))
		}
		return fmt.Errorf("project: %s, so nothing was taken: %s is kept and the copies already written were removed; run the command again", what, now.From)
	}
	for _, f := range now.Files {
		data, err := src.ReadFile(filepath.FromSlash(f.Source))
		if err != nil {
			return undo(fmt.Sprintf("%s could not be read (%v)", f.Source, err))
		}
		if digestOf(data) != f.Digest {
			return undo(f.Source + " changed while it was taken")
		}
		if _, err := dst.Lstat(filepath.FromSlash(f.Target)); errors.Is(err, fs.ErrNotExist) {
			wrote = append(wrote, f.Target)
		}
		if err := writeStateFileIn(dst, filepath.FromSlash(f.Target), data); err != nil {
			return undo(fmt.Sprintf("%s could not be written (%v)", f.Target, err))
		}
	}
	for _, f := range now.Files {
		data, err := dst.ReadFile(filepath.FromSlash(f.Target))
		if err != nil || digestOf(data) != f.Digest {
			return undo(filepath.Join(l.LocalStateDir(), filepath.FromSlash(f.Target)) + " did not read back as it was written")
		}
	}
	switch {
	case now.Retire != "" && now.RemovesSource:
		// The state directory a relink took from sits in the data root's
		// checkouts directory; it is removed through a root on that directory.
		checkouts, err := os.OpenRoot(filepath.Dir(now.Retire))
		if err != nil {
			return err
		}
		defer checkouts.Close()
		return checkouts.RemoveAll(filepath.Base(now.Retire))
	case removeSource:
		var kept []string
		for _, f := range now.Files {
			if err := src.Remove(filepath.FromSlash(f.Source)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				kept = append(kept, f.Source)
			}
		}
		if len(kept) > 0 {
			return &SourcesKeptError{Files: kept}
		}
	}
	return nil
}

// SourcesKeptError reports an import that took every file and could not
// remove some of the copies it was asked to remove. The state is in place;
// Files are the copies still in the checkout.
type SourcesKeptError struct {
	Files []string
}

func (e *SourcesKeptError) Error() string {
	return "project: the state was taken, and these copies in the checkout could not be removed: " + strings.Join(e.Files, ", ")
}

func sameTransfer(a, b StateTransfer) bool {
	return a.From == b.From && a.RemovesSource == b.RemovesSource &&
		slices.Equal(a.Files, b.Files) && slices.Equal(a.Withheld, b.Withheld)
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
