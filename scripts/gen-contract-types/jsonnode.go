package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// jnode is a JSON value decoded with its object keys in document order. The
// change-set schema lists each object's properties in the order the Go types
// declare them, and the generated TypeScript keeps that order, which a Go map
// would lose.
type jnode struct {
	isObj bool
	isArr bool
	keys  []string
	obj   map[string]*jnode
	arr   []*jnode
	// val is a scalar: a string, a json.Number, a bool, or nil for null.
	val any
}

// decodeOrdered decodes one JSON value.
func decodeOrdered(b []byte) (*jnode, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	n, err := decodeNode(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("data after the JSON value")
	}
	return n, nil
}

func decodeNode(dec *json.Decoder) (*jnode, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return &jnode{val: tok}, nil
	}
	switch d {
	case '{':
		n := &jnode{isObj: true, obj: map[string]*jnode{}}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			k, ok := kt.(string)
			if !ok {
				return nil, fmt.Errorf("object key %v is not a string", kt)
			}
			v, err := decodeNode(dec)
			if err != nil {
				return nil, err
			}
			n.set(k, v)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return n, nil
	case '[':
		n := &jnode{isArr: true}
		for dec.More() {
			v, err := decodeNode(dec)
			if err != nil {
				return nil, err
			}
			n.arr = append(n.arr, v)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return n, nil
	}
	return nil, fmt.Errorf("unexpected %v", d)
}

// set adds or replaces key k of an object, keeping the order keys were first
// set in.
func (n *jnode) set(k string, v *jnode) {
	if _, ok := n.obj[k]; !ok {
		n.keys = append(n.keys, k)
	}
	n.obj[k] = v
}

// get is the value of key k of an object, nil when n is no object or lacks k.
func (n *jnode) get(k string) *jnode {
	if n == nil || !n.isObj {
		return nil
	}
	return n.obj[k]
}

// has reports whether n is an object holding key k.
func (n *jnode) has(k string) bool {
	return n.get(k) != nil
}

// str is the string value of key k, empty when it is missing or not a string.
func (n *jnode) str(k string) string {
	s, _ := n.get(k).scalar().(string)
	return s
}

func (n *jnode) scalar() any {
	if n == nil {
		return nil
	}
	return n.val
}

// strings is the string elements of an array.
func (n *jnode) strings() []string {
	if n == nil {
		return nil
	}
	var out []string
	for _, v := range n.arr {
		if s, ok := v.val.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// types is a schema's type keyword as a list: one type, or several.
func (n *jnode) types() []string {
	t := n.get("type")
	if t == nil {
		return nil
	}
	if s, ok := t.val.(string); ok {
		return []string{s}
	}
	return t.strings()
}
