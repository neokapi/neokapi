//go:build !onnx

package llm

// stubEngine is the default-build Engine. It links no native libraries; every
// Generate call returns ErrNoONNX. This keeps `go build`/`go test` green on
// machines without onnxruntime + the tokenizer library, while the real engine
// (engine_onnx.go) is selected with `-tags onnx`.
type stubEngine struct{}

// NewEngine returns the stub engine in the default build. cacheLogf is accepted
// for signature parity with the ONNX build and ignored.
func NewEngine(cacheLogf func(string, ...any)) (Engine, error) {
	return &stubEngine{}, nil
}

func (*stubEngine) Generate(GenerateParams) (Result, error) { return Result{}, ErrNoONNX }

func (*stubEngine) Loaded(string) bool { return false }

// Modalities still advertises the engine's intended input modalities so `info`
// reports them even on a stub build; Generate is what fails without ONNX.
// (Matches the ONNX engine: image + audio; video needs frame extraction.)
func (*stubEngine) Modalities() []string { return []string{"image", "audio"} }

func (*stubEngine) Close() error { return nil }
