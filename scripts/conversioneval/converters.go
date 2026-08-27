package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// The converters, each reduced to one job: turn a document into text.
//
// They are not the same kind of tool and the report says so. pandoc and
// LibreOffice convert between document formats and text is one target among
// many; textutil is a macOS text-extraction utility; `kapi kcat` prints the
// content kapi read into its model, which is a step in a round trip rather than
// a product in itself. What they have in common is that each claims to see the
// text in the file, and that claim is comparable.

// Converter is one tool under test.
type Converter struct {
	// ID is the column name.
	ID string `json:"id"`
	// Version is recorded per run, because a comparison against an unnamed
	// version is not reproducible.
	Version string `json:"version"`
	// Command is what runs, for a reader who wants to check a row by hand.
	Command string `json:"command"`
	// Exts is what this converter is asked to handle. textutil reads Word
	// documents and not spreadsheets, and asking it to would score a
	// capability it does not claim.
	Exts []string `json:"exts"`
	// Note says what kind of tool this is, so a table of four numbers is not
	// read as four attempts at the same thing.
	Note string `json:"note"`

	run func(ctx context.Context, file, workdir string) (string, error)
}

// available returns the converters present on this machine, with versions.
//
// A converter that is not installed is left out of the report rather than
// scored zero: absent and failing are different, and a zero row would read as
// the second.
func available(ctx context.Context, kapiBin string) []Converter {
	all := []Converter{
		{
			ID:      "kapi",
			Command: "kapi kcat <file>",
			Exts:    []string{".docx", ".pptx", ".xlsx"},
			Note:    "prints the content kapi read into its model; the read half of a round trip",
			Version: firstLine(output(ctx, kapiBin, "version")),
			run: func(ctx context.Context, file, _ string) (string, error) {
				return capture(ctx, kapiBin, []string{"kcat", file}, kapiIsolation())
			},
		},
		{
			ID:      "pandoc",
			Command: "pandoc -t plain <file>",
			Exts:    []string{".docx", ".pptx"},
			Note:    "a document converter; text is one of many targets",
			Version: firstWord(firstLine(output(ctx, "pandoc", "--version")), 1),
			run: func(ctx context.Context, file, _ string) (string, error) {
				return capture(ctx, "pandoc", []string{"-t", "plain", file}, nil)
			},
		},
		{
			ID:      "libreoffice",
			Command: "soffice --headless --convert-to <filter> --outdir <dir> <file>",
			// Not .pptx. `--convert-to txt` has no Impress target at all: the
			// conversion produces no file and exits 0. Asking anyway and
			// counting the empty result would have scored a capability
			// LibreOffice does not claim as eight failures — which is what the
			// first version of this did, and it made LibreOffice look broken
			// on two thirds of the corpus.
			Exts:    []string{".docx", ".xlsx"},
			Note:    "a full office suite rendering to text; its txt targets are Writer and Calc, and Impress has none",
			Version: firstWord(firstLine(output(ctx, "soffice", "--version")), 1),
			run:     runLibreOffice,
		},
		{
			ID:      "textutil",
			Command: "textutil -convert txt -stdout <file>",
			Exts:    []string{".docx"},
			Note:    "the macOS text-extraction utility; Word documents only",
			Version: "macos",
			run: func(ctx context.Context, file, _ string) (string, error) {
				return capture(ctx, "textutil", []string{"-convert", "txt", "-stdout", file}, nil)
			},
		},
		{
			ID:      "markitdown",
			Command: "markitdown <file>",
			Exts:    []string{".docx", ".pptx", ".xlsx"},
			Note:    "a text-extraction tool aimed at feeding documents to models",
			Version: firstLine(output(ctx, "markitdown", "--version")),
			run: func(ctx context.Context, file, _ string) (string, error) {
				return capture(ctx, "markitdown", []string{file}, nil)
			},
		},
	}

	var out []Converter
	for _, c := range all {
		bin := strings.Fields(c.Command)[0]
		if c.ID == "kapi" {
			if kapiBin == "" {
				continue
			}
		} else if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		if c.Version == "" {
			c.Version = "unknown"
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// handles reports whether this converter is asked to read this extension.
func (c Converter) handles(ext string) bool {
	for _, e := range c.Exts {
		if e == ext {
			return true
		}
	}
	return false
}

// loFilters is the conversion target per source format.
//
// One filter does not serve all three. `txt:Text` is Writer's; Calc needs its
// own and produces a .csv; Impress has no text target, which is why .pptx is
// not in the converter's extension list.
var loFilters = map[string]struct{ filter, ext string }{
	".docx": {"txt:Text", ".txt"},
	".xlsx": {"csv:Text - txt - csv (StarCalc)", ".csv"},
}

// runLibreOffice converts into a temporary directory and reads the result back.
//
// It writes a file rather than to stdout, and it keeps a user profile which it
// will happily take from the developer's home directory — so each run is given
// its own, both to keep the measurement clean and to avoid touching a profile
// that belongs to someone.
func runLibreOffice(ctx context.Context, file, workdir string) (string, error) {
	f, ok := loFilters[strings.ToLower(filepath.Ext(file))]
	if !ok {
		return "", fmt.Errorf("no LibreOffice text filter for %s", filepath.Ext(file))
	}
	outDir, err := os.MkdirTemp(workdir, "lo-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	profile := filepath.Join(outDir, "profile")
	args := []string{
		"-env:UserInstallation=file://" + profile,
		"--headless", "--convert-to", f.filter, "--outdir", outDir, file,
	}
	// soffice exits 0 whether or not it wrote anything, so the exit code is not
	// the signal; the file is.
	if _, err := capture(ctx, "soffice", args, nil); err != nil {
		return "", err
	}
	want := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)) + f.ext
	b, err := os.ReadFile(filepath.Join(outDir, want))
	if err != nil {
		return "", fmt.Errorf("converted nothing: %w", err)
	}
	return string(b), nil
}

// capture runs a command and returns stdout, with stderr folded into the error.
func capture(ctx context.Context, bin string, args, env []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("timed out")
		}
		return "", fmt.Errorf("%w: %s", err, clip(strings.TrimSpace(stderr.String()), 200))
	}
	return stdout.String(), nil
}

// kapiIsolation keeps an in-repo kapi off the dogfood project and off the
// developer's config, per the contract in CLAUDE.md.
func kapiIsolation() []string {
	dir, _ := os.MkdirTemp("", "conveval-kapi-")
	return []string{
		"KAPI_NO_PROJECT=1",
		"KAPI_HOME=" + filepath.Join(dir, "home"),
		"KAPI_CONFIG_DIR=" + filepath.Join(dir, "config"),
		"XDG_DATA_HOME=" + filepath.Join(dir, "data"),
		"XDG_CACHE_HOME=" + filepath.Join(dir, "cache"),
		"KAPI_PLUGINS_DIR_ONLY=1",
		"KAPI_PLUGINS_DIR=" + filepath.Join(dir, "plugins"),
	}
}

func output(ctx context.Context, bin string, args ...string) string {
	s, err := capture(ctx, bin, args, nil)
	if err != nil {
		return ""
	}
	return s
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}

// firstWord returns field n of a line, for version strings shaped
// "pandoc 3.10.1" and "LibreOffice 26.2.5.2 <hash>".
func firstWord(line string, n int) string {
	f := strings.Fields(line)
	if n < len(f) {
		return f[n]
	}
	return line
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
