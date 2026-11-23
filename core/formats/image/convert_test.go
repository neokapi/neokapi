package image

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"

	"github.com/neokapi/neokapi/core/model"
)

// solid returns a w×h image with a single fill color.
func solid(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{120, 160, 200, 255})
		}
	}
	return img
}

func writeFile(t *testing.T, path string, encode func(*os.File) error) string {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := encode(f); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// fileSource opens path as the reader would receive it: a descriptor the
// caller opened, read through its random-access view.
func fileSource(t *testing.T, path string) *imageSource {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	src, err := openImageSource(&model.RawDocument{Reader: f, ReaderAt: f, Size: info.Size()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(src.close)
	return src
}

// requireSameBytes checks that the file at got holds the bytes of the file at want.
func requireSameBytes(t *testing.T, got, want string) {
	t.Helper()
	g, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	w, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(g, w) {
		t.Errorf("%s does not hold the bytes of %s", got, want)
	}
}

func makeGIFFile(t *testing.T, dir string) string {
	return writeFile(t, filepath.Join(dir, "x.gif"), func(f *os.File) error {
		return gif.Encode(f, solid(48, 24), nil)
	})
}

func makeBMPFile(t *testing.T, dir string) string {
	return writeFile(t, filepath.Join(dir, "x.bmp"), func(f *os.File) error {
		return bmp.Encode(f, solid(48, 24))
	})
}

func makeTIFFFile(t *testing.T, dir string) string {
	return writeFile(t, filepath.Join(dir, "x.tiff"), func(f *os.File) error {
		return tiff.Encode(f, solid(48, 24), nil)
	})
}

// assertPNG checks that path holds a decodable PNG of the expected dimensions.
func assertPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open transcoded: %v", err)
	}
	defer func() { _ = f.Close() }()
	cfg, fmtName, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatalf("decode transcoded: %v", err)
	}
	if fmtName != "png" {
		t.Errorf("transcoded format = %q, want png", fmtName)
	}
	if cfg.Width != w || cfg.Height != h {
		t.Errorf("transcoded dims = %dx%d, want %dx%d", cfg.Width, cfg.Height, w, h)
	}
}

func TestNormalizeForOCR_PassThrough(t *testing.T) {
	dir := t.TempDir()
	for _, mk := range []struct {
		name string
		make func() string
	}{
		{"png", func() string {
			return writeFile(t, filepath.Join(dir, "p.png"), func(f *os.File) error {
				_, err := f.Write(makePNG(t, 10, 10))
				return err
			})
		}},
		{"jpeg", func() string {
			return writeFile(t, filepath.Join(dir, "p.jpg"), func(f *os.File) error {
				_, err := f.Write(makeJPEG(t, 10, 10))
				return err
			})
		}},
	} {
		t.Run(mk.name, func(t *testing.T) {
			src := mk.make()
			out, cleanup, err := normalizeForOCR(context.Background(), fileSource(t, src))
			defer cleanup()
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			// The engine gets a private copy of the bytes, never the source's name.
			if out == src {
				t.Fatalf("PNG/JPEG should reach the engine as a private copy, got the source %q", src)
			}
			requireSameBytes(t, out, src)
		})
	}
}

func TestNormalizeForOCR_TranscodeGo(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"gif":  makeGIFFile(t, dir),
		"bmp":  makeBMPFile(t, dir),
		"tiff": makeTIFFFile(t, dir),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			out, cleanup, err := normalizeForOCR(context.Background(), fileSource(t, src))
			if err != nil {
				t.Fatalf("normalize %s: %v", name, err)
			}
			if out == src {
				t.Fatalf("%s should be transcoded to a new temp file", name)
			}
			assertPNG(t, out, 48, 24)
			cleanup()
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Errorf("cleanup should remove temp %s, stat err=%v", out, err)
			}
		})
	}
}

func TestNormalizeForOCR_UnknownPassThrough(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, filepath.Join(dir, "x.bin"), func(f *os.File) error {
		_, err := f.Write([]byte("definitely not an image"))
		return err
	})
	out, cleanup, err := normalizeForOCR(context.Background(), fileSource(t, src))
	defer cleanup()
	if err != nil {
		t.Fatalf("unknown should pass through, got err: %v", err)
	}
	if out == src {
		t.Fatalf("unknown should reach the engine as a private copy, got the source %q", src)
	}
	requireSameBytes(t, out, src)
}

// withFakeFFmpeg swaps the ffmpeg seams for the duration of the test. convert
// receives (src, dst) and is expected to produce dst; available controls the
// FFmpegAvailable gate.
func withFakeFFmpeg(t *testing.T, available bool, convert func(ctx context.Context, src, dst string) error) {
	t.Helper()
	origConv, origAvail := ffmpegConvert, ffmpegAvailable
	ffmpegConvert = convert
	ffmpegAvailable = func() bool { return available }
	t.Cleanup(func() { ffmpegConvert, ffmpegAvailable = origConv, origAvail })
}

// The HEIC/AVIF transcode path routes through the ffmpeg seam. We don't shell
// out to a real ffmpeg here (encoding AVIF/HEIC is environment-dependent and
// slow); the real av.ConvertImage wiring is covered in core/av's TestConvertImage.
func TestNormalizeForOCR_ISOBMFF_Routing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x.avif")
	if err := os.WriteFile(src, ftypBox("avif"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotSrc string
	withFakeFFmpeg(t, true, func(_ context.Context, s, dst string) error {
		gotSrc = s
		// ffmpeg reads a private copy holding the source bytes.
		requireSameBytes(t, s, src)
		// Stand in for ffmpeg: write a real PNG to dst.
		return os.WriteFile(dst, makePNG(t, 64, 16), 0o644)
	})

	out, cleanup, err := normalizeForOCR(context.Background(), fileSource(t, src))
	if err != nil {
		t.Fatalf("normalize avif: %v", err)
	}
	defer cleanup()
	if gotSrc == "" || gotSrc == src {
		t.Errorf("ffmpeg seam received src %q, want a private copy of %q", gotSrc, src)
	}
	if out == src {
		t.Fatal("avif should be transcoded to a new temp PNG")
	}
	assertPNG(t, out, 64, 16)
}

// When ffmpeg is unavailable, an ISOBMFF still image yields errUnsupportedOCR so
// the reader degrades to a Media-only asset rather than failing.
func TestNormalizeForOCR_ISOBMFF_NoFFmpeg(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x.heic")
	if err := os.WriteFile(src, ftypBox("heic"), 0o644); err != nil {
		t.Fatal(err)
	}
	withFakeFFmpeg(t, false, func(context.Context, string, string) error {
		t.Fatal("ffmpegConvert must not be called when ffmpeg is unavailable")
		return nil
	})

	out, cleanup, err := normalizeForOCR(context.Background(), fileSource(t, src))
	defer cleanup()
	if !errors.Is(err, errUnsupportedOCR) {
		t.Fatalf("err = %v, want errUnsupportedOCR", err)
	}
	if out != "" {
		t.Errorf("out = %q, want empty when unsupported", out)
	}
}
