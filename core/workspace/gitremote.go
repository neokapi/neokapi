package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// DefaultContextRef is the ref a git remote keeps the context on: outside
// every branch, so no checkout, merge or pull request ever sees it.
const DefaultContextRef = "refs/kapi/context"

// gitPushAttempts bounds how often a push that lost a race to another
// machine's push is rebuilt on the new tip and tried again.
const gitPushAttempts = 8

// GitRemote keeps a project's shared context on a ref of the project's own
// repository.
//
// Each Put is one commit on the ref adding files, pushed to the repository's
// remote without force. Two machines pushing at once never write one file, so
// the push that loses the race fetches the new tip, lays its files over it and
// pushes again. The ref is fetched explicitly (+refs/kapi/*:refs/kapi/*), since
// a clone fetches branches and tags only. Access is the repository's own: what
// can push a branch can push the ref.
type GitRemote struct {
	repo   string
	remote string
	ref    string

	mu      sync.Mutex
	fetched bool
}

// NewGitRemote names a ref of the repository checked out at repo, shared
// through the git remote named remote ("origin" when empty).
func NewGitRemote(repo, remote, ref string) *GitRemote {
	if remote == "" {
		remote = "origin"
	}
	if ref == "" {
		ref = DefaultContextRef
	}
	return &GitRemote{repo: repo, remote: remote, ref: ref}
}

// Describe reports the repository remote and the ref.
func (r *GitRemote) Describe() RemoteDescriptor {
	return RemoteDescriptor{Kind: "git", Location: r.remote + " " + r.ref}
}

// List reads the names under a directory at the ref's tip, fetching the ref
// once per remote handle.
func (r *GitRemote) List(ctx context.Context, dir string) ([]string, error) {
	if err := checkListDir(dir); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// A sync starts by listing the log, so that listing reads the current tip;
	// the blobs and checkpoints it lists next are read at the same tip.
	fetch := r.fetchOnce
	if dir == RemoteLogDir {
		fetch = r.fetch
	}
	if err := fetch(ctx); err != nil {
		return nil, err
	}
	tip, err := r.tip(ctx)
	if err != nil || tip == "" {
		return nil, err
	}
	out, err := r.git(ctx, nil, "ls-tree", "-r", "--name-only", tip, "--", strings.TrimSuffix(dir, "/"))
	if err != nil {
		return nil, err
	}
	var names []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if ValidObjectName(line) {
			names = append(names, line)
		}
	}
	return names, nil
}

// Get reads one object at the ref's tip.
func (r *GitRemote) Get(ctx context.Context, name string) ([]byte, error) {
	if err := checkObjectName(name); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.fetchOnce(ctx); err != nil {
		return nil, err
	}
	tip, err := r.tip(ctx)
	if err != nil {
		return nil, err
	}
	if tip == "" {
		return nil, fmt.Errorf("%w: %s", ErrNoObject, name)
	}
	data, ok, err := r.blobAt(ctx, tip, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoObject, name)
	}
	return data, nil
}

// Put commits the objects the ref does not hold yet and pushes the commit.
func (r *GitRemote) Put(ctx context.Context, objs ...Object) error {
	for _, obj := range objs {
		if err := checkObjectName(obj.Name); err != nil {
			return err
		}
	}
	if len(objs) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var last error
	for range gitPushAttempts {
		if err := r.fetch(ctx); err != nil {
			return err
		}
		pushed, err := r.commitAndPush(ctx, objs)
		if err != nil || pushed {
			return err
		}
		last = errors.New("another machine pushed first")
	}
	return fmt.Errorf("workspace: push to %s %s: %w after %d attempts; run the push again", r.remote, r.ref, last, gitPushAttempts)
}

// commitAndPush lays the objects over the fetched tip as one commit and
// pushes it. pushed is false when the push lost a race and should be retried.
func (r *GitRemote) commitAndPush(ctx context.Context, objs []Object) (pushed bool, err error) {
	tip, err := r.tip(ctx)
	if err != nil {
		return false, err
	}
	index, err := os.CreateTemp("", "kapi-context-index-*")
	if err != nil {
		return false, err
	}
	indexPath := index.Name()
	_ = index.Close()
	_ = os.Remove(indexPath)
	defer func() { _ = os.Remove(indexPath) }()
	env := []string{"GIT_INDEX_FILE=" + indexPath}
	if tip != "" {
		if _, err := r.git(ctx, env, "read-tree", tip); err != nil {
			return false, err
		}
	}
	added := 0
	for _, obj := range objs {
		if tip != "" {
			held, ok, err := r.blobAt(ctx, tip, obj.Name)
			if err != nil {
				return false, err
			}
			if ok {
				if !bytes.Equal(held, obj.Data) {
					return false, fmt.Errorf("%w: %s", ErrObjectExists, obj.Name)
				}
				continue
			}
		}
		sha, err := r.gitIn(ctx, env, obj.Data, "hash-object", "-w", "--stdin")
		if err != nil {
			return false, err
		}
		if _, err := r.git(ctx, env, "update-index", "--add", "--cacheinfo",
			"100644,"+strings.TrimSpace(string(sha))+","+obj.Name); err != nil {
			return false, err
		}
		added++
	}
	if added == 0 {
		return true, nil
	}
	tree, err := r.git(ctx, env, "write-tree")
	if err != nil {
		return false, err
	}
	args := []string{"commit-tree", strings.TrimSpace(string(tree)), "-m", fmt.Sprintf("kapi context: add %d files", added)}
	if tip != "" {
		args = append(args, "-p", tip)
	}
	commit, err := r.git(ctx, r.identity(ctx), args...)
	if err != nil {
		return false, err
	}
	sha := strings.TrimSpace(string(commit))
	out, err := r.gitCombined(ctx, "push", "--quiet", "--no-verify", r.remote, sha+":"+r.ref)
	if err != nil {
		text := string(out)
		if strings.Contains(text, "rejected") || strings.Contains(text, "non-fast-forward") ||
			strings.Contains(text, "fetch first") || strings.Contains(text, "stale info") ||
			strings.Contains(text, "cannot lock ref") {
			return false, nil
		}
		return false, fmt.Errorf("%w: git push %s: %s", ErrRemoteUnreachable, r.remote, strings.TrimSpace(text))
	}
	if _, err := r.git(ctx, nil, "update-ref", r.ref, sha); err != nil {
		return false, err
	}
	return true, nil
}

// Close holds nothing open.
func (r *GitRemote) Close() error { return nil }

func (r *GitRemote) fetchOnce(ctx context.Context) error {
	if r.fetched {
		return nil
	}
	return r.fetch(ctx)
}

// fetch brings the ref's tip from the repository remote. A remote that has no
// such ref yet is a context nobody has pushed, and leaves the local ref alone.
func (r *GitRemote) fetch(ctx context.Context) error {
	out, err := r.gitCombined(ctx, "fetch", "--quiet", "--no-tags", r.remote, "+"+r.ref+":"+r.ref)
	if err != nil {
		text := string(out)
		if strings.Contains(text, "couldn't find remote ref") || strings.Contains(text, "could not find remote ref") {
			r.fetched = true
			return nil
		}
		return fmt.Errorf("%w: git fetch %s %s: %s", ErrRemoteUnreachable, r.remote, r.ref, strings.TrimSpace(text))
	}
	r.fetched = true
	return nil
}

// tip resolves the local copy of the ref, empty when there is none.
func (r *GitRemote) tip(ctx context.Context) (string, error) {
	out, err := r.git(ctx, nil, "rev-parse", "--verify", "--quiet", r.ref+"^{commit}")
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// blobAt reads a file at a commit.
func (r *GitRemote) blobAt(ctx context.Context, commit, name string) ([]byte, bool, error) {
	if _, err := r.git(ctx, nil, "cat-file", "-e", commit+":"+name); err != nil {
		return nil, false, nil
	}
	data, err := r.git(ctx, nil, "cat-file", "blob", commit+":"+name)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// identity supplies a committer where the repository configures none, which
// is what a CI runner meets.
func (r *GitRemote) identity(ctx context.Context) []string {
	if out, err := r.git(ctx, nil, "config", "user.email"); err == nil && strings.TrimSpace(string(out)) != "" {
		return nil
	}
	return []string{
		"GIT_AUTHOR_NAME=kapi", "GIT_AUTHOR_EMAIL=kapi@localhost",
		"GIT_COMMITTER_NAME=kapi", "GIT_COMMITTER_EMAIL=kapi@localhost",
	}
}

func (r *GitRemote) git(ctx context.Context, env []string, args ...string) ([]byte, error) {
	return r.gitIn(ctx, env, nil, args...)
}

func (r *GitRemote) gitIn(ctx context.Context, env []string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.repo
	cmd.Env = append(os.Environ(), env...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			exit.Stderr = stderr.Bytes()
			return out, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}

func (r *GitRemote) gitCombined(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.repo
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}

// GitRepoRoot returns the top of the git work tree dir sits in, and false when
// it is in none.
func GitRepoRoot(dir string) (string, bool) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return filepath.Clean(strings.TrimSpace(string(out))), true
}
