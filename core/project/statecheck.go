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
// kapi does not read it as its own. It is asked where a context is held:
// before a command runs, for each MCP call, on each store hand-out, when the
// desktop opens a project, and by the readers of the sync state and filters.

// Asking git sees only what git tracks. State that arrived untracked (in an
// archive, or in a checkout copied or synchronised from elsewhere) is not seen
// here: the walk in CheckState still refuses a link or an irregular file in
// it, and a regular store or claim token copied in is used as kapi's own.

// TrustStateEnv is the environment variable a person sets in kapi's own
// environment to vouch for the state of the checkout kapi runs in: with it
// set to 1, CheckStateTracked asks git nothing and refuses nothing. It is for
// an image with no git, or a checkout git will not read. It is read from the
// process environment and nowhere else: no recipe key, MCP argument, plugin
// request or repository file sets it.
const TrustStateEnv = "KAPI_TRUST_STATE"

// gitQueryTimeout bounds the git query within the caller's context.
var gitQueryTimeout = 5 * time.Second

// stateUntracked holds, per project root, the stamp of the git index
// (gitIndexStamp) when git last answered that it tracks none of the state.
// git is asked again only once the index changes, so a check costs one stat,
// and staging or untracking a file is seen at the next check.
var stateUntracked sync.Map

// errGitUnanswered wraps every refusal made because git gave no answer.
var errGitUnanswered = errors.New("kapi could not ask git whether it tracks kapi's state")

// CheckStateTracked reports an error when git tracks anything beneath an
// entry of StateEntries in the layout's state directory, spelled in any way
// the file system resolves to that entry, or when the state directory or one
// of its entries is a git repository of its own or a submodule. A directory
// in no git repository and a state directory kapi keeps outside the project
// pass without running git, and so does a process run with TrustStateEnv set
// to 1. Inside a repository, a git that gives no answer refuses, naming the
// cause and its remedy; it never reads as "not tracked". A cancelled ctx
// returns its error.
func CheckStateTracked(ctx context.Context, l Layout) error {
	if l.Root == "" || !rootpath.IsBeneath(l.Root, l.StateDir) {
		return nil
	}
	entries, err := rootpath.ReadDir(l.Root, l.StateDir)
	if err != nil {
		return nil // no state directory: nothing kapi keeps is there yet
	}
	if os.Getenv(TrustStateEnv) == "1" {
		return nil
	}
	gitDir, ok := findGitDir(l.Root)
	if !ok {
		return nil
	}
	rel := stateRel(l)
	// A repository of its own inside the state directory is listed by none of
	// the parent's queries, so it is refused whatever it holds.
	for _, e := range append([]os.DirEntry{nil}, entries...) {
		dir := l.StateDir
		if e != nil {
			if !e.IsDir() {
				continue
			}
			dir = filepath.Join(l.StateDir, e.Name())
		}
		if _, err := rootpath.Stat(l.Root, filepath.Join(dir, ".git")); err == nil {
			name := rel
			if e != nil {
				name += "/" + e.Name()
			}
			return fmt.Errorf("project: %s is a git repository of its own, so kapi does not read it as its own state. "+
				"Remove it, or move it out of %s, and run again", name, rel)
		}
	}
	stamp, stamped := gitIndexStamp(gitDir)
	if stamped {
		if seen, ok := stateUntracked.Load(l.Root); ok && seen == stamp {
			return nil
		}
	}
	listing, err := gitListing(ctx, l.Root)
	if err != nil {
		return err
	}
	tracked := trackedState(l, listing)
	if len(tracked) == 0 {
		if stamped {
			stateUntracked.Store(l.Root, stamp)
		}
		return nil
	}
	lines := make([]string, len(tracked))
	for i, p := range tracked {
		name := p[strings.LastIndex(p, "/")+1:]
		if !strings.EqualFold(name, LocalFiltersFilename) {
			name += "/"
		}
		lines[i] = name
	}
	return fmt.Errorf("project: git tracks %s, which kapi keeps for one checkout, so kapi does not read it as its own. "+
		"If it is yours, run `git rm -r --cached %s`, add %s to %s/%s, and commit; if it came with someone else's repository, delete it as well. Then run again",
		strings.Join(tracked, ", "), strings.Join(tracked, " "), strings.Join(lines, " and "), rel, StateGitignoreFilename)
}

// stateRel is the state directory relative to the project root, with
// forward slashes.
func stateRel(l Layout) string {
	rel, err := filepath.Rel(l.Root, l.StateDir)
	if err != nil {
		return StateDirName
	}
	return filepath.ToSlash(rel)
}

// trackedState returns, for each path git lists, the state path it is
// beneath, spelled as git lists it, sorted with no repeats. A listed path is
// compared with the state directory and its entries by the file each names on
// this file system (os.SameFile), not by its spelling, so a case-insensitive
// or normalising volume that resolves `.KAPI/WORK` to `.kapi/work` is judged
// as it resolves. A listed path that is the state directory itself (a
// submodule there) counts as tracking all of it.
func trackedState(l Layout, listing []string) []string {
	stateInfo, err := rootpath.Stat(l.Root, l.StateDir)
	if err != nil {
		return nil
	}
	var entryInfos []fs.FileInfo
	for _, name := range StateEntries {
		if info, err := rootpath.Stat(l.Root, filepath.Join(l.StateDir, name)); err == nil {
			entryInfos = append(entryInfos, info)
		}
	}
	depth := len(strings.Split(stateRel(l), "/"))
	stats := map[string]fs.FileInfo{}
	same := func(prefix string, want fs.FileInfo) bool {
		info, seen := stats[prefix]
		if !seen {
			// Through the root: a listed name that leads out of the project
			// names no state of this one.
			info, _ = rootpath.Stat(l.Root, filepath.Join(l.Root, filepath.FromSlash(prefix)))
			stats[prefix] = info
		}
		return info != nil && os.SameFile(info, want)
	}
	var tracked []string
	add := func(p string) {
		if !slices.Contains(tracked, p) {
			tracked = append(tracked, p)
		}
	}
	for _, p := range listing {
		parts := strings.Split(p, "/")
		if len(parts) < depth {
			continue
		}
		dirPrefix := strings.Join(parts[:depth], "/")
		if !same(dirPrefix, stateInfo) {
			continue
		}
		if len(parts) == depth {
			add(dirPrefix)
			continue
		}
		entryPrefix := strings.Join(parts[:depth+1], "/")
		for _, want := range entryInfos {
			if same(entryPrefix, want) {
				add(entryPrefix)
			}
		}
	}
	slices.Sort(tracked)
	return tracked
}

// findGitDir finds the git repository holding root, walking up from it, and
// returns its git directory. It reports false when root is in no repository.
func findGitDir(root string) (string, bool) {
	for dir := root; ; dir = filepath.Dir(dir) {
		dotgit := filepath.Join(dir, ".git")
		if info, err := os.Stat(dotgit); err == nil {
			if info.IsDir() {
				return dotgit, true
			}
			// A worktree or submodule: .git is a file naming the directory.
			b, err := os.ReadFile(dotgit)
			if err != nil {
				return dotgit, true
			}
			after, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
			if !ok {
				return dotgit, true
			}
			if !filepath.IsAbs(after) {
				after = filepath.Join(dir, after)
			}
			return after, true
		}
		if filepath.Dir(dir) == dir {
			return "", false
		}
	}
}

// gitIndexStamp returns the size and modification time of the repository's
// index, to tell whether git must be asked again. It reports false when there
// is no index yet, and then git is asked every time.
func gitIndexStamp(gitDir string) (string, bool) {
	idx, err := os.Stat(filepath.Join(gitDir, "index"))
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%s %d %d", gitDir, idx.Size(), idx.ModTime().UnixNano()), true
}

// gitQueryArgs is the fixed argument list of the query. The query runs inside
// a checkout kapi does not trust, so the repository's own configuration must
// not change what it means or run anything. Without these overrides a
// repository-supplied core.fsmonitor command runs on `git ls-files`, and
// core.untrackedCache has the index rewritten. ls-files pages
// nothing and runs no hook, and a builtin command cannot be aliased. It lists
// every path the index holds beneath the project root, in any spelling, and
// trackedState decides which are kapi's state.
var gitQueryArgs = []string{
	"-c", "core.fsmonitor=false",
	"-c", "core.untrackedCache=false",
	"ls-files", "-z",
}

// gitListing returns every path the index lists beneath root, relative to
// root, by running git in root under ctx, bounded by gitQueryTimeout. A git
// that cannot answer returns an error naming the cause and its remedy; a
// cancelled ctx returns its own error.
func gitListing(ctx context.Context, root string) ([]string, error) {
	vouch := fmt.Sprintf("or set %s=1 in kapi's environment to vouch for the state in this checkout", TrustStateEnv)
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("project: %w in %s: git is not installed. Install git, %s", errGitUnanswered, root, vouch)
	}
	qctx, cancel := context.WithTimeout(ctx, gitQueryTimeout)
	defer cancel()
	args := append([]string{"-C", root}, gitQueryArgs...)
	cmd := exec.CommandContext(qctx, git, args...)
	cmd.Env = gitQueryEnv()
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		if qctx.Err() != nil {
			return nil, fmt.Errorf("project: %w in %s: git gave no answer within %s. Run again once the repository's disk answers, %s",
				errGitUnanswered, root, gitQueryTimeout, vouch)
		}
		msg := strings.TrimSpace(stderr.String())
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		if strings.Contains(stderr.String(), "dubious ownership") {
			return nil, fmt.Errorf("project: %w in %s: git refuses to read a repository another user owns. "+
				"If you trust it, run `git config --global --add safe.directory %s`, %s", errGitUnanswered, root, root, vouch)
		}
		return nil, fmt.Errorf("project: %w in %s: git failed (%s: %w). Repair the repository so git reads it, %s",
			errGitUnanswered, root, msg, err, vouch)
	}
	var listing []string
	for f := range strings.SplitSeq(out.String(), "\x00") {
		if f != "" {
			listing = append(listing, f)
		}
	}
	return listing, nil
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
