package store

import (
	"context"
	"errors"
	"time"
)

// ErrContextObjectExists reports a put of a context object the project already
// holds with other bytes.
var ErrContextObjectExists = errors.New("the project already holds that context object with other bytes")

// ContextObjectExistsError names the object a put was refused on. It matches
// ErrContextObjectExists under errors.Is.
type ContextObjectExistsError struct {
	Name string
}

func (e *ContextObjectExistsError) Error() string {
	return ErrContextObjectExists.Error() + ": " + e.Name
}

// Is reports a match with ErrContextObjectExists.
func (e *ContextObjectExistsError) Is(target error) bool { return target == ErrContextObjectExists }

// ErrNoContextObject reports a context object the project does not hold.
var ErrNoContextObject = errors.New("the project holds no context object by that name")

// A ContextObject is one object of a project's context, under the name the
// context layout of core/workspace gives it: a segment of operations under
// log/, a blob under blobs/, a checkpoint under checkpoints/.
type ContextObject struct {
	Name string
	Data []byte
	// SHA256 is the hex digest of Data, which the store computes on put.
	SHA256 string
	// PushedBy is the member whose push created the object.
	PushedBy string
	// PushedAt is when the object was created.
	PushedAt time.Time
}

// ContextObjectStore holds the objects a project's context is shared through.
// An object is created once: a put of a name the project holds with the same
// bytes leaves it as it is, and one with other bytes is refused with
// ErrContextObjectExists.
type ContextObjectStore interface {
	// ListContextObjects returns the names under dir, which is one of the
	// layout's directories (workspace.RemoteLogDir, RemoteBlobDir,
	// RemoteCheckpointsDir), sorted in byte order.
	ListContextObjects(ctx context.Context, projectID, dir string) ([]string, error)
	// GetContextObject returns one object, and ErrNoContextObject for a name
	// the project does not hold.
	GetContextObject(ctx context.Context, projectID, name string) (*ContextObject, error)
	// PutContextObjects creates the objects of batch in one transaction and
	// stamps each new one with pushedBy. When one name is refused, no object
	// of the batch is written and the error is a *ContextObjectExistsError
	// naming it. A blob whose name is not the address of its bytes is
	// refused before anything is written.
	PutContextObjects(ctx context.Context, projectID, pushedBy string, batch []ContextObject) error
}

// ContextObjects finds the context object store behind a content store,
// looking through any decorator that names what it wraps, as ShipVerdicts does.
func ContextObjects(cs ContentStore) (ContextObjectStore, bool) {
	for cs != nil {
		if s, ok := cs.(ContextObjectStore); ok {
			return s, true
		}
		inner, ok := cs.(interface{ Unwrap() ContentStore })
		if !ok {
			return nil, false
		}
		cs = inner.Unwrap()
	}
	return nil, false
}
