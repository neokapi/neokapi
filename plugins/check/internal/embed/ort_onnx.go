//go:build onnx

package embed

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

func readFile(path string) ([]byte, error) { return os.ReadFile(path) }

var (
	ortOnce sync.Once
	ortErr  error
)

// initORT initializes the onnxruntime environment once per process, pointing
// onnxruntime_go at the shared library resolved by resolveORTLib.
func initORT() error {
	ortOnce.Do(func() {
		if lib := resolveORTLib(); lib != "" {
			ort.SetSharedLibraryPath(lib)
		}
		if err := ort.InitializeEnvironment(); err != nil {
			ortErr = fmt.Errorf("check: initialize onnxruntime (set KAPI_CHECK_ORT_LIB to the libonnxruntime path): %w", err)
		}
	})
	return ortErr
}

// resolveORTLib returns the onnxruntime shared-library path to load:
//  1. $KAPI_CHECK_ORT_LIB (explicit override);
//  2. a copy bundled next to the executable (`<dir>/lib/<name>` or
//     `<dir>/<name>`) — the layout the plugin tarball installs;
//  3. "" — fall back to onnxruntime_go's default loader search.
func resolveORTLib() string {
	if lib := os.Getenv("KAPI_CHECK_ORT_LIB"); lib != "" {
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
