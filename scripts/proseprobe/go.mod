module github.com/neokapi/neokapi/scripts/proseprobe

go 1.27.0

// proseprobe scores the Prose maturity axis by running the rung tests it finds
// across the workspace and the plugin modules. It links nothing from the
// framework: it reads files and starts `go test`, so it keeps a module of its
// own rather than adding a subprocess runner to the root module.
require (
	github.com/stretchr/testify v1.11.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
)
