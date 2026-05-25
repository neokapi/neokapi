module github.com/neokapi/neokapi/scripts/gen-refs

go 1.26.0

// gen-refs introspects the framework registries (formats/tools) and the kapi
// CLI command tree (which lives in the cli module and pulls in Cobra). Keeping
// it in its own module preserves the framework module's Cobra-free isolation
// (verified by `GOWORK=off go build ./...`). Local modules resolve via go.work.
require (
	github.com/neokapi/neokapi v0.0.0-00010101000000-000000000000
	github.com/neokapi/neokapi/cli v0.0.0-00010101000000-000000000000
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	gopkg.in/yaml.v3 v3.0.1
)
