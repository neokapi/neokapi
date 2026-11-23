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
		keyring.MockInit()
	}
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
