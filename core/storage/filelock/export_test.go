package filelock

// BreakGoroutineIDs makes every goroutine read as one whose number cannot be
// told, by handing the real parser a stack line it does not recognise, and
// returns the function that undoes it.
func BreakGoroutineIDs() (restore func()) {
	saved := currentGoroutine
	currentGoroutine = func() uint64 { return parseGoroutineID([]byte("goroutine-header-of-another-shape")) }
	return func() { currentGoroutine = saved }
}

// ParseGoroutineID exposes the parser to the tests.
var ParseGoroutineID = parseGoroutineID
