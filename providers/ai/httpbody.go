package aiprovider

import (
	"io"

	"github.com/neokapi/neokapi/core/safeio"
)

// MaxProviderResponseBytes bounds a provider's HTTP response body.
//
// A model reply is a few megabytes at the largest output ceilings, and the JSON
// around it is small; the cap is orders of magnitude above that. It is not a
// quota — it is the difference between "an endpoint misbehaved" and "the process
// grew until the machine gave up", for a body whose length the caller does not
// control and cannot check in advance.
const MaxProviderResponseBytes int64 = 64 << 20 // 64 MiB

// readCappedBody reads an API response body under MaxProviderResponseBytes,
// failing with a typed safeio.LimitError rather than truncating — a partial body
// would parse as a malformed reply and be blamed on the model.
func readCappedBody(r io.Reader) ([]byte, error) {
	return io.ReadAll(safeio.NewLimitedReader(r, MaxProviderResponseBytes))
}
