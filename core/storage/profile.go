package storage

// Profile describes what the SQLite driver compiled into this build gives a
// store. Every build has a driver (driver_cgo.go, driver_nocgo.go,
// driver_js.go), and they differ in a few properties a store can observe. Code
// that depends on one of them asks the profile rather than assuming the native
// driver.
//
// The properties a store may NOT depend on for correctness are the ones a
// driver can lack: a second connection beside a writer (MaxConns), readers
// running beside a writer (WAL), and a database outliving the process
// (Durable). A store method that needs a transaction takes one from its caller
// or begins and commits its own; nothing asks a connection whether it is
// already inside one.
type Profile struct {
	// Driver names the driver, for messages.
	Driver string

	// MaxConns is the most connections one pool opens to a database file.
	// It is 1 where a second connection's busy wait would spin the only
	// thread (the browser build), so code must never hold a transaction or
	// open rows on a pool and then wait for a second session on the same
	// pool from the same goroutine.
	MaxConns int

	// WAL reports that databases run in write-ahead-log mode, where a reader
	// proceeds beside a writer. Without it a read waits for the write
	// transaction ahead of it.
	WAL bool

	// CrossProcessLock reports that Options.CrossProcessWrites takes an
	// advisory lock other processes on this machine honour.
	CrossProcessLock bool

	// Durable reports that a database outlives the process that wrote it.
	Durable bool
}

// DriverProfile returns the profile of this build's driver.
//
// A build with the storage_oneconn tag holds every pool to one connection and
// runs databases without WAL, as the browser's driver does, whatever the native
// driver allows. It is a test mode. Running the suites natively under it finds,
// without a browser, the code that holds a transaction or open rows and waits
// for a second session on the same pool (a call that waits 20 seconds for the
// pool panics with every goroutine's stack rather than leaving the test binary
// to its timeout), and the code that reads on one pool while it holds a write
// on another, or commits while it holds rows open on another, which WAL
// allows. Without WAL that second call waits for the busy timeout and fails
// with "database is locked", as it fails at once in the browser.
func DriverProfile() Profile {
	p := driverProfile
	if singleConnection {
		p.MaxConns = 1
		p.WAL = false
	}
	return p
}
