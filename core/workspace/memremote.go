package workspace

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// MemoryRemote holds the context layout in memory. It is what a transfer
// file is read into and written from: `kapi context export` pushes a project
// into an empty one and packs its objects into one archive, and `kapi context
// import` unpacks an archive into one and pulls from it.
type MemoryRemote struct {
	location string

	mu   sync.Mutex
	objs map[string][]byte
}

// NewMemoryRemote returns a remote holding the objects given, described as a
// transfer file at location. Every name must be one the layout holds.
func NewMemoryRemote(location string, objs ...Object) (*MemoryRemote, error) {
	r := &MemoryRemote{location: location, objs: map[string][]byte{}}
	for _, obj := range objs {
		if err := checkObjectName(obj.Name); err != nil {
			return nil, err
		}
		r.objs[obj.Name] = obj.Data
	}
	return r, nil
}

// Describe reports the file the objects belong to.
func (r *MemoryRemote) Describe() RemoteDescriptor {
	return RemoteDescriptor{Kind: "transfer", Location: r.location}
}

// List returns the names under a directory, sorted.
func (r *MemoryRemote) List(_ context.Context, dir string) ([]string, error) {
	if err := checkListDir(dir); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for name := range r.objs {
		if strings.HasPrefix(name, dir) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out, nil
}

// Get returns one object.
func (r *MemoryRemote) Get(_ context.Context, name string) ([]byte, error) {
	if err := checkObjectName(name); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	data, ok := r.objs[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoObject, name)
	}
	return data, nil
}

// Put creates objects.
func (r *MemoryRemote) Put(_ context.Context, objs ...Object) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, obj := range objs {
		if err := checkObjectName(obj.Name); err != nil {
			return err
		}
		if held, ok := r.objs[obj.Name]; ok && !bytes.Equal(held, obj.Data) {
			return fmt.Errorf("%w: %s", ErrObjectExists, obj.Name)
		}
	}
	for _, obj := range objs {
		r.objs[obj.Name] = slices.Clone(obj.Data)
	}
	return nil
}

// Objects returns every object, in name order.
func (r *MemoryRemote) Objects() []Object {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Object, 0, len(r.objs))
	for name, data := range r.objs {
		out = append(out, Object{Name: name, Data: data})
	}
	slices.SortFunc(out, func(a, b Object) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Close holds nothing open.
func (r *MemoryRemote) Close() error { return nil }
