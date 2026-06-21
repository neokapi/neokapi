// Command kapi-av is the video-demux dependency plugin: it bundles ffmpeg and
// ffprobe (LGPL) per platform so the host's in-process demux (core/av) can find
// them without a system install. Unlike kapi-asr/kapi-vision it runs no daemon —
// the host discovers the plugin dir and points core/av at the bundled binaries.
//
// Subcommands:
//
//	kapi-av version   print the plugin version
//	kapi-av doctor    self-check: report the bundled ffmpeg/ffprobe paths
//	                  (the standard self-check that `kapi plugins doctor` runs)
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const version = "0.1.0"

func main() {
	sub := ""
	if len(os.Args) >= 2 {
		sub = os.Args[1]
	}
	switch sub {
	case "version":
		fmt.Println(version)
	case "doctor":
		os.Exit(runDoctor())
	default:
		fmt.Fprintf(os.Stderr, "kapi-av %s\nusage: kapi-av version | kapi-av doctor\n"+
			"This plugin bundles ffmpeg/ffprobe for the in-process video demux; it runs no daemon.\n", version)
		os.Exit(2)
	}
}

// runDoctor reports the bundled ffmpeg/ffprobe paths and exits non-zero if
// either is missing — the host's in-process demux needs both.
func runDoctor() int {
	ffmpeg, ffprobe := bundled("ffmpeg"), bundled("ffprobe")
	fmt.Printf("kapi-av %s\n  ffmpeg:  %s\n  ffprobe: %s\n", version, statusOf(ffmpeg), statusOf(ffprobe))
	if ffmpeg == "" || ffprobe == "" {
		return 1
	}
	return 0
}

func bundled(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func statusOf(path string) string {
	if path != "" {
		return path
	}
	return "(not bundled beside binary)"
}
