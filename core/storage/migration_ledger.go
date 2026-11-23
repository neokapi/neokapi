package storage

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// The committed ledger file (migration_ledger.json) holds, for every ledger
// kapi's stores keep, each shipped version's identity and the names it has
// carried. The platform keeps its own beside its runner (bowrain/storage).
// `make migration-ledger` regenerates both: it runs the test suites of the
// packages that own a migration list with RecordLedgerEnv set, which makes
// every runner write what it migrates (RecordLedger), and
// scripts/migrationledger joins that with the names read from git history.

//go:embed migration_ledger.json
var shippedLedgerJSON []byte

var shippedLedgers = mustParseShippedLedgers(shippedLedgerJSON)

// ParseShippedLedgers reads a committed ledger file.
func ParseShippedLedgers(b []byte) (ShippedLedgers, error) {
	var out ShippedLedgers
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("read the committed migration ledger: %w", err)
	}
	return out, nil
}

func mustParseShippedLedgers(b []byte) ShippedLedgers {
	l, err := ParseShippedLedgers(b)
	if err != nil {
		panic(err)
	}
	return l
}

// RecordLedgerEnv names the directory `make migration-ledger` collects what
// each runner migrates in. Unset, nothing is recorded.
const RecordLedgerEnv = "NEOKAPI_MIGRATION_LEDGER_RECORD"

var recordMu sync.Mutex

// RecordLedger writes what a runner is about to migrate, when RecordLedgerEnv
// names a directory: one JSON line per migration, under side ("core" for
// kapi's stores, "bowrain" for the platform's).
func RecordLedger(side, ledger string, migrations []Migration) error {
	dir := os.Getenv(RecordLedgerEnv)
	if dir == "" {
		return nil
	}
	recordMu.Lock()
	defer recordMu.Unlock()
	f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%s-%d.jsonl", side, os.Getpid())), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("record the migration ledger: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, m := range migrations {
		if m.Retired {
			continue
		}
		if err := enc.Encode(map[string]any{
			"ledger": ledger, "version": m.Version, "description": m.Description, "identity": MigrationIdentity(m),
		}); err != nil {
			return fmt.Errorf("record the migration ledger: %w", err)
		}
	}
	return nil
}

// ShippedCoverage refuses, in a test binary, a migration of a ledger the
// committed file holds that the file does not list yet: a new migration is
// pinned before it merges (`make migration-ledger`). Outside tests, and while
// recording, it passes.
func ShippedCoverage(ledger string, migrations []Migration, shipped map[int]ShippedMigration) error {
	if shipped == nil || !testing.Testing() || os.Getenv(RecordLedgerEnv) != "" {
		return nil
	}
	var missing []string
	for _, m := range migrations {
		if _, ok := shipped[m.Version]; !ok && !m.Retired {
			missing = append(missing, fmt.Sprintf("%d (%q)", m.Version, m.Description))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	slices.Sort(missing)
	return fmt.Errorf("the committed migration ledger does not list %s of %q; run make migration-ledger", strings.Join(missing, ", "), ledger)
}
