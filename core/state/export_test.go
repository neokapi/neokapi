package state

import "github.com/neokapi/neokapi/core/storage"

// MigrationsThrough returns the store's migrations up to and including version
// v, so a test can build a database as an earlier build left it.
func MigrationsThrough(v int) []storage.Migration {
	var out []storage.Migration
	for _, m := range workMigrations {
		if m.Version <= v {
			out = append(out, m)
		}
	}
	return out
}

// CheckoutID is the view id a handle opened on the record directory at path
// reads.
func CheckoutID(path string) string { return checkoutID(path) }
