package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// The workspace is a throwaway directory holding the profiles and the corpus,
// and every kapi process launched into it is isolated per the contract in
// CLAUDE.md. Without that, a run started anywhere inside this checkout walks up
// to the repo's own kapi.yaml and measures the dogfood project.

// isolationEnv is the environment every kapi invocation in this eval carries.
func isolationEnv(workdir string) []string {
	home := filepath.Join(workdir, "kapi-home")
	return []string{
		"KAPI_NO_PROJECT=1",
		"KAPI_HOME=" + home,
		"KAPI_CONFIG_DIR=" + filepath.Join(workdir, "config"),
		"XDG_DATA_HOME=" + filepath.Join(workdir, "data"),
		"XDG_CACHE_HOME=" + filepath.Join(workdir, "cache"),
		// XDG_DATA_HOME alone isolates the user plugin root. Without this a
		// run still discovers Homebrew-installed plugins.
		"KAPI_PLUGINS_DIR_ONLY=1",
		"KAPI_PLUGINS_DIR=" + filepath.Join(workdir, "plugins"),
	}
}

// setupWorkspace writes the profiles and the corpus, and imports the reference
// profile into the isolated store so the tools that resolve by id can find it.
func setupWorkspace(ctx context.Context, bin, dir string) error {
	for _, sub := range []string{"docs", "kapi-home", "config", "data", "cache", "plugins"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
	}
	files := map[string]string{
		"voice.yaml":    referenceProfile,
		"contrast.yaml": contrastProfile,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			return err
		}
	}
	for _, d := range corpus {
		if err := os.WriteFile(filepath.Join(dir, "docs", d.Name), []byte(d.Body), 0o644); err != nil {
			return err
		}
	}

	// A profile kapi rejects would make every number below a measurement of the
	// fixture. Both are validated before anything is scored against them.
	for _, name := range []string{"voice.yaml", "contrast.yaml"} {
		if out, err := kapiRun(ctx, bin, dir, "voice", "validate", filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("%s is not a profile kapi accepts: %w: %s", name, err, out)
		}
	}
	if _, err := kapiRun(ctx, bin, dir, "voice", "import", filepath.Join(dir, "voice.yaml")); err != nil {
		return fmt.Errorf("import reference profile: %w", err)
	}
	return nil
}

// toolTimeout bounds one kapi invocation.
//
// `kapi exec voice-check` exits 0 having written nothing (#2225), and behind a
// provider that shells out it does not return promptly either. A measurement
// that hangs reports nothing at all, where a measurement that times out reports
// which tool hung — so every invocation carries a deadline and a timeout is
// recorded as the result.
var toolTimeout = 90 * time.Second

// withTimeout bounds ctx by toolTimeout.
func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, toolTimeout)
}

// kapiRun runs one kapi command in the workspace and returns its combined
// output.
func kapiRun(ctx context.Context, bin, dir string, args ...string) (string, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), isolationEnv(dir)...)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	return buf.String(), err
}
