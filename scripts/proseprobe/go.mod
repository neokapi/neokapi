module github.com/neokapi/neokapi/scripts/proseprobe

go 1.27.1

// proseprobe scores the Prose maturity axis by running the rung tests it finds
// across the workspace and the plugin modules. It links nothing from the
// framework: it reads files and starts `go test`, so it keeps a module of its
// own rather than adding a subprocess runner to the root module.
require (
	github.com/stretchr/testify v1.12.1
	gopkg.in/yaml.v3 v3.0.1
)

require go.yaml.in/yaml/v3 v3.0.5 // indirect
