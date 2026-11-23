package main

// allowCoord lists the places in the change, extract and MCP files that may
// reach a file by path.
var allowCoord = map[string]string{
	"host/apply_comment.go:writeNamedBackup":     "the backup beside a file a person named outside any project; inside a project the backup is staged through the root",
	"host/extract.go:makeNamedOutDir":            "creates the --out-dir the person named on the command line",
	"host/extract.go:writeNamedExtractOutput":    "writes into the --out-dir the person named on the command line; the default out/ is written through the project root",
	"host/extract.go:stateFileExists":            "stats a skeleton in the extraction batch directory EnsureExtractionDir created, after Layout.CheckState, which refuses any link or irregular file beneath .kapi/",
	"host/extract.go:removeStateFile":            "removes, as the name it is (os.Remove follows no link), the batch vault extract just created after PrepareVault ran Layout.CheckState, which refuses any link or irregular file beneath .kapi/",
	"host/extract.go:copyFileContents":           "copies a skeleton between extraction batch directories of a layout LayoutFor checked with Layout.CheckState, which refuses any link or irregular file beneath .kapi/; the skeleton name comes from kapi's own manifest in an untracked .kapi/work",
	"host/agentrunsession_linux.go:readProcStat": "reads the kernel's /proc stat file for a process id, never a project path",
}
