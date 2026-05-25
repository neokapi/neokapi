//go:build onnx

package sat

import (
	"fmt"
	"os"
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

// initORT initializes the onnxruntime environment exactly once per process. The
// shared library path is taken from KAPI_SAT_ORT_LIB when set; otherwise
// onnxruntime_go uses its built-in default name (e.g. "onnxruntime.so" /
// "onnxruntime.dylib" on the loader path). Set KAPI_SAT_ORT_LIB to the absolute
// path of the extracted libonnxruntime shared library if it is not on the
// default search path.
func initORT() error {
	ortOnce.Do(func() {
		if lib := os.Getenv("KAPI_SAT_ORT_LIB"); lib != "" {
			ort.SetSharedLibraryPath(lib)
		}
		if err := ort.InitializeEnvironment(); err != nil {
			ortErr = fmt.Errorf("sat: initialize onnxruntime (set KAPI_SAT_ORT_LIB to the libonnxruntime path): %w", err)
		}
	})
	return ortErr
}
