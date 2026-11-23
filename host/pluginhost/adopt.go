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
		// Another file system: copy through an os.Root on each side, so no
		// name in the tree can lead outside either directory, then remove
		// the original.
		if err := os.Mkdir(dest, pluginDirMode); err != nil {
			return fmt.Errorf("adopt %s: %w", src, err)
		}
		from, err := os.OpenRoot(src)
		if err != nil {
			return fmt.Errorf("adopt %s: %w", src, err)
		}
		to, err := os.OpenRoot(dest)
		if err != nil {
			from.Close()
			return fmt.Errorf("adopt %s: %w", src, err)
		}
		err = copyPluginTree(from, to)
		from.Close()
		to.Close()
		if err != nil {
			_ = os.RemoveAll(dest)
			return fmt.Errorf("adopt %s: %w", src, err)
		}
		if err := os.RemoveAll(src); err != nil {
			return fmt.Errorf("adopt %s: remove the original: %w", src, err)
		}
	}
	root, err := os.OpenRoot(dest)
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
