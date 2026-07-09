// Package content_test guards the canonical content-model wire schema
// (core/proto/content/v1, AD-034): exactly one proto definition of the
// Block/Run content model may exist in the repo. Any other proto that needs
// run-level content imports the canonical package; anything else must be an
// explicitly-labeled projection listed in the allowlist below.
package content_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// canonicalProto is the single allowed home of content-model message
// definitions, relative to the repo root.
const canonicalProto = "core/proto/content/v1/content.proto"

// projectionAllowlist maps proto files (relative to the repo root) to the
// Block/Run-named messages they may define. Every entry must be an
// explicitly-labeled projection or envelope, never a second content-model
// contract.
var projectionAllowlist = map[string][]string{
	// Sync-protocol envelope: item scoping + hashes + *_json escapes around
	// canonical SegmentMessage/RunMessage payloads (embeds the canonical
	// schema; see sync.proto header). SyncSegmentList is a repeated-field
	// wrapper around canonical SegmentMessages (proto3 maps cannot have
	// repeated-message values directly), not a segment redefinition.
	"bowrain/core/proto/sync/v1/sync.proto": {"SyncBlock", "SyncSegmentList"},

	// Store-service projection: a deliberately lossy flat block (source as
	// plain string, targets as locale→string) for block storage/streaming.
	"bowrain/proto/v1/neokapi_service.proto": {"BlockMessage"},
}

// messageDefRe matches a proto message definition whose name redefines the
// content model: names ending in Run, Block, or Segment, bare or with an
// s/List/Message suffix (Runs, RunList, BlockMessage, SegmentList, …).
// RPC envelopes like SegmentRequest/SegmentResponse do not match.
var messageDefRe = regexp.MustCompile(`^\s*message\s+(\w*(?:Run|Block|Segment)(?:s|List|Message)?)\s*\{`)

// skipDirs are directory names never scanned for proto files.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "bin": true, "dist": true,
	"build": true, ".kapi": true, ".kapi-iso": true, "coverage": true,
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller must resolve the test file path")
	// file = <root>/core/proto/content/guard_test.go
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file))))
	require.FileExists(t, filepath.Join(root, "go.work"), "repo root detection")
	return root
}

func TestCanonicalContentSchemaIsUnique(t *testing.T) {
	root := repoRoot(t)
	require.FileExists(t, filepath.Join(root, canonicalProto),
		"the canonical content-model proto must exist")

	var violations []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".proto") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == canonicalProto {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		allowed := projectionAllowlist[rel]
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			m := messageDefRe.FindStringSubmatch(sc.Text())
			if m == nil {
				continue
			}
			if slices.Contains(allowed, m[1]) {
				continue
			}
			violations = append(violations, rel+": message "+m[1])
		}
		return sc.Err()
	})
	require.NoError(t, err)

	require.Empty(t, violations,
		"content-model messages (Block/Run definitions) must live only in %s;\n"+
			"import the canonical schema instead, or — for a deliberately lossy,\n"+
			"purpose-built shape — label it a projection and add it to the\n"+
			"allowlist in %s", canonicalProto, "core/proto/content/guard_test.go")
}

// TestProjectionAllowlistIsCurrent fails when an allowlisted message
// disappears, so the allowlist shrinks in step with the code (e.g. when the
// editor proto is retired by plan 07).
func TestProjectionAllowlistIsCurrent(t *testing.T) {
	root := repoRoot(t)
	for rel, msgs := range projectionAllowlist {
		data, err := os.ReadFile(filepath.Join(root, rel))
		require.NoError(t, err, "allowlisted proto %s must exist (remove stale entries)", rel)
		for _, msg := range msgs {
			require.Contains(t, string(data), "message "+msg,
				"%s no longer defines %s — remove it from the allowlist", rel, msg)
		}
	}
}
