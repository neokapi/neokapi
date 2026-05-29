//go:build !onnx

package embed

// stubEngine is the default-build Engine. It links no native libraries; every
// Embed call returns ErrNoONNX. This keeps `go build`/`go test` green on
// machines without onnxruntime + the tokenizer library, while the real engine
// (engine_onnx.go) is selected with `-tags onnx`.
type stubEngine struct{}

// NewEngine returns the stub engine in the default build. logf is accepted for
// signature parity with the ONNX build and ignored.
func NewEngine(logf func(string, ...any)) (Engine, error) {
	return &stubEngine{}, nil
}

func (*stubEngine) Embed(string, string) ([]float32, error) { return nil, ErrNoONNX }
func (*stubEngine) Loaded(string) bool                      { return false }
func (*stubEngine) Close() error                            { return nil }
