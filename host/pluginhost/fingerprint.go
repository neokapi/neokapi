package pluginhost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"

	"github.com/neokapi/neokapi/core/rootpath"
)

// MaxFingerprintFiles bounds how many files Fingerprint reads in a plugin
// directory that has no install record. A plugin with more is not cached.
const MaxFingerprintFiles = 4096

// ErrTooManyFiles is why a plugin with no install record and more than
// MaxFingerprintFiles files has no fingerprint.
var ErrTooManyFiles = fmt.Errorf("its directory holds more than %d files", MaxFingerprintFiles)

// ErrNoFingerprint is why a plugin with no manifest, or a directory kapi
// cannot read, has no fingerprint.
var ErrNoFingerprint = errors.New("kapi could not read its install record or its files")

// Fingerprint names the plugin's bytes, for the parse cache key, whatever its
// version string says. It digests the whole manifest (the version, which for okapi-bridge is the Okapi release it runs,
// and each model's identifier and revision) with one of:
//
//   - the sha256 of the archive `kapi plugins install` installed it from
//     (installed.json). A reinstall of the same release repeats it, so a fresh
//     install on every CI run still finds the cache.
//   - for a plugin installed another way (a development build, a package
//     manager), the path, size and modification time of every regular file in
//     its directory, so a rebuilt payload (a launcher script and its JARs)
//     misses. A directory with more than MaxFingerprintFiles files has none,
//     and ErrTooManyFiles says why.
//
// It is read from disk on every call, never from a cache.
func (p *Plugin) Fingerprint() (string, error) {
	if p == nil || p.Manifest == nil {
		return "", ErrNoFingerprint
	}
	return p.fingerprint()
}

func (p *Plugin) fingerprint() (string, error) {
	m, err := json.Marshal(p.Manifest)
	if err != nil {
		return "", ErrNoFingerprint
	}
	h := sha256.New()
	h.Write(m)
	if meta, err := ReadInstalledMetadata(p.Dir); err == nil && meta.Archive != "" {
		fmt.Fprintf(h, "\x00archive:%s", meta.Archive)
		return hex.EncodeToString(h.Sum(nil))[:16], nil
	}
	files, err := payloadFiles(p.Dir)
	if err != nil {
		return "", err
	}
	for _, f := range files {
		fmt.Fprintf(h, "\x00%s", f)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// payloadFiles lists every entry beneath dir as "path:size:mtime", sorted,
// read through dir so no link leads the walk out of it. A link is listed by
// its name and not followed. It stops with ErrTooManyFiles past
// MaxFingerprintFiles.
func payloadFiles(dir string) ([]string, error) {
	var out []string
	var walk func(rel string) error
	walk = func(rel string) error {
		entries, err := rootpath.ReadDir(dir, filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return ErrNoFingerprint
		}
		for _, e := range entries {
			name := path.Join(rel, e.Name())
			switch {
			case e.Type()&fs.ModeSymlink != 0:
				// A link is recorded by its own size, the length of its target,
				// and its time, which retargeting it (a new link) changes. The
				// target string itself would need a readlink through the plugin
				// directory, which core/rootpath does not offer.
				info, err := e.Info()
				if err != nil {
					return ErrNoFingerprint
				}
				out = append(out, fmt.Sprintf("%s:link:%d:%d", name, info.Size(), info.ModTime().UnixNano()))
			case e.IsDir():
				if err := walk(name); err != nil {
					return err
				}
				continue
			default:
				info, err := e.Info()
				if err != nil {
					return ErrNoFingerprint
				}
				out = append(out, fmt.Sprintf("%s:%d:%d", name, info.Size(), info.ModTime().UnixNano()))
			}
			if len(out) > MaxFingerprintFiles {
				return ErrTooManyFiles
			}
		}
		return nil
	}
	if err := walk(""); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}
