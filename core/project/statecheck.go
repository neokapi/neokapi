package project

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/rootpath"
)

// The state directory (.kapi/) holds kapi's own files, and kapi opens them by
// path: SQLite opens its databases by name, and the caches, parts logs,
// drafts, vaults and sync state are read and written where they lie. A
// repository could commit a .kapi/ entry that is a symbolic link, to a
// directory or to a single file such as work/store.db, and every access would
// follow it out of the project. CheckState refuses such a layout before
// anything is opened there, so the by-path access that follows reaches only
// files kapi itself made inside the project.

// CheckState reports an error when the state directory, or anything beneath
// it, is a symbolic link or another kind of file that is neither a directory
// nor a regular file, or when the state directory resolves outside the
// project root. It also refuses kapi's own state when git tracks it
// (checkStateUntracked). A state directory that does not exist yet passes:
// kapi creates it, and what it creates there is its own.
func (l Layout) CheckState() error {
	if l.Root == "" {
		return nil
	}
	if _, err := os.Lstat(l.StateDir); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	real, err := rootpath.Contain(l.Root, l.StateDir)
	if err != nil {
		return stateError(l.Root, l.StateDir, "leads outside the project", err)
	}
	// The state directory may itself be a link that stays inside the project;
	// beneath it, nothing is.
	name := func(p string) string {
		rel, rerr := filepath.Rel(real, p)
		if rerr != nil {
			return l.StateDir
		}
		return filepath.Join(l.StateDir, rel)
	}
	var present []string
	err = filepath.WalkDir(real, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // removed while the walk ran
			}
			return err
		}
		if filepath.Dir(p) == real && slices.Contains(StateEntries, d.Name()) {
			present = append(present, d.Name())
		}
		switch t := d.Type(); {
		case t&fs.ModeSymlink != 0:
			return stateError(l.Root, name(p), "is a symbolic link", nil)
		case t.IsDir(), t.IsRegular():
			return nil
		default:
			return stateError(l.Root, name(p), "is not a regular file", nil)
		}
	})
	if err != nil {
		return err
	}
	return checkStateUntracked(l, present)
}

// stateError names p relative to root in a refusal of the state layout.
func stateError(root, p, what string, cause error) error {
	rel, rerr := filepath.Rel(root, p)
	if rerr != nil {
		rel = filepath.Base(p)
	}
	msg := fmt.Sprintf("project: %s %s, so kapi keeps no state there. Remove it and run again", filepath.ToSlash(rel), what)
	if cause != nil {
		return fmt.Errorf("%s: %w", msg, cause)
	}
	return errors.New(msg)
}

// StateEntries are the entries of the state directory that hold kapi's own
// state for one checkout. A project may commit source there too, such as a
// terms bundle or a voice profile a person authors; these entries are not
// source. The work directory holds the project store, the caches, the drafts
// and the locks. The sync directory holds what a venue keeps: its confirmed
// hashes, its refs and the claim token of an anonymous project. The vault
// holds redacted originals, and the filters file the personal saved filters.
var StateEntries = []string{WorkDirName, SyncDirName, VaultDirName, LocalFiltersFilename}

// State that git tracks arrived, or will arrive, with the repository, made on
// another machine by someone else, and kapi does not read it as its own. A
// checkout that tracks any of StateEntries is refused, naming each and how to
// untrack it. EnsureLayout writes the rule that keeps the work directory out
// of version control, and EnsureLocalDir the rule for the vault and the sync
// state.

const gitQueryTimeout = 5 * time.Second

// stateUntracked holds, per project root, the stamp of the git index
// (gitIndexStamp) when git last answered that it tracks none of the state.
// git is asked again only once the index changes, so a check costs one stat,
// and staging or untracking a file is seen at the next check. A directory in
// no git repository passes without asking.
var stateUntracked sync.Map

func checkStateUntracked(l Layout, present []string) error {
	if len(present) == 0 {
		return nil
	}
	stamp, ok := gitIndexStamp(l.Root)
	if !ok {
		return nil
	}
	if seen, ok := stateUntracked.Load(l.Root); ok && seen == stamp {
		return nil
	}
	rel, err := filepath.Rel(l.Root, l.StateDir)
	if err != nil {
		rel = StateDirName
	}
	rel = filepath.ToSlash(rel)
	paths := make([]string, len(present))
	for i, e := range present {
		paths[i] = rel + "/" + e
	}
	tracked := gitTracked(l.Root, paths)
	if len(tracked) == 0 {
		stateUntracked.Store(l.Root, stamp)
		return nil
	}
	lines := make([]string, len(tracked))
	for i, p := range tracked {
		name := strings.TrimPrefix(p, rel+"/")
		if name != LocalFiltersFilename {
			name += "/"
		}
		lines[i] = name
	}
	return fmt.Errorf("project: git tracks %s, which kapi keeps for one checkout, so kapi does not read it as its own. "+
		"If it is yours, run `git rm -r --cached %s`, add %s to %s/%s, and commit; if it came with someone else's repository, delete it as well. Then run again",
		strings.Join(tracked, ", "), strings.Join(tracked, " "), strings.Join(lines, " and "), rel, StateGitignoreFilename)
}

// gitIndexStamp finds the git repository holding root, walking up from it,
// and returns its index's size and modification time. It reports false when
// root is in no repository, or the repository has no index yet.
func gitIndexStamp(root string) (string, bool) {
	for dir := root; ; dir = filepath.Dir(dir) {
		dotgit := filepath.Join(dir, ".git")
		if info, err := os.Stat(dotgit); err == nil {
			gitDir := dotgit
			if !info.IsDir() {
				// A worktree or submodule: .git is a file naming the directory.
				b, err := os.ReadFile(dotgit)
				if err != nil {
					return "", false
				}
				after, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
				if !ok {
					return "", false
				}
				if !filepath.IsAbs(after) {
					after = filepath.Join(dir, after)
				}
				gitDir = after
			}
			idx, err := os.Stat(filepath.Join(gitDir, "index"))
			if err != nil {
				return "", false
			}
			return fmt.Sprintf("%s %d %d", gitDir, idx.Size(), idx.ModTime().UnixNano()), true
		}
		if filepath.Dir(dir) == dir {
			return "", false
		}
	}
}

// gitTracked returns which of paths (slash-separated, relative to root) git
// lists a tracked file under.
// Any failure (no git, not a repository, no answer within gitQueryTimeout)
// reads as none tracked.
//
// The layout API it runs under (ResolveLayout, LayoutFor, EnsureLayout) takes
// no context, so the query is bounded by its own timer.
func gitTracked(root string, paths []string) []string {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil
	}
	args := append([]string{"-C", root, "ls-files", "-z", "--"}, paths...)
	cmd := exec.Command(git, args...) //nolint:noctx // bounded by gitQueryTimeout below; the layout API carries no context
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		return nil
	}
	timer := time.AfterFunc(gitQueryTimeout, func() { _ = cmd.Process.Kill() })
	err = cmd.Wait()
	timer.Stop()
	if err != nil {
		return nil
	}
	// ls-files run in root names each file relative to root.
	var tracked []string
	for f := range strings.SplitSeq(out.String(), "\x00") {
		for _, p := range paths {
			if (f == p || strings.HasPrefix(f, p+"/")) && !slices.Contains(tracked, p) {
				tracked = append(tracked, p)
			}
		}
	}
	slices.Sort(tracked)
	return tracked
}
