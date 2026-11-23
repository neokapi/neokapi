package pluginhost

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// AdoptPluginDir moves the plugin directory src into the install root target
// (as target/<base of src>) under the checks an install makes. Every entry is
// a regular file or a directory (no links), and nobody but the person or root
// owns or may write it. Every file ends with the install mode cap
// (installFileMask). It refuses a plugin target already holds; the copy there
// wins and src is left for the caller to remove.
func AdoptPluginDir(src, target string) error {
	name := filepath.Base(src)
	dest := filepath.Join(target, name)
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("adopt %s: %s is already installed in %s", src, name, target)
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
	if err := os.Rename(src, dest); err != nil {
		if _, ok := errors.AsType[*os.LinkError](err); !ok {
			return fmt.Errorf("adopt %s: %w", src, err)
		}
		// Another file system: copy, then remove the original.
		if err := copyPluginTree(src, dest); err != nil {
			_ = os.RemoveAll(dest)
			return fmt.Errorf("adopt %s: %w", src, err)
		}
		if err := os.RemoveAll(src); err != nil {
			return fmt.Errorf("adopt %s: remove the original: %w", src, err)
		}
	}
	return capModes(dest)
}

// capModes applies the install mode cap to every file under dir.
func capModes(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		special := info.Mode() & (fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky)
		if capped := info.Mode().Perm() & installFileMask; special != 0 || capped != info.Mode().Perm() {
			return os.Chmod(path, capped)
		}
		return nil
	})
}

// copyPluginTree copies the regular files and directories under src to dest.
func copyPluginTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		to := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_EXCL, info.Mode().Perm()&installFileMask)
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
