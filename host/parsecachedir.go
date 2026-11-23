package host

import (
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/dataroot"
	"github.com/neokapi/neokapi/core/project"
)

// EnvParseCacheDir names a directory to keep the parse cache in, instead of
// work/cache/docs in the checkout's state directory. A CI job sets it to a
// fixed directory and caches that directory, because the state directory's
// name changes with every fresh checkout.
const EnvParseCacheDir = "KAPI_PARSE_CACHE_DIR"

// parseCacheDirName is the parse cache's directory under a cache directory.
const parseCacheDirName = "docs"

// ParseCacheDir is where the parse cache of the project at layout lives:
// $KAPI_PARSE_CACHE_DIR when it is set, and work/cache/docs in the checkout's
// state directory when it is not. It is the one place that names the
// directory; every reader and writer of the parse cache asks it.
//
// A set variable is checked, and the directory created, by
// dataroot.PrepareCacheDir: an absolute path outside the checkout, and no
// link.
func ParseCacheDir(layout project.Layout) (string, error) {
	if v := os.Getenv(EnvParseCacheDir); v != "" {
		return dataroot.PrepareCacheDir(EnvParseCacheDir, v, layout.Root)
	}
	return filepath.Join(layout.CacheDir(), parseCacheDirName), nil
}

// parseCacheDirUnder is the parse cache directory beneath cacheDir, for a cache
// directory that belongs to no project.
func parseCacheDirUnder(cacheDir string) string {
	return filepath.Join(cacheDir, parseCacheDirName)
}
