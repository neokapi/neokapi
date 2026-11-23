package pluginhost

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// ErrAlreadyInstalled reports a plugin the install root already holds in a
// usable form (its manifest parses): that copy wins.
var ErrAlreadyInstalled = errors.New("already installed")

// adoptTempPrefix begins the name of the hidden directory a copy is made in
// beside its destination. Discovery skips hidden entries.
const adoptTempPrefix = ".kapi-adopt-"

// openRoot is os.OpenRoot, replaceable in a test that makes it fail.
var openRoot = os.OpenRoot

// moveDir is the rename of src to its destination, replaceable in a test that
// makes it fail as a rename across file systems does.
var moveDir = os.Rename

// AdoptPluginDir moves the plugin directory src into the install root target
// (as target/<base of src>) under the checks an install makes: no links,
// nobody but the person or root owns or may write it, and every file ends
// with the install mode cap.
//
// The destination only ever appears complete. Across file systems src is
// copied into a hidden directory beside it and renamed into place, and src is
// removed only after that rename. Every failure removes the hidden directory,
// and a leftover from a killed run is removed first.
//
// A usable plugin already in place wins (ErrAlreadyInstalled). An empty
// directory there is removed, so it never wins over a working plugin; any
// other unusable entry there is refused and src stays where it is.
func AdoptPluginDir(src, target string) error {
	name := filepath.Base(src)
	dest := filepath.Join(target, name)
	if leftovers, err := filepath.Glob(filepath.Join(target, adoptTempPrefix+name+"-*")); err == nil {
		for _, l := range leftovers {
			_ = os.RemoveAll(l)
		}
	}
	if _, err := os.Lstat(dest); err == nil {
		if data, rerr := os.ReadFile(filepath.Join(dest, "manifest.json")); rerr == nil {
			if _, perr := manifest.Parse(data); perr == nil {
				return fmt.Errorf("adopt %s: %s is in %s: %w", src, name, target, ErrAlreadyInstalled)
			}
		}
		if rerr := os.Remove(dest); rerr != nil {
			return fmt.Errorf("adopt %s: %s holds no plugin kapi can load and is not empty, so the earlier install stays: %w", src, dest, rerr)
		}
	}
	if err := checkPath(src); err != nil {
		return fmt.Errorf("adopt %s: %w", src, err)
	}
	if err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a link; an installed plugin holds none", path)
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
		return checkPath(path)
	}); err != nil {
		return fmt.Errorf("adopt %s: %w", src, err)
	}
	if err := os.MkdirAll(target, pluginDirMode); err != nil {
		return fmt.Errorf("adopt %s: %w", src, err)
	}
	if err := checkPath(target); err != nil {
		return fmt.Errorf("adopt %s: %w", src, err)
	}
	if err := moveDir(src, dest); err == nil {
		return capInPlace(src, dest)
	} else if _, ok := errors.AsType[*os.LinkError](err); !ok || !crossDevice(err) {
		return fmt.Errorf("adopt %s: %w", src, err)
	}
	// Another file system: copy into a hidden directory beside the
	// destination through an os.Root on each side, so no name in the tree can
	// lead outside either directory, then rename it into place.
	tmp, err := os.MkdirTemp(target, adoptTempPrefix+name+"-")
	if err != nil {
		return fmt.Errorf("adopt %s: %w", src, err)
	}
	if err := copyIntoTemp(src, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("adopt %s: %w", src, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("adopt %s: %w", src, err)
	}
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("adopt %s: remove the original: %w", src, err)
	}
	return nil
}

// crossDevice reports a rename refused because source and destination are on
// different file systems. Any other refusal is returned to the caller.
var crossDevice = isCrossDevice

// copyIntoTemp copies src into the hidden directory tmp and caps its modes,
// each opened as an os.Root.
func copyIntoTemp(src, tmp string) error {
	from, err := openRoot(src)
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := openRoot(tmp)
	if err != nil {
		return err
	}
	defer to.Close()
	if err := to.Chmod(".", pluginDirMode); err != nil {
		return err
	}
	if err := copyPluginTree(from, to); err != nil {
		return err
	}
	return capModes(to)
}

// capInPlace caps the modes of a plugin directory renamed into dest from src.
func capInPlace(src, dest string) error {
	root, err := openRoot(dest)
	if err != nil {
		return fmt.Errorf("adopt %s: %w", src, err)
	}
	defer root.Close()
	return capModes(root)
}

// capModes applies the install mode cap to every file under root: no setuid,
// setgid or sticky bit, and nothing writable by group or others.
func capModes(root *os.Root) error {
	return fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		special := info.Mode() & (fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky)
		if capped := info.Mode().Perm() & installFileMask; special != 0 || capped != info.Mode().Perm() {
			return root.Chmod(rel, capped)
		}
		return nil
	})
}

// copyPluginTree copies the regular files and directories of from into to.
// A link anywhere in from is refused.
func copyPluginTree(from, to *os.Root) error {
	return fs.WalkDir(from.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || rel == "." {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a link; an installed plugin holds none", rel)
		}
		if d.IsDir() {
			return to.Mkdir(rel, 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		in, err := from.Open(rel)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := to.OpenFile(rel, os.O_CREATE|os.O_WRONLY|os.O_EXCL|oNoFollow, info.Mode().Perm()&installFileMask)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
