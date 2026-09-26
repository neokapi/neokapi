package workspace

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// A project's context is shared through a remote: a place several machines
// read and add files to. Every remote keeps the same layout, whatever it is
// built on:
//
//	log/<writer>/<first-op-id>.jsonl   operations one machine recorded, in id order
//	blobs/<sha256>                     payloads the operations name
//	checkpoints/<op-id>.kpz            the projections as of one operation
//
// <writer> is an id each workspace mints once (Workspace.WriterID), so a
// machine only ever adds files under its own name and two machines never write
// one object. Nothing is ever rewritten or deleted, and no lock is taken: a
// remote needs only a way to list names, read an object and create one.

// Remote directories.
const (
	RemoteLogDir         = "log/"
	RemoteBlobDir        = "blobs/"
	RemoteCheckpointsDir = "checkpoints/"
)

// RemoteDescriptor is what a remote says about itself.
type RemoteDescriptor struct {
	// Kind names the implementation: "file", "git", "s3", "transfer".
	Kind string
	// Location is where the remote is, in the spelling a person would
	// recognise: a directory, a repository ref, a bucket and prefix. Kind and
	// Location together identify the remote, which is what the workspace keys
	// what it knows about the remote by.
	Location string
}

// ID identifies the remote for the workspace's sync state.
func (d RemoteDescriptor) ID() string { return d.Kind + ":" + d.Location }

// Object is one file of a remote.
type Object struct {
	// Name is the object's path in the layout.
	Name string
	// Data is its bytes.
	Data []byte
}

// Remote is a place a project's context is shared through.
//
// Every method is safe for concurrent use by one process. Two processes, or
// two machines, may use one remote at once: Put creates objects and never
// replaces one, so their writes never collide.
type Remote interface {
	// Describe reports the remote's kind and location.
	Describe() RemoteDescriptor

	// List returns the names of the objects under a directory of the layout
	// (RemoteLogDir, RemoteBlobDir, RemoteCheckpointsDir), sorted.
	List(ctx context.Context, dir string) ([]string, error)

	// Get returns an object's bytes, and ErrNoObject for a name the remote
	// does not hold.
	Get(ctx context.Context, name string) ([]byte, error)

	// Put creates objects. An object the remote already holds with the same
	// bytes is left as it is; one it holds with different bytes is refused
	// with ErrObjectExists, and no object of the call is then promised to have
	// been written. A remote that can write several objects as one change (a
	// git commit) does so.
	Put(ctx context.Context, objs ...Object) error

	// Close releases what the remote holds open. It is idempotent.
	Close() error
}

// ErrNoObject reports a name a remote does not hold.
var ErrNoObject = errors.New("workspace: the remote holds no object by that name")

// ErrObjectExists reports a Put of a name the remote already holds with other
// bytes.
var ErrObjectExists = errors.New("workspace: the remote already holds that object with other bytes")

// ErrRemoteUnreachable wraps a failure to reach a remote at all: no network,
// a mount that is not there, credentials refused.
var ErrRemoteUnreachable = errors.New("workspace: the context backend could not be reached")

var (
	segmentName    = regexp.MustCompile(`^log/[0-9a-z]{1,64}/[0-9a-hjkmnp-tv-z]{24}\.jsonl$`)
	blobName       = regexp.MustCompile(`^blobs/[0-9a-f]{64}$`)
	checkpointName = regexp.MustCompile(`^checkpoints/[0-9a-hjkmnp-tv-z]{24}\.kpz$`)
)

// ValidObjectName reports whether name is a path the layout holds.
func ValidObjectName(name string) bool {
	return segmentName.MatchString(name) || blobName.MatchString(name) || checkpointName.MatchString(name)
}

// checkObjectName refuses a name outside the layout, so no adapter is handed a
// path that climbs out of where it keeps the remote.
func checkObjectName(name string) error {
	if !ValidObjectName(name) {
		return fmt.Errorf("workspace: %q is not a name in the context layout", name)
	}
	return nil
}

// checkListDir refuses a directory the layout does not have.
func checkListDir(dir string) error {
	switch dir {
	case RemoteLogDir, RemoteBlobDir, RemoteCheckpointsDir:
		return nil
	}
	return fmt.Errorf("workspace: %q is not a directory of the context layout", dir)
}

// SegmentName is the object a segment of operations is kept in.
func SegmentName(writer, firstOpID string) string {
	return RemoteLogDir + writer + "/" + firstOpID + ".jsonl"
}

// SegmentWriter returns the writer a segment name belongs to.
func SegmentWriter(name string) string {
	rest, _ := strings.CutPrefix(name, RemoteLogDir)
	writer, _, _ := strings.Cut(rest, "/")
	return writer
}

// BlobObjectName is the object a blob is kept in.
func BlobObjectName(address string) string {
	return RemoteBlobDir + strings.TrimPrefix(address, BlobPrefix)
}

// CheckpointName is the object a checkpoint through an operation is kept in.
func CheckpointName(through string) string {
	return RemoteCheckpointsDir + through + ".kpz"
}

// CheckpointThrough returns the operation a checkpoint object stands at.
func CheckpointThrough(name string) string {
	return strings.TrimSuffix(path.Base(name), ".kpz")
}
