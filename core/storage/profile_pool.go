//go:build !storage_oneconn

package storage

// singleConnection is false: pools open as many connections as the driver's
// profile allows (see DriverProfile).
const singleConnection = false

// watchPool watches nothing outside the storage_oneconn test mode.
func watchPool(string) func() { return noWatch }

func noWatch() {}
