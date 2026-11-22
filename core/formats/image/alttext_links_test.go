//go:build unix

package image

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/vision"
)

// captionOf reads the image at in and returns the caption block's source
// text, or "" when the reader emitted none.
func captionOf(t *testing.T, in string, src []byte) string {
	t.Helper()
	for _, p := range readParts(t, in, src) {
		if b, ok := p.Resource.(*model.Block); ok && b.SemanticRole() == model.RoleCaption {
			return b.SourceText()
		}
	}
	return ""
}

// The alt-text sidecar is read only when it is a regular file inside the
// image's directory. A link that leads out of it, or a FIFO, is ignored as a
// missing sidecar is; a link that stays inside is read.
func TestAltSidecarStaysBesideTheImage(t *testing.T) {
	vision.ResetForTest()
	defer vision.ResetForTest()

	base := t.TempDir()
	dir := filepath.Join(base, "images")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{dir, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("outside the directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := makePNG(t, 8, 8)

	cases := map[string]struct {
		place func(sidecar string) error
		want  string
	}{
		"a link out of the directory": {
			place: func(sidecar string) error { return os.Symlink(secret, sidecar) },
			want:  "",
		},
		"a FIFO": {
			place: func(sidecar string) error { return syscall.Mkfifo(sidecar, 0o600) },
			want:  "",
		},
		"a link that stays inside": {
			place: func(sidecar string) error {
				if err := os.WriteFile(filepath.Join(dir, "shared.txt"), []byte("A shared caption\n"), 0o644); err != nil {
					return err
				}
				return os.Symlink("shared.txt", sidecar)
			},
			want: "A shared caption",
		},
		"a regular file": {
			place: func(sidecar string) error { return os.WriteFile(sidecar, []byte("A blue boat\n"), 0o644) },
			want:  "A blue boat",
		},
	}
	n := 0
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			n++
			in := filepath.Join(dir, "img"+string(rune('a'+n))+".png")
			if err := os.WriteFile(in, src, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := tc.place(in + ".alt.txt"); err != nil {
				t.Fatal(err)
			}
			if got := captionOf(t, in, src); got != tc.want {
				t.Fatalf("caption = %q, want %q", got, tc.want)
			}
		})
	}
}
