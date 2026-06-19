// Command kapi-av is the video-demux dependency plugin: it bundles ffmpeg and
// ffprobe (LGPL) per platform so the host's in-process demux (core/av) can find
// them without a system install. Unlike kapi-asr/kapi-vision it runs no daemon —
// the host discovers the plugin dir and points core/av at the bundled binaries.
// The `av` command is a self-check.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const version = "0.1.0"

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "av" {
		fmt.Printf("kapi-av %s\n  ffmpeg:  %s\n  ffprobe: %s\n", version, status("ffmpeg"), status("ffprobe"))
		return
	}
	fmt.Fprintf(os.Stderr, "kapi-av %s\nusage: kapi-av av   (self-check)\n"+
		"This plugin bundles ffmpeg/ffprobe for the in-process video demux; it runs no daemon.\n", version)
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

func status(name string) string {
	if p := bundled(name); p != "" {
		return p
	}
	return "(not bundled beside binary)"
}
