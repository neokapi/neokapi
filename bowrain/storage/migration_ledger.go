package storage

import (
	_ "embed"

	corestorage "github.com/neokapi/neokapi/core/storage"
)

// The committed ledger file of the platform's ledgers (migration_ledger.json):
// each shipped version's identity and the names it has carried, which the
// runner compares a database's ledger with (corestorage.CheckLedger).
// `make migration-ledger` regenerates it.

//go:embed migration_ledger.json
var shippedLedgerJSON []byte

var shippedLedgers = func() corestorage.ShippedLedgers {
	l, err := corestorage.ParseShippedLedgers(shippedLedgerJSON)
	if err != nil {
		panic(err)
	}
	return l
}()

// ShippedLedgers is the platform's committed ledger file, for the start-up
// check of ledgers migrated later (CheckPostgresLedger) and for tests.
func ShippedLedgers() corestorage.ShippedLedgers { return shippedLedgers }
