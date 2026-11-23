package main

// allowBowrain lists the places in its area that may reach a file by path.
var allowBowrain = map[string]string{
	"bowrain/connector/git.go:ensureRepo":                     "the connector's checkout directory, from its local_path setting or under the OS temp dir",
	"bowrain/jobs/contextscan_worker.go:fetchContextScanRepo": "removes the scan checkout directory it created with os.MkdirTemp",
}
