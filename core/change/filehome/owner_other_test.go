//go:build !unix

package filehome_test

// ownersAreUsers is false where a directory's owner is not a user id to
// compare (dataroot.CheckOwnedDir accepts every owner there), so no lock
// directory is refused for its owner.
const ownersAreUsers = false
