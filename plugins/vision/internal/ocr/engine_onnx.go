//go:build onnx

// This file is the onnxruntime-backed OCR engine, compiled only with
// `-tags onnx` (the configuration shipped in release tarballs). It runs the
// RapidOCR / PP-OCRv4 detection and recognition ONNX models and assembles their
// output into recognized text lines, using the pure-Go algorithms in algo.go
// (CTC decoding, connected-component box extraction) for the parts that carry no
// native dependency.
//
// The pipeline is validated end-to-end against the PP-OCRv4 mobile models
// (TestOCRSmoke reads the committed hello.png). Detection binarizes the DBNet
// probability map, extracts connected components, and "unclips" the shrunk
// kernels; recognition normalizes in BGR (PP-OCR's training order), runs the
// CRNN, and CTC-decodes against the PP-OCR dictionary. Known limitations: the
// detection uses axis-aligned boxes (not rotated polygons) and the mobile
// recognizer occasionally drops inter-word spaces — both acceptable for v1 and
// improvable later (rotated-box DB postproc, a larger recognizer).
package ocr

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
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
	if e.rec, err = ort.NewDynamicAdvancedSession(recPath, []string{"x"}, []string{"softmax_11.tmp_0"}, nil); err != nil {
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

// OCR runs detection then recognition over the image file and returns text lines
// in original-image pixel coordinates. The plugin opens and decodes the file
// itself, so the bytes never live in the host (kapi) process.
func (e *onnxEngine) OCR(imagePath, _, _ string) (*visionproto.OCRResult, error) {
	if err := e.ensure(); err != nil {
		return nil, err
	}
	f, err := os.Open(imagePath)
	if err != nil {
		return nil, fmt.Errorf("vision: open image: %w", err)
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
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
		// DBNet predicts shrunk text kernels; expand the box ("unclip") to
		// recover the full glyph extent before cropping for recognition.
		ox0, oy0, ox1, oy1 = unclip(ox0, oy0, ox1, oy1, ow, oh)
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

	// Auto-allocated output (the binding fills outs[0] from the run).
	outs := []ort.Value{nil}
	e.mu.Lock()
	err = e.det.Run([]ort.Value{inT}, outs)
	e.mu.Unlock()
	if err != nil {
		return nil, 0, 0, fmt.Errorf("vision: det run: %w", err)
	}
	outT, ok := outs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, 0, 0, fmt.Errorf("vision: det output is not float32")
	}
	defer outT.Destroy()

	// Output shape [1,1,H,W] — use its own H,W for the mask.
	sh := outT.GetShape()
	ph, pw := dh, dw
	if len(sh) == 4 {
		ph, pw = int(sh[2]), int(sh[3])
	}
	prob := outT.GetData()
	mask := make([]bool, len(prob))
	for i, p := range prob {
		mask[i] = p >= detThreshold
	}
	minArea := (pw * ph) / 5000 // drop specks relative to the page
	if minArea < 4 {
		minArea = 4
	}
	return connectedBoxes(mask, pw, ph, minArea), pw, ph, nil
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

	outs := []ort.Value{nil}
	e.mu.Lock()
	err = e.rec.Run([]ort.Value{inT}, outs)
	e.mu.Unlock()
	if err != nil {
		return "", 0, fmt.Errorf("vision: rec run: %w", err)
	}
	outT, ok := outs[0].(*ort.Tensor[float32])
	if !ok {
		return "", 0, fmt.Errorf("vision: rec output is not float32")
	}
	defer outT.Destroy()

	// Output shape [1, T, C]; reshape the flat data into T timesteps of C classes.
	sh := outT.GetShape()
	if len(sh) != 3 {
		return "", 0, fmt.Errorf("vision: unexpected rec output shape %v", sh)
	}
	steps, classes := int(sh[1]), int(sh[2])
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

// resizeRGBA bilinearly resizes to w×h. Bilinear (vs nearest-neighbor) markedly
// improves recognition accuracy on glyph edges.
func resizeRGBA(src *image.RGBA, w, h int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if sw == 0 || sh == 0 {
		return out
	}
	minX, minY := src.Bounds().Min.X, src.Bounds().Min.Y
	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)*float64(sh)/float64(h) - 0.5
		y0 := int(math.Floor(fy))
		wy := fy - float64(y0)
		y1 := y0 + 1
		y0, y1 = clamp(y0, 0, sh-1), clamp(y1, 0, sh-1)
		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)*float64(sw)/float64(w) - 0.5
			x0 := int(math.Floor(fx))
			wx := fx - float64(x0)
			x1 := x0 + 1
			x0, x1 = clamp(x0, 0, sw-1), clamp(x1, 0, sw-1)
			c00 := src.RGBAAt(minX+x0, minY+y0)
			c10 := src.RGBAAt(minX+x1, minY+y0)
			c01 := src.RGBAAt(minX+x0, minY+y1)
			c11 := src.RGBAAt(minX+x1, minY+y1)
			out.SetRGBA(x, y, bilerp(c00, c10, c01, c11, wx, wy))
		}
	}
	return out
}

// bilerp bilinearly interpolates four RGBA corners.
func bilerp(c00, c10, c01, c11 color.RGBA, wx, wy float64) color.RGBA {
	lerp := func(a, b uint8, t float64) uint8 {
		return uint8(float64(a)*(1-t) + float64(b)*t + 0.5)
	}
	top := color.RGBA{R: lerp(c00.R, c10.R, wx), G: lerp(c00.G, c10.G, wx), B: lerp(c00.B, c10.B, wx), A: lerp(c00.A, c10.A, wx)}
	bot := color.RGBA{R: lerp(c01.R, c11.R, wx), G: lerp(c01.G, c11.G, wx), B: lerp(c01.B, c11.B, wx), A: lerp(c01.A, c11.A, wx)}
	return color.RGBA{R: lerp(top.R, bot.R, wy), G: lerp(top.G, bot.G, wy), B: lerp(top.B, bot.B, wy), A: lerp(top.A, bot.A, wy)}
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
// Channels are in BGR order: PaddleOCR/PP-OCR models are trained on cv2's BGR
// images, so channel 0 is blue.
func normalizeRec(img *image.RGBA) []float32 {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	out := make([]float32, 3*h*w)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.RGBAAt(img.Bounds().Min.X+x, img.Bounds().Min.Y+y)
			bgr := [3]float32{float32(c.B), float32(c.G), float32(c.R)}
			for ch := 0; ch < 3; ch++ {
				out[ch*h*w+y*w+x] = (bgr[ch]/255 - 0.5) / 0.5
			}
		}
	}
	return out
}

// unclip expands a shrunk DBNet box outward to approximate the full text
// extent: vertically by ~0.6× the box height each side, horizontally by ~0.2×.
// Clamped to the image bounds.
func unclip(x0, y0, x1, y1, w, h int) (int, int, int, int) {
	bh := y1 - y0 + 1
	padY := int(0.6 * float64(bh))
	padX := int(0.35 * float64(bh))
	return clamp(x0-padX, 0, w-1), clamp(y0-padY, 0, h-1),
		clamp(x1+padX, 0, w-1), clamp(y1+padY, 0, h-1)
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
