package project

// mustPath is the path an expansion returns, for a test whose expansion must
// not fail.
func mustPath(p string, err error) string {
	if err != nil {
		panic(err)
	}
	return p
}
