package output

import (
	"errors"
	"syscall"
)

// errorNoData is Windows' ERROR_NO_DATA ("the pipe is being closed"), which
// package syscall does not name. A write racing the reader's close returns it
// instead of ERROR_BROKEN_PIPE.
const errorNoData syscall.Errno = 232

// isPlatformClosedConsumer covers the Windows spellings of "the reader closed the
// pipe". A write to a pipe whose read end is gone fails with ERROR_BROKEN_PIPE,
// and ERROR_NO_DATA when the close is in flight; neither is translated to EPIPE,
// so errors.Is(err, syscall.EPIPE) alone would classify a departed consumer as a
// write failure and fail a run whose reader simply stopped reading.
func isPlatformClosedConsumer(err error) bool {
	return errors.Is(err, syscall.ERROR_BROKEN_PIPE) || errors.Is(err, errorNoData)
}
