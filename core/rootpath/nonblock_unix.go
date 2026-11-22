//go:build unix

package rootpath

import "syscall"

// openNonBlock keeps an open of a FIFO from waiting for the other end; the
// check on the descriptor then refuses it. A regular file ignores the flag.
const openNonBlock = syscall.O_NONBLOCK
