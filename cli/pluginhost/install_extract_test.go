package pluginhost

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tarEntry is one record to write into a synthetic plugin tarball.
type tarEntry struct {
	name     string
	typeflag byte
	linkname string // for symlinks
	body     string // for regular files
	mode     int64
}

// makeTarGz builds a gzip-compressed tar archive from entries.
func makeTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Linkname: e.linkname,
			Mode:     mode,
		}
		if e.typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		require.NoError(t, tw.WriteHeader(hdr))
		if e.typeflag == tar.TypeReg {
			_, err := tw.Write([]byte(e.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// TestExtractTarGzSecurity exercises the symlink/path containment and
// O_NOFOLLOW hardening (#17/#69). The boundary is the per-plugin dir
// (<target>/<plugin>), not the shared install root.
func TestExtractTarGzSecurity(t *testing.T) {
	const plugin = "myplugin"

	tests := []struct {
		name    string
		entries []tarEntry
		// preTarget runs before extraction to plant on-disk state
		// (e.g. a sibling dir or a pre-existing symlink). Receives the
		// install root (target).
		preTarget func(t *testing.T, target string)
		wantErr   bool
		errSubstr string
		// verify runs on success to assert the extracted tree.
		verify func(t *testing.T, target string)
	}{
		{
			name: "normal tarball extracts",
			entries: []tarEntry{
				{name: plugin + "/", typeflag: tar.TypeDir},
				{name: plugin + "/manifest.json", typeflag: tar.TypeReg, body: `{"plugin":"myplugin"}`},
				{name: plugin + "/bin/", typeflag: tar.TypeDir},
				{name: plugin + "/bin/run", typeflag: tar.TypeReg, body: "binary", mode: 0o755},
			},
			verify: func(t *testing.T, target string) {
				b, err := os.ReadFile(filepath.Join(target, plugin, "manifest.json"))
				require.NoError(t, err)
				assert.Equal(t, `{"plugin":"myplugin"}`, string(b))
				b, err = os.ReadFile(filepath.Join(target, plugin, "bin", "run"))
				require.NoError(t, err)
				assert.Equal(t, "binary", string(b))
			},
		},
		{
			name: "relative symlink within plugin dir allowed",
			entries: []tarEntry{
				{name: plugin + "/", typeflag: tar.TypeDir},
				{name: plugin + "/real.txt", typeflag: tar.TypeReg, body: "hello"},
				{name: plugin + "/link.txt", typeflag: tar.TypeSymlink, linkname: "real.txt"},
			},
			verify: func(t *testing.T, target string) {
				dst, err := os.Readlink(filepath.Join(target, plugin, "link.txt"))
				require.NoError(t, err)
				assert.Equal(t, "real.txt", dst)
			},
		},
		{
			name: "symlink escaping via ../ rejected",
			entries: []tarEntry{
				{name: plugin + "/", typeflag: tar.TypeDir},
				{name: plugin + "/evil", typeflag: tar.TypeSymlink, linkname: "../../../../etc/passwd"},
			},
			wantErr:   true,
			errSubstr: "outside plugin dir",
		},
		{
			name: "sibling-dir target (plugins-evil) rejected",
			entries: []tarEntry{
				{name: plugin + "/", typeflag: tar.TypeDir},
				// From <root>/myplugin, ../../myplugin-evil/x resolves
				// to a sibling of the install root — the old
				// HasPrefix(abs, root) check would have admitted a
				// path that merely starts with the root string.
				{name: plugin + "/evil", typeflag: tar.TypeSymlink, linkname: "../myplugin-evil/secret"},
			},
			wantErr:   true,
			errSubstr: "outside plugin dir",
		},
		{
			name: "cross-plugin ../otherplugin target rejected",
			entries: []tarEntry{
				{name: plugin + "/", typeflag: tar.TypeDir},
				{name: plugin + "/evil", typeflag: tar.TypeSymlink, linkname: "../otherplugin/data"},
			},
			wantErr:   true,
			errSubstr: "outside plugin dir",
		},
		{
			name: "regular file written through pre-planted symlink rejected",
			entries: []tarEntry{
				{name: plugin + "/", typeflag: tar.TypeDir},
				{name: plugin + "/conf/", typeflag: tar.TypeDir},
				{name: plugin + "/conf/target.txt", typeflag: tar.TypeReg, body: "overwrite"},
			},
			preTarget: func(t *testing.T, target string) {
				// Plant a symlink at the exact path a later regular
				// file entry will write to. O_NOFOLLOW must refuse to
				// follow it.
				confDir := filepath.Join(target, plugin, "conf")
				require.NoError(t, os.MkdirAll(confDir, 0o755))
				outside := filepath.Join(target, "outside.txt")
				require.NoError(t, os.WriteFile(outside, []byte("original"), 0o644))
				require.NoError(t, os.Symlink(outside, filepath.Join(confDir, "target.txt")))
			},
			wantErr: true,
			verify:  nil,
		},
		{
			name: "entry outside plugin dir rejected",
			entries: []tarEntry{
				{name: "otherplugin/file.txt", typeflag: tar.TypeReg, body: "x"},
			},
			wantErr:   true,
			errSubstr: "outside plugin dir",
		},
		{
			name: "absolute entry name rejected",
			entries: []tarEntry{
				// archive/tar refuses absolute names, so build the
				// traversal via a name that filepath.Clean keeps
				// escaping the target root.
				{name: "../" + plugin + "-escape/x.txt", typeflag: tar.TypeReg, body: "x"},
			},
			wantErr:   true,
			errSubstr: "escapes target dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := t.TempDir()
			if tt.preTarget != nil {
				tt.preTarget(t, target)
			}
			body := makeTarGz(t, tt.entries)
			err := extractTarGz(body, target, plugin)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr)
				}
				// On a rejected write-through, the original symlink
				// target must remain untouched.
				if outside := filepath.Join(target, "outside.txt"); fileExists(outside) {
					b, rerr := os.ReadFile(outside)
					require.NoError(t, rerr)
					assert.Equal(t, "original", string(b), "pre-planted symlink target must not be overwritten")
				}
				return
			}
			require.NoError(t, err)
			if tt.verify != nil {
				tt.verify(t, target)
			}
		})
	}
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
