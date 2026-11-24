//go:build unix

package filehome_test

// ownersAreUsers says a directory's owner is a user id this platform compares
// with the process's, so a lock directory another user owns is refused.
const ownersAreUsers = true
