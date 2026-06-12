package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeRepoRoot creates a temp dir that looks like the repo root (it
// contains go.work) and chdirs into it so the walk-up helpers resolve
// against the fake layout instead of the real repository. It returns
// the root exactly as os.Getwd reports it — the walk-up helpers start
// from Getwd, and on macOS the temp dir sits behind the /var →
// /private/var symlink, so neither the raw t.TempDir path nor its
// EvalSymlinks resolution is guaranteed to match.
func makeRepoRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.work"), []byte("go 1.26.0\n"), 0o644))
	t.Chdir(root)
	wd, err := os.Getwd()
	require.NoError(t, err)
	return wd
}

func TestFindCorpusRoot(t *testing.T) {
	tests := []struct {
		name string
		// dirs are created under <root>/corpus/; files too (as plain
		// files, to prove non-dirs never count as version dirs).
		dirs  []string
		files []string
		// noCorpusDir skips creating corpus/ entirely.
		noCorpusDir bool
		wantSubdir  string // expected root, relative to <root>/corpus/
		wantErr     string // substring required in the error
	}{
		{
			name:       "single version dir",
			dirs:       []string{"format-corpus-v1"},
			wantSubdir: "format-corpus-v1",
		},
		{
			name:       "picks lexically latest version dir",
			dirs:       []string{"format-corpus-v1", "format-corpus-v2"},
			wantSubdir: "format-corpus-v2",
		},
		{
			// The contract is lexical max (format-ops.md §7 /
			// format-maturity.md §2.5), matching the okapi-testdata
			// idiom — so v9 outranks v10.
			name:       "lexical order is the documented contract",
			dirs:       []string{"format-corpus-v10", "format-corpus-v9"},
			wantSubdir: "format-corpus-v9",
		},
		{
			name:       "ignores dirs without the tag prefix",
			dirs:       []string{"format-corpus-v1", "scratch", "zzz-not-a-corpus"},
			wantSubdir: "format-corpus-v1",
		},
		{
			name:    "plain files are not version dirs",
			files:   []string{"format-corpus-v3"},
			wantErr: "make fetch-corpus",
		},
		{
			name:        "missing corpus dir names the fetch command",
			noCorpusDir: true,
			wantErr:     "make fetch-corpus",
		},
		{
			name:    "no matching version dirs names the fetch command",
			dirs:    []string{"random"},
			wantErr: "make fetch-corpus",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := makeRepoRoot(t)
			base := filepath.Join(root, "corpus")
			if !tt.noCorpusDir {
				require.NoError(t, os.MkdirAll(base, 0o755))
			}
			for _, d := range tt.dirs {
				require.NoError(t, os.MkdirAll(filepath.Join(base, d), 0o755))
			}
			for _, f := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(base, f), []byte("x"), 0o644))
			}

			got, err := FindCorpusRoot()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr,
					"the error must name the fetch command so the runner's skip message points at it")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(base, tt.wantSubdir), got)
		})
	}
}

func TestFindCorpusRootNoRepoRoot(t *testing.T) {
	// A temp dir with no go.work anywhere up the chain: the walk must
	// stop at the filesystem root with a clear error.
	t.Chdir(t.TempDir())
	_, err := FindCorpusRoot()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not find repo root (go.work)")
}

func TestResolveFilePathSchemes(t *testing.T) {
	tests := []struct {
		name string
		// layout maps dir paths (relative to the fake repo root) to
		// create before resolving.
		layout  []string
		rel     string
		want    string // expected path, relative to the fake repo root; ignored when wantAbs/wantErr set
		wantAbs string // expected absolute path (for the abs-path case)
		wantErr string // substring required in the error
	}{
		{
			name:   "corpus scheme resolves under the latest corpus version dir",
			layout: []string{"corpus/format-corpus-v1/po", "corpus/format-corpus-v2/po"},
			rel:    "corpus:po/wild/sample.po",
			want:   "corpus/format-corpus-v2/po/wild/sample.po",
		},
		{
			name:    "corpus scheme absent corpus skips with the fetch command",
			rel:     "corpus:po/wild/sample.po",
			wantErr: "make fetch-corpus",
		},
		{
			name:   "okapi scheme resolves under the latest testdata version dir",
			layout: []string{"okapi-testdata/1.48.0-v4"},
			rel:    "okapi:okapi/filters/po/sample.po",
			want:   "okapi-testdata/1.48.0-v4/okapi/filters/po/sample.po",
		},
		{
			name:    "okapi scheme absent testdata names its fetch script",
			rel:     "okapi:okapi/filters/po/sample.po",
			wantErr: "scripts/fetch-okapi-testdata.sh",
		},
		{
			name:    "absolute path is returned as-is",
			rel:     filepath.Join(string(filepath.Separator), "tmp", "fixture.xml"),
			wantAbs: filepath.Join(string(filepath.Separator), "tmp", "fixture.xml"),
		},
		{
			name: "plain relative path joins the spec dir",
			rel:  "testdata/sample.xml",
			want: "specdir/testdata/sample.xml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := makeRepoRoot(t)
			for _, d := range tt.layout {
				require.NoError(t, os.MkdirAll(filepath.Join(root, d), 0o755))
			}
			s := &Spec{dir: filepath.Join(root, "specdir")}

			got, err := ResolveFilePath(s, tt.rel)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			want := tt.wantAbs
			if want == "" {
				want = filepath.Join(root, filepath.FromSlash(tt.want))
			}
			assert.Equal(t, want, got)
		})
	}
}
