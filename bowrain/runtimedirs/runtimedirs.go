// Package runtimedirs checks, at start-up, that the directories the server and
// the worker write at run time are writable.
//
// The containers run as an unprivileged user and are meant to run with a
// read-only root filesystem, so every directory a process writes is a named
// path that a deployment provides as a writable volume or tmpfs. A directory
// that is not writable stops the process before it serves anything, with a
// message that names the directory and the variable that sets it, instead of
// failing at the first request that needs it.
package runtimedirs

import (
	"errors"
	"fmt"
	"os"
)

// Dir is one directory a process writes at run time.
type Dir struct {
	// Purpose says what the process keeps there, for the error message.
	Purpose string
	// Env is the environment variable that sets the directory.
	Env string
	// Path is the directory in effect.
	Path string
}

// Check creates each directory that does not exist yet and writes and removes
// a file in it. It returns one error that names every directory that failed.
func Check(dirs ...Dir) error {
	var errs []error
	for _, d := range dirs {
		if err := writable(d.Path); err != nil {
			errs = append(errs, fmt.Errorf("%s directory %s (set by %s) is not writable: %w; "+
				"mount a writable volume or tmpfs there, or point %s at a writable directory",
				d.Purpose, d.Path, d.Env, err, d.Env))
		}
	}
	return errors.Join(errs...)
}

// writable reports why dir cannot be written, or nil.
func writable(dir string) error {
	if dir == "" {
		return errors.New("no directory is set")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".write-check-*")
	if err != nil {
		return err
	}
	name := f.Name()
	closeErr := f.Close()
	removeErr := os.Remove(name)
	return errors.Join(closeErr, removeErr)
}

// Temp is the system temporary directory, which holds forge checkouts, git
// clones, skeleton stores and the other per-request scratch files.
func Temp() Dir {
	return Dir{Purpose: "temporary", Env: "TMPDIR", Path: os.TempDir()}
}
