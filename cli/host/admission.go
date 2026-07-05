package host

import "github.com/neokapi/neokapi/core/safeio"

// newFanoutAdmission returns the in-flight-bytes admission controller shared
// by one file fan-out (tool runs and flow batch runs). The per-document
// safeio budget bounds a single read, but the fan-out runs up to NumCPU
// files at once, so without a global cap the memory envelope is
// concurrency × per-file peak. Each fan-out creates one Admission and every
// worker acquires its file's on-disk size (safeio.FileWeight) before
// processing.
//
// The total defaults to safeio.DefaultMaxInflightBytes (2 GiB) and is
// overridable via KAPI_MAX_INFLIGHT_BYTES (negative disables the cap; see
// safeio.NewAdmissionFromEnv).
func newFanoutAdmission() *safeio.Admission {
	return safeio.NewAdmissionFromEnv()
}
