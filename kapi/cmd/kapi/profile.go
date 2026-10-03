package main

import (
	"fmt"
	"os"
	"runtime/pprof"
)

// cpuProfileEnv names a file to write a CPU profile of the whole command to.
// It is a measurement aid for contributors (`go tool pprof bin/kapi <file>`),
// read from the environment so no command grows a flag for it.
const cpuProfileEnv = "KAPI_CPUPROFILE"

// startCPUProfile starts the CPU profile KAPI_CPUPROFILE asks for and returns
// the function that stops it and closes the file. With the variable unset it
// does nothing; a profile that cannot start is reported on stderr and the
// command runs without one.
func startCPUProfile() func() {
	path := os.Getenv(cpuProfileEnv)
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cpuProfileEnv, err)
		return func() {}
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cpuProfileEnv, err)
		_ = f.Close()
		return func() {}
	}
	return func() {
		pprof.StopCPUProfile()
		_ = f.Close()
	}
}
