package main

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func osName() string {
	return runtime.GOOS
}

func archName() string {
	return runtime.GOARCH
}

func goVersion() string {
	return runtime.Version()
}

func cpuCores() int {
	return runtime.NumCPU()
}

func cpuModel() string {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err != nil {
			return "unknown"
		}
		return strings.TrimSpace(string(out))
	case "linux":
		out, err := exec.Command("grep", "-m1", "model name", "/proc/cpuinfo").Output()
		if err != nil {
			return "unknown"
		}
		if _, after, ok := strings.Cut(string(out), ":"); ok {
			return strings.TrimSpace(after)
		}
		return "unknown"
	default:
		return "unknown"
	}
}

func memoryGB() float64 {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		bytes, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err != nil {
			return 0
		}
		return float64(bytes) / (1024 * 1024 * 1024)
	case "linux":
		out, err := exec.Command("grep", "MemTotal", "/proc/meminfo").Output()
		if err != nil {
			return 0
		}
		parts := strings.Fields(string(out))
		if len(parts) >= 2 {
			kb, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return 0
			}
			return float64(kb) / (1024 * 1024)
		}
		return 0
	default:
		return 0
	}
}
