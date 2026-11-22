package project

import (
	"bytes"
	"context"
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
// project root. It reads the file system only; whether git tracks kapi's
// state is CheckStateTracked's question. A state directory that does not
// exist yet passes: kapi creates it, and what it creates there is its own.
func (l Layout) CheckState() error {
	if l.Root == "" {
		return nil
	}
	if _, err := os.Lstat(l.StateDir); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	// A layout kapi places outside the project (the desktop keeps the state of
	// a folder with no recipe in its own data directory) is checked beneath
	// itself: no repository holds it, so git is not asked about it.
	inProject := rootpath.IsBeneath(l.Root, l.StateDir)
	containIn := l.Root
	if !inProject {
		containIn = l.StateDir
	}
	real, err := rootpath.Contain(containIn, l.StateDir)
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
	err = filepath.WalkDir(real, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // removed while the walk ran
			}
			return err
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
	return err
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

// State that git tracks came with the repository, made by someone else, and
// kapi does not read it as its own. EnsureLayout and EnsureLocalDir write the
// rules that keep it out of version control.
//
// It is asked where a context is held: before a command runs, for each MCP
// call, when the desktop opens a project, when a store opens, and by the
// readers of the sync state and the saved filters.

// gitQueryTimeout bounds the git query within the caller's context.
var gitQueryTimeout = 5 * time.Second

// stateUntracked holds, per project root, the stamp of the git index
// (gitIndexStamp) when git last answered that it tracks none of the state.
// git is asked again only once the index changes, so a check costs one stat,
// and staging or untracking a file is seen at the next check.
var stateUntracked sync.Map

// CheckStateTracked reports an error when git tracks any of the state
// entries the layout's state directory holds. A directory in no git
// repository, a state directory kapi keeps outside the project, and a
// machine with no git binary pass. A git query that fails or does not answer
// in time refuses: it never reads as "not tracked". A cancelled ctx returns
// its error.
func CheckStateTracked(ctx context.Context, l Layout) error {
	if l.Root == "" || !rootpath.IsBeneath(l.Root, l.StateDir) {
		return nil
	}
	entries, err := rootpath.ReadDir(l.Root, l.StateDir)
	if err != nil {
		return nil // no state directory: nothing kapi keeps is there yet
	}
	var present []string
	for _, e := range entries {
		if slices.Contains(StateEntries, e.Name()) {
			present = append(present, e.Name())
		}
	}
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
	tracked, err := gitTracked(ctx, l.Root, paths)
	if err != nil {
		return err
	}
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

// gitQueryArgs is the fixed argument list of the query, ahead of the paths.
// The query runs inside a checkout kapi does not trust, so the repository's
// own configuration must not change what it means or run anything: a
// repository-supplied core.fsmonitor command runs on `git ls-files` otherwise,
// and core.untrackedCache would have the index rewritten. Pathspecs are
// literal, so a state path is never read as pathspec magic. ls-files pages
// nothing and runs no hook, and a builtin command cannot be aliased.
var gitQueryArgs = []string{
	"-c", "core.fsmonitor=false",
	"-c", "core.untrackedCache=false",
	"--literal-pathspecs",
	"ls-files", "-z", "--",
}

// errGitUnanswered is the refusal when git could not say whether it tracks
// kapi's state.
var errGitUnanswered = errors.New("project: kapi could not ask git whether it tracks kapi's state")

// gitTracked returns which of paths (slash-separated, relative to root) git
// lists a tracked file under. It runs git in root under ctx, bounded by
// gitQueryTimeout. A machine with no git binary returns none; a failed or
// unanswered query returns an error, and a cancelled ctx its own error.
func gitTracked(ctx context.Context, root string, paths []string) ([]string, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, nil
	}
	qctx, cancel := context.WithTimeout(ctx, gitQueryTimeout)
	defer cancel()
	args := append([]string{"-C", root}, gitQueryArgs...)
	args = append(args, paths...)
	cmd := exec.CommandContext(qctx, git, args...)
	cmd.Env = gitQueryEnv()
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		if qctx.Err() != nil {
			return nil, fmt.Errorf("%w: no answer within %s. Run again", errGitUnanswered, gitQueryTimeout)
		}
		return nil, fmt.Errorf("%w: %w. Run again", errGitUnanswered, err)
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
	return tracked, nil
}

// gitQueryEnv is the environment the query runs in: the process's own, less
// every GIT_ variable but GIT_EXEC_PATH, which would point the query at
// another repository, index or work tree or add configuration, with optional
// locks and prompts off.
func gitQueryEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") && !strings.HasPrefix(kv, "GIT_EXEC_PATH=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
}
