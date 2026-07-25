//go:build !windows

package output

// isPlatformClosedConsumer has no extra cases outside Windows: a closed reader
// reaches us as syscall.EPIPE (an os.File) or io.ErrClosedPipe (an io.Pipe), both
// handled in IsClosedConsumer.
func isPlatformClosedConsumer(error) bool { return false }
