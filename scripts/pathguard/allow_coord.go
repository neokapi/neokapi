package main

// allowCoord lists the places in the change, extract and MCP files that may
// reach a file by path.
var allowCoord = map[string]string{
	"host/apply_comment.go:writeNamedBackup":     "the backup beside a file a person named outside any project; inside a project the backup is staged through the root",
	"host/extract.go:makeNamedOutDir":            "creates the --out-dir the person named on the command line",
	"host/extract.go:writeNamedExtractOutput":    "writes into the --out-dir the person named on the command line; the default out/ is written through the project root",
	"host/extract.go:stateFileExists":            "stats a skeleton in the extraction batch directory EnsureExtractionDir created in the checkout's state directory under the data root (Layout.LocalStateDir), which kapi creates and no checkout supplies",
	"host/extract.go:removeStateFile":            "removes, as the name it is (os.Remove follows no link), the batch vault extract just created in the checkout's state directory under the data root (Layout.LocalStateDir), which no checkout supplies",
	"host/extract.go:copyFileContents":           "copies a skeleton between extraction batch directories in the checkout's state directory under the data root (Layout.LocalStateDir), which kapi creates and no checkout supplies; the skeleton names are content hashes",
	"host/agentrunsession_linux.go:readProcStat": "reads the kernel's /proc stat file for a process id, never a project path",
}
