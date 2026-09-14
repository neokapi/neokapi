package host

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/diffscope"
)

// gitDiffArgs begin every git diff a diff-scoped check reads. They pin each
// option user configuration could change about the output: colour, external
// diff drivers, text conversion, path prefixes, renames and submodules.
var gitDiffArgs = []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv",
	"--src-prefix=a/", "--dst-prefix=b/", "-M", "--submodule=short"}

// gitlinkMode is the mode of a submodule's entry, which records a commit of
// another repository rather than a file.
const gitlinkMode = "160000"

// readScopedObject reads the file at path, relative to the top of the work
// tree, from objects. A test replaces it to show that content read from
// anywhere else is refused.
var readScopedObject = func(ctx context.Context, objects *gitObjects, path string) ([]byte, error) {
	return objects.read(ctx, path)
}

// gitDiffStaged diffs the index in dir against HEAD, which is the change a
// commit made now would record, and reads each file from the index. Untracked
// files and unstaged edits are no part of it. An index holding a conflict is
// refused, because it holds no single version of the conflicted files.
func gitDiffStaged(ctx context.Context, dir string) (*diffSource, error) {
	top, err := gitOutput(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("--staged needs a git work tree: %w", err)
	}
	root := strings.TrimSpace(string(top))
	objects, err := indexObjects(ctx, root)
	if err != nil {
		return nil, err
	}
	out, err := gitOutput(ctx, root, slices.Concat(gitDiffArgs, []string{"--cached", "--end-of-options", "--"})...)
	if err != nil {
		return nil, fmt.Errorf("git diff --cached: %w", err)
	}
	files, err := diffscope.Parse(out)
	if err != nil {
		return nil, fmt.Errorf("parse git diff --cached: %w", err)
	}
	return &diffSource{label: "staged", root: root, files: files, objects: objects}, nil
}

// indexObjects lists the blob the index holds at each path. A path with an
// entry at a stage other than 0 is unmerged.
func indexObjects(ctx context.Context, root string) (*gitObjects, error) {
	out, err := gitOutput(ctx, root, "ls-files", "--stage", "-z")
	if err != nil {
		return nil, fmt.Errorf("list the index: %w", err)
	}
	objects := &gitObjects{root: root, where: "in the index", blobs: map[string]string{}}
	var unmerged []string
	for entry := range strings.SplitSeq(string(out), "\x00") {
		// <mode> SP <object> SP <stage> TAB <path>
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			continue
		}
		switch {
		case fields[2] != "0":
			if n := len(unmerged); n == 0 || unmerged[n-1] != path {
				unmerged = append(unmerged, path)
			}
		case fields[0] != gitlinkMode:
			objects.blobs[path] = fields[1]
		}
	}
	if len(unmerged) > 0 {
		return nil, fmt.Errorf("the index holds unmerged paths, and no single version of them to check: %s. Resolve the conflict and stage the result",
			strings.Join(unmerged, ", "))
	}
	return objects, nil
}

// gitObjects reads one version of a work tree's files from git's object store:
// the blob the index or a commit holds at each path. One git cat-file --batch
// process serves every read, and the first read starts it.
type gitObjects struct {
	root string
	// where names the version for a message, such as "in the index".
	where string
	// blobs maps each path to the object id of its blob.
	blobs map[string]string

	batch  *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Reader
	stderr bytes.Buffer
}

// read returns the content of the file at path.
func (o *gitObjects) read(ctx context.Context, path string) ([]byte, error) {
	id, ok := o.blobs[path]
	if !ok {
		return nil, fmt.Errorf("%s is not a file %s", path, o.where)
	}
	if o.batch == nil {
		if err := o.start(ctx); err != nil {
			return nil, err
		}
	}
	if _, err := io.WriteString(o.in, id+"\n"); err != nil {
		return nil, o.failed(err)
	}
	header, err := o.out.ReadString('\n')
	if err != nil {
		return nil, o.failed(err)
	}
	// <object> SP <type> SP <size> LF <content> LF, or <object> SP missing LF
	fields := strings.Fields(header)
	if len(fields) != 3 || fields[1] != "blob" {
		return nil, fmt.Errorf("read %s %s: git cat-file answered %q", path, o.where, strings.TrimSpace(header))
	}
	size, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, fmt.Errorf("read %s %s: git cat-file answered %q", path, o.where, strings.TrimSpace(header))
	}
	content := make([]byte, size+1)
	if _, err := io.ReadFull(o.out, content); err != nil {
		return nil, o.failed(err)
	}
	return content[:size], nil
}

func (o *gitObjects) start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "-C", o.root, "cat-file", "--batch")
	cmd.Stderr = &o.stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start git cat-file: %w", err)
	}
	o.batch, o.in, o.out = cmd, in, bufio.NewReader(out)
	return nil
}

// failed ends the cat-file process after a broken read and says what git
// reported.
func (o *gitObjects) failed(err error) error {
	_ = o.close()
	if msg := strings.TrimSpace(o.stderr.String()); msg != "" {
		return fmt.Errorf("git cat-file: %w: %s", err, msg)
	}
	return fmt.Errorf("git cat-file: %w", err)
}

// close ends the cat-file process, if a read started one.
func (o *gitObjects) close() error {
	if o == nil || o.batch == nil {
		return nil
	}
	_ = o.in.Close()
	err := o.batch.Wait()
	o.batch = nil
	return err
}
