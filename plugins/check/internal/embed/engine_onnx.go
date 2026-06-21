//go:build onnx

// This file is the cgo-backed embedding engine, compiled only with `-tags onnx`
// (when the onnxruntime + tokenizer native libraries must be present). The
// default build uses engine_stub.go, so the module and pure-Go tests build with
// no native dependency.
package embed

import (
	"fmt"
	"sync"

	"github.com/daulet/tokenizers"
	ort "github.com/yalue/onnxruntime_go"

	"github.com/neokapi/neokapi/plugins/check/internal/model"
	"github.com/neokapi/neokapi/plugins/check/internal/vec"
)

type onnxEngine struct {
	logf func(string, ...any)

	mu     sync.Mutex
	models map[string]*loadedModel
}

type loadedModel struct {
	tk         *tokenizers.Tokenizer
	session    *ort.DynamicAdvancedSession
	dim        int
	inputNames []string
	outputName string
}

// NewEngine creates the ONNX-backed engine.
func NewEngine(logf func(string, ...any)) (Engine, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if err := initORT(); err != nil {
		return nil, err
	}
	return &onnxEngine{
		logf:   logf,
		models: map[string]*loadedModel{},
	}, nil
}

func (e *onnxEngine) Loaded(name string) bool {
	if name == "" {
		name = model.DefaultModelName()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.models[name]
	return ok
}

func (e *onnxEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, m := range e.models {
		if m.session != nil {
			m.session.Destroy()
		}
		if m.tk != nil {
			m.tk.Close()
		}
	}
	e.models = map[string]*loadedModel{}
	ort.DestroyEnvironment()
	return nil
}

func (e *onnxEngine) get(name string) (*loadedModel, error) {
	if name == "" {
		name = model.DefaultModelName()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if m, ok := e.models[name]; ok {
		return m, nil
	}
	root, err := model.ModelsRoot()
	if err != nil {
		return nil, err
	}
	paths, err := model.ResolveInDir(name, root)
	if err != nil {
		return nil, err
	}
	spec, _ := model.Lookup(name)
	tkBytes, err := readFile(paths.Tokenizer)
	if err != nil {
		return nil, fmt.Errorf("check: read tokenizer: %w", err)
	}
	tk, err := tokenizers.FromBytes(tkBytes)
	if err != nil {
		return nil, fmt.Errorf("check: load tokenizer: %w", err)
	}
	// Discover the model's input and output names so we feed exactly what it
	// expects (this e5 export needs input_ids + attention_mask + token_type_ids;
	// others may differ) and read its actual output tensor.
	inInfo, outInfo, err := ort.GetInputOutputInfo(paths.ONNX)
	if err != nil {
		tk.Close()
		return nil, fmt.Errorf("check: inspect onnx io: %w", err)
	}
	if len(outInfo) == 0 {
		tk.Close()
		return nil, fmt.Errorf("check: model %q has no outputs", name)
	}
	inNames := make([]string, len(inInfo))
	for i, in := range inInfo {
		inNames[i] = in.Name
	}
	outName := outInfo[0].Name
	session, err := ort.NewDynamicAdvancedSession(paths.ONNX, inNames, []string{outName}, nil)
	if err != nil {
		tk.Close()
		return nil, fmt.Errorf("check: create onnx session: %w", err)
	}
	m := &loadedModel{tk: tk, session: session, dim: spec.Dim, inputNames: inNames, outputName: outName}
	e.models[name] = m
	e.logf("loaded model %s", name)
	return m, nil
}

// Embed tokenizes text (with the e5 "query:" convention and special tokens),
// runs the model, mean-pools the last hidden state over the real tokens, and
// L2-normalizes the result.
func (e *onnxEngine) Embed(text, name string) ([]float32, error) {
	m, err := e.get(name)
	if err != nil {
		return nil, err
	}
	enc := m.tk.EncodeWithOptions("query: "+text, true)
	seqLen := len(enc.IDs)
	if seqLen == 0 {
		return make([]float32, m.dim), nil
	}
	ids := make([]int64, seqLen)
	mask := make([]int64, seqLen)
	tokenType := make([]int64, seqLen) // zeros — single-segment input
	for i, id := range enc.IDs {
		ids[i] = int64(id)
		mask[i] = 1
	}

	shape := ort.NewShape(1, int64(seqLen))
	tensors := map[string]*ort.Tensor[int64]{}
	for _, nm := range m.inputNames {
		var data []int64
		switch nm {
		case "input_ids":
			data = ids
		case "attention_mask":
			data = mask
		case "token_type_ids":
			data = tokenType
		default:
			return nil, fmt.Errorf("check: unexpected model input %q", nm)
		}
		tn, err := ort.NewTensor(shape, data)
		if err != nil {
			return nil, fmt.Errorf("check: %s tensor: %w", nm, err)
		}
		defer tn.Destroy()
		tensors[nm] = tn
	}
	feeds := make([]ort.Value, len(m.inputNames))
	for i, nm := range m.inputNames {
		feeds[i] = tensors[nm]
	}

	outData := make([]float32, seqLen*m.dim)
	out, err := ort.NewTensor(ort.NewShape(1, int64(seqLen), int64(m.dim)), outData)
	if err != nil {
		return nil, fmt.Errorf("check: output tensor: %w", err)
	}
	defer out.Destroy()

	if err := m.session.Run(feeds, []ort.Value{out}); err != nil {
		return nil, fmt.Errorf("check: onnx run: %w", err)
	}

	hidden := make([][]float32, seqLen)
	for i := 0; i < seqLen; i++ {
		hidden[i] = outData[i*m.dim : (i+1)*m.dim]
	}
	return vec.L2Normalize(vec.MeanPool(hidden, mask)), nil
}
