package host

import (
	"context"
	"io"

	"github.com/neokapi/neokapi/core/change/filehome"
)

// commitWithin writes what write produces as the file at path, beneath root,
// through a file home rooted there (filehome.Home.Produce). The file is staged
// beside its destination and renamed into place through the root, never cut
// in place, and only while the file still holds what it held when the write
// began. A link that leads out of root is refused, and the directories the
// file needs are made at commit, through the root.
func commitWithin(ctx context.Context, root, path string, write func(io.Writer) error) error {
	home := filehome.New(nil, filehome.Options{Root: root})
	before, err := home.Digest(path)
	if err != nil {
		return err
	}
	p, err := home.Produce(ctx, path, before, write)
	if err != nil {
		return err
	}
	defer func() { _ = p.Release() }()
	return p.Commit(ctx)
}
