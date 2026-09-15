package golang

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/stretchr/testify/require"
)

// blockCorpus is what rewriting the delimited comments of a tree with their own
// prose came to.
type blockCorpus struct {
	files, comments, rewrote int
	// multiline counts the rewritten comments that run over several lines.
	multiline int
	// refused counts the comments refused, by reason, and failures lists every
	// comment whose rewrite changed the file or was refused for a reason a
	// comment written in the language cannot have.
	refused  map[comment.RefusalReason]int
	failures []string
}

// goFilesUnder lists the Go files under root, skipping worktrees, dependencies
// and vendored trees.
func goFilesUnder(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".claude", "node_modules", "vendor", ".git":
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".go") {
			paths = append(paths, p)
		}
		return nil
	})
	require.NoError(t, err)
	return paths
}

// goroot is the source tree of the Go toolchain running the test.
func goroot(t *testing.T) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "go", "env", "GOROOT").Output()
	require.NoError(t, err)
	root := filepath.Join(strings.TrimSpace(string(out)), "src")
	info, err := os.Stat(root)
	require.NoError(t, err, "the Go toolchain's source tree is the corpus of delimited comments")
	require.True(t, info.IsDir())
	return root
}

// rewriteBlocks rewrites every delimited comment in paths with its own prose.
// Each file is located once, and each of its delimited comments is rendered
// and held to Contain, the two steps Rewrite takes after it locates the file.
// A rewrite must leave the file's bytes as they are. It may be refused only as
// RefusedLayout, for a comment made of several comments, or as RefusedText,
// for a comment whose own text holds a character a rewrite never writes.
func rewriteBlocks(paths []string) blockCorpus {
	results := make([]blockCorpus, len(paths))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for i, path := range paths {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			results[i] = rewriteBlocksIn(path)
		}()
	}
	wg.Wait()
	total := blockCorpus{refused: map[comment.RefusalReason]int{}}
	for _, r := range results {
		total.files += r.files
		total.comments += r.comments
		total.rewrote += r.rewrote
		total.multiline += r.multiline
		for reason, n := range r.refused {
			total.refused[reason] += n
		}
		total.failures = append(total.failures, r.failures...)
	}
	return total
}

func rewriteBlocksIn(path string) blockCorpus {
	out := blockCorpus{refused: map[comment.RefusalReason]int{}}
	src, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(src, []byte("/*")) {
		return out
	}
	located, err := Provider{}.Locate(path, src)
	if err != nil {
		return out // a file that does not parse holds no located comment
	}
	out.files = 1
	ids := located.Blocks()
	for i, c := range located.Comments {
		if c.Style != comment.StyleBlock {
			continue
		}
		out.comments++
		where := path + ":" + strconv.Itoa(c.Lines.First) + " " + ids[i].ID
		prose, err := Provider{}.Prose(src, c)
		if err == nil {
			var span []byte
			span, err = Provider{}.Render(path, src, c, prose, comment.RenderOptions{})
			if err == nil {
				after := bytes.Join([][]byte{src[:c.Start], span, src[c.End:]}, nil)
				var r *comment.Rewritten
				r, err = comment.Contain(Provider{}, path, src, after, nil, located, i)
				if err == nil && (r.Changed || !bytes.Equal(r.Source, src)) {
					out.failures = append(out.failures, where+": rewriting the comment with its own prose changed the file")
					continue
				}
			}
		}
		refusal, refused := comment.AsRefusal(err)
		switch {
		case err == nil:
			out.rewrote++
			if c.Lines.Last > c.Lines.First {
				out.multiline++
			}
		case refused && refusal.Reason == comment.RefusedLayout && strings.Contains(refusal.Detail, "several comments"):
			out.refused[refusal.Reason]++
		case refused && refusal.Reason == comment.RefusedText:
			out.refused[refusal.Reason]++
		default:
			out.failures = append(out.failures, where+": "+err.Error())
		}
	}
	return out
}
