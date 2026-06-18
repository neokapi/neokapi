//go:build onnx

package ocr

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/neokapi/neokapi/plugins/vision/internal/models"
	"github.com/neokapi/neokapi/plugins/vision/visionproto"
)

// layout_onnx.go runs PP-DocLayoutV3 (RT-DETR) document layout detection. The
// model is RT-DETR (NMS-free): it emits already-decoded detections in the
// ORIGINAL image's pixel coordinates, given PaddleDetection's three inputs —
// the 800×800 image, scale_factor = [800/H, 800/W], and im_shape = [H, W]. The
// detection output is [N, ≥6] rows of [class, score, x1, y1, x2, y2, …]. Classes
// map to content roles via layoutRole (layoutmap.go). Reading order is assigned
// host-side (core/vision.SortReadingOrder); the plugin returns regions in
// detection order.

const (
	layoutSize           = 800
	layoutScoreThreshold = 0.5 // PP-DocLayoutV3 inference.yml draw_threshold
)

// ensureLayout lazily downloads the layout model and opens its session. The
// model is large (~132 MB) and download-on-demand, so this is separate from the
// OCR ensure().
func (e *onnxEngine) ensureLayout() error {
	e.layoutMu.Lock()
	defer e.layoutMu.Unlock()
	if e.layoutSess != nil {
		return nil
	}
	a, ok := models.Get("layout")
	if !ok {
		return fmt.Errorf("vision: no layout model in registry")
	}
	p, err := models.Ensure(a, e.logf)
	if err != nil {
		return err
	}
	ins, outs, err := ort.GetInputOutputInfo(p)
	if err != nil {
		return err
	}
	inN := make([]string, len(ins))
	for i, v := range ins {
		inN[i] = v.Name
	}
	outN := make([]string, len(outs))
	for i, v := range outs {
		outN[i] = v.Name
	}
	sess, err := ort.NewDynamicAdvancedSession(p, inN, outN, nil)
	if err != nil {
		return fmt.Errorf("vision: layout session: %w", err)
	}
	e.layoutSess = sess
	e.layoutInputs = inN
	e.layoutOutputs = outN
	return nil
}

// Layout detects layout regions in the image file and returns them in
// original-image pixel coordinates. The plugin opens and decodes the file
// itself, so the bytes never live in the host (kapi) process.
func (e *onnxEngine) Layout(imagePath, _, _ string) (*visionproto.LayoutResult, error) {
	if err := e.ensureLayout(); err != nil {
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

	// Inputs: image (RGB, /255, 800×800 stretch), scale_factor, im_shape.
	resized := resizeRGBA(rgba, layoutSize, layoutSize)
	imgT, err := ort.NewTensor(ort.NewShape(1, 3, layoutSize, layoutSize), normalizeLayout(resized))
	if err != nil {
		return nil, err
	}
	defer imgT.Destroy()
	sf, err := ort.NewTensor(ort.NewShape(1, 2), []float32{float32(layoutSize) / float32(oh), float32(layoutSize) / float32(ow)})
	if err != nil {
		return nil, err
	}
	defer sf.Destroy()
	ish, err := ort.NewTensor(ort.NewShape(1, 2), []float32{float32(oh), float32(ow)})
	if err != nil {
		return nil, err
	}
	defer ish.Destroy()

	byName := map[string]ort.Value{"image": imgT, "scale_factor": sf, "im_shape": ish}
	inV := make([]ort.Value, len(e.layoutInputs))
	for i, n := range e.layoutInputs {
		v, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("vision: layout model wants unknown input %q", n)
		}
		inV[i] = v
	}
	outV := make([]ort.Value, len(e.layoutOutputs))

	e.layoutMu.Lock()
	err = e.layoutSess.Run(inV, outV)
	e.layoutMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("vision: layout run: %w", err)
	}

	// The detection output is the float32 tensor ([N, ≥6]); free the rest
	// (count + mask int32 tensors are unused).
	var det *ort.Tensor[float32]
	for _, v := range outV {
		if v == nil {
			continue
		}
		if t, ok := v.(*ort.Tensor[float32]); ok && det == nil {
			det = t
		} else {
			_ = v.Destroy()
		}
	}
	if det == nil {
		return nil, fmt.Errorf("vision: layout produced no float detection output")
	}
	defer det.Destroy()

	sh := det.GetShape()
	if len(sh) != 2 || sh[1] < 6 {
		return nil, fmt.Errorf("vision: unexpected layout output shape %v", sh)
	}
	rows, cols := int(sh[0]), int(sh[1])
	data := det.GetData()
	res := &visionproto.LayoutResult{Width: ow, Height: oh}
	for r := 0; r < rows; r++ {
		row := data[r*cols : (r+1)*cols]
		score := float64(row[1])
		if score < layoutScoreThreshold {
			continue
		}
		x1, y1, x2, y2 := float64(row[2]), float64(row[3]), float64(row[4]), float64(row[5])
		if x2 <= x1 || y2 <= y1 {
			continue
		}
		res.Regions = append(res.Regions, visionproto.Region{
			Role:       layoutRole(int(row[0])),
			X:          x1,
			Y:          y1,
			W:          x2 - x1,
			H:          y2 - y1,
			Confidence: score,
		})
	}
	return res, nil
}

// normalizeLayout returns CHW float data scaled to [0,1] in RGB order — the
// PP-DocLayoutV3 preprocessing (NormalizeImage mean 0 / std 1 over /255).
func normalizeLayout(img *image.RGBA) []float32 {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	out := make([]float32, 3*h*w)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.RGBAAt(img.Bounds().Min.X+x, img.Bounds().Min.Y+y)
			out[0*h*w+y*w+x] = float32(c.R) / 255
			out[1*h*w+y*w+x] = float32(c.G) / 255
			out[2*h*w+y*w+x] = float32(c.B) / 255
		}
	}
	return out
}
