//go:build !unix

package safeio

// openNonBlock is empty where the platform has no FIFOs to block on.
const openNonBlock = 0
