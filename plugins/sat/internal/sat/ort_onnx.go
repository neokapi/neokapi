//go:build onnx

package sat

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// readFile is a thin wrapper kept here so engine_onnx.go has no direct os
// import beyond what it needs.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

var (
	ortOnce sync.Once
	ortErr  error
)

// initORT initializes the onnxruntime environment exactly once per process,
// pointing onnxruntime_go at the shared library resolved by resolveORTLib.
func initORT() error {
	ortOnce.Do(func() {
		if lib := resolveORTLib(); lib != "" {
			ort.SetSharedLibraryPath(lib)
		}
		if err := ort.InitializeEnvironment(); err != nil {
			ortErr = fmt.Errorf("sat: initialize onnxruntime (set KAPI_SAT_ORT_LIB to the libonnxruntime path): %w", err)
		}
	})
	return ortErr
}

// resolveORTLib returns the onnxruntime shared-library path to load:
//  1. $KAPI_SAT_ORT_LIB if set (explicit override);
//  2. a copy bundled next to the executable — the layout the registry plugin
//     tarball installs (`<dir>/lib/<name>` or `<dir>/<name>`), so an installed
//     plugin works with no environment configuration;
//  3. "" — let onnxruntime_go fall back to its default loader search (e.g. a
//     system-installed onnxruntime on LD_LIBRARY_PATH / DYLD_LIBRARY_PATH).
func resolveORTLib() string {
	if lib := os.Getenv("KAPI_SAT_ORT_LIB"); lib != "" {
		return lib
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	name := ortLibName()
	for _, cand := range []string{filepath.Join(dir, "lib", name), filepath.Join(dir, name)} {
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return cand
		}
	}
	return ""
}

// ortLibName is the unversioned onnxruntime shared-library filename the plugin
// tarball bundles for the current platform.
func ortLibName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libonnxruntime.dylib"
	case "windows":
		return "onnxruntime.dll"
	default:
		return "libonnxruntime.so"
	}
}
