//go:build !unix

package rootpath

// openNonBlock is empty where the platform has no FIFOs to block on.
const openNonBlock = 0

// openNoFollow is empty where the platform has no such flag; OpenRegularNoLink
// then relies on its check of the path before the open.
const openNoFollow = 0
