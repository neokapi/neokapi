// Package av demuxes video into the two streams a localization flow extracts
// from: an audio track (→ kapi-asr) and a set of sampled, deduplicated frames
// (→ kapi-vision frame OCR), per AD-030. It is the engine a video format reader
// drives; it stays pure-Go and shells out to ffmpeg/ffprobe (a runtime
// dependency, not a cgo link), and is PATH-based — it reads the video file and
// writes derived files under a work dir, never holding the whole video in
// memory.
package av

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Frame is one sampled video frame: its timestamp (ms from start) and the path
// to the extracted PNG.
type Frame struct {
	TimeMS int64
	Path   string
}

// DemuxResult is what a demux produces: the extracted audio file (when the video
// has an audio track), the video duration, and the kept (deduplicated) frames.
type DemuxResult struct {
	AudioPath  string // "" when the video has no audio track
	HasAudio   bool
	DurationMS int64
	Frames     []Frame
}

// Options tune the demux.
type Options struct {
	// FPS is how many frames per second to sample for OCR (default 1). On-screen
	// text changes slowly, so 1 fps is plenty and bounds the OCR cost.
	FPS float64
	// DedupDistance is the max aHash Hamming distance at which a frame counts as
	// a duplicate of the previous kept frame and is dropped (default 5). Higher =
	// drop more aggressively.
	DedupDistance int
}

const (
	defaultFPS      = 1.0
	defaultDedupDst = 5
)

// A bundled ffmpeg/ffprobe (the kapi-av plugin) is preferred over a system one.
// Resolution order: an explicit SetBinDir → a host-provided locator (discovers
// the kapi-av plugin, called lazily on first use) → $KAPI_AV_DIR → PATH.
var (
	binDir  string
	binOnce sync.Once
	locator func() string
)

// SetBinDir points av at a directory containing bundled ffmpeg/ffprobe. Empty is
// ignored.
func SetBinDir(dir string) { binDir = dir }

// SetBinLocator registers a host function that finds the kapi-av bundle dir. It
// is called at most once, on first ffmpeg use, so discovery cost is paid only by
// commands that actually demux video.
func SetBinLocator(f func() string) { locator = f }

func resolveDir() string {
	if binDir != "" {
		return binDir
	}
	if locator != nil {
		binOnce.Do(func() {
			if d := locator(); d != "" {
				binDir = d
			}
		})
	}
	return binDir
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// resolveBin returns the path to a bundled ffmpeg/ffprobe if one is configured
// and present, else the bare name (resolved on PATH by exec).
func resolveBin(name string) string {
	for _, dir := range []string{resolveDir(), os.Getenv("KAPI_AV_DIR")} {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, exeName(name))
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return name
}

// FFmpegAvailable reports whether ffmpeg and ffprobe are resolvable (bundled or
// on PATH). A video format reader uses this to decide whether demux is possible
// (degrading to "video as an opaque Media asset" when not).
func FFmpegAvailable() bool {
	for _, n := range []string{"ffmpeg", "ffprobe"} {
		p := resolveBin(n)
		if p == n { // not a bundled path — must be on PATH
			if _, err := exec.LookPath(n); err != nil {
				return false
			}
		}
	}
	return true
}

// ConvertImage transcodes a single still image at srcPath to dstPath via ffmpeg,
// decoding formats the in-core Go decoders can't read (HEIC/AVIF and other
// ISOBMFF still images) with ffmpeg's built-in HEVC/AV1 decoders. The
// destination extension selects the encoder — pass a ".png" path for the OCR
// pipeline. Like Demux, it is path-based and never holds the image in memory.
func ConvertImage(ctx context.Context, srcPath, dstPath string) error {
	if !FFmpegAvailable() {
		return errors.New("av: ffmpeg not found on PATH")
	}
	cmd := exec.CommandContext(ctx, resolveBin("ffmpeg"), "-nostdin", "-y",
		"-i", srcPath, "-frames:v", "1", dstPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("av: convert image %q: %w: %s", srcPath, err, lastLine(out))
	}
	return nil
}

type probeFormat struct {
	Duration string `json:"duration"`
}
type probeStream struct {
	CodecType string `json:"codec_type"`
}
type probeOut struct {
	Format  probeFormat   `json:"format"`
	Streams []probeStream `json:"streams"`
}

// Probe reports whether the video has an audio track and its duration, via
// ffprobe.
func Probe(ctx context.Context, videoPath string) (hasAudio bool, durationMS int64, err error) {
	cmd := exec.CommandContext(ctx, resolveBin("ffprobe"),
		"-v", "error",
		"-show_entries", "format=duration:stream=codec_type",
		"-of", "json", videoPath)
	out, err := cmd.Output()
	if err != nil {
		return false, 0, fmt.Errorf("av: ffprobe %q: %w", videoPath, err)
	}
	var p probeOut
	if err := json.Unmarshal(out, &p); err != nil {
		return false, 0, fmt.Errorf("av: parse ffprobe output: %w", err)
	}
	for _, s := range p.Streams {
		if s.CodecType == "audio" {
			hasAudio = true
			break
		}
	}
	if secs, perr := strconv.ParseFloat(strings.TrimSpace(p.Format.Duration), 64); perr == nil {
		durationMS = int64(secs * 1000)
	}
	return hasAudio, durationMS, nil
}

// Demux extracts the audio track (when present) to <workDir>/audio.wav and
// samples frames to <workDir>/frame_*.png at opts.FPS, then drops near-duplicate
// frames (persistent on-screen text) by aHash. workDir must exist and be
// writable; the caller owns its lifecycle (and cleanup).
func Demux(ctx context.Context, videoPath, workDir string, opts Options) (*DemuxResult, error) {
	if opts.FPS <= 0 {
		opts.FPS = defaultFPS
	}
	if opts.DedupDistance == 0 {
		opts.DedupDistance = defaultDedupDst
	}
	if !FFmpegAvailable() {
		return nil, errors.New("av: ffmpeg/ffprobe not found on PATH")
	}

	hasAudio, durationMS, err := Probe(ctx, videoPath)
	if err != nil {
		return nil, err
	}
	res := &DemuxResult{HasAudio: hasAudio, DurationMS: durationMS}

	if hasAudio {
		audioPath := filepath.Join(workDir, "audio.wav")
		// 16 kHz mono PCM — the canonical Whisper input.
		cmd := exec.CommandContext(ctx, resolveBin("ffmpeg"), "-nostdin", "-y", "-i", videoPath,
			"-vn", "-ac", "1", "-ar", "16000", "-f", "wav", audioPath)
		if out, aerr := cmd.CombinedOutput(); aerr != nil {
			return nil, fmt.Errorf("av: extract audio: %w: %s", aerr, lastLine(out))
		}
		res.AudioPath = audioPath
	}

	frames, err := sampleFrames(ctx, videoPath, workDir, opts.FPS)
	if err != nil {
		return nil, err
	}
	res.Frames, err = dedupFrames(frames, opts.DedupDistance)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// sampleFrames extracts frames at fps to workDir, returning them ordered by time.
func sampleFrames(ctx context.Context, videoPath, workDir string, fps float64) ([]Frame, error) {
	pattern := filepath.Join(workDir, "frame_%06d.png")
	cmd := exec.CommandContext(ctx, resolveBin("ffmpeg"), "-nostdin", "-y", "-i", videoPath,
		"-vf", fmt.Sprintf("fps=%g", fps), pattern)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("av: sample frames: %w: %s", err, lastLine(out))
	}
	matches, err := filepath.Glob(filepath.Join(workDir, "frame_*.png"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	frames := make([]Frame, len(matches))
	for i, m := range matches {
		// ffmpeg numbers frames from 1 at the sampling rate; frame i (0-based) is
		// at i/fps seconds.
		frames[i] = Frame{TimeMS: int64(float64(i) / fps * 1000), Path: m}
	}
	return frames, nil
}

// dedupFrames keeps frames whose content changed (by aHash), dropping persistent
// near-identical frames and removing the dropped files.
func dedupFrames(frames []Frame, maxDistance int) ([]Frame, error) {
	if len(frames) == 0 {
		return nil, nil
	}
	hashes := make([]uint64, len(frames))
	for i, f := range frames {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, err
		}
		h, err := aHashBytes(data)
		if err != nil {
			return nil, err
		}
		hashes[i] = h
	}
	keepIdx := dedupKeep(hashes, maxDistance)
	keepSet := make(map[int]bool, len(keepIdx))
	for _, i := range keepIdx {
		keepSet[i] = true
	}
	kept := make([]Frame, 0, len(keepIdx))
	for i, f := range frames {
		if keepSet[i] {
			kept = append(kept, f)
		} else {
			_ = os.Remove(f.Path) // reclaim dropped duplicates
		}
	}
	return kept, nil
}

// Runner executes a prepared ffmpeg command and returns its combined output. It
// is a package var so the slice helpers (SliceAudio / ExtractFrame) can be
// exercised in tests without a real ffmpeg on PATH — a test swaps in a stub that
// writes a fixture to the command's output path. The default shells out for real.
var Runner = func(cmd *exec.Cmd) ([]byte, error) { return cmd.CombinedOutput() }

// SliceAudio cuts the half-open span [startMS, endMS) out of the audio (or
// audio track) at srcPath and writes it to outPath as 16 kHz mono PCM WAV — the
// bounded clip a media-refine pass sends to a multimodal LLM to re-read a
// low-confidence ASR segment (AD-030). It never holds the whole track in memory.
func SliceAudio(ctx context.Context, srcPath string, startMS, endMS int64, outPath string) error {
	if endMS <= startMS {
		return fmt.Errorf("av: slice audio: empty span [%d,%d)", startMS, endMS)
	}
	ss := strconv.FormatFloat(float64(startMS)/1000, 'f', 3, 64)
	dur := strconv.FormatFloat(float64(endMS-startMS)/1000, 'f', 3, 64)
	// -ss before -i seeks at the demuxer (fast); -t bounds the duration.
	cmd := exec.CommandContext(ctx, resolveBin("ffmpeg"), "-nostdin", "-y",
		"-ss", ss, "-i", srcPath, "-t", dur,
		"-vn", "-ac", "1", "-ar", "16000", "-f", "wav", outPath)
	if out, err := Runner(cmd); err != nil {
		return fmt.Errorf("av: slice audio: %w: %s", err, lastLine(out))
	}
	return nil
}

// ExtractFrame extracts the single video frame at atMS (ms from start) of the
// video at srcPath and writes it to outPath as a PNG — the still a media-refine
// pass crops to a block's geometry before re-reading a low-confidence
// video-frame OCR unit (AD-030).
func ExtractFrame(ctx context.Context, srcPath string, atMS int64, outPath string) error {
	if atMS < 0 {
		return fmt.Errorf("av: extract frame: negative timecode %d", atMS)
	}
	ss := strconv.FormatFloat(float64(atMS)/1000, 'f', 3, 64)
	cmd := exec.CommandContext(ctx, resolveBin("ffmpeg"), "-nostdin", "-y",
		"-ss", ss, "-i", srcPath, "-frames:v", "1", "-f", "image2", outPath)
	if out, err := Runner(cmd); err != nil {
		return fmt.Errorf("av: extract frame: %w: %s", err, lastLine(out))
	}
	return nil
}

// lastLine returns the last non-empty line of ffmpeg output, for concise errors.
func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}
