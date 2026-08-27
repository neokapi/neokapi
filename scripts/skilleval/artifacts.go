package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Publishing what the agent actually produced.
//
// A completion scenario's deliverable is usually a file: a translated .docx, a
// rewritten .pptx, a catalog. The dataset records the text of one it can show
// and, for everything else, a byte count. The completion sweep alone produces
// 39 such files and about 4MB of content nobody outside the machine that ran it
// can look at; across all three surfaces at three repeats it is 163 files and
// roughly 50MB.
//
// A byte count is the weakest form of the claim. "The gate went green" is
// stronger, and "here is the document, open it" is stronger still, because it
// is the only one a reader can check against their own idea of what faithful
// write-back means. The same file from the control arm sits beside it, which is
// the comparison the whole eval is built on.
//
// They do not go in git: megabytes a sweep, rewritten every run. They go
// where the walkthrough videos go, an S3 bucket behind CloudFront, staged under
// web/static so a local run serves them same-origin exactly as videos do when
// the CDN is off. See web/docs/contribute/implementation/repo/cdn-assets.md.

// ArtifactDir is where a sweep stages what it will publish, relative to the
// repo root. Gitignored, mirrored to the CDN by `make publish-cdn-eval-artifacts`.
const ArtifactDir = "web/static/skill-eval/artifacts"

// maxArtifactBytes is the largest single file a sweep will publish.
//
// The corpus fixtures are documents rather than datasets, so nothing legitimate
// comes near this. A scenario that produces something enormous has usually gone
// wrong, and uploading it every sweep would be paying for the accident.
const maxArtifactBytes = 25 << 20

// stageArtifacts writes every unpublishable change to the staging tree and
// records where it landed.
//
// "Unpublishable" is exactly `Binary`, which diffWorkspace sets for a file that
// holds a NUL byte or exceeds maxShownBytes: the set whose content the dataset
// carries as a number.
func stageArtifacts(root string, r *Report) (int, int64, error) {
	dir := filepath.Join(root, ArtifactDir)
	key := slug(r.Key())
	// A re-run of this surface replaces its own tree, for the same reason
	// writeSessions prunes per key: what another surface published is still
	// current and this run cannot reproduce it.
	if err := os.RemoveAll(filepath.Join(dir, key)); err != nil {
		return 0, 0, err
	}

	var n int
	var bytes int64
	for i := range r.Results {
		res := &r.Results[i]
		for _, arm := range []struct {
			name string
			runs []Run
		}{{"kapi", res.Runs}, {"control", res.Unaided}} {
			for j := range arm.runs {
				run := &arm.runs[j]
				for k := range run.Changed {
					c := &run.Changed[k]
					if !c.Binary || len(c.content) == 0 {
						continue
					}
					if len(c.content) > maxArtifactBytes {
						c.Skipped = fmt.Sprintf("%d bytes, larger than the %d-byte publishing limit",
							len(c.content), maxArtifactBytes)
						continue
					}
					rel := filepath.Join(key, slug(res.Scenario.ID),
						fmt.Sprintf("%s-%d", arm.name, j+1), safeRelPath(c.Path))
					dst := filepath.Join(dir, rel)
					if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
						return n, bytes, err
					}
					if err := os.WriteFile(dst, c.content, 0o644); err != nil {
						return n, bytes, err
					}
					c.Artifact = filepath.ToSlash(rel)
					n++
					bytes += int64(len(c.content))
				}
			}
		}
	}
	return n, bytes, nil
}

// safeRelPath keeps a workspace-relative path readable while making sure it
// cannot climb out of the staging tree.
//
// The path comes from a directory an agent had write access to, so it is
// untrusted input to a file write. Each segment is cleaned and any traversal
// dropped rather than rejected, because a scenario whose one odd filename fails
// a whole sweep is worse than one whose odd filename lands somewhere flat.
func safeRelPath(p string) string {
	var out []string
	for seg := range strings.SplitSeq(filepath.ToSlash(p), "/") {
		switch seg {
		case "", ".", "..":
			continue
		}
		out = append(out, slugPath(seg))
	}
	if len(out) == 0 {
		return "file"
	}
	return filepath.Join(out...)
}

// slugPath keeps a filename recognisable (its extension decides what a browser
// does with it) while allowing only characters that are safe in an S3 key and a
// URL.
func slugPath(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
