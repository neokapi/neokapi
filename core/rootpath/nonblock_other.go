//go:build !unix

package rootpath

// openNonBlock is empty where the platform has no FIFOs to block on.
const openNonBlock = 0
