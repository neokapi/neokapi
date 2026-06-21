//go:build !cgo

// Package uax29 would provide an ICU-backed UAX-29 sentence-segmentation
// engine, but ICU is reached via cgo and is not available on this build
// (cgo disabled — e.g. the wasm build). This stub keeps the package compiling
// without registering the "uax29" engine, so segment.HasEngine("uax29")
// reports false and segment.Build("uax29", …) returns
// segment.ErrEngineUnavailable.
package uax29
