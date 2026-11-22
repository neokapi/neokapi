package main

// allowCoord lists the places in the change, extract and MCP files that may
// reach a file by path.
var allowCoord = map[string]string{
	"host/apply_comment.go:writeNamedBackup":  "the backup beside a file a person named outside any project; inside a project the backup is staged through the root",
	"host/extract.go:makeNamedOutDir":         "creates the --out-dir the person named on the command line",
	"host/extract.go:writeNamedExtractOutput": "writes into the --out-dir the person named on the command line; the default out/ is written through the project root",
	"host/extract.go:stateFileExists":         "kapi's own skeleton file in the project state directory, which Layout.CheckState keeps inside the project",
	"host/extract.go:removeStateFile":         "kapi's own vault file in the project state directory, which Layout.CheckState keeps inside the project",
	"host/extract.go:loadReusablePrior":       "lists kapi's own extraction batches in the project state directory, which Layout.CheckState keeps inside the project",
	"host/extract.go:copyFileContents":        "copies kapi's own skeleton between extraction batches in the project state directory, which Layout.CheckState keeps inside the project",
}
