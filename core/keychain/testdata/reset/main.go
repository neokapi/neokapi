// Command reset calls keychain.Reset from a binary `go test` did not build.
// TestResetRefusesOutsideATestBinary expects it to panic before it can
// touch the real keychain.
package main

import "github.com/neokapi/neokapi/core/keychain"

func main() {
	keychain.Reset()
}
