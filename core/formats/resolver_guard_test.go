package formats_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A format package reaches another format's reader only through the
// format.SubfilterResolver it is given. A plugin's reader is reachable only
// through the format registry, so no format package may import the registry
// or the host. The parse cache keys a parse by the plugins its resolver
// handed out (format.SubfilterTrace), and a reader that found a plugin's
// reader another way would leave that plugin out of the key.
//
// The walk stops at core/formats, where every reader kapi runs in its own
// process lives. A plugin's readers (plugins/, okapi-bridge) run in the
// plugin's process. The key names that plugin as a whole, so whatever reader
// it reaches inside its process is covered by its identity. Nothing under
// bowrain/ implements a reader.
func TestFormatPackagesReachOtherReadersOnlyThroughTheResolver(t *testing.T) {
	forbidden := []string{
		"github.com/neokapi/neokapi/core/registry",
		"github.com/neokapi/neokapi/host",
	}
	var offenders []string
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		// The top-level package registers every reader; it is the registry's
		// client, not a reader.
		if filepath.Dir(p) == "." {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			for _, bad := range forbidden {
				if path == bad || strings.HasPrefix(path, bad+"/") {
					offenders = append(offenders, p+" imports "+path)
				}
			}
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, offenders)
}
