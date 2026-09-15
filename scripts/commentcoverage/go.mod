module github.com/neokapi/neokapi/scripts/commentcoverage

go 1.27.0

// commentcoverage holds the recipe to every tracked file whose comments it
// checks. It needs only the framework module, so it lives in its own module
// like the other scripts/* tools. Local modules resolve via go.work.
require (
	github.com/neokapi/neokapi v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
)
