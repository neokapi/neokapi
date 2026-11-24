package main

// allowCore lists the places in its area that may reach a file by path.
var allowCore = map[string]string{
	// The file home's lock directory and a destination outside its root.
	"core/change/filehome/home.go:ensureLockDir":  "the lock directory, locks under the kapi data dir (filehome.LockDir), refused unless it is a directory this user alone owns and can write",
	"core/change/filehome/stage.go:backUpAsNamed": "a destination outside the home's root, which a person named (PermitOutside)",

	// Output destinations: diagnosis only, the write goes through the file home.
	"core/flow/outputpath.go:CheckOutputPath":         "stats a destination and its parent only to refuse or classify it; nothing it returns names a file to open, and the write itself goes through the file home, which keeps it inside the root",
	"core/flow/outputpath.go:ClassifyOutputPathError": "stats a destination only to classify a failed write for the error message; it decides nothing the write does",

	// Temp files and directories a reader created itself.
	"core/formats/audio/reader.go:materialize":           "removes the temp file the reader created with os.CreateTemp",
	"core/formats/epub/reader.go:Open":                   "removes the temp file the reader created with os.CreateTemp",
	"core/formats/epub/reader.go:Close":                  "removes the temp file the reader created with os.CreateTemp",
	"core/formats/image/convert.go:encodePNGTemp":        "removes the temp file it created with os.CreateTemp",
	"core/formats/image/convert.go:transcodeFFmpegToPNG": "removes the temp file it created with os.CreateTemp",
	"core/formats/image/reader.go:openImageSource":       "removes the temp file it created with os.CreateTemp",
	"core/formats/image/reader.go:close":                 "removes the spill file openImageSource created with os.CreateTemp",
	"core/formats/image/reader.go:privateCopy":           "removes the temp file it created with os.CreateTemp",
	"core/formats/odf/reader.go:Close":                   "removes the temp file the reader created with os.CreateTemp",
	"core/formats/openxml/reader.go:Close":               "removes the temp file the reader created with os.CreateTemp",
	"core/formats/video/reader.go:materialize":           "removes the temp file the reader created with os.CreateTemp",
	"core/formats/video/reader.go:readContent":           "removes the work directory the reader created with os.MkdirTemp",

	// An output path given to SetOutput.
	"core/formats/jsx/jsx.go:writeKBF": "the output path a caller gave SetOutput, which callers take from an output flag a person named",
}
