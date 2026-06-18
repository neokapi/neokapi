//go:build onnx

// This file is the onnxruntime-backed OCR engine, compiled only with
// `-tags onnx` (the configuration shipped in release tarballs). It runs the
// RapidOCR / PP-OCRv4 detection and recognition ONNX models and assembles their
// output into recognized text lines, using the pure-Go algorithms in algo.go
// (CTC decoding, connected-component box extraction) for the parts that carry no
// native dependency.
//
// NOTE: the numeric pipeline (normalization constants, channel order, detection
// binarization threshold, and the recognition output shape) follows the standard
// PP-OCRv4 export and requires validation against the real models on a machine
// with onnxruntime before it is relied upon. The orchestration, session
// management, and the algorithms in algo.go are what this build verifies.
package ocr

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"sync"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/neokapi/neokapi/plugins/vision/internal/models"
	"github.com/neokapi/neokapi/plugins/vision/visionproto"
)

const (
	detMaxSide   = 960  // longest detection input side (downscaled if larger)
	detThreshold = 0.30 // probability-map binarization threshold
	recHeight    = 48   // recognition input height
	recWidthDiv  = 8    // CRNN width→timestep downsample factor
)

// onnxEngine owns the onnxruntime sessions and the recognition dictionary.
type onnxEngine struct {
	logf Logf

	mu     sync.Mutex
	det    *ort.DynamicAdvancedSession
	rec    *ort.DynamicAdvancedSession
	dict   []string
	loaded bool
}

// NewEngine constructs the ONNX-backed engine: it initializes onnxruntime,
// ensures the model assets are present, loads the dictionary, and creates the
// detection and recognition sessions (lazily on first OCR).
func NewEngine(logf Logf) (Engine, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if err := initORT(); err != nil {
		return nil, err
	}
	return &onnxEngine{logf: logf}, nil
}

func (e *onnxEngine) ensure() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.loaded {
		return nil
	}
	detAsset, _ := models.Get("det")
	recAsset, _ := models.Get("rec")
	dictAsset, _ := models.Get("dict")

	detPath, err := models.Ensure(detAsset, e.logf)
	if err != nil {
		return err
	}
	recPath, err := models.Ensure(recAsset, e.logf)
	if err != nil {
		return err
	}
	dictPath, err := models.Ensure(dictAsset, e.logf)
	if err != nil {
		return err
	}
	if e.dict, err = loadDict(dictPath); err != nil {
		return fmt.Errorf("vision: load dict: %w", err)
	}
	if e.det, err = ort.NewDynamicAdvancedSession(detPath, []string{"x"}, []string{"sigmoid_0.tmp_0"}, nil); err != nil {
		return fmt.Errorf("vision: det session: %w", err)
	}
	if e.rec, err = ort.NewDynamicAdvancedSession(recPath, []string{"x"}, []string{"softmax_5.tmp_0"}, nil); err != nil {
		return fmt.Errorf("vision: rec session: %w", err)
	}
	e.loaded = true
	return nil
}

func (e *onnxEngine) Loaded() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.loaded
}

func (e *onnxEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.det != nil {
		e.det.Destroy()
		e.det = nil
	}
	if e.rec != nil {
		e.rec.Destroy()
		e.rec = nil
	}
	e.loaded = false
	ort.DestroyEnvironment()
	return nil
}

// OCR runs detection then recognition over the image and returns text lines in
// original-image pixel coordinates.
func (e *onnxEngine) OCR(imageBytes []byte, _, _ string) (*visionproto.OCRResult, error) {
	if err := e.ensure(); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, fmt.Errorf("vision: decode image: %w", err)
	}
	rgba := toRGBA(img)
	ow, oh := rgba.Bounds().Dx(), rgba.Bounds().Dy()

	boxes, dw, dh, err := e.detect(rgba)
	if err != nil {
		return nil, err
	}
	sx, sy := float64(ow)/float64(dw), float64(oh)/float64(dh)

	res := &visionproto.OCRResult{Width: ow, Height: oh}
	for _, b := range boxes {
		// Scale the detection box back to original-image coordinates.
		ox0, oy0 := int(float64(b.X0)*sx), int(float64(b.Y0)*sy)
		ox1, oy1 := int(float64(b.X1)*sx), int(float64(b.Y1)*sy)
		crop := cropRGBA(rgba, ox0, oy0, ox1, oy1)
		if crop == nil {
			continue
		}
		text, conf, err := e.recognize(crop)
		if err != nil {
			return nil, err
		}
		if text == "" {
			continue
		}
		res.Lines = append(res.Lines, visionproto.OCRLine{
			Text:       text,
			X:          float64(ox0),
			Y:          float64(oy0),
			W:          float64(ox1 - ox0 + 1),
			H:          float64(oy1 - oy0 + 1),
			Confidence: conf,
		})
	}
	return res, nil
}

// detect runs the detection model and returns text-region boxes (in the resized
// detection coordinate space) plus that space's dimensions.
func (e *onnxEngine) detect(img *image.RGBA) ([]box, int, int, error) {
	dw, dh := detInputSize(img.Bounds().Dx(), img.Bounds().Dy())
	resized := resizeRGBA(img, dw, dh)
	input := normalizeCHW(resized, []float32{0.485, 0.456, 0.406}, []float32{0.229, 0.224, 0.225})

	inT, err := ort.NewTensor(ort.NewShape(1, 3, int64(dh), int64(dw)), input)
	if err != nil {
		return nil, 0, 0, err
	}
	defer inT.Destroy()
	outT, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 1, int64(dh), int64(dw)))
	if err != nil {
		return nil, 0, 0, err
	}
	defer outT.Destroy()

	e.mu.Lock()
	err = e.det.Run([]ort.Value{inT}, []ort.Value{outT})
	e.mu.Unlock()
	if err != nil {
		return nil, 0, 0, fmt.Errorf("vision: det run: %w", err)
	}

	prob := outT.GetData()
	mask := make([]bool, len(prob))
	for i, p := range prob {
		mask[i] = p >= detThreshold
	}
	minArea := (dw * dh) / 5000 // drop specks relative to the page
	if minArea < 4 {
		minArea = 4
	}
	return connectedBoxes(mask, dw, dh, minArea), dw, dh, nil
}

// recognize runs the recognition model on a single line crop and returns the
// decoded text and mean confidence.
func (e *onnxEngine) recognize(crop *image.RGBA) (string, float64, error) {
	cw, ch := crop.Bounds().Dx(), crop.Bounds().Dy()
	if cw == 0 || ch == 0 {
		return "", 0, nil
	}
	rw := int(math.Round(float64(cw) * float64(recHeight) / float64(ch)))
	if rw < recWidthDiv {
		rw = recWidthDiv
	}
	resized := resizeRGBA(crop, rw, recHeight)
	input := normalizeRec(resized)

	inT, err := ort.NewTensor(ort.NewShape(1, 3, int64(recHeight), int64(rw)), input)
	if err != nil {
		return "", 0, err
	}
	defer inT.Destroy()

	classes := len(e.dict) + 1 // + CTC blank
	steps := rw / recWidthDiv
	if steps < 1 {
		steps = 1
	}
	outT, err := ort.NewEmptyTensor[float32](ort.NewShape(1, int64(steps), int64(classes)))
	if err != nil {
		return "", 0, err
	}
	defer outT.Destroy()

	e.mu.Lock()
	err = e.rec.Run([]ort.Value{inT}, []ort.Value{outT})
	e.mu.Unlock()
	if err != nil {
		return "", 0, fmt.Errorf("vision: rec run: %w", err)
	}

	flat := outT.GetData()
	probs := make([][]float32, steps)
	for t := 0; t < steps; t++ {
		probs[t] = flat[t*classes : (t+1)*classes]
	}
	text, conf := ctcGreedyDecode(probs, e.dict)
	return text, conf, nil
}

// --- pure image helpers (onnx build only; kept off the default build) ---

func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

// detInputSize rounds the image down to fit detMaxSide and to multiples of 32.
func detInputSize(w, h int) (int, int) {
	scale := 1.0
	if m := max(w, h); m > detMaxSide {
		scale = float64(detMaxSide) / float64(m)
	}
	rw := int(math.Round(float64(w)*scale/32)) * 32
	rh := int(math.Round(float64(h)*scale/32)) * 32
	return max(rw, 32), max(rh, 32)
}

// resizeRGBA nearest-neighbor resizes to w×h. (Bilinear is a later refinement.)
func resizeRGBA(src *image.RGBA, w, h int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	for y := 0; y < h; y++ {
		sy := y * sh / h
		for x := 0; x < w; x++ {
			sx := x * sw / w
			out.SetRGBA(x, y, src.RGBAAt(src.Bounds().Min.X+sx, src.Bounds().Min.Y+sy))
		}
	}
	return out
}

func cropRGBA(src *image.RGBA, x0, y0, x1, y1 int) *image.RGBA {
	x0, y0 = clamp(x0, 0, src.Bounds().Dx()-1), clamp(y0, 0, src.Bounds().Dy()-1)
	x1, y1 = clamp(x1, 0, src.Bounds().Dx()-1), clamp(y1, 0, src.Bounds().Dy()-1)
	if x1 <= x0 || y1 <= y0 {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, x1-x0+1, y1-y0+1))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			out.SetRGBA(x-x0, y-y0, src.RGBAAt(src.Bounds().Min.X+x, src.Bounds().Min.Y+y))
		}
	}
	return out
}

// normalizeCHW returns float32 CHW data normalized as (v/255 - mean)/std.
func normalizeCHW(img *image.RGBA, mean, std []float32) []float32 {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	out := make([]float32, 3*h*w)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.RGBAAt(img.Bounds().Min.X+x, img.Bounds().Min.Y+y)
			rgb := [3]float32{float32(c.R), float32(c.G), float32(c.B)}
			for ch := 0; ch < 3; ch++ {
				out[ch*h*w+y*w+x] = (rgb[ch]/255 - mean[ch]) / std[ch]
			}
		}
	}
	return out
}

// normalizeRec returns CHW data normalized as (v/255 - 0.5)/0.5 (PP-OCR rec).
func normalizeRec(img *image.RGBA) []float32 {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	out := make([]float32, 3*h*w)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.RGBAAt(img.Bounds().Min.X+x, img.Bounds().Min.Y+y)
			rgb := [3]float32{float32(c.R), float32(c.G), float32(c.B)}
			for ch := 0; ch < 3; ch++ {
				out[ch*h*w+y*w+x] = (rgb[ch]/255 - 0.5) / 0.5
			}
		}
	}
	return out
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
