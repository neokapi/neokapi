// Package keychain is the one door to the operating system's keychain. Every
// secret kapi keeps there (provider keys, venue tokens) goes through Set, Get
// and Delete here.
//
// In a test binary the door opens onto an in-memory keyring instead
// (keyring.MockInit), so no test reads or writes the developer's real
// keychain, whether or not it remembered to ask.
package keychain

import (
	"github.com/zalando/go-keyring"

	"github.com/neokapi/neokapi/core/dataroot"
)

// ErrNotFound is the keyring's answer for a secret it does not hold.
var ErrNotFound = keyring.ErrNotFound

// A test binary gets the in-memory keyring before any test runs. Installed on
// first use instead, it would replace a keyring a test had already filled,
// whenever that first use came after the test's own setup, which made the
// outcome depend on which test, or which background goroutine, reached the
// keychain first.
func init() {
	if dataroot.UnderTest() {
		installTestKeyring()
	}
}

// installTestKeyring puts an empty in-memory keyring in front of the library:
// at package load in a test binary, and on every Reset. It is the one way the
// test keyring is made.
func installTestKeyring() {
	keyring.MockInit()
}

// Reset empties the in-memory keyring a test binary uses, so a test starts
// with no secrets. It is the only way a test prepares the keyring; outside a
// test binary it panics, since the real keychain is never emptied.
func Reset() {
	if !dataroot.UnderTest() {
		panic("keychain: Reset is for test binaries only")
	}
	installTestKeyring()
}

// Set stores a secret for service and user.
func Set(service, user, secret string) error {
	return keyring.Set(service, user, secret)
}

// Get reads the secret stored for service and user.
func Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

// Delete removes the secret stored for service and user.
func Delete(service, user string) error {
	return keyring.Delete(service, user)
}
